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
