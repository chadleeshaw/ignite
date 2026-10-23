package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ignite/config"
	"ignite/osimage"

	"github.com/stretchr/testify/require"
)

func TestOSImagesPageSupportedLineMatchesCatalog(t *testing.T) {
	chdirRepoRoot(t)
	cfg, err := config.NewConfigBuilder().Build()
	require.NoError(t, err)

	h := NewOSImageHandlers(&Container{
		Config:         cfg,
		OSImageService: emptyOSImages{},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/osimages", nil)
	h.OSImagesPage(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	want := "Supported: " + strings.Join(cfg.OSImages.SupportedNames(), " · ")
	require.Contains(t, body, want)
	require.NotContains(t, body, "Supported: Ubuntu · Debian · Fedora · CentOS · NixOS")
	require.Contains(t, cfg.OSImages.SupportedNames(), "openSUSE")
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
