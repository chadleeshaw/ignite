package handlers

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"
)

// ModePreferenceURL is the UI-mode endpoint the parallel backend serves.
// POST form or JSON mode=cozy|console. 204 on success, 400 for any other
// value, 401 when /api/* is unauthenticated, 404 until the route exists.
const ModePreferenceURL = "/api/preferences/mode"

// OverviewSummaryURL is the htmx partial for Overview lease and pool counts.
const OverviewSummaryURL = "/dashboard/summary"

// fieldString reads an exported string field or map key. Missing fields,
// nil data, and non-strings return "" so templates can render before a
// parallel change adds the field.
func fieldString(data any, name string) string {
	if data == nil || name == "" {
		return ""
	}
	v := reflect.ValueOf(data)
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return ""
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Map:
		if v.Type().Key().Kind() != reflect.String {
			return ""
		}
		mv := v.MapIndex(reflect.ValueOf(name))
		if mv.IsValid() && mv.Kind() == reflect.String {
			return mv.String()
		}
		return ""
	case reflect.Struct:
		f := v.FieldByName(name)
		if f.IsValid() && f.Kind() == reflect.String && f.CanInterface() {
			return f.String()
		}
		return ""
	default:
		return ""
	}
}

// uiMode is the density for this render. Empty or unknown values, including
// data that has no Mode field yet, default to cozy.
func uiMode(data any) string {
	switch fieldString(data, "Mode") {
	case "console":
		return "console"
	default:
		return "cozy"
	}
}

// pageID is the nav key for the current page, or "" when the view has none.
func pageID(data any) string {
	return fieldString(data, "Page")
}

// relTime renders t as a short relative age. The absolute timestamp belongs
// in the title attribute. A zero time means the lease has never been seen.
func relTime(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	if d < 0 {
		d = 0
	}
	switch {
	case d < 45*time.Second:
		return "just now"
	case d < 90*time.Second:
		return "1m ago"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 90*time.Minute:
		return "1h ago"
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 48*time.Hour:
		return "1d ago"
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// rfc3339 is the single timestamp helper for <time datetime>.
// A time value is formatted from the field the page already has.
// A string is kept only when it is already RFC3339, so a later layout
// attribute can be passed through on this line without a second formatter.
func rfc3339(v any) string {
	if s, ok := v.(string); ok {
		s = strings.TrimSpace(s)
		if s == "" {
			return ""
		}
		if _, err := time.Parse(time.RFC3339, s); err == nil {
			return s
		}
		return ""
	}
	t, ok := asTime(v)
	if !ok || t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// tftpRootCrumb is the first TFTP breadcrumb.
// root is the absolute directory the server is using. The label is
// relative to that directory, so the host path is not shown. A later
// display field replaces the return on this line.
func tftpRootCrumb(root string) string {
	root = strings.TrimSpace(root)
	if root == "" || root == "." || root == "/" {
		return "/"
	}
	return "/"
}

// utcLabel is the no-JS fallback for an absolute timestamp. The zone is
// written out so the string is not mistaken for the browser's local time.
func utcLabel(v any, layout string) string {
	t, ok := asTime(v)
	if !ok || t.IsZero() {
		return ""
	}
	if layout == "" {
		layout = "2006-01-02 15:04:05"
	}
	return t.UTC().Format(layout) + " UTC"
}

func asTime(v any) (time.Time, bool) {
	switch t := v.(type) {
	case time.Time:
		return t, true
	case *time.Time:
		if t == nil {
			return time.Time{}, false
		}
		return *t, true
	default:
		return time.Time{}, false
	}
}

// describeStatus turns tracked service state into the Overview subtitle.
// It only names services the status data actually reports.
func describeStatus(s *StatusPageData) (cozy, console string) {
	if s == nil {
		return "Service status is unavailable", "status unavailable"
	}
	copy := *s
	annotateStatus(&copy)
	return copy.AttentionCozy, copy.AttentionConsole
}

// annotateStatus fills OverallStatus and the subtitle from one issue list.
// HTTP not running is "down". A stopped listener is "partial". A DHCP
// server that is only configured has not been started this boot, so it
// does not count.
func annotateStatus(s *StatusPageData) {
	if s == nil {
		return
	}
	cozyIssues, consoleIssues := statusIssues(s)
	switch {
	case s.HTTPServer.Status != "running":
		s.OverallStatus = "down"
	case len(cozyIssues) > 0:
		s.OverallStatus = "partial"
	default:
		s.OverallStatus = "healthy"
	}
	if len(cozyIssues) == 0 {
		if !dhcpListenersRunning(s) {
			s.AttentionCozy = "No DHCP server is running"
			s.AttentionConsole = "no dhcp server running"
		} else {
			s.AttentionCozy = "All services are up"
			s.AttentionConsole = "services up"
		}
		noteHostHealth(s)
		return
	}
	s.AttentionCozy = attentionLead(len(cozyIssues)) + ": " + strings.Join(cozyIssues, " · ")
	s.AttentionConsole = countPhrase(len(consoleIssues), "issue", "issues") + " · " + strings.Join(consoleIssues, " · ")
	noteHostHealth(s)
}

// statusIssues lists services that are down. Configured DHCP servers are
// omitted: they are defined and have not been started, which is not partial
// health. Servers that share a status are one issue. The cozy line is a
// sentence; the console line is the same fact in lowercase.
func statusIssues(s *StatusPageData) (cozy, console []string) {
	if s == nil {
		return nil, nil
	}
	if s.HTTPServer.Status != "running" {
		word := statusWord(s.HTTPServer.Status)
		cozy = append(cozy, "HTTP is "+word)
		console = append(console, "http "+word)
	}
	if s.TFTPServer.Status != "running" {
		word := statusWord(s.TFTPServer.Status)
		cozy = append(cozy, "TFTP is "+word)
		console = append(console, "tftp "+word)
	}
	grouped := map[string][]string{}
	var order []string
	for _, srv := range s.DHCPServers {
		switch srv.Status {
		case "running", "configured":
			continue
		}
		word := strings.ToLower(strings.TrimSpace(srv.Status))
		if word == "" {
			word = "unknown"
		}
		if _, ok := grouped[word]; !ok {
			order = append(order, word)
		}
		label := srv.IP
		if label == "" {
			label = srv.ID
		}
		grouped[word] = append(grouped[word], label)
	}
	for _, word := range order {
		ips := grouped[word]
		verb := "is"
		if len(ips) != 1 {
			verb = "are"
		}
		cozy = append(cozy, "DHCP "+strings.Join(ips, ", ")+" "+verb+" "+word)
		console = append(console, "dhcp "+strings.Join(ips, ", ")+" "+word)
	}
	return cozy, console
}

// dhcpListenersRunning reports whether any DHCP listener is bound.
func dhcpListenersRunning(s *StatusPageData) bool {
	if s == nil {
		return false
	}
	for _, srv := range s.DHCPServers {
		if srv.Status == "running" {
			return true
		}
	}
	return false
}

// dhcpBarDetail is the status-bar clause for DHCP listeners. Stopped
// servers are the partial-health fact. Configured servers are counted
// so an idle pool stays visible, and they do not change overall health.
func dhcpBarDetail(s *StatusPageData) (cozy, console string) {
	if s == nil {
		return "", ""
	}
	var stopped, configured int
	for _, srv := range s.DHCPServers {
		switch srv.Status {
		case "stopped":
			stopped++
		case "configured":
			configured++
		}
	}
	var cozyParts, consoleParts []string
	if stopped > 0 {
		cozyParts = append(cozyParts, countPhrase(stopped, "DHCP server stopped", "DHCP servers stopped"))
		consoleParts = append(consoleParts, countPhrase(stopped, "stopped", "stopped"))
	}
	if configured > 0 {
		cozyParts = append(cozyParts, countPhrase(configured, "not started", "not started"))
		consoleParts = append(consoleParts, countPhrase(configured, "configured", "configured"))
	}
	return strings.Join(cozyParts, " · "), strings.Join(consoleParts, " · ")
}

func statusWord(status string) string {
	if strings.TrimSpace(status) == "" {
		return "unknown"
	}
	return status
}

func attentionLead(n int) string {
	return countPhrase(n, "thing needs attention", "things need attention")
}

// statusKey is one console shortcut drawn in the shared status bar.
type statusKey struct {
	Key   string
	Label string
}

// statusBarView is the shell footer. Empty fields fall back to page data
// and then to a shared default, so a page cannot drop the bar.
type statusBarView struct {
	Health      string
	Detail      string
	ConsoleLine string
	Tip         string
	Counts      string
	Keys        []statusKey
}

// hostHealth is the last overall status computed from StatusPageData.
// Shell pages read it so the status bar can lead with the same sentence
// on every page. It is filled only after BindStatusContainer, which the
// process sets when it starts. Template tests do not bind it, so they
// keep the per-page fallback.
type hostHealth struct {
	Overall string
	Cozy    string
	Console string
}

var (
	statusMu        sync.Mutex
	statusContainer *Container
	hostSnap        hostHealth
)

// BindStatusContainer lets shell pages recompute overall health from the
// same status payload Overview already uses. The name is local: the layout
// field the backend is adding is not guessed here.
func BindStatusContainer(container *Container) {
	statusMu.Lock()
	statusContainer = container
	statusMu.Unlock()
}

func hostHealthBound() bool {
	statusMu.Lock()
	defer statusMu.Unlock()
	return statusContainer != nil
}

func boundStatusContainer() *Container {
	statusMu.Lock()
	defer statusMu.Unlock()
	return statusContainer
}

func noteHostHealth(s *StatusPageData) {
	if s == nil {
		return
	}
	cozy, console := dhcpBarDetail(s)
	statusMu.Lock()
	hostSnap = hostHealth{Overall: s.OverallStatus, Cozy: cozy, Console: console}
	statusMu.Unlock()
}

func readHostHealth() hostHealth {
	statusMu.Lock()
	defer statusMu.Unlock()
	return hostSnap
}

// refreshHostHealth rebuilds the status payload for a shell page so the
// bar leads with overall health. A missing container leaves the fallback
// sentence in place.
func refreshHostHealth(r *http.Request) {
	container := boundStatusContainer()
	if container == nil || r == nil || container.ServerService == nil || container.LeaseService == nil || container.Config == nil {
		return
	}
	data, err := (&StatusHandlers{container: container}).getStatusData(r)
	if err != nil || data == nil {
		return
	}
	_ = data
}

func shellPage(name string) bool {
	switch name {
	case "index", "dhcp", "tftp", "osimages", "syslinux", "provision":
		return true
	default:
		return false
	}
}

// pageHealth is the status-bar sentence.
// OverallStatus is the word on the merged status payload (healthy, partial,
// down). When the process has bound a status container, every page uses
// that word. Pages rendered without it keep the sentence derived from
// their own fields.
func pageHealth(data any) string {
	if health := fieldString(data, "StatusHealth"); health != "" {
		return health
	}
	if health := cozyHealth(fieldString(data, "OverallStatus")); health != "" {
		return health
	}
	if hostHealthBound() {
		if health := cozyHealth(readHostHealth().Overall); health != "" {
			return health
		}
	}
	if health := derivedHealth(data, pageID(data)); health != "" {
		return health
	}
	return "Ready"
}

// statusBar builds the flame status bar from the page value. StatusHealth,
// StatusTip, and StatusCounts override the derived copy when a handler sets
// them. Health comes from pageHealth.
func statusBar(data any) statusBarView {
	page := pageID(data)
	health := pageHealth(data)
	counts := fieldString(data, "StatusCounts")
	if counts == "" {
		counts = derivedCounts(data, page)
	}
	detail, consoleDetail := "", ""
	if hostHealthBound() {
		snap := readHostHealth()
		overall := fieldString(data, "OverallStatus")
		if overall == "" || overall == snap.Overall {
			detail = snap.Cozy
			consoleDetail = snap.Console
		}
	}
	lineCounts := counts
	if page == "overview" {
		lineCounts = ""
	}
	return statusBarView{
		Health:      health,
		Detail:      detail,
		ConsoleLine: consoleStatusLine(health, consoleDetail, lineCounts),
		Tip:         fieldString(data, "StatusTip"),
		Counts:      counts,
		Keys:        defaultStatusKeys(page),
	}
}

func consoleStatusLine(health, detail, counts string) string {
	var parts []string
	if word := consoleHealthWord(health); word != "" {
		parts = append(parts, word)
	}
	if detail != "" {
		parts = append(parts, detail)
	}
	if counts != "" {
		parts = append(parts, counts)
	}
	return strings.Join(parts, " · ")
}

func consoleHealthWord(health string) string {
	switch health {
	case "All services up":
		return "up"
	case "Partial service":
		return "partial"
	case "System down":
		return "down"
	case "Ready", "":
		return ""
	default:
		return strings.ToLower(health)
	}
}

func cozyHealth(overall string) string {
	switch overall {
	case "":
		return ""
	case "healthy":
		return "All services up"
	case "partial":
		return "Partial service"
	case "down":
		return "System down"
	default:
		return overall
	}
}

func derivedHealth(data any, page string) string {
	switch page {
	case "dhcp":
		words, ok := dhcpStateWords(data)
		if !ok {
			return ""
		}
		return dhcpStateHealth(words)
	case "tftp":
		switch fieldString(data, "ServerStatus") {
		case "running":
			return "TFTP is running"
		case "stopped":
			return "TFTP is stopped"
		case "unknown":
			return "TFTP status is unknown"
		case "":
			return ""
		default:
			return "TFTP is " + fieldString(data, "ServerStatus")
		}
	default:
		return ""
	}
}

func derivedCounts(data any, page string) string {
	switch page {
	case "overview":
		leases, okL := fieldInt(data, "LeaseTotal")
		servers, okS := fieldInt(data, "ServerCount")
		if !okL && !okS {
			return ""
		}
		return countPhrase(leases, "lease", "leases") + " · " + countPhrase(servers, "server", "servers")
	case "dhcp":
		servers, _, leases, ok := dhcpTotals(data)
		if !ok {
			return ""
		}
		return countPhrase(servers, "server", "servers") + " · " + countPhrase(leases, "lease", "leases")
	case "tftp":
		files, okF := fieldInt(data, "FileCount")
		dirs, okD := fieldInt(data, "DirCount")
		if !okF && !okD {
			return ""
		}
		return countPhrase(files, "file", "files") + " · " + countPhrase(dirs, "folder", "folders")
	case "osimages":
		n, ok := fieldLen(data, "OSImages")
		if !ok {
			return ""
		}
		return countPhrase(n, "image", "images")
	case "syslinux":
		n, ok := fieldLen(data, "Versions")
		if !ok {
			return ""
		}
		return countPhrase(n, "version", "versions")
	case "provision":
		n, ok := fieldLen(data, "Files")
		if !ok {
			return ""
		}
		return countPhrase(n, "file", "files")
	default:
		return ""
	}
}

func defaultStatusKeys(page string) []statusKey {
	switch page {
	case "overview":
		return []statusKey{{Key: "1", Label: "overview"}}
	case "dhcp":
		return []statusKey{{Key: "2", Label: "dhcp"}}
	case "tftp":
		return []statusKey{{Key: "3", Label: "tftp"}}
	case "osimages":
		return []statusKey{{Key: "4", Label: "images"}}
	case "syslinux":
		return []statusKey{{Key: "5", Label: "syslinux"}}
	case "provision":
		return []statusKey{{Key: "6", Label: "provision"}}
	default:
		return nil
	}
}

func countPhrase(n int, one, many string) string {
	word := many
	if n == 1 {
		word = one
	}
	return fmt.Sprintf("%d %s", n, word)
}

func dataValue(data any) reflect.Value {
	if data == nil {
		return reflect.Value{}
	}
	v := reflect.ValueOf(data)
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return reflect.Value{}
		}
		v = v.Elem()
	}
	return v
}

func structField(data any, name string) (reflect.Value, bool) {
	v := dataValue(data)
	if !v.IsValid() || v.Kind() != reflect.Struct {
		return reflect.Value{}, false
	}
	f := v.FieldByName(name)
	if !f.IsValid() || !f.CanInterface() {
		return reflect.Value{}, false
	}
	return f, true
}

func fieldInt(data any, name string) (int, bool) {
	f, ok := structField(data, name)
	if !ok {
		return 0, false
	}
	switch f.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return int(f.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int(f.Uint()), true
	default:
		return 0, false
	}
}

func fieldLen(data any, name string) (int, bool) {
	f, ok := structField(data, name)
	if !ok {
		return 0, false
	}
	if f.Kind() != reflect.Slice && f.Kind() != reflect.Array {
		return 0, false
	}
	return f.Len(), true
}

// dhcpStateWords reads the raw DHCP status from each server view. The
// status bar maps those words through dhcpStateLabel.
func dhcpStateWords(data any) ([]string, bool) {
	f, found := structField(data, "Servers")
	if !found || f.Kind() != reflect.Slice {
		return nil, false
	}
	words := make([]string, 0, f.Len())
	for i := 0; i < f.Len(); i++ {
		item := f.Index(i)
		for item.Kind() == reflect.Pointer || item.Kind() == reflect.Interface {
			if item.IsNil() {
				item = reflect.Value{}
				break
			}
			item = item.Elem()
		}
		if !item.IsValid() || item.Kind() != reflect.Struct {
			continue
		}
		raw := ""
		if status := item.FieldByName("Status"); status.IsValid() && status.Kind() == reflect.String {
			raw = strings.TrimSpace(status.String())
		}
		if raw == "" {
			if word := item.FieldByName("StatusWord"); word.IsValid() && word.Kind() == reflect.String {
				raw = strings.TrimSpace(word.String())
			}
		}
		words = append(words, raw)
	}
	return words, true
}

// dhcpStateHealth is the DHCP status-bar sentence. Every server sharing one
// word reads "DHCP is <word>". Mixed words are listed in server order.
func dhcpStateHealth(words []string) string {
	if len(words) == 0 {
		return "No DHCP servers yet"
	}
	var order []string
	seen := map[string]bool{}
	for _, word := range words {
		if word == "" || seen[word] {
			continue
		}
		seen[word] = true
		order = append(order, word)
	}
	if len(order) == 0 {
		return "No DHCP servers yet"
	}
	labels := make([]string, len(order))
	for i, word := range order {
		labels[i] = dhcpStateLabel(word)
	}
	if len(labels) == 1 {
		return "DHCP is " + labels[0]
	}
	return "DHCP is " + strings.Join(labels, " and ")
}

func dhcpTotals(data any) (servers, running, leases int, ok bool) {
	f, found := structField(data, "Servers")
	if !found || f.Kind() != reflect.Slice {
		return 0, 0, 0, false
	}
	servers = f.Len()
	for i := 0; i < servers; i++ {
		item := f.Index(i)
		for item.Kind() == reflect.Pointer || item.Kind() == reflect.Interface {
			if item.IsNil() {
				item = reflect.Value{}
				break
			}
			item = item.Elem()
		}
		if !item.IsValid() || item.Kind() != reflect.Struct {
			continue
		}
		if status := item.FieldByName("Status"); status.IsValid() && status.Kind() == reflect.String && status.String() == "running" {
			running++
		}
		if list := item.FieldByName("Leases"); list.IsValid() && (list.Kind() == reflect.Slice || list.Kind() == reflect.Array) {
			leases += list.Len()
		}
	}
	return servers, running, leases, true
}
