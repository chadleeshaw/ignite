package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// NixOS reads nix-community/nixos-images GitHub releases.
// A 403 or any other failure is a source error so the previous NixOS
// releases stay in the snapshot.
type NixOS struct {
	Client *http.Client
	APIURL string
	Token  string
}

var nixosTag = regexp.MustCompile(`^nixos-(\d+\.\d+)$`)

func (n NixOS) OS() string { return "nixos" }

func (n NixOS) apiURL() string {
	if n.APIURL != "" {
		return n.APIURL
	}
	return "https://api.github.com/repos/nix-community/nixos-images/releases?per_page=100"
}

// Releases returns stable nixos-YY.MM releases that publish both x86_64 boot files.
func (n NixOS) Releases(ctx context.Context) ([]Release, error) {
	next := n.apiURL()
	seen := map[string]bool{}
	var out []Release
	for page := 0; page < 3 && next != ""; page++ {
		headers := http.Header{}
		headers.Set("Accept", "application/vnd.github+json")
		if n.Token != "" {
			headers.Set("Authorization", "Bearer "+n.Token)
		}
		body, respHeaders, err := fetch(ctx, n.Client, next, headers)
		if err != nil {
			return nil, fmt.Errorf("nixos: %w", err)
		}
		var rows []struct {
			TagName    string `json:"tag_name"`
			Draft      bool   `json:"draft"`
			Prerelease bool   `json:"prerelease"`
			Assets     []struct {
				Name string `json:"name"`
				URL  string `json:"browser_download_url"`
			} `json:"assets"`
		}
		if err := json.Unmarshal(body, &rows); err != nil {
			return nil, fmt.Errorf("nixos: decode releases: %w", err)
		}
		for _, row := range rows {
			if row.Draft || row.Prerelease {
				continue
			}
			match := nixosTag.FindStringSubmatch(row.TagName)
			if match == nil || seen[match[1]] {
				continue
			}
			var kernel, initrd string
			for _, asset := range row.Assets {
				switch asset.Name {
				case "bzImage-x86_64-linux":
					kernel = asset.URL
				case "initrd-x86_64-linux":
					initrd = asset.URL
				}
			}
			if kernel == "" || initrd == "" {
				continue
			}
			seen[match[1]] = true
			out = append(out, Release{
				OS:          "nixos",
				Version:     match[1],
				DisplayName: match[1],
				Arch:        "x86_64",
				KernelURL:   kernel,
				InitrdURL:   initrd,
			})
		}
		if respHeaders == nil {
			break
		}
		next = nextLink(respHeaders.Get("Link"))
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("nixos: no releases found")
	}
	return out, nil
}

func nextLink(header string) string {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if !strings.Contains(part, `rel="next"`) {
			continue
		}
		start := strings.Index(part, "<")
		end := strings.Index(part, ">")
		if start >= 0 && end > start {
			return part[start+1 : end]
		}
	}
	return ""
}
