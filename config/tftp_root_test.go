package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveTFTPRootOverride(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "outside")
	// t.TempDir is under the system temp dir, so use a directory we create
	// beside the module only when it is not itself temporary. The override
	// test below uses a path under TempDir on purpose for the warning case.
	parent := t.TempDir()
	if ephemeralPath(parent) {
		dir = filepath.Join(parent, "tftp")
	}
	root, err := ResolveTFTPRoot(dir)
	require.NoError(t, err)
	abs, err := filepath.Abs(dir)
	require.NoError(t, err)
	require.Equal(t, abs, root.Path)
	info, err := os.Stat(root.Path)
	require.NoError(t, err)
	require.True(t, info.IsDir())
	if ephemeralPath(abs) {
		require.True(t, root.Ephemeral)
		require.Contains(t, root.Warning, "will not survive a restart")
		require.NotContains(t, root.Warning, "Falling back")
	}
	text := strings.Join(root.StartupLines(), "\n")
	require.Contains(t, text, "TFTP root: "+abs)
}

func TestResolveTFTPRootKeepsPersistentOverride(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "custom-tftp")
	root, err := ResolveTFTPRoot(dir)
	require.NoError(t, err)

	abs, err := filepath.Abs(dir)
	require.NoError(t, err)
	require.Equal(t, abs, root.Path)
	require.NotContains(t, root.Path, "ignite-tftp")
	require.NotContains(t, root.Warning, "Falling back")
	info, err := os.Stat(root.Path)
	require.NoError(t, err)
	require.True(t, info.IsDir())
}

func TestResolveTFTPRootDefaultStaysPublicTFTP(t *testing.T) {
	work := t.TempDir()
	prev, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(work))
	t.Cleanup(func() { _ = os.Chdir(prev) })

	root, err := ResolveTFTPRoot("")
	require.NoError(t, err)
	want, err := filepath.Abs(DefaultTFTPDir)
	require.NoError(t, err)
	require.Equal(t, want, root.Path)
	require.Equal(t, DefaultTFTPDir, root.Display)
	require.NotContains(t, root.Path, "ignite-tftp")
	require.NotContains(t, root.Warning, "Falling back")
	_, err = os.Stat(filepath.Join(work, "public", "tftp"))
	require.NoError(t, err)
}

func TestResolveTFTPRootFallsBackWhenCreateFails(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(blocked, []byte("x"), 0o644))
	configured := filepath.Join(blocked, "tftp")

	root, err := ResolveTFTPRoot(configured)
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(root.Path) })
	require.True(t, root.Ephemeral)
	require.Contains(t, root.Path, "ignite-tftp")
	require.Equal(t, root.Path, root.Display)
	require.NotEqual(t, configured, root.Path)
	require.NotEqual(t, configured, root.Display)
	require.Contains(t, root.Warning, "WARNING:")
	require.Contains(t, root.Warning, "will not survive a restart")
	require.Contains(t, strings.Join(root.StartupLines(), "\n"), root.Warning)
	info, err := os.Stat(root.Path)
	require.NoError(t, err)
	require.True(t, info.IsDir())
}

func TestResolveTFTPRootDisplayKeepsConfiguredString(t *testing.T) {
	work := t.TempDir()
	prev, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(work))
	t.Cleanup(func() { _ = os.Chdir(prev) })

	root, err := ResolveTFTPRoot("./public/tftp")
	require.NoError(t, err)
	require.Equal(t, "./public/tftp", root.Display)
	abs, err := filepath.Abs("./public/tftp")
	require.NoError(t, err)
	require.Equal(t, abs, root.Path)
	require.NotEqual(t, root.Display, root.Path)

	absolute := filepath.Join(t.TempDir(), "app", "public", "tftp")
	root, err = ResolveTFTPRoot(absolute)
	require.NoError(t, err)
	require.Equal(t, absolute, root.Display)
	abs, err = filepath.Abs(absolute)
	require.NoError(t, err)
	require.Equal(t, abs, root.Path)
}

func TestResolveTFTPRootWarnsForTempOverride(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ignite-tftp.UPoTst")
	root, err := ResolveTFTPRoot(dir)
	require.NoError(t, err)
	abs, err := filepath.Abs(dir)
	require.NoError(t, err)
	require.Equal(t, abs, root.Path)
	require.True(t, root.Ephemeral)
	require.Contains(t, root.Warning, "system temp directory")
	require.Contains(t, root.Warning, "will not survive a restart")
}
