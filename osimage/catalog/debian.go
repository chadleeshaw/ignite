package catalog

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// Debian discovers stable and oldstable from the Debian mirror Release files.
type Debian struct {
	Client *http.Client
	Mirror string
}

func (d Debian) OS() string { return "debian" }

func (d Debian) mirror() string {
	if d.Mirror != "" {
		return strings.TrimRight(d.Mirror, "/")
	}
	return "http://deb.debian.org/debian"
}

// Releases returns the current stable and oldstable installers.
func (d Debian) Releases(ctx context.Context) ([]Release, error) {
	var out []Release
	seen := map[string]bool{}
	for _, suite := range []string{"stable", "oldstable"} {
		rel, err := d.suite(ctx, suite)
		if err != nil {
			return nil, err
		}
		if seen[rel.Version] {
			continue
		}
		seen[rel.Version] = true
		out = append(out, rel)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("debian: no releases found")
	}
	return out, nil
}

func (d Debian) suite(ctx context.Context, suite string) (Release, error) {
	body, _, err := fetch(ctx, d.Client, d.mirror()+"/dists/"+suite+"/Release", nil)
	if err != nil {
		return Release{}, fmt.Errorf("debian %s: %w", suite, err)
	}
	version, codename, err := parseDebianRelease(string(body))
	if err != nil {
		return Release{}, fmt.Errorf("debian %s: %w", suite, err)
	}
	major := majorVersion(version)
	images := d.mirror() + "/dists/" + codename + "/main/installer-amd64/current/images"
	base := images + "/netboot/debian-installer/amd64"
	rel := Release{
		OS:          "debian",
		Version:     major,
		DisplayName: major + " (" + titleWord(codename) + ")",
		Arch:        "x86_64",
		KernelURL:   base + "/linux",
		InitrdURL:   base + "/initrd.gz",
	}
	if sums, _, err := fetch(ctx, d.Client, images+"/SHA256SUMS", nil); err == nil {
		rel.KernelSHA256, rel.InitrdSHA256 = debianBootSums(sums)
	}
	return rel, nil
}

func parseDebianRelease(body string) (version, codename string, err error) {
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "Version":
			version = strings.TrimSpace(value)
		case "Codename":
			codename = strings.TrimSpace(value)
		}
	}
	if version == "" || codename == "" {
		return "", "", fmt.Errorf("Release is missing Version or Codename")
	}
	return version, codename, nil
}

func majorVersion(version string) string {
	if dot := strings.Index(version, "."); dot > 0 {
		return version[:dot]
	}
	return version
}

func titleWord(value string) string {
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}
