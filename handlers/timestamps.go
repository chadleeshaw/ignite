package handlers

import "time"

// formatRFC3339 is the machine-readable timestamp for templates and JSON.
// The human-readable text stays a separate field. A zero time is empty so
// the frontend can keep its "never" copy. RFC3339 includes a numeric offset,
// or Z for UTC.
func formatRFC3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
