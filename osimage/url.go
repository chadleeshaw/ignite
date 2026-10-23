package osimage

import (
	"fmt"
	"net/url"
	"strings"
)

// bootFileURL joins a catalog base URL with the upstream boot file name.
// A trailing slash on the base and a leading slash on the name become one separator.
func bootFileURL(baseURL, name string) (string, error) {
	if strings.TrimSpace(baseURL) == "" {
		return "", fmt.Errorf("base URL is required")
	}
	name = strings.Trim(strings.TrimSpace(name), "/")
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("invalid boot file name %q", name)
	}
	joined, err := url.JoinPath(baseURL, name)
	if err != nil {
		return "", fmt.Errorf("join %q and %q: %w", baseURL, name, err)
	}
	return joined, nil
}
