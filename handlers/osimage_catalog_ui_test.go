package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ignite/osimage/catalog"

	"github.com/stretchr/testify/require"
)

func TestCatalogStatusChipStates(t *testing.T) {
	chdirRepoRoot(t)
	refreshed := time.Date(2026, 10, 22, 15, 4, 5, 0, time.UTC)

	t.Run("live", func(t *testing.T) {
		body := renderCatalogStatus(t, catalog.Snapshot{
			Origin:      catalog.OriginLive,
			RefreshedAt: refreshed,
			Releases:    []catalog.Release{{OS: "ubuntu", Version: "24.04", DisplayName: "24.04 LTS", Arch: "x86_64"}},
		}, false)
		require.Contains(t, body, "Catalog: live · refreshed")
		require.Contains(t, body, `datetime="2026-10-22T15:04:05Z"`)
		require.NotContains(t, body, "catalog-warn")
		require.NotContains(t, body, "Using the built-in list")
		require.NotContains(t, body, "Using cached list")
		require.NotContains(t, body, "aria-busy")
		require.NotContains(t, body, " disabled")
	})

	t.Run("cached", func(t *testing.T) {
		body := renderCatalogStatus(t, catalog.Snapshot{
			Origin:      catalog.OriginCached,
			RefreshedAt: refreshed,
			Releases:    []catalog.Release{{OS: "ubuntu", Version: "24.04", DisplayName: "24.04 LTS", Arch: "x86_64"}},
		}, false)
		require.Contains(t, body, "Using cached list from")
		require.Contains(t, body, `datetime="2026-10-22T15:04:05Z"`)
		require.Contains(t, body, "catalog-warn")
		require.NotContains(t, body, "last saved")
		require.NotContains(t, body, "built-in")
	})

	t.Run("bundled", func(t *testing.T) {
		body := renderCatalogStatus(t, catalog.Snapshot{
			Origin:   catalog.OriginBundled,
			Releases: []catalog.Release{{OS: "ubuntu", Version: "24.04", DisplayName: "24.04 LTS", Arch: "x86_64"}},
		}, false)
		require.Contains(t, body, "Using the built-in list")
		require.Contains(t, body, "Never")
		require.NotContains(t, body, "last saved")
		require.NotContains(t, body, "Using cached list")
	})

	t.Run("errors", func(t *testing.T) {
		body := renderCatalogStatus(t, catalog.Snapshot{
			Origin:      catalog.OriginLive,
			RefreshedAt: refreshed,
			Errors: map[string]string{
				"fedora": "timed out",
				"debian": "checksum mismatch",
			},
			Releases: []catalog.Release{{OS: "ubuntu", Version: "24.04", DisplayName: "24.04 LTS", Arch: "x86_64"}},
		}, false)
		require.Contains(t, body, "2 release sources failed")
		require.Contains(t, body, "fedora: timed out")
		require.Contains(t, body, "debian: checksum mismatch")
		require.Contains(t, body, "<span class=\"mono\">fedora</span> timed out")
		require.Contains(t, body, `popovertarget="catalog-errors"`)
	})

	t.Run("refreshing", func(t *testing.T) {
		body := renderCatalogStatus(t, catalog.Snapshot{
			Origin:      catalog.OriginLive,
			RefreshedAt: refreshed,
			Releases:    []catalog.Release{{OS: "ubuntu", Version: "24.04", DisplayName: "24.04 LTS", Arch: "x86_64"}},
		}, true)
		require.Contains(t, body, `disabled`)
		require.Contains(t, body, `aria-busy="true"`)
		require.Contains(t, body, "Refreshing…")
		require.Contains(t, body, `data-refreshing="true"`)
	})
}

func TestVersionSelectDefaultsToBackendLatest(t *testing.T) {
	chdirRepoRoot(t)
	// openSUSE is the only OS, so it is the chosen one. Whichever release the
	// catalog marks latest (Tumbleweed today) is the selected option. This
	// test does not pick a version of its own.
	snap := catalog.Snapshot{
		Origin: catalog.OriginLive,
		Releases: []catalog.Release{
			{OS: "opensuse", Version: "16.0", DisplayName: "Leap 16.0", Arch: "x86_64"},
			{OS: "opensuse", Version: "tumbleweed", DisplayName: "Tumbleweed (Rolling)", Arch: "x86_64"},
		},
	}
	listing := catalog.BuildListing(snap, time.Now(), nil, false, false)
	require.Len(t, listing.OSes, 1)
	var latest string
	for _, version := range listing.OSes[0].Versions {
		if version.Latest && version.Arch == "x86_64" {
			latest = version.Value
		}
	}
	require.NotEmpty(t, latest)

	body := renderOSImages(t, snap, false)
	require.Contains(t, body, `value="opensuse"`)
	require.Contains(t, body, "Tumbleweed (Rolling)")
	require.Contains(t, body, "Leap 16.0")
	require.Regexp(t, `<option[^>]*value="`+latest+`"[^>]*data-latest="true"[^>]*selected>`, body)
	for _, version := range listing.OSes[0].Versions {
		if version.Latest {
			require.Contains(t, body, version.DisplayName+" · latest")
			continue
		}
		require.NotRegexp(t, `<option[^>]*value="`+version.Value+`"[^>]*selected>`, body)
		require.NotContains(t, body, version.DisplayName+" · latest")
	}
}

func renderCatalogStatus(t *testing.T, snap catalog.Snapshot, refreshing bool) string {
	t.Helper()
	h := NewOSImageHandlers(&Container{
		Config:  testOSConfig(t),
		Catalog: &catalog.Fixed{Snap: snap, InFlight: refreshing},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/osimages/catalog/refresh", nil)
	req.Header.Set("HX-Request", "true")
	h.RefreshCatalog(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	return rec.Body.String()
}

func renderOSImages(t *testing.T, snap catalog.Snapshot, refreshing bool) string {
	t.Helper()
	h := NewOSImageHandlers(&Container{
		Config:         testOSConfig(t),
		OSImageService: emptyOSImages{},
		Catalog:        &catalog.Fixed{Snap: snap, InFlight: refreshing},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/osimages", nil)
	h.OSImagesPage(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return rec.Body.String()
}
