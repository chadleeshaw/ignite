package handlers

import (
	"strings"
	"testing"
	"time"

	"ignite/osimage"
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
		LeaseTotal:    5,
		ServerCount:   2,
		DHCPRunning:   0,
		Status:        mockStatusPageData(true),
	})
	for _, want := range []string{
		`data-theme="cozy"`,
		"/dashboard/summary",
		"System Healthy",
		"Active Leases: 3",
		"All services are up",
		"Leases",
	} {
		if !strings.Contains(overview, want) {
			t.Errorf("overview missing %q", want)
		}
	}
	if strings.Contains(overview, "localStorage") {
		t.Error("overview still mentions localStorage")
	}
	for _, want := range []string{
		`class="sb"`,
		"All services up",
		"5 leases · 2 servers",
		"CONSOLE",
		`<kbd>1</kbd> overview`,
		`<kbd>\</kbd> cozy · <kbd>?</kbd> help`,
		"Tip: <kbd>\\</kbd> switches Console",
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
		Title   string
		Mode    string
		Page    string
		Servers []DHCPServerView
	}{
		Title: "DHCP",
		Mode:  "console",
		Page:  "dhcp",
		Servers: []DHCPServerView{{
			TFTPIP:     "192.168.1.2",
			Running:    false,
			LeaseRange: 50,
			Used:       1,
			Free:       49,
			Leases: []LeaseView{{
				MAC:              "aa:bb:cc:dd:ee:ff",
				IP:               "192.168.1.100",
				State:            "assigned",
				StateBadgeClass:  "badge-info",
				StateDisplayName: "Assigned",
				LastSeen:         time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
			}, {
				MAC:              "11:22:33:44:55:66",
				IP:               "192.168.1.101",
				StateDisplayName: "Offline",
				StateBadgeClass:  "badge-ghost",
			}},
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
	if !strings.Contains(console, `data-seen-title="2026-01-02 03:04:05"`) {
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
		"DHCP is stopped",
		"1 server · 2 leases",
		"CONSOLE",
		`<kbd>2</kbd> dhcp`,
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

	tftp := renderNamed(t, "tftp", &TFTPData{
		Title:           "TFTP",
		Page:            "tftp",
		ServerStatus:    "unknown",
		Root:            "/srv/tftp",
		FileCount:       1,
		DirCount:        1,
		ServerDirectory: "ubuntu",
		Breadcrumbs:     []TFTPCrumb{{Name: "ubuntu", Dir: "ubuntu"}},
		Files: []FileInfo{{
			Name: "ubuntu", Size: "—", LastModified: "2026-01-02 03:04:05", IsDir: true,
		}},
	})
	if !strings.Contains(tftp, "/tftp/open?dir=ubuntu") || !strings.Contains(tftp, "/srv/tftp") {
		t.Error("tftp breadcrumbs missing")
	}
	if !strings.Contains(tftp, `data-theme="cozy"`) {
		t.Error("missing Mode should still be cozy")
	}
	for _, want := range []string{"TFTP status is unknown", "1 file · 1 folder", `<kbd>3</kbd> tftp`, `class="sb"`} {
		if !strings.Contains(tftp, want) {
			t.Errorf("tftp status bar missing %q", want)
		}
	}

	osOut := renderNamed(t, "osimages", struct {
		Title         string
		Mode          string
		Page          string
		OSImages      []*osimage.OSImage
		Downloads     []*osimage.DownloadStatus
		Architectures []string
	}{
		Title: "OS Images", Page: "osimages", Architectures: []string{"x86_64"},
		OSImages: []*osimage.OSImage{{
			ID: "img1", OS: "ubuntu", Version: "24.04", Architecture: "x86_64",
			KernelSize: 1000, CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		}},
	})
	if !strings.Contains(osOut, "24.04") || !strings.Contains(osOut, `id="versionSelect"`) || !strings.Contains(osOut, `class="ico ubuntu"`) {
		t.Error("os images list or download form missing")
	}
	if !strings.Contains(osOut, "1 image") || !strings.Contains(osOut, `<kbd>4</kbd> images`) || !strings.Contains(osOut, "Ready") {
		t.Error("os images status bar missing")
	}

	sys := renderNamed(t, "syslinux", struct {
		Title     string
		Mode      string
		Page      string
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
		Versions  []*syslinux.SyslinuxVersion
		Downloads []*syslinux.DownloadStatus
	}{
		Title: "Syslinux", Page: "syslinux",
		Versions: []*syslinux.SyslinuxVersion{{
			Version: "6.04-pre1", Downloaded: true, Active: false, Size: 2048,
		}},
	})
	if strings.Contains(idle, "/api/syslinux/activate/") {
		t.Error("activate control shown before the route works")
	}
	if !strings.Contains(sys, "1 version") || !strings.Contains(sys, `<kbd>5</kbd> syslinux`) {
		t.Error("syslinux status bar missing")
	}

	prov := renderNamed(t, "provision", &ProvisionData{
		Title: "Provision", Page: "provision", Files: nil, Categories: []string{"ipxe"},
	})
	for _, id := range []string{"file-search", "type-filter", "category-filter", "current-filename", "editor-textarea", "save-btn", "help-content", "editor-info"} {
		if !strings.Contains(prov, `id="`+id+`"`) {
			t.Errorf("provision missing %s", id)
		}
	}
	for _, want := range []string{`class="sb"`, "CONSOLE", "0 files", `<kbd>6</kbd> provision`, `<kbd>⌘S</kbd> save`, `<kbd>\</kbd> cozy · <kbd>?</kbd> help`} {
		if !strings.Contains(prov, want) {
			t.Errorf("provision status bar missing %q", want)
		}
	}
	if strings.Contains(prov, "innerHTML") || strings.Contains(prov, "outerHTML") || strings.Contains(prov, "insertAdjacentHTML") {
		t.Error("provision page still has an HTML string sink")
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
