package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
)

// OpenSUSE reads get.opensuse.org and keeps Leap releases whose state is
// Stable, plus one rolling Tumbleweed entry. Leap Micro, release candidates,
// and entries already marked EOL by openSUSE are left out.
type OpenSUSE struct {
	Client           *http.Client
	DistributionsURL string
}

func (o OpenSUSE) OS() string { return "opensuse" }

func (o OpenSUSE) distributionsURL() string {
	if o.DistributionsURL != "" {
		return o.DistributionsURL
	}
	return "https://get.opensuse.org/api/v0/distributions.json"
}

// Releases returns Leap Stable versions and Tumbleweed.
// The Tumbleweed version key stays "tumbleweed" so the rolling URLs match
// images that were downloaded from the bundled catalog.
func (o OpenSUSE) Releases(ctx context.Context) ([]Release, error) {
	body, _, err := fetch(ctx, o.Client, o.distributionsURL(), nil)
	if err != nil {
		return nil, fmt.Errorf("opensuse: %w", err)
	}
	var doc struct {
		Leap []struct {
			Version string `json:"version"`
			State   string `json:"state"`
		} `json:"Leap"`
		Tumbleweed []struct {
			Version string `json:"version"`
		} `json:"Tumbleweed"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("opensuse: decode distributions: %w", err)
	}
	seen := map[string]bool{}
	var versions []string
	for _, leap := range doc.Leap {
		if leap.State != "Stable" || leap.Version == "" || seen[leap.Version] {
			continue
		}
		seen[leap.Version] = true
		versions = append(versions, leap.Version)
	}
	sort.Slice(versions, func(i, j int) bool {
		return compareVersions(versions[i], versions[j]) < 0
	})
	out := make([]Release, 0, len(versions)+1)
	for _, version := range versions {
		base := "http://download.opensuse.org/distribution/leap/" + version + "/repo/oss/boot/x86_64/loader"
		out = append(out, Release{
			OS:          "opensuse",
			Version:     version,
			DisplayName: version,
			Arch:        "x86_64",
			KernelURL:   base + "/linux",
			InitrdURL:   base + "/initrd",
		})
	}
	if len(doc.Tumbleweed) > 0 {
		base := "http://download.opensuse.org/tumbleweed/repo/oss/boot/x86_64/loader"
		out = append(out, Release{
			OS:          "opensuse",
			Version:     "tumbleweed",
			DisplayName: "Tumbleweed (Rolling)",
			Arch:        "x86_64",
			KernelURL:   base + "/linux",
			InitrdURL:   base + "/initrd",
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("opensuse: no releases found")
	}
	return out, nil
}
