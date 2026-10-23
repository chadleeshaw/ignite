package catalog

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	userAgent     = "ignite"
	maxBodyBytes  = 16 << 20
	clientTimeout = 20 * time.Second
)

// SourceTimeout bounds one release source and one end-of-life lookup.
var SourceTimeout = 15 * time.Second

// NewClient is the HTTP client catalog sources use. Callers can pass a
// different client, including an httptest client, to Releases and EOLDates.
func NewClient() *http.Client {
	return &http.Client{Timeout: clientTimeout}
}

func fetch(ctx context.Context, client *http.Client, rawURL string, headers http.Header) ([]byte, http.Header, error) {
	if client == nil {
		return nil, nil, fmt.Errorf("http client is nil")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("GET %s: %w", rawURL, err)
	}
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", userAgent)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("GET %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, resp.Header, fmt.Errorf("GET %s: %w", rawURL, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, resp.Header, fmt.Errorf("GET %s: HTTP %d", rawURL, resp.StatusCode)
	}
	return body, resp.Header, nil
}

func withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := SourceTimeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return context.WithTimeout(ctx, timeout)
}
