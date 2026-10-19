package handlers

import (
	"fmt"
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

// describeStatus turns tracked service state into the Overview subtitle.
// It only names services the status data actually reports.
func describeStatus(s *StatusPageData) (cozy, console string) {
	if s == nil {
		return "Service status is unavailable", "status unavailable"
	}
	var parts []string
	if s.HTTPServer.Status != "running" {
		parts = append(parts, "HTTP "+s.HTTPServer.Status)
	}
	if s.TFTPServer.Status != "running" {
		parts = append(parts, "TFTP "+s.TFTPServer.Status)
	}
	var stopped []string
	for _, dhcp := range s.DHCPServers {
		if dhcp.Status != "running" {
			stopped = append(stopped, dhcp.IP)
		}
	}
	if len(stopped) > 0 {
		parts = append(parts, "DHCP "+strings.Join(stopped, ", "))
	}
	if len(parts) == 0 {
		return "All services are up", "services up"
	}
	noun := "things"
	if len(parts) == 1 {
		noun = "thing"
	}
	joined := strings.Join(parts, " · ")
	return fmt.Sprintf("%d %s need attention: %s", len(parts), noun, joined), joined
}
