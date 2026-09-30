package handlers

// Regression test for the v2.2.0 status-page outage: the status templates
// referenced fields that did not exist on the handler structs (and called
// .Format on a string), so /status returned HTTP 200 with a truncated body.
// Executing the templates to completion with fully-populated data is the
// only check that catches this — a status-code assertion does not.

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func mockStatusPageData(withServers bool) *StatusPageData {
	now := time.Now()
	data := &StatusPageData{
		Title: "System Status",
		HTTPServer: ServiceStatus{
			Name:        "HTTP Server",
			Status:      "running",
			Description: "Web interface and REST API",
			Details:     "Port 8080 - responding",
			Port:        8080,
			LastCheck:   now,
		},
		TFTPServer: ServiceStatus{
			Name:        "TFTP Server",
			Status:      "running",
			Description: "Network boot file server for PXE clients",
			Details:     "Port 69",
			Port:        69,
			LastCheck:   now,
		},
		LastUpdated:   now.Format("2006-01-02 15:04:05"),
		OverallStatus: "healthy",
		AutoRefresh:   true,
		RefreshSecs:   5,
	}
	if withServers {
		data.DHCPServers = []DHCPServerStatus{
			{
				ID:          "srv1",
				IP:          "192.168.1.1",
				Status:      "running",
				Description: "Dynamic IP assignment for network boot clients",
				LeaseCount:  3,
				LastCheck:   now,
			},
		}
	}
	return data
}

// renderStatusTemplate executes a cached template by name with the given
// data and fails the test on any execution error.
func renderStatusTemplate(t *testing.T, name string, data *StatusPageData) string {
	t.Helper()

	// LoadTemplates resolves "templates/..." relative to the working
	// directory, so run from the module root.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test source file")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..")
	prevWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevWd) })

	tmpl, ok := LoadTemplates()[name]
	if !ok || tmpl == nil {
		t.Fatalf("template %q not available", name)
	}
	var sb strings.Builder
	if err := tmpl.Execute(&sb, data); err != nil {
		t.Fatalf("template %q execution failed: %v", name, err)
	}
	return sb.String()
}

func TestStatusPageRendersFully(t *testing.T) {
	out := renderStatusTemplate(t, "status", mockStatusPageData(true))

	for _, want := range []string{
		"System Healthy",   // OverallStatus badge
		"192.168.1.1",      // DHCP server IP
		"Active Leases: 3", // DHCP lease count
		"Last Updated:",    // string LastUpdated, not .Format on it
		"Port 8080 - responding",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("status page missing %q", want)
		}
	}
	// The truncated-body failure rendered HTTP 200 with the page cut off
	// mid-template; the full page must end with the base layout's </html>.
	if got := strings.TrimSpace(out); !strings.HasSuffix(got, "</html>") {
		t.Errorf("status page appears truncated: ends with %q", truncateTail(got, 60))
	}
}

func TestStatusContentRendersFully(t *testing.T) {
	out := renderStatusTemplate(t, "status-content", mockStatusPageData(true))

	for _, want := range []string{
		"System Healthy",
		"192.168.1.1",
		"Active Leases: 3",
		"Status automatically refreshes every 10 seconds",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("status-content missing %q", want)
		}
	}

	// Empty DHCP list must take the {{else}} branch, not error.
	empty := renderStatusTemplate(t, "status-content", mockStatusPageData(false))
	if !strings.Contains(empty, "No DHCP servers configured") {
		t.Errorf("status-content with no DHCP servers missing empty-state message")
	}
}

func truncateTail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
