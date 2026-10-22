package handlers

import (
	"os"
	"path/filepath"
	"strings"

	"ignite/syslinux"
)

// markMissingSyslinuxCache flags versions the database still calls downloaded
// when the extract cache is not in the current data directory. Downloads from
// before the cache moved to that directory are not copied across.
func markMissingSyslinuxCache(svc syslinux.Service, versions []*syslinux.SyslinuxVersion) {
	if svc == nil {
		return
	}
	tempDir := svc.GetConfig().TempDir
	for _, version := range versions {
		if version == nil || !version.Downloaded {
			continue
		}
		version.NeedsRedownload = !syslinuxExtractReady(tempDir, version.Version)
	}
}

func syslinuxExtractReady(tempDir, version string) bool {
	if strings.TrimSpace(tempDir) == "" || strings.TrimSpace(version) == "" {
		return false
	}
	sets := []struct {
		name  string
		files map[string]string
	}{
		{"bios", syslinux.GetRequiredBiosFiles()},
		{"efi", syslinux.GetRequiredEfiFiles()},
	}
	for _, set := range sets {
		dir := syslinux.ExtractedBootDir(tempDir, version, set.name)
		for name := range set.files {
			if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
				return false
			}
		}
	}
	return true
}
