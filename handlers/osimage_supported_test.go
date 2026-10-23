package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"ignite/config"
	"ignite/osimage"
	"ignite/osimage/catalog"

	"github.com/stretchr/testify/require"
)

func TestOSImagesPageSupportedLineMatchesCatalog(t *testing.T) {
	chdirRepoRoot(t)
	cfg, err := config.NewConfigBuilder().Build()
	require.NoError(t, err)

	h := NewOSImageHandlers(&Container{
		Config:         cfg,
		OSImageService: emptyOSImages{},
		Catalog: &catalog.Fixed{Snap: catalog.Snapshot{
			Origin: catalog.OriginLive,
			Releases: []catalog.Release{
				{OS: "opensuse", Version: "16.0", DisplayName: "16.0", Arch: "x86_64"},
				{OS: "ubuntu", Version: "24.04", DisplayName: "24.04 LTS", Arch: "x86_64"},
			},
		}},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/osimages", nil)
	h.OSImagesPage(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	require.Contains(t, body, `id="catalog-status"`)
	require.Contains(t, body, `data-catalog-source="live"`)
	require.Contains(t, body, `value="opensuse"`)
	require.Contains(t, body, "openSUSE")
	require.Contains(t, cfg.OSImages.SupportedNames(), "openSUSE")
	require.NotContains(t, body, "Supported:")
}

func TestOSImageRowsMarkEOL(t *testing.T) {
	chdirRepoRoot(t)
	cfg, err := config.NewConfigBuilder().Build()
	require.NoError(t, err)
	h := NewOSImageHandlers(&Container{
		Config:         cfg,
		OSImageService: oneEOLImage{},
		Catalog:        &catalog.Fixed{Snap: catalog.Snapshot{Origin: catalog.OriginBundled}},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/osimages", nil)
	h.OSImagesPage(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	require.Contains(t, body, `data-eol="true"`)
	require.Contains(t, body, `class="st wa eol-badge" title="End of life 2026-05-27"`)
	require.Contains(t, body, ">EOL 2026-05-27<")
	require.Contains(t, body, `hx-delete="/osimages/delete/eol"`)
	require.NotRegexp(t, `delete/eol"[^>]*disabled`, body)
	require.NotContains(t, body, `class="st ok">Ready</span>`)
	require.NotContains(t, body, `data-eol="false"`)
}

type oneEOLImage struct{ emptyOSImages }

func (oneEOLImage) GetAllOSImages(context.Context) ([]*osimage.OSImage, error) {
	return []*osimage.OSImage{{
		ID: "eol", OS: "fedora", Version: "42", Architecture: "x86_64", EOL: true, EOLDate: "2026-05-27",
	}}, nil
}

type emptyOSImages struct{}

func (emptyOSImages) GetAllOSImages(context.Context) ([]*osimage.OSImage, error) {
	return nil, nil
}
func (emptyOSImages) GetOSImagesByOS(context.Context, string) ([]*osimage.OSImage, error) {
	return nil, nil
}
func (emptyOSImages) GetOSImage(context.Context, string) (*osimage.OSImage, error) {
	return nil, nil
}
func (emptyOSImages) GetDefaultVersion(context.Context, string) (*osimage.OSImage, error) {
	return nil, nil
}
func (emptyOSImages) SetDefaultVersion(context.Context, string) error { return nil }
func (emptyOSImages) DeleteOSImage(context.Context, string) error     { return nil }
func (emptyOSImages) DownloadOSImage(context.Context, osimage.OSImageConfig) (*osimage.DownloadStatus, error) {
	return nil, nil
}
func (emptyOSImages) GetDownloadStatus(context.Context, string) (*osimage.DownloadStatus, error) {
	return nil, nil
}
func (emptyOSImages) GetActiveDownloads(context.Context) ([]*osimage.DownloadStatus, error) {
	return nil, nil
}
func (emptyOSImages) CancelDownload(context.Context, string) error { return nil }
func (emptyOSImages) GetAvailableVersions(context.Context, string) ([]string, error) {
	return nil, nil
}
