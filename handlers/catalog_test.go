package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ignite/config"
	"ignite/osimage/catalog"

	"github.com/stretchr/testify/require"
)

func TestCatalogRefreshFragmentAndJSON(t *testing.T) {
	chdirRepoRoot(t)
	refreshed := time.Date(2026, 10, 23, 1, 2, 3, 0, time.UTC)
	cat := &catalog.Fixed{Snap: catalog.Snapshot{
		Origin:      catalog.OriginLive,
		RefreshedAt: refreshed,
		Releases: []catalog.Release{
			{OS: "ubuntu", Version: "22.04", DisplayName: "22.04 LTS", Arch: "x86_64"},
			{OS: "ubuntu", Version: "24.04", DisplayName: "24.04 LTS", Arch: "x86_64"},
		},
	}}
	h := NewOSImageHandlers(&Container{Config: testOSConfig(t), Catalog: cat})

	jsonRec := httptest.NewRecorder()
	jsonReq := httptest.NewRequest(http.MethodPost, "/osimages/catalog/refresh", nil)
	h.RefreshCatalog(jsonRec, jsonReq)
	require.Equal(t, http.StatusOK, jsonRec.Code)
	require.Contains(t, jsonRec.Header().Get("Content-Type"), "application/json")
	var listing catalog.Listing
	require.NoError(t, json.Unmarshal(jsonRec.Body.Bytes(), &listing))
	require.Equal(t, "live", listing.Source)
	require.Equal(t, "2026-10-23T01:02:03Z", listing.RefreshedAt)
	require.False(t, listing.Refreshing)
	require.NotNil(t, listing.Errors)
	require.Equal(t, "24.04", listing.OSes[0].Versions[0].Value)
	require.True(t, listing.OSes[0].Versions[0].Latest)
	require.Equal(t, "22.04", listing.OSes[0].Versions[1].Value)
	require.False(t, listing.OSes[0].Versions[1].Latest)

	fragRec := httptest.NewRecorder()
	fragReq := httptest.NewRequest(http.MethodPost, "/osimages/catalog/refresh", nil)
	fragReq.Header.Set("HX-Request", "true")
	h.RefreshCatalog(fragRec, fragReq)
	require.Equal(t, http.StatusOK, fragRec.Code)
	require.Contains(t, fragRec.Header().Get("Content-Type"), "text/html")
	body := fragRec.Body.String()
	require.Contains(t, body, `id="catalog-status"`)
	require.Contains(t, body, `data-catalog-source="live"`)
	require.Contains(t, body, `data-refreshed="2026-10-23T01:02:03Z"`)
	require.Contains(t, body, "Catalog: live · refreshed 2026-10-23T01:02:03Z")
	require.NotContains(t, body, `"source"`)
	require.NotContains(t, body, "chip")
}

func TestCatalogRefreshConflictAndUnavailableArePlainText(t *testing.T) {
	h := NewOSImageHandlers(&Container{
		Catalog: &catalog.Fixed{RefreshErr: catalog.ErrRefreshInProgress},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/osimages/catalog/refresh", nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("Accept", "text/html")
	h.RefreshCatalog(rec, req)
	require.Equal(t, http.StatusConflict, rec.Code)
	require.Contains(t, rec.Header().Get("Content-Type"), "text/plain")
	require.Equal(t, "A catalog refresh is already running.", rec.Body.String())
	require.NotContains(t, rec.Body.String(), "<html")
	require.NotContains(t, rec.Body.String(), "user_message")

	h = NewOSImageHandlers(&Container{
		Catalog: &catalog.Fixed{RefreshErr: catalog.ErrAllSourcesFailed},
	})
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/osimages/catalog/refresh", nil)
	h.RefreshCatalog(rec, req)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Contains(t, rec.Header().Get("Content-Type"), "text/plain")
	require.Equal(t, "Couldn't reach any release source. Showing the last saved catalog.", rec.Body.String())
	require.NotContains(t, rec.Body.String(), "{")
}

func TestGetCatalogRefreshingSortAndLatest(t *testing.T) {
	past := time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)
	cat := &catalog.Fixed{
		InFlight: true,
		Snap: catalog.Snapshot{
			Origin: catalog.OriginCached,
			Releases: []catalog.Release{
				{OS: "fedora", Version: "42", DisplayName: "42", Arch: "x86_64", EOLDate: past},
				{OS: "fedora", Version: "43", DisplayName: "43", Arch: "x86_64"},
				{OS: "fedora", Version: "44", DisplayName: "44", Arch: "x86_64"},
				{OS: "fedora", Version: "44", DisplayName: "44", Arch: "aarch64"},
			},
		},
	}
	h := NewOSImageHandlers(&Container{Config: testOSConfig(t), Catalog: cat})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/osimages/catalog", nil)
	h.GetCatalog(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var listing catalog.Listing
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listing))
	require.True(t, listing.Refreshing)
	require.Equal(t, "cached", listing.Source)
	require.Equal(t, "", listing.RefreshedAt)
	require.Empty(t, listing.Errors["fedora"])
	require.Len(t, listing.OSes, 1)
	require.Equal(t, []string{"44", "44", "43"}, versionValues(listing.OSes[0].Versions))
	require.True(t, listing.OSes[0].Versions[0].Latest)
	require.True(t, listing.OSes[0].Versions[1].Latest)
	require.False(t, listing.OSes[0].Versions[2].Latest)
	require.Equal(t, "aarch64", listing.OSes[0].Versions[0].Arch)
	require.Equal(t, "x86_64", listing.OSes[0].Versions[1].Arch)

	verRec := httptest.NewRecorder()
	verReq := httptest.NewRequest(http.MethodGet, "/osimages/available-versions?os=fedora&include_eol=true", nil)
	h.GetAvailableVersions(verRec, verReq)
	require.Equal(t, http.StatusOK, verRec.Code)
	var versions []catalog.Version
	require.NoError(t, json.Unmarshal(verRec.Body.Bytes(), &versions))
	require.Equal(t, []string{"44", "44", "43", "42"}, versionValues(versions))
	require.Equal(t, "aarch64", versions[0].Arch)
	require.True(t, versions[0].Latest)
	require.Equal(t, "x86_64", versions[1].Arch)
	require.True(t, versions[1].Latest)
	require.False(t, versions[2].Latest)
	require.True(t, versions[3].EOL)
	require.False(t, versions[3].Latest)
	require.Equal(t, "2020-01-02", versions[3].EOLDate)
	require.Equal(t, "", versions[1].EOLDate)

	hidden := httptest.NewRecorder()
	hiddenReq := httptest.NewRequest(http.MethodGet, "/osimages/available-versions?os=fedora", nil)
	h.GetAvailableVersions(hidden, hiddenReq)
	var current []catalog.Version
	require.NoError(t, json.Unmarshal(hidden.Body.Bytes(), &current))
	require.Equal(t, []string{"44", "44", "43"}, versionValues(current))
}

func TestAvailableVersionsUnknownOS(t *testing.T) {
	h := NewOSImageHandlers(&Container{
		Config:  testOSConfig(t),
		Catalog: &catalog.Fixed{Snap: catalog.Snapshot{Origin: catalog.OriginBundled}},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/osimages/available-versions?os=haiku", nil)
	req.Header.Set("Accept", "application/json")
	h.GetAvailableVersions(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func testOSConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.NewConfigBuilder().Build()
	require.NoError(t, err)
	return cfg
}

func versionValues(versions []catalog.Version) []string {
	out := make([]string, len(versions))
	for i, version := range versions {
		out[i] = version.Value
	}
	return out
}
