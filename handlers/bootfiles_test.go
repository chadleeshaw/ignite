package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"ignite/osimage"

	"github.com/stretchr/testify/require"
)

func TestBootFileHandlerServesStoredImage(t *testing.T) {
	dir := t.TempDir()
	rel, err := osimage.RelPath("debian", "12", osimage.KernelName)
	require.NoError(t, err)
	body := []byte("debian-kernel")
	full := filepath.Join(dir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, body, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "secret"), []byte("nope"), 0o644))

	handler := NewBootFileHandler(dir)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/"+rel, nil)
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, body, rec.Body.Bytes())
	require.Equal(t, "13", rec.Header().Get("Content-Length"))

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/os-images/debian/12/", nil)
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/os-images/../secret", nil)
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.NotContains(t, rec.Body.String(), "nope")

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/os-images/debian/12/missing", nil)
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}
