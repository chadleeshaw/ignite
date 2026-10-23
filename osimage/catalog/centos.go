package catalog

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
)

var centosStreamLink = regexp.MustCompile(`href="([0-9]+)-stream/"`)

// CentOS discovers CentOS Stream versions from the mirror directory index.
type CentOS struct {
	Client *http.Client
	Mirror string
}

func (c CentOS) OS() string { return "centos" }

func (c CentOS) mirror() string {
	if c.Mirror != "" {
		return strings.TrimRight(c.Mirror, "/")
	}
	return "http://mirror.stream.centos.org"
}

// Releases returns one boot entry per <N>-stream directory.
func (c CentOS) Releases(ctx context.Context) ([]Release, error) {
	body, _, err := fetch(ctx, c.Client, c.mirror()+"/", nil)
	if err != nil {
		return nil, fmt.Errorf("centos: %w", err)
	}
	matches := centosStreamLink.FindAllStringSubmatch(string(body), -1)
	seen := map[string]bool{}
	var versions []string
	for _, match := range matches {
		if seen[match[1]] {
			continue
		}
		seen[match[1]] = true
		versions = append(versions, match[1])
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("centos: no releases found")
	}
	sort.Slice(versions, func(i, j int) bool {
		return compareVersions(versions[i], versions[j]) < 0
	})
	out := make([]Release, 0, len(versions))
	for _, version := range versions {
		osURL := fmt.Sprintf("%s/%s-stream/BaseOS/x86_64/os", c.mirror(), version)
		base := osURL + "/images/pxeboot"
		rel := Release{
			OS:          "centos",
			Version:     version,
			DisplayName: version + " Stream",
			Arch:        "x86_64",
			KernelURL:   base + "/vmlinuz",
			InitrdURL:   base + "/initrd.img",
		}
		if tree, _, err := fetch(ctx, c.Client, osURL+"/.treeinfo", nil); err == nil {
			rel.KernelSHA256, rel.InitrdSHA256 = pxeBootSums(tree)
		}
		out = append(out, rel)
	}
	return out, nil
}
