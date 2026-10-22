package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveSyslinuxCacheDirDefault(t *testing.T) {
	dbPath := t.TempDir()
	got, err := ResolveSyslinuxCacheDir(dbPath, "")
	require.NoError(t, err)
	want, err := filepath.Abs(filepath.Join(dbPath, SyslinuxCacheDirName))
	require.NoError(t, err)
	require.Equal(t, want, got)
	info, err := os.Stat(got)
	require.NoError(t, err)
	require.True(t, info.IsDir())
}

func TestResolveSyslinuxCacheDirOverride(t *testing.T) {
	dbPath := t.TempDir()
	override := filepath.Join(t.TempDir(), "custom-cache")
	got, err := ResolveSyslinuxCacheDir(dbPath, override)
	require.NoError(t, err)
	want, err := filepath.Abs(override)
	require.NoError(t, err)
	require.Equal(t, want, got)
	require.NotEqual(t, filepath.Join(dbPath, SyslinuxCacheDirName), override)
	info, err := os.Stat(got)
	require.NoError(t, err)
	require.True(t, info.IsDir())
	_, err = os.Stat(filepath.Join(dbPath, SyslinuxCacheDirName))
	require.True(t, os.IsNotExist(err))
}

func TestResolveSyslinuxCacheDirTrimsAndCreatesNested(t *testing.T) {
	nested := filepath.Join(t.TempDir(), "a", "b")
	got, err := ResolveSyslinuxCacheDir(t.TempDir(), "  "+nested+"  ")
	require.NoError(t, err)
	want, err := filepath.Abs(nested)
	require.NoError(t, err)
	require.Equal(t, want, got)
	info, err := os.Stat(got)
	require.NoError(t, err)
	require.True(t, info.IsDir())
}
