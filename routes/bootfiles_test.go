package routes

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"ignite/osimage"

	"github.com/stretchr/testify/require"
)

func TestBootFileRouteMatchesMenuPathWithoutSession(t *testing.T) {
	dir := t.TempDir()
	rel, err := osimage.RelPath("debian", "12", osimage.KernelName)
	require.NoError(t, err)
	body := []byte("stored-kernel")
	full := filepath.Join(dir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, body, 0o644))

	container := createTestContainer()
	container.Config.TFTP.Dir = dir
	router := SetupWithContainerAndStatic(container, createTestStaticHandler())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/"+rel, nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, body, rec.Body.Bytes())

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/"+rel, nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	require.NotContains(t, rec.Body.String(), string(body))
}
