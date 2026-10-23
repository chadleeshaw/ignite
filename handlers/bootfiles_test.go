package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"ignite/osimage"

	"github.com/stretchr/testify/require"
)

const bootSecret = "LEAKED-SECRET"

func TestBootFileHandlerServesRegularFile(t *testing.T) {
	handler, _ := bootFixture(t)
	rel, err := osimage.RelPath("debian", "12", osimage.KernelName)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/"+rel, nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "debian-kernel", rec.Body.String())
	require.Equal(t, "application/octet-stream", rec.Header().Get("Content-Type"))
	require.Equal(t, "13", rec.Header().Get("Content-Length"))
}

func TestBootFileHandlerHead(t *testing.T) {
	handler, _ := bootFixture(t)
	rel, err := osimage.RelPath("debian", "12", osimage.KernelName)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodHead, "/"+rel, nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Empty(t, rec.Body.String())
	require.Equal(t, "application/octet-stream", rec.Header().Get("Content-Type"))
	require.Equal(t, "13", rec.Header().Get("Content-Length"))
}

func TestBootFileHandlerRejectsMethods(t *testing.T) {
	handler, _ := bootFixture(t)
	rel, err := osimage.RelPath("debian", "12", osimage.KernelName)
	require.NoError(t, err)

	for _, method := range []string{
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
		http.MethodOptions,
	} {
		t.Run(method, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(method, "/"+rel, nil))
			require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
			require.Equal(t, "GET, HEAD", rec.Header().Get("Allow"))
			require.NotContains(t, rec.Body.String(), "debian-kernel")
		})
	}
}

func TestBootFileHandlerRejectsTraversal(t *testing.T) {
	handler, dir := bootFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "secret"), []byte(bootSecret), 0o644))

	for _, raw := range bootTraversalPaths() {
		t.Run(raw, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, bootRequest(t, http.MethodGet, raw))
			requireBootRejected(t, rec)
		})
	}
}

func TestBootFileHandlerRejectsSymlinkEscape(t *testing.T) {
	handler, dir := bootFixture(t)
	outside := filepath.Join(dir, "outside-secret")
	require.NoError(t, os.WriteFile(outside, []byte(bootSecret), 0o644))
	link := filepath.Join(dir, osimage.DirName, "escape")
	require.NoError(t, os.Symlink(outside, link))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/os-images/escape", nil))
	requireBootRejected(t, rec)

	relTarget := filepath.Join(dir, osimage.DirName, "rel-escape")
	require.NoError(t, os.Symlink("../outside-secret", relTarget))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/os-images/rel-escape", nil))
	requireBootRejected(t, rec)
}

func TestBootFileHandlerHidesDirectories(t *testing.T) {
	handler, _ := bootFixture(t)
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
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, raw, nil))
			require.Equal(t, http.StatusNotFound, rec.Code)
			require.NotContains(t, rec.Body.String(), "vmlinuz")
			require.NotContains(t, rec.Body.String(), "debian-kernel")
			require.NotContains(t, rec.Body.String(), "<a href")
		})
	}
}

func bootFixture(t *testing.T) (http.Handler, string) {
	t.Helper()
	dir := t.TempDir()
	rel, err := osimage.RelPath("debian", "12", osimage.KernelName)
	require.NoError(t, err)
	full := filepath.Join(dir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte("debian-kernel"), 0o644))
	return NewBootFileHandler(dir), dir
}

func bootTraversalPaths() []string {
	return []string{
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
	}
}

func bootRequest(t *testing.T, method, raw string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, "http://example.com/", nil)
	u, err := url.ParseRequestURI(raw)
	if err != nil {
		req.URL.Path = raw
		req.URL.RawPath = raw
		return req
	}
	req.URL.Path = u.Path
	req.URL.RawPath = u.RawPath
	if req.URL.Path == "" {
		req.URL.Path = raw
		req.URL.RawPath = raw
	}
	return req
}

func requireBootRejected(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 400 or 404; body %q", rec.Code, rec.Body.String())
	}
	require.NotContains(t, rec.Body.String(), bootSecret)
	require.NotContains(t, rec.Body.String(), "root:x:0:0:")
	require.NotContains(t, rec.Body.String(), "root:*:0:0:")
}
