package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SyslinuxCacheDirName is the directory created under DB_PATH when
// SYSLINUX_CACHE_DIR is unset.
const SyslinuxCacheDirName = "syslinux"

// ResolveSyslinuxCacheDir chooses the Syslinux download and extract cache.
//
// A non-empty configured path (the SYSLINUX_CACHE_DIR override) is kept.
// An empty path is a syslinux directory under dbPath, which is /app/data/syslinux
// when the image's DB_PATH is /app/data/. The directory is created. The
// returned path is absolute so the startup log names the directory that is
// actually used. An existing /tmp/syslinux-downloads tree is not read or moved.
func ResolveSyslinuxCacheDir(dbPath, configured string) (string, error) {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		dbPath = strings.TrimSpace(dbPath)
		if dbPath == "" {
			dbPath = "."
		}
		configured = filepath.Join(dbPath, SyslinuxCacheDirName)
	}
	if err := os.MkdirAll(configured, 0o755); err != nil {
		return "", fmt.Errorf("create Syslinux cache %s: %w", configured, err)
	}
	abs, err := filepath.Abs(configured)
	if err != nil {
		return configured, nil
	}
	return abs, nil
}
