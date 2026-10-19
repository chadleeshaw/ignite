package syslinux

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"ignite/config"
	"ignite/db"

	"github.com/stretchr/testify/require"
)

func testSyslinuxService(t *testing.T) (*service, Repository) {
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

	repo, err := NewBoltRepository(database)
	require.NoError(t, err)

	root := t.TempDir()
	svc := NewService(repo, SyslinuxConfig{
		TFTPDir: root,
		BiosDir: "boot-bios",
		EfiDir:  "boot-efi",
		TempDir: filepath.Join(root, "tmp"),
	}).(*service)
	return svc, repo
}

func writeExtractedBootFiles(t *testing.T, cfg SyslinuxConfig, version, contents string) {
	t.Helper()
	bios := ExtractedBootDir(cfg.TempDir, version, "bios")
	efi := ExtractedBootDir(cfg.TempDir, version, "efi")
	require.NoError(t, os.MkdirAll(bios, 0o755))
	require.NoError(t, os.MkdirAll(efi, 0o755))
	for name := range GetRequiredBiosFiles() {
		require.NoError(t, os.WriteFile(filepath.Join(bios, name), []byte(contents), 0o644))
	}
	for name := range GetRequiredEfiFiles() {
		require.NoError(t, os.WriteFile(filepath.Join(efi, name), []byte(contents), 0o644))
	}
}

func TestActivateVersionRequiresDownload(t *testing.T) {
	svc, repo := testSyslinuxService(t)
	ctx := context.Background()
	require.NoError(t, repo.SaveVersion(ctx, &SyslinuxVersion{
		ID:         "6.03",
		Version:    "6.03",
		Downloaded: false,
	}))

	err := svc.ActivateVersion(ctx, "6.03")
	require.ErrorIs(t, err, ErrVersionNotDownloaded)

	_, err = repo.GetActiveVersion(ctx)
	require.Error(t, err)
}

func TestActivateVersionUnknown(t *testing.T) {
	svc, _ := testSyslinuxService(t)
	err := svc.ActivateVersion(context.Background(), "9.99")
	require.ErrorIs(t, err, ErrVersionNotFound)
}

func TestActivateVersionMissingBootFiles(t *testing.T) {
	svc, repo := testSyslinuxService(t)
	ctx := context.Background()
	require.NoError(t, repo.SaveVersion(ctx, &SyslinuxVersion{
		ID:         "6.03",
		Version:    "6.03",
		Downloaded: true,
	}))

	live := filepath.Join(svc.config.TFTPDir, svc.config.BiosDir, "pxelinux.0")
	require.NoError(t, os.MkdirAll(filepath.Dir(live), 0o755))
	require.NoError(t, os.WriteFile(live, []byte("still-booting"), 0o644))

	err := svc.ActivateVersion(ctx, "6.03")
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrVersionNotDownloaded))
	require.Contains(t, err.Error(), "missing stored")

	body, err := os.ReadFile(live)
	require.NoError(t, err)
	require.Equal(t, "still-booting", string(body))

	got, err := repo.GetVersionByNumber(ctx, "6.03")
	require.NoError(t, err)
	require.True(t, got.Downloaded)
	require.False(t, got.Active)
}

func TestActivateVersionCopyFailureKeepsLiveFiles(t *testing.T) {
	svc, repo := testSyslinuxService(t)
	ctx := context.Background()
	live := filepath.Join(svc.config.TFTPDir, svc.config.BiosDir, "pxelinux.0")
	require.NoError(t, os.MkdirAll(filepath.Dir(live), 0o755))
	require.NoError(t, os.WriteFile(live, []byte("still-booting"), 0o644))
	writeExtractedBootFiles(t, svc.config, "6.04", "stored-6.04")
	require.NoError(t, repo.SaveVersion(ctx, &SyslinuxVersion{
		ID:         "6.04",
		Version:    "6.04",
		Downloaded: true,
	}))

	orig := stageBootFile
	calls := 0
	stageBootFile = func(src, dst string) error {
		calls++
		if calls == 2 {
			return errors.New("injected copy failure")
		}
		return orig(src, dst)
	}
	t.Cleanup(func() { stageBootFile = orig })

	err := svc.ActivateVersion(ctx, "6.04")
	require.Error(t, err)
	require.Contains(t, err.Error(), "injected copy failure")
	require.GreaterOrEqual(t, calls, 2)

	body, err := os.ReadFile(live)
	require.NoError(t, err)
	require.Equal(t, "still-booting", string(body))
	got, err := repo.GetVersionByNumber(ctx, "6.04")
	require.NoError(t, err)
	require.False(t, got.Active)

	entries, err := os.ReadDir(svc.config.TFTPDir)
	require.NoError(t, err)
	for _, entry := range entries {
		require.NotContains(t, entry.Name(), ".activate-")
	}
}

func TestActivateVersionKeepsOtherDownloads(t *testing.T) {
	svc, repo := testSyslinuxService(t)
	ctx := context.Background()
	// The previous version's bytes are what TFTP is serving now.
	oldBIOS := filepath.Join(svc.config.TFTPDir, svc.config.BiosDir)
	require.NoError(t, os.MkdirAll(oldBIOS, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(oldBIOS, "pxelinux.0"), []byte("old-6.03"), 0o644))
	writeExtractedBootFiles(t, svc.config, "6.03", "stored-6.03")
	writeExtractedBootFiles(t, svc.config, "6.04", "stored-6.04")

	require.NoError(t, repo.SaveVersion(ctx, &SyslinuxVersion{
		ID:         "6.03",
		Version:    "6.03",
		Downloaded: true,
		Active:     true,
	}))
	require.NoError(t, repo.SaveVersion(ctx, &SyslinuxVersion{
		ID:         "6.04",
		Version:    "6.04",
		Downloaded: true,
		Active:     false,
	}))

	require.NoError(t, svc.ActivateVersion(ctx, "6.04"))

	active, err := repo.GetActiveVersion(ctx)
	require.NoError(t, err)
	require.Equal(t, "6.04", active.Version)

	previous, err := repo.GetVersionByNumber(ctx, "6.03")
	require.NoError(t, err)
	require.True(t, previous.Downloaded, "activating one version must not mark the other as not downloaded")
	require.False(t, previous.Active)

	current, err := repo.GetVersionByNumber(ctx, "6.04")
	require.NoError(t, err)
	require.True(t, current.Downloaded)
	require.True(t, current.Active)

	installed, err := os.ReadFile(filepath.Join(svc.config.TFTPDir, svc.config.BiosDir, "pxelinux.0"))
	require.NoError(t, err)
	require.Equal(t, "stored-6.04", string(installed))
	efi, err := os.ReadFile(filepath.Join(svc.config.TFTPDir, svc.config.EfiDir, "syslinux.efi"))
	require.NoError(t, err)
	require.Equal(t, "stored-6.04", string(efi))

	// Activating the version that is already active installs that version again.
	require.NoError(t, svc.ActivateVersion(ctx, "6.04"))
	installed, err = os.ReadFile(filepath.Join(svc.config.TFTPDir, svc.config.BiosDir, "pxelinux.0"))
	require.NoError(t, err)
	require.Equal(t, "stored-6.04", string(installed))
}
