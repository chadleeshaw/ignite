package handlers

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
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

// rfc3339 renders a timestamp for a <time datetime> attribute.
// Zero and unusable values return an empty string.
func rfc3339(v any) string {
	t, ok := asTime(v)
	if !ok || t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
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
	issues := statusIssues(s)
	switch {
	case s.HTTPServer.Status != "running":
		s.OverallStatus = "down"
	case len(issues) > 0:
		s.OverallStatus = "partial"
	default:
		s.OverallStatus = "healthy"
	}
	if len(issues) == 0 {
		s.AttentionCozy = "All services are up"
		s.AttentionConsole = "services up"
		return
	}
	joined := strings.Join(issues, " · ")
	s.AttentionCozy = attentionLead(len(issues)) + ": " + joined
	s.AttentionConsole = joined
}

// statusIssues lists services that are down. Configured DHCP servers are
// omitted: they are defined and have not been started, which is not partial
// health. Servers that share a status are one issue.
func statusIssues(s *StatusPageData) []string {
	if s == nil {
		return nil
	}
	var issues []string
	if s.HTTPServer.Status != "running" {
		issues = append(issues, "HTTP "+statusWord(s.HTTPServer.Status))
	}
	if s.TFTPServer.Status != "running" {
		issues = append(issues, "TFTP "+statusWord(s.TFTPServer.Status))
	}
	grouped := map[string][]string{}
	var order []string
	for _, srv := range s.DHCPServers {
		switch srv.Status {
		case "running", "configured":
			continue
		}
		word := dhcpStateLabel(srv.Status)
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
		issues = append(issues, "DHCP "+strings.Join(grouped[word], ", ")+" "+word)
	}
	return issues
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
	Health string
	Tip    string
	Counts string
	Keys   []statusKey
}

// pageHealthFn supplies OverallStatus for shells that do not set it.
// The application installs it. A nil function leaves the page fallback.
var pageHealthFn func(*http.Request) string

// SetPageHealth installs the host-health lookup used while rendering pages.
func SetPageHealth(fn func(*http.Request) string) {
	pageHealthFn = fn
}

// applyOverallStatus copies host health onto data when the page left
// OverallStatus empty. A missing field is left alone.
func applyOverallStatus(r *http.Request, data any) any {
	if pageHealthFn == nil || r == nil || fieldString(data, "OverallStatus") != "" {
		return data
	}
	status := pageHealthFn(r)
	if status == "" {
		return data
	}
	v := reflect.ValueOf(data)
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() || v.Elem().Kind() != reflect.Struct {
			return data
		}
		setStringField(v.Elem(), "OverallStatus", status)
		return data
	case reflect.Struct:
		cp := reflect.New(v.Type())
		cp.Elem().Set(v)
		setStringField(cp.Elem(), "OverallStatus", status)
		return cp.Interface()
	default:
		return data
	}
}

// statusBar builds the flame status bar from the page value. StatusHealth,
// StatusTip, and StatusCounts override the derived copy when a handler sets
// them. OverallStatus, when the page or the host lookup set it, is the
// health sentence on every page. A page without it keeps its own fallback.
func statusBar(data any) statusBarView {
	page := pageID(data)
	health := fieldString(data, "StatusHealth")
	if health == "" {
		health = cozyHealth(fieldString(data, "OverallStatus"))
	}
	if health == "" {
		health = derivedHealth(data, page)
	}
	if health == "" {
		health = "Ready"
	}
	counts := fieldString(data, "StatusCounts")
	if counts == "" {
		counts = derivedCounts(data, page)
	}
	return statusBarView{
		Health: health,
		Tip:    fieldString(data, "StatusTip"),
		Counts: counts,
		Keys:   defaultStatusKeys(page),
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
		return []statusKey{{Key: "6", Label: "provision"}, {Key: "⌘S", Label: "save"}}
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
