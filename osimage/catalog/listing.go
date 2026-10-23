package catalog

import (
	"sort"
	"time"
)

// Version is one row in the download picker and in oses[].versions.
type Version struct {
	Value       string `json:"value"`
	DisplayName string `json:"display_name"`
	Arch        string `json:"arch"`
	EOL         bool   `json:"eol"`
	EOLDate     string `json:"eol_date"`
	Latest      bool   `json:"latest"`
}

// OSListing is one distribution in the catalog document.
type OSListing struct {
	OS          string    `json:"os"`
	DisplayName string    `json:"display_name"`
	Versions    []Version `json:"versions"`
}

// Listing is the catalog document served by GET /osimages/catalog
// and rendered on the OS Images page.
type Listing struct {
	Source      string            `json:"source"`
	RefreshedAt string            `json:"last_refreshed_at"`
	Refreshing  bool              `json:"refreshing"`
	Errors      map[string]string `json:"errors"`
	OSes        []OSListing       `json:"oses"`
}

// Preferred order matches the previous OS Images dropdown.
var osOrder = []string{"ubuntu", "debian", "fedora", "centos", "opensuse", "nixos"}

var defaultNames = map[string]string{
	"ubuntu":   "Ubuntu",
	"debian":   "Debian",
	"fedora":   "Fedora",
	"centos":   "CentOS",
	"opensuse": "openSUSE",
	"nixos":    "NixOS",
}

// BuildListing groups releases into the API document. EOL releases are left
// out unless includeEOL is set. latest is the newest non-EOL version of each
// OS and architecture. Versions are newest first.
func BuildListing(snap Snapshot, now time.Time, names map[string]string, refreshing, includeEOL bool) Listing {
	byOS := map[string][]Release{}
	for _, rel := range snap.Releases {
		byOS[rel.OS] = append(byOS[rel.OS], rel)
	}
	order := append([]string{}, osOrder...)
	var extras []string
	for osName := range byOS {
		if !contains(order, osName) {
			extras = append(extras, osName)
		}
	}
	sort.Strings(extras)
	order = append(order, extras...)

	oses := make([]OSListing, 0, len(byOS))
	for _, osName := range order {
		releases := byOS[osName]
		if len(releases) == 0 {
			continue
		}
		versions := Versions(osName, releases, now, includeEOL)
		if len(versions) == 0 {
			continue
		}
		oses = append(oses, OSListing{
			OS:          osName,
			DisplayName: displayName(osName, names),
			Versions:    versions,
		})
	}

	source := string(snap.Origin)
	if source == "" {
		source = string(OriginBundled)
	}
	errors := snap.Errors
	if errors == nil {
		errors = map[string]string{}
	} else {
		copied := make(map[string]string, len(errors))
		for key, value := range errors {
			copied[key] = value
		}
		errors = copied
	}
	return Listing{
		Source:      source,
		RefreshedAt: FormatRFC3339(snap.RefreshedAt),
		Refreshing:  refreshing,
		Errors:      errors,
		OSes:        oses,
	}
}

// Versions returns the picker rows for one OS, newest first.
func Versions(osName string, releases []Release, now time.Time, includeEOL bool) []Version {
	var mine []Release
	for _, rel := range releases {
		if rel.OS == osName || osName == "" {
			mine = append(mine, rel)
		}
	}
	sort.SliceStable(mine, func(i, j int) bool {
		if mine[i].Version != mine[j].Version {
			return compareVersions(mine[i].Version, mine[j].Version) > 0
		}
		return mine[i].Arch < mine[j].Arch
	})

	latest := map[string]string{}
	for _, rel := range mine {
		if rel.EOL(now) {
			continue
		}
		if _, ok := latest[rel.Arch]; !ok {
			latest[rel.Arch] = rel.Version
		}
	}

	out := make([]Version, 0, len(mine))
	for _, rel := range mine {
		eol := rel.EOL(now)
		if eol && !includeEOL {
			continue
		}
		name := rel.DisplayName
		if name == "" {
			name = rel.Version
		}
		out = append(out, Version{
			Value:       rel.Version,
			DisplayName: name,
			Arch:        rel.Arch,
			EOL:         eol,
			EOLDate:     FormatDay(rel.EOLDate),
			Latest:      !eol && latest[rel.Arch] == rel.Version,
		})
	}
	return out
}

func displayName(osName string, names map[string]string) string {
	if name := names[osName]; name != "" {
		return name
	}
	if name := defaultNames[osName]; name != "" {
		return name
	}
	return osName
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
