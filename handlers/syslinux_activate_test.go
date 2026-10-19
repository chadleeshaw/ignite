package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"ignite/config"
	"ignite/db"
	"ignite/syslinux"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
)

func testSyslinuxHandler(t *testing.T) (*SyslinuxHandler, syslinux.SyslinuxConfig, syslinux.Repository) {
	t.Helper()
	cfg, err := config.NewConfigBuilder().
		WithDBPath(t.TempDir()).
		WithDBFile("syslinux.db").
		WithBucket("test").
		Build()
	require.NoError(t, err)
	database, err := db.NewBoltDB(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })

	repo, err := syslinux.NewBoltRepository(database)
	require.NoError(t, err)
	root := t.TempDir()
	slCfg := syslinux.SyslinuxConfig{
		TFTPDir: root,
		BiosDir: "boot-bios",
		EfiDir:  "boot-efi",
		TempDir: filepath.Join(root, "tmp"),
	}
	return &SyslinuxHandler{service: syslinux.NewService(repo, slCfg)}, slCfg, repo
}

func postActivate(handler *SyslinuxHandler, version string) *httptest.ResponseRecorder {
	router := mux.NewRouter()
	router.HandleFunc("/api/syslinux/activate/{version}", handler.ActivateVersion).Methods(http.MethodPost)
	req := httptest.NewRequest(http.MethodPost, "/api/syslinux/activate/"+version, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestActivateVersionNotDownloaded(t *testing.T) {
	handler, _, repo := testSyslinuxHandler(t)
	require.NoError(t, repo.SaveVersion(context.Background(), &syslinux.SyslinuxVersion{
		ID:         "6.03",
		Version:    "6.03",
		Downloaded: false,
	}))

	rec := postActivate(handler, "6.03")
	require.Equal(t, http.StatusConflict, rec.Code)
	var body ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.False(t, body.Success)
	require.Equal(t, "This version is not downloaded. Download it before activating.", body.Error.UserMessage)
}

func TestActivateVersionNotFound(t *testing.T) {
	handler, _, _ := testSyslinuxHandler(t)
	rec := postActivate(handler, "9.99")
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestActivateVersionSuccess(t *testing.T) {
	handler, cfg, repo := testSyslinuxHandler(t)
	bios := syslinux.ExtractedBootDir(cfg.TempDir, "6.03", "bios")
	efi := syslinux.ExtractedBootDir(cfg.TempDir, "6.03", "efi")
	require.NoError(t, os.MkdirAll(bios, 0o755))
	require.NoError(t, os.MkdirAll(efi, 0o755))
	for name := range syslinux.GetRequiredBiosFiles() {
		require.NoError(t, os.WriteFile(filepath.Join(bios, name), []byte("x"), 0o644))
	}
	for name := range syslinux.GetRequiredEfiFiles() {
		require.NoError(t, os.WriteFile(filepath.Join(efi, name), []byte("x"), 0o644))
	}
	require.NoError(t, repo.SaveVersion(context.Background(), &syslinux.SyslinuxVersion{
		ID:         "6.03",
		Version:    "6.03",
		Downloaded: true,
	}))

	rec := postActivate(handler, "6.03")
	require.Equal(t, http.StatusOK, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, true, body["success"])
	require.Equal(t, "6.03", body["version"])
	require.Equal(t, "Syslinux 6.03 is active", body["message"])
}
