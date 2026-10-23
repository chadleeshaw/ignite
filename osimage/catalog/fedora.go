package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
)

var numericVersion = regexp.MustCompile(`^[0-9]+$`)

// Fedora reads fedoraproject.org/releases.json and keeps stable Everything
// x86_64 releases. Interim test releases such as "45 Beta" are skipped.
type Fedora struct {
	Client       *http.Client
	ReleasesURL  string
	DownloadBase string
}

func (f Fedora) OS() string { return "fedora" }

func (f Fedora) releasesURL() string {
	if f.ReleasesURL != "" {
		return f.ReleasesURL
	}
	return "https://fedoraproject.org/releases.json"
}

func (f Fedora) downloadBase() string {
	if f.DownloadBase != "" {
		return strings.TrimRight(f.DownloadBase, "/")
	}
	return "https://download.fedoraproject.org/pub/fedora/linux/releases"
}

// Releases returns one boot entry per stable Fedora version.
func (f Fedora) Releases(ctx context.Context) ([]Release, error) {
	body, _, err := fetch(ctx, f.Client, f.releasesURL(), nil)
	if err != nil {
		return nil, fmt.Errorf("fedora: %w", err)
	}
	var rows []struct {
		Version string `json:"version"`
		Arch    string `json:"arch"`
		Variant string `json:"variant"`
	}
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("fedora: decode releases.json: %w", err)
	}
	seen := map[string]bool{}
	var versions []string
	for _, row := range rows {
		if row.Arch != "x86_64" || row.Variant != "Everything" {
			continue
		}
		if !numericVersion.MatchString(row.Version) {
			continue
		}
		if seen[row.Version] {
			continue
		}
		seen[row.Version] = true
		versions = append(versions, row.Version)
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("fedora: no releases found")
	}
	sort.Slice(versions, func(i, j int) bool {
		return compareVersions(versions[i], versions[j]) < 0
	})
	out := make([]Release, 0, len(versions))
	for _, version := range versions {
		osURL := fmt.Sprintf("%s/%s/Everything/x86_64/os", f.downloadBase(), version)
		base := osURL + "/images/pxeboot"
		rel := Release{
			OS:          "fedora",
			Version:     version,
			DisplayName: version,
			Arch:        "x86_64",
			KernelURL:   base + "/vmlinuz",
			InitrdURL:   base + "/initrd.img",
		}
		if tree, _, err := fetch(ctx, f.Client, osURL+"/.treeinfo", nil); err == nil {
			rel.KernelSHA256, rel.InitrdSHA256 = pxeBootSums(tree)
		}
		out = append(out, rel)
	}
	return out, nil
}
