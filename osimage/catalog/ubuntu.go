package catalog

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Ubuntu reads netboot.xyz endpoints.yml and keeps LTS netboot entries.
// Interim releases such as 26.10, and HWE kernels, are left out.
type Ubuntu struct {
	Client       *http.Client
	EndpointsURL string
}

var ubuntuNetbootKey = regexp.MustCompile(`^ubuntu-netboot-(\d+)\.(\d+)-amd64$`)

func (u Ubuntu) OS() string { return "ubuntu" }

func (u Ubuntu) endpointsURL() string {
	if u.EndpointsURL != "" {
		return u.EndpointsURL
	}
	return "https://raw.githubusercontent.com/netbootxyz/netboot.xyz/master/endpoints.yml"
}

// Releases returns one x86_64 boot entry per Ubuntu LTS cycle.
// The version is the cycle (24.04), not the point release in the asset path.
func (u Ubuntu) Releases(ctx context.Context) ([]Release, error) {
	body, _, err := fetch(ctx, u.Client, u.endpointsURL(), nil)
	if err != nil {
		return nil, fmt.Errorf("ubuntu: %w", err)
	}
	var doc struct {
		Endpoints map[string]struct {
			Path string `yaml:"path"`
		} `yaml:"endpoints"`
	}
	if err := yaml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("ubuntu: decode endpoints.yml: %w", err)
	}
	paths := map[string]string{}
	var versions []string
	for key, entry := range doc.Endpoints {
		match := ubuntuNetbootKey.FindStringSubmatch(key)
		if match == nil || match[2] != "04" || entry.Path == "" {
			continue
		}
		version := match[1] + "." + match[2]
		path := entry.Path
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		if !strings.HasSuffix(path, "/") {
			path += "/"
		}
		if _, ok := paths[version]; ok {
			continue
		}
		paths[version] = path
		versions = append(versions, version)
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("ubuntu: no releases found")
	}
	sort.Slice(versions, func(i, j int) bool {
		return compareVersions(versions[i], versions[j]) < 0
	})
	const assetBase = "https://github.com/netbootxyz"
	out := make([]Release, 0, len(versions))
	for _, version := range versions {
		out = append(out, Release{
			OS:          "ubuntu",
			Version:     version,
			DisplayName: version + " LTS",
			Arch:        "x86_64",
			KernelURL:   assetBase + paths[version] + "vmlinuz",
			InitrdURL:   assetBase + paths[version] + "initrd",
		})
	}
	return out, nil
}
