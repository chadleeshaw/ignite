package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTFTPPageShowsConfiguredRoot(t *testing.T) {
	chdirRepoRoot(t)

	dir := t.TempDir()
	prevDir, prevDisplay := TFTPDir, TFTPDisplayRoot
	SetTFTPRoot(dir)
	SetTFTPDisplayRoot("./public/tftp")
	t.Cleanup(func() {
		SetTFTPRoot(prevDir)
		SetTFTPDisplayRoot(prevDisplay)
	})
	require.NoError(t, os.WriteFile(filepath.Join(dir, "boot.ipxe"), []byte("#!ipxe\n"), 0o644))

	req := httptest.NewRequest(http.MethodGet, "/tftp", nil)
	rec := httptest.NewRecorder()
	(&TFTPHandlers{}).HandleTFTPPage(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	require.Contains(t, body, "./public/tftp")
	require.NotContains(t, body, dir)

	data, err := (&TFTPHandlers{}).getFileInfo("./")
	require.NoError(t, err)
	require.Equal(t, "./public/tftp", data.Root)
}
