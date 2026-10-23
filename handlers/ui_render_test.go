package handlers

import (
	"strings"
	"testing"
	"time"

	"ignite/osimage"
	"ignite/osimage/catalog"
	"ignite/syslinux"
)

func TestUIModeDefaultsToCozy(t *testing.T) {
	if got := uiMode(nil); got != "cozy" {
		t.Fatalf("nil data: got %q", got)
	}
	if got := uiMode(struct{ Title string }{Title: "Ignite"}); got != "cozy" {
		t.Fatalf("missing Mode: got %q", got)
	}
	if got := uiMode(struct{ Mode string }{Mode: "nope"}); got != "cozy" {
		t.Fatalf("unknown Mode: got %q", got)
	}
	if got := uiMode(map[string]string{"Mode": "console"}); got != "console" {
		t.Fatalf("map Mode: got %q", got)
	}
}

func TestRelTime(t *testing.T) {
	if got := relTime(time.Time{}); got != "never" {
		t.Fatalf("zero: %q", got)
	}
	if got := relTime(time.Now()); got != "just now" {
		t.Fatalf("now: %q", got)
	}
	if got := relTime(time.Now().Add(-3 * time.Minute)); got != "3m ago" {
		t.Fatalf("3m: %q", got)
	}
}

func TestTFTPCrumbs(t *testing.T) {
	crumbs := tftpCrumbs("ubuntu/casper")
	if len(crumbs) != 2 || crumbs[0].Dir != "ubuntu" || crumbs[1].Dir != "ubuntu/casper" || crumbs[1].Name != "casper" {
		t.Fatalf("crumbs: %+v", crumbs)
	}
	if tftpCrumbs("") != nil || tftpCrumbs(".") != nil {
		t.Fatal("root should have no crumbs")
	}
}

func TestDescribeStatus(t *testing.T) {
	cozy, console := describeStatus(nil)
	if !strings.Contains(cozy, "unavailable") || console != "status unavailable" {
		t.Fatalf("nil: %q / %q", cozy, console)
	}
	cozy, console = describeStatus(&StatusPageData{
		HTTPServer:  ServiceStatus{Status: "running"},
		TFTPServer:  ServiceStatus{Status: "running"},
		DHCPServers: []DHCPServerStatus{{IP: "10.0.0.1", Status: "running"}},
	})
	if cozy != "All services are up" || console != "services up" {
		t.Fatalf("healthy: %q / %q", cozy, console)
	}
}

func TestAttentionAgreesWithDHCPStatus(t *testing.T) {
	one := &StatusPageData{
		HTTPServer: ServiceStatus{Status: "running"},
		TFTPServer: ServiceStatus{Status: "running"},
		DHCPServers: []DHCPServerStatus{
			{IP: "192.168.1.2", Status: "configured"},
			{IP: "10.0.1.2", Status: "configured"},
		},
	}
	annotateStatus(one)
	if one.OverallStatus != "healthy" {
		t.Fatalf("overall %q", one.OverallStatus)
	}
	if one.AttentionCozy != "No DHCP server is running" || one.AttentionConsole != "no dhcp server running" {
		t.Fatalf("configured is not an issue: %q / %q", one.AttentionCozy, one.AttentionConsole)
	}

	stopped := &StatusPageData{
		HTTPServer:  ServiceStatus{Status: "running"},
		TFTPServer:  ServiceStatus{Status: "running"},
		DHCPServers: []DHCPServerStatus{{IP: "10.0.0.2", Status: "stopped"}},
	}
	annotateStatus(stopped)
	const wantStopped = "1 thing needs attention: DHCP 10.0.0.2 is stopped"
	if stopped.OverallStatus != "partial" || stopped.AttentionCozy != wantStopped {
		t.Fatalf("stopped: %s %q", stopped.OverallStatus, stopped.AttentionCozy)
	}

	two := &StatusPageData{
		HTTPServer:  ServiceStatus{Status: "running"},
		TFTPServer:  ServiceStatus{Status: "stopped"},
		DHCPServers: []DHCPServerStatus{{IP: "10.0.0.1", Status: "configured"}},
	}
	annotateStatus(two)
	const wantTwo = "1 thing needs attention: TFTP is stopped"
	if two.OverallStatus != "partial" || two.AttentionCozy != wantTwo {
		t.Fatalf("two issues: %s %q", two.OverallStatus, two.AttentionCozy)
	}

	down := &StatusPageData{
		HTTPServer: ServiceStatus{Status: "down"},
		TFTPServer: ServiceStatus{Status: "running"},
	}
	annotateStatus(down)
	if down.OverallStatus != "down" || down.AttentionCozy != "1 thing needs attention: HTTP is down" {
		t.Fatalf("down: %s %q", down.OverallStatus, down.AttentionCozy)
	}
}

func renderNamed(t *testing.T, name string, data any) string {
	t.Helper()
	chdirRepoRoot(t)
	tmpl, ok := LoadTemplates()[name]
	if !ok || tmpl == nil {
		t.Fatalf("template %q not available", name)
	}
	var sb strings.Builder
	if err := tmpl.Execute(&sb, data); err != nil {
		t.Fatalf("template %q: %v", name, err)
	}
	return sb.String()
}

func TestPagesRenderThemesFromMode(t *testing.T) {
	overview := renderNamed(t, "index", OverviewPage{
		Title:         "Overview",
		Page:          "overview",
		OverallStatus: "healthy",
		CozySubtitle:  "All services are up",
		HTTPStatus:    "running",
		TFTPStatus:    "unknown",
		CSRFToken:     "session-token",
		LeaseTotal:    5,
		ServerCount:   2,
		DHCPRunning:   0,
		Status:        mockStatusPageData(true),
	})
	for _, want := range []string{
		`data-theme="cozy"`,
		"/dashboard/summary",
		"All services up",
		"3 leases",
		"All services are up",
		"Active leases",
		"In flight",
		"Free addresses",
	} {
		if !strings.Contains(overview, want) {
			t.Errorf("overview missing %q", want)
		}
	}
	if strings.Contains(overview, "localStorage") || strings.Contains(overview, "document.cookie") || strings.Contains(overview, "ignite_mode") {
		t.Error("overview still reads client storage or the mode cookie")
	}
	for _, want := range []string{
		`class="sb"`,
		"All services up",
		"CONSOLE",
		`<kbd>1</kbd>–<kbd>6</kbd> pages`,
		`<kbd>\</kbd> cozy`,
		"Tip: <kbd>\\</kbd> switches to Console",
	} {
		if !strings.Contains(overview, want) {
			t.Errorf("overview status bar missing %q", want)
		}
	}
	for _, want := range []string{
		`method="post"`,
		`action="/api/preferences/mode"`,
		`name="mode"`,
		`value="cozy"`,
		`value="console"`,
		`aria-current="page"`,
		`aria-pressed="true"`,
		`name="csrf-token" content="session-token"`,
		`name="csrf_token" value="session-token"`,
		`id="logout-form"`,
		`action="/auth/logout"`,
		`hx-headers=`,
		`X-CSRF-Token`,
		`function igniteFetch`,
	} {
		if !strings.Contains(overview, want) {
			t.Errorf("overview mode form missing %q", want)
		}
	}
	if i := strings.Index(overview, "IGNITE_MODE_URL"); i >= 0 {
		end := i + 120
		if end > len(overview) {
			end = len(overview)
		}
		script := overview[i:end]
		if !strings.Contains(script, `\/api\/preferences\/mode`) {
			t.Fatalf("mode url script: %q", script)
		}
	} else {
		t.Fatal("mode script missing")
	}

	console := renderNamed(t, "dhcp", struct {
		Title         string
		Mode          string
		Page          string
		CSRFToken     string
		OverallStatus string
		Servers       []DHCPServerView
		Strip         leaseStrip
	}{
		Title:         "DHCP",
		Mode:          "console",
		Page:          "dhcp",
		OverallStatus: "partial",
		Servers: []DHCPServerView{{
			TFTPIP:     "192.168.1.2",
			Status:     "configured",
			StatusWord: "Configured",
			LeaseRange: 50,
			Used:       1,
			Free:       49,
			Leases: []LeaseView{{
				MAC:              "aa:bb:cc:dd:ee:ff",
				IP:               "192.168.1.100",
				State:            "assigned",
				StateBadgeClass:  "badge-info",
				StateDisplayName: "Assigned",
				MenuName:         "/tmp/ignite-tftp.UPoTst/pxelinux.cfg/01-00-11-22-33-44-55",
				LastSeen:         time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
				LastSeenAt:       "2026-01-02T03:04:05Z",
			}, {
				MAC:              "11:22:33:44:55:66",
				IP:               "192.168.1.101",
				StateDisplayName: "Offline",
				StateBadgeClass:  "badge-ghost",
				MenuName:         "ubuntu 24.04",
			}},
		}, {
			TFTPIP:     "10.0.1.2",
			Status:     "configured",
			StatusWord: "Configured",
			LeaseRange: 25,
		}},
	})
	if !strings.Contains(console, `data-theme="console"`) {
		t.Error("console mode did not set data-theme")
	}
	if !strings.Contains(console, `class="lease-pick"`) || !strings.Contains(console, `tabindex="0"`) {
		t.Error("console lease row is not keyboard focusable")
	}
	if strings.Contains(console, "insertAdjacentHTML") {
		t.Error("lease history still builds HTML from strings")
	}
	if !strings.Contains(console, `data-seen-title="2026-01-02 03:04:05 UTC"`) || !strings.Contains(console, `datetime="2026-01-02T03:04:05Z"`) {
		t.Error("missing absolute last-seen title")
	}
	if !strings.Contains(console, "No heartbeat recorded") {
		t.Error("missing zero last-seen title")
	}
	if !strings.Contains(console, ">never<") && !strings.Contains(console, ">never") {
		t.Error("missing relative never")
	}
	if strings.Contains(console, "localStorage") {
		t.Error("dhcp page still mentions localStorage")
	}
	for _, want := range []string{
		`class="sb"`,
		"Partial service",
		"2 servers · 2 leases",
		"Select a host",
		"In flight",
		"Last refresh",
		`id="det-title"`,
		"Last seen",
		"ubuntu 24.04",
		"No leases on this server.",
		`class="main"`,
		"CONSOLE",
		`<kbd>1</kbd>–<kbd>6</kbd> pages`,
		`class="label-fit"`,
		`aria-hidden="true">Reserved`,
		`aria-hidden="true">Start`,
		`class="actions stable"`,
	} {
		if !strings.Contains(console, want) {
			t.Errorf("dhcp actions or status bar missing %q", want)
		}
	}
	if strings.Contains(console, "innerHTML") || strings.Contains(console, "outerHTML") || strings.Contains(console, "insertAdjacentHTML") {
		t.Error("dhcp page still has an HTML string sink")
	}
	if strings.Contains(console, "/tmp/") || strings.Contains(console, "pxelinux.cfg") {
		t.Error("dhcp page renders an internal menu path")
	}
	if !strings.Contains(console, ">Configured<") {
		t.Error("dhcp page did not show the server state")
	}
	if !strings.Contains(console, `class="st cf"`) {
		t.Error("configured server is missing the idle pill")
	}
	if strings.Contains(console, ">Configured, not started<") {
		t.Error("configured pill still uses the old label")
	}

	tftp := renderNamed(t, "tftp", &TFTPData{
		Title:           "TFTP",
		Page:            "tftp",
		OverallStatus:   "healthy",
		ServerStatus:    "unknown",
		Root:            "/srv/tftp",
		FileCount:       1,
		DirCount:        1,
		ServerDirectory: "ubuntu",
		Breadcrumbs:     []TFTPCrumb{{Name: "ubuntu", Dir: "ubuntu"}},
		Files: []FileInfo{{
			Name: "ubuntu", Base: "ubuntu", Size: "—", IsDir: true,
			ModTime:        time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
			LastModified:   "2026-01-02 03:04:05 UTC",
			LastModifiedAt: "2026-01-02T03:04:05Z",
		}},
	})
	if !strings.Contains(tftp, "/tftp/open?dir=ubuntu") || !strings.Contains(tftp, `href="/tftp">⌂ TFTP root</a>`) {
		t.Error("tftp breadcrumbs missing")
	}
	if strings.Contains(tftp, `href="/tftp">/</a>`) || strings.Contains(tftp, "tftp root / /") {
		t.Error("tftp breadcrumb has an empty slash")
	}
	if strings.Contains(tftp, `href="/tftp">/srv/tftp</a>`) || strings.Contains(tftp, `class="mu">/srv/tftp`) {
		t.Error("tftp header printed the host directory")
	}
	if !strings.Contains(tftp, `stat-title">Root`) || !strings.Contains(tftp, `/srv/tftp`) {
		t.Error("tftp root tile missing")
	}
	if !strings.Contains(tftp, `data-theme="cozy"`) {
		t.Error("missing Mode should still be cozy")
	}
	for _, want := range []string{
		"All services up",
		"1 file · 1 folder",
		`<kbd>1</kbd>–<kbd>6</kbd> pages`,
		`class="sb"`,
		`>Up</a>`,
		`href="/tftp">⌂ TFTP root</a>`,
		`stat-title">Root`,
		`class="actions stable slots"`,
		`class="label-fit"`,
		`aria-hidden="true">Download`,
		`aria-hidden="true">View`,
		`fas fa-folder`,
		`ubuntu/`,
		`data-modified="2026-01-02T03:04:05Z"`,
		`data-time="datetime"`,
		`2026-01-02 03:04:05 UTC`,
		`disabled`,
	} {
		if !strings.Contains(tftp, want) {
			t.Errorf("tftp page missing %q", want)
		}
	}

	osOut := renderNamed(t, "osimages", struct {
		Title         string
		Mode          string
		Page          string
		CSRFToken     string
		OSImages      []*osimage.OSImage
		Downloads     []*osimage.DownloadStatus
		Architectures []string
		SupportedOS   []string
		Catalog       catalog.Listing
		KernelBytes   int64
		InitrdBytes   int64
		ArchLabel     string
		DownloadCount int
	}{
		Title: "OS Images", Page: "osimages", Architectures: []string{"x86_64"},
		Catalog: catalog.Listing{
			Source: "live", RefreshedAt: "2026-10-23T00:00:00Z",
			Errors: map[string]string{},
			OSes:   []catalog.OSListing{{OS: "ubuntu", DisplayName: "Ubuntu"}},
		},
		ArchLabel: "x86_64", KernelBytes: 1000,
		OSImages: []*osimage.OSImage{{
			ID: "img1", OS: "ubuntu", Version: "24.04", Architecture: "x86_64",
			KernelSize: 1000, CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		}},
	})
	if !strings.Contains(osOut, "24.04") || !strings.Contains(osOut, `id="versionSelect"`) || !strings.Contains(osOut, `class="ico ubuntu"`) || !strings.Contains(osOut, `data-eol="false"`) {
		t.Error("os images list or download form missing")
	}
	if !strings.Contains(osOut, "1 image") || !strings.Contains(osOut, `<kbd>1</kbd>–<kbd>6</kbd> pages`) || !strings.Contains(osOut, "Ready") {
		t.Error("os images status bar missing")
	}

	sys := renderNamed(t, "syslinux", struct {
		Title     string
		Mode      string
		Page      string
		CSRFToken string
		Versions  []*syslinux.SyslinuxVersion
		Downloads []*syslinux.DownloadStatus
	}{
		Title: "Syslinux", Page: "syslinux",
		Versions: []*syslinux.SyslinuxVersion{{
			Version: "6.03", Downloaded: true, Active: true, Size: 2048,
		}},
	})
	if !strings.Contains(sys, "6.03") || !strings.Contains(sys, "/api/syslinux/deactivate/6.03") || !strings.Contains(sys, "BIOS and EFI") {
		t.Error("syslinux active version actions missing")
	}
	if strings.Contains(sys, "/api/syslinux/activate/") {
		t.Error("activate control shown for the active version")
	}
	idle := renderNamed(t, "syslinux", struct {
		Title     string
		Mode      string
		Page      string
		CSRFToken string
		Versions  []*syslinux.SyslinuxVersion
		Downloads []*syslinux.DownloadStatus
	}{
		Title: "Syslinux", Page: "syslinux",
		Versions: []*syslinux.SyslinuxVersion{{
			Version: "6.04-pre1", Downloaded: true, Active: false, Size: 2048,
		}},
	})
	if !strings.Contains(idle, "/api/syslinux/activate/6.04-pre1") {
		t.Error("activate control missing for a downloaded inactive version")
	}
	moved := renderNamed(t, "syslinux", struct {
		Title     string
		Mode      string
		Page      string
		CSRFToken string
		Versions  []*syslinux.SyslinuxVersion
		Downloads []*syslinux.DownloadStatus
	}{
		Title: "Syslinux", Page: "syslinux",
		Versions: []*syslinux.SyslinuxVersion{{
			Version: "6.03", Downloaded: true, Active: true, Size: 2048, NeedsRedownload: true,
		}},
	})
	if !strings.Contains(moved, ">re-download<") || !strings.Contains(moved, ">Re-download<") || !strings.Contains(moved, "data directory") {
		t.Error("missing syslinux cache did not read as a re-download")
	}
	if strings.Contains(moved, "/api/syslinux/activate/") {
		t.Error("activate control shown when the cache is missing")
	}
	if !strings.Contains(sys, "1 version") || !strings.Contains(sys, `<kbd>1</kbd>–<kbd>6</kbd> pages`) {
		t.Error("syslinux status bar missing")
	}

	prov := renderNamed(t, "provision", &ProvisionData{
		Title: "Provision", Page: "provision", Files: nil, Categories: []string{"ipxe"},
	})
	for _, id := range []string{"file-list", "current-filename", "editor-textarea", "editor-lines", "save-btn", "help-content", "editor-info"} {
		if !strings.Contains(prov, `id="`+id+`"`) {
			t.Errorf("provision missing %s", id)
		}
	}
	for _, want := range []string{`class="sb"`, "CONSOLE", "0 files", `<kbd>1</kbd>–<kbd>6</kbd> pages`, `<kbd>\</kbd> cozy`} {
		if !strings.Contains(prov, want) {
			t.Errorf("provision status bar missing %q", want)
		}
	}
	if strings.Contains(prov, "innerHTML") || strings.Contains(prov, "outerHTML") || strings.Contains(prov, "insertAdjacentHTML") {
		t.Error("provision page still has an HTML string sink")
	}
	if !strings.Contains(prov, "/public/http/js/highlight.js") || !strings.Contains(prov, "igniteHighlight(") {
		t.Error("provision preview is not wired to the shared highlighter")
	}
}

func TestPendingLayoutHelpers(t *testing.T) {
	when := time.Date(2026, 10, 19, 22, 30, 54, 0, time.FixedZone("IST", 5*3600+30*60))
	if got := rfc3339(when); got != "2026-10-19T17:00:54Z" {
		t.Fatalf("time value: %q", got)
	}
	if got := rfc3339("2026-10-19T22:30:54+05:30"); got != "2026-10-19T22:30:54+05:30" {
		t.Fatalf("rfc3339 string: %q", got)
	}
	if rfc3339("2026-10-19 22:30:54") != "" || rfc3339(time.Time{}) != "" {
		t.Fatal("non-rfc3339 or zero time should be empty")
	}
	overview := statusSentence(struct {
		Page          string
		OverallStatus string
	}{Page: "overview", OverallStatus: "healthy"})
	if overview != "All services up" {
		t.Fatalf("overview health %q", overview)
	}
	dhcp := statusSentence(struct {
		Page          string
		OverallStatus string
		Servers       []DHCPServerView
	}{Page: "dhcp", OverallStatus: "partial", Servers: []DHCPServerView{{Status: "configured"}}})
	if dhcp != "Partial service" {
		t.Fatalf("dhcp health %q", dhcp)
	}
	if got := statusSentence(struct {
		Page    string
		Servers []DHCPServerView
	}{Page: "dhcp", Servers: []DHCPServerView{{Status: "configured"}}}); got != "Ready" {
		t.Fatalf("missing overall status %q", got)
	}
}

func TestStatusBarDefaults(t *testing.T) {
	bar := statusBar(nil)
	if bar.Health != "Ready" || bar.Counts != "" || bar.Keys != nil {
		t.Fatalf("nil: %+v", bar)
	}
	bar = statusBar(map[string]string{"Page": "overview", "StatusCounts": "4 pools"})
	if bar.Counts != "4 pools" || bar.Health != "Ready" || len(bar.Keys) != 1 || bar.Keys[0].Label != "overview" {
		t.Fatalf("override: %+v", bar)
	}
	bar = statusBar(struct {
		Page          string
		StatusHealth  string
		OverallStatus string
	}{Page: "overview", StatusHealth: "Custom health", OverallStatus: "down"})
	if bar.Health != "Custom health" {
		t.Fatalf("health override: %+v", bar)
	}
}

func TestLoginThemeFallsBackToCozy(t *testing.T) {
	empty := renderNamed(t, "login", struct {
		Title string
		Mode  string
	}{Title: "Ignite"})
	if !strings.Contains(empty, `data-theme="cozy"`) || !strings.Contains(empty, `class="login-card"`) {
		t.Fatalf("login did not default to cozy: %s", empty[:min(180, len(empty))])
	}
	console := renderNamed(t, "login", struct {
		Title string
		Mode  string
	}{Title: "Ignite", Mode: "console"})
	if !strings.Contains(console, `data-theme="console"`) {
		t.Fatal("login ignored console mode")
	}
}
