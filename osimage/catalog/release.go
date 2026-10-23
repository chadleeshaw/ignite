package catalog

import (
	"encoding/json"
	"fmt"
	"time"
)

// Release is one bootable OS version offered for download.
type Release struct {
	OS           string    `json:"os"`
	Version      string    `json:"version"`
	DisplayName  string    `json:"display_name"`
	Arch         string    `json:"arch"`
	KernelURL    string    `json:"kernel_url"`
	InitrdURL    string    `json:"initrd_url"`
	KernelSHA256 string    `json:"kernel_sha256,omitempty"`
	InitrdSHA256 string    `json:"initrd_sha256,omitempty"`
	EOLDate      time.Time `json:"eol_date"`
}

// EOL reports whether the release is past its end-of-life date at now.
// A zero EOLDate means the date is unknown, which is treated as supported.
// The end-of-life date itself is still supported; the next UTC day is not.
func (r Release) EOL(now time.Time) bool {
	if r.EOLDate.IsZero() {
		return false
	}
	return dateOnly(now).After(dateOnly(r.EOLDate))
}

// FormatDay renders an end-of-life date as YYYY-MM-DD. A zero time is "".
func FormatDay(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return dateOnly(t).Format("2006-01-02")
}

// FormatRFC3339 renders a timestamp as UTC RFC3339. A zero time is "".
func FormatRFC3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func dateOnly(t time.Time) time.Time {
	if t.IsZero() {
		return time.Time{}
	}
	year, month, day := t.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

type releaseWire struct {
	OS           string `json:"os"`
	Version      string `json:"version"`
	DisplayName  string `json:"display_name"`
	Arch         string `json:"arch"`
	KernelURL    string `json:"kernel_url"`
	InitrdURL    string `json:"initrd_url"`
	KernelSHA256 string `json:"kernel_sha256,omitempty"`
	InitrdSHA256 string `json:"initrd_sha256,omitempty"`
	EOLDate      string `json:"eol_date,omitempty"`
}

// MarshalJSON writes eol_date as YYYY-MM-DD, or omits it when unknown.
func (r Release) MarshalJSON() ([]byte, error) {
	return json.Marshal(releaseWire{
		OS:           r.OS,
		Version:      r.Version,
		DisplayName:  r.DisplayName,
		Arch:         r.Arch,
		KernelURL:    r.KernelURL,
		InitrdURL:    r.InitrdURL,
		KernelSHA256: r.KernelSHA256,
		InitrdSHA256: r.InitrdSHA256,
		EOLDate:      FormatDay(r.EOLDate),
	})
}

// UnmarshalJSON accepts an empty eol_date, a YYYY-MM-DD date, or RFC3339.
func (r *Release) UnmarshalJSON(data []byte) error {
	var wire releaseWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	parsed := Release{
		OS:           wire.OS,
		Version:      wire.Version,
		DisplayName:  wire.DisplayName,
		Arch:         wire.Arch,
		KernelURL:    wire.KernelURL,
		InitrdURL:    wire.InitrdURL,
		KernelSHA256: wire.KernelSHA256,
		InitrdSHA256: wire.InitrdSHA256,
	}
	if wire.EOLDate != "" {
		day, err := time.Parse("2006-01-02", wire.EOLDate)
		if err != nil {
			day, err = time.Parse(time.RFC3339, wire.EOLDate)
			if err != nil {
				return fmt.Errorf("eol_date %q: %w", wire.EOLDate, err)
			}
		}
		parsed.EOLDate = dateOnly(day)
	}
	*r = parsed
	return nil
}
