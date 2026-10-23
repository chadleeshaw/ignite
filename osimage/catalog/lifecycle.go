package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// EndOfLife reads cycle end dates from endoflife.date.
type EndOfLife struct {
	Client  *http.Client
	BaseURL string
}

// EOLDates returns cycle -> UTC date. A cycle whose eol field is not a date
// is omitted. The caller keeps those releases and does not hide them.
func (e EndOfLife) EOLDates(ctx context.Context, product string) (map[string]time.Time, error) {
	base := strings.TrimRight(e.BaseURL, "/")
	if base == "" {
		base = "https://endoflife.date/api"
	}
	product = strings.Trim(product, "/")
	if product == "" {
		return nil, fmt.Errorf("endoflife product is empty")
	}
	body, _, err := fetch(ctx, e.Client, base+"/"+product+".json", nil)
	if err != nil {
		return nil, fmt.Errorf("endoflife.date %s: %w", product, err)
	}
	return parseEOLDates(body)
}

func parseEOLDates(body []byte) (map[string]time.Time, error) {
	var cycles []struct {
		Cycle string          `json:"cycle"`
		EOL   json.RawMessage `json:"eol"`
	}
	if err := json.Unmarshal(body, &cycles); err != nil {
		return nil, fmt.Errorf("decode endoflife.date: %w", err)
	}
	dates := make(map[string]time.Time, len(cycles))
	for _, cycle := range cycles {
		if cycle.Cycle == "" || len(cycle.EOL) == 0 || string(cycle.EOL) == "null" {
			continue
		}
		var text string
		if err := json.Unmarshal(cycle.EOL, &text); err != nil {
			continue
		}
		day, err := time.Parse("2006-01-02", text)
		if err != nil {
			continue
		}
		dates[cycle.Cycle] = dateOnly(day)
	}
	return dates, nil
}

// eolProduct is the endoflife.date slug for an Ignite OS key.
func eolProduct(osName string) string {
	if osName == "centos" {
		return "centos-stream"
	}
	return osName
}

// lookupCycle finds the end date for version, then for each shorter prefix
// ("24.04.1" -> "24.04", "13.7" -> "13").
func lookupCycle(version string, dates map[string]time.Time) (time.Time, bool) {
	for version != "" {
		if day, ok := dates[version]; ok {
			return day, true
		}
		dot := strings.LastIndex(version, ".")
		if dot <= 0 {
			return time.Time{}, false
		}
		version = version[:dot]
	}
	return time.Time{}, false
}
