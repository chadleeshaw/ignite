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

	router := bootRouter(t, dir)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/"+rel, nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, body, rec.Body.Bytes())
	require.Equal(t, "application/octet-stream", rec.Header().Get("Content-Type"))
	require.Equal(t, "13", rec.Header().Get("Content-Length"))

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodHead, "/"+rel, nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Empty(t, rec.Body.Bytes())
	require.Equal(t, "application/octet-stream", rec.Header().Get("Content-Type"))
	require.Equal(t, "13", rec.Header().Get("Content-Length"))
}

func TestBootFileRouteRejectsMethods(t *testing.T) {
	dir := t.TempDir()
	rel, err := osimage.RelPath("debian", "12", osimage.KernelName)
	require.NoError(t, err)
	full := filepath.Join(dir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte("stored-kernel"), 0o644))
	router := bootRouter(t, dir)

	for _, method := range []string{
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
		http.MethodOptions,
	} {
		t.Run(method, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(method, "/"+rel, nil)
			router.ServeHTTP(rec, req)
			require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
			require.Equal(t, "GET, HEAD", rec.Header().Get("Allow"))
			require.NotContains(t, rec.Body.String(), "stored-kernel")
		})
	}
}

func TestBootFileRouteRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, osimage.DirName), 0o755))
	secret := []byte("LEAKED-SECRET")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "secret"), secret, 0o644))
	router := bootRouter(t, dir)

	for _, raw := range []string{
		"/os-images/../secret",
		"/os-images/../../secret",
		"/os-images/%2e%2e%2fsecret",
		"/os-images/%2e%2e/secret",
		"/os-images/%252e%252e%252fsecret",
		"/os-images/%252e%252e/secret",
		`/os-images/..\secret`,
		`/os-images/..%5csecret`,
		`/os-images/%2e%2e%5csecret`,
		`/os-images/%5c..%5csecret`,
		"//etc/passwd",
		"/os-images/..//etc/passwd",
	} {
		t.Run(raw, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, raw, nil))
			if rec.Code != http.StatusBadRequest && rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 400 or 404; body %q", rec.Code, rec.Body.String())
			}
			require.NotContains(t, rec.Body.String(), string(secret))
			require.NotContains(t, rec.Body.String(), "root:x:0:0:")
			require.NotContains(t, rec.Body.String(), "root:*:0:0:")
		})
	}
}

func TestBootFileRouteRejectsSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	imageDir := filepath.Join(dir, osimage.DirName)
	require.NoError(t, os.MkdirAll(imageDir, 0o755))
	outside := filepath.Join(dir, "outside-secret")
	require.NoError(t, os.WriteFile(outside, []byte("LEAKED-SECRET"), 0o644))
	require.NoError(t, os.Symlink(outside, filepath.Join(imageDir, "escape")))
	require.NoError(t, os.Symlink("../outside-secret", filepath.Join(imageDir, "rel-escape")))
	router := bootRouter(t, dir)

	for _, raw := range []string{"/os-images/escape", "/os-images/rel-escape"} {
		t.Run(raw, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, raw, nil))
			if rec.Code != http.StatusBadRequest && rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 400 or 404; body %q", rec.Code, rec.Body.String())
			}
			require.NotContains(t, rec.Body.String(), "LEAKED-SECRET")
		})
	}
}

func TestBootFileRouteHidesDirectories(t *testing.T) {
	dir := t.TempDir()
	rel, err := osimage.RelPath("debian", "12", osimage.KernelName)
	require.NoError(t, err)
	full := filepath.Join(dir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte("stored-kernel"), 0o644))
	router := bootRouter(t, dir)

	for _, raw := range []string{
		"/os-images",
		"/os-images/",
		"/os-images/debian",
		"/os-images/debian/",
		"/os-images/debian/12",
		"/os-images/debian/12/",
	} {
		t.Run(raw, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, raw, nil))
			require.Equal(t, http.StatusNotFound, rec.Code)
			require.NotContains(t, rec.Body.String(), "stored-kernel")
			require.NotContains(t, rec.Body.String(), "vmlinuz")
			require.NotContains(t, rec.Body.String(), "<a href")
		})
	}
}

func TestBootFileRouteIgnoresFilesOutsideImageDir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, osimage.DirName), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "secret"), []byte("LEAKED-SECRET"), 0o644))
	router := bootRouter(t, dir)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/secret", nil))
	require.NotEqual(t, http.StatusOK, rec.Code)
	require.NotContains(t, rec.Body.String(), "LEAKED-SECRET")
}

func bootRouter(t *testing.T, dir string) http.Handler {
	t.Helper()
	container := createTestContainer()
	container.Config.TFTP.Dir = dir
	return SetupWithContainerAndStatic(container, createTestStaticHandler())
}
