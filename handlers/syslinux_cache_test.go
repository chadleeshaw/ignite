package handlers

import (
	"os"
	"path/filepath"
	"testing"

	"ignite/syslinux"

	"github.com/stretchr/testify/require"
)

type cacheConfigService struct {
	syslinux.Service
	cfg syslinux.SyslinuxConfig
}

func (s cacheConfigService) GetConfig() syslinux.SyslinuxConfig { return s.cfg }

func TestMissingSyslinuxCacheNeedsRedownload(t *testing.T) {
	dir := t.TempDir()
	version := &syslinux.SyslinuxVersion{Version: "6.03", Downloaded: true, Active: true}
	markMissingSyslinuxCache(cacheConfigService{cfg: syslinux.SyslinuxConfig{TempDir: dir}}, []*syslinux.SyslinuxVersion{version})
	require.True(t, version.NeedsRedownload)

	for _, boot := range []string{"bios", "efi"} {
		files := syslinux.GetRequiredBiosFiles()
		if boot == "efi" {
			files = syslinux.GetRequiredEfiFiles()
		}
		bootDir := syslinux.ExtractedBootDir(dir, "6.03", boot)
		require.NoError(t, os.MkdirAll(bootDir, 0o755))
		for name := range files {
			require.NoError(t, os.WriteFile(filepath.Join(bootDir, name), []byte("x"), 0o644))
		}
	}
	version.NeedsRedownload = false
	markMissingSyslinuxCache(cacheConfigService{cfg: syslinux.SyslinuxConfig{TempDir: dir}}, []*syslinux.SyslinuxVersion{version})
	require.False(t, version.NeedsRedownload)

	fresh := &syslinux.SyslinuxVersion{Version: "6.04", Downloaded: false}
	markMissingSyslinuxCache(cacheConfigService{cfg: syslinux.SyslinuxConfig{TempDir: dir}}, []*syslinux.SyslinuxVersion{fresh})
	require.False(t, fresh.NeedsRedownload)
}
