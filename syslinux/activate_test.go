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

func writeRequiredBootFiles(t *testing.T, cfg SyslinuxConfig) {
	t.Helper()
	bios := filepath.Join(cfg.TFTPDir, cfg.BiosDir)
	efi := filepath.Join(cfg.TFTPDir, cfg.EfiDir)
	require.NoError(t, os.MkdirAll(bios, 0o755))
	require.NoError(t, os.MkdirAll(efi, 0o755))
	for name := range GetRequiredBiosFiles() {
		require.NoError(t, os.WriteFile(filepath.Join(bios, name), []byte(name), 0o644))
	}
	for name := range GetRequiredEfiFiles() {
		require.NoError(t, os.WriteFile(filepath.Join(efi, name), []byte(name), 0o644))
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

	err := svc.ActivateVersion(ctx, "6.03")
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrVersionNotDownloaded))
	require.Contains(t, err.Error(), "missing")

	got, err := repo.GetVersionByNumber(ctx, "6.03")
	require.NoError(t, err)
	require.True(t, got.Downloaded)
	require.False(t, got.Active)
}

func TestActivateVersionKeepsOtherDownloads(t *testing.T) {
	svc, repo := testSyslinuxService(t)
	ctx := context.Background()
	writeRequiredBootFiles(t, svc.config)

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

	// Activating the version that is already active is a no-op success.
	require.NoError(t, svc.ActivateVersion(ctx, "6.04"))
}
