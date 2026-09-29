package syslinux

import (
	"fmt"
	"strings"
	"time"

	"ignite/dlstatus"
)

// SyslinuxVersion represents a Syslinux version available for download
type SyslinuxVersion struct {
	ID           string     `json:"id"`
	Version      string     `json:"version"`      // 6.03, 6.04-pre1, etc.
	BootType     string     `json:"boot_type"`    // bios, efi, ipxe
	DownloadURL  string     `json:"download_url"` // Full URL to tar.gz
	FileName     string     `json:"file_name"`    // syslinux-6.03.tar.gz
	Size         int64      `json:"size"`         // Size in bytes
	Checksum     string     `json:"checksum"`     // SHA256 hash if available
	Downloaded   bool       `json:"downloaded"`   // Whether it's been downloaded
	Active       bool       `json:"active"`       // Currently active version
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	DownloadedAt *time.Time `json:"downloaded_at,omitempty"`
}

// SyslinuxBootFile represents an individual boot file extracted from Syslinux
type SyslinuxBootFile struct {
	ID          string    `json:"id"`
	Version     string    `json:"version"`     // 6.03
	BootType    string    `json:"boot_type"`   // bios, efi
	FileName    string    `json:"file_name"`   // pxelinux.0, syslinux.efi
	FilePath    string    `json:"file_path"`   // boot-bios/pxelinux.0
	Size        int64     `json:"size"`        // Size in bytes
	Description string    `json:"description"` // Human readable description
	Required    bool      `json:"required"`    // Essential boot file
	Installed   bool      `json:"installed"`   // Whether file is in TFTP dir
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// SyslinuxConfig holds configuration for Syslinux downloads
type SyslinuxConfig struct {
	BaseURL        string `json:"base_url"`        // https://mirrors.kernel.org/pub/linux/utils/boot/syslinux/
	TFTPDir        string `json:"tftp_dir"`        // ./public/tftp
	BiosDir        string `json:"bios_dir"`        // boot-bios
	EfiDir         string `json:"efi_dir"`         // boot-efi
	TempDir        string `json:"temp_dir"`        // /tmp/syslinux-downloads
	AutoExtract    bool   `json:"auto_extract"`    // Automatically extract after download
	KeepArchive    bool   `json:"keep_archive"`    // Keep downloaded tar.gz files
	VerifyChecksum bool   `json:"verify_checksum"` // Verify downloads with checksums
}

// DownloadStatus is the shared download-status type (see ignite/dlstatus).
// It is aliased so existing references keep compiling.
type DownloadStatus = dlstatus.DownloadStatus

// SyslinuxMirror represents available versions scraped from kernel.org
type SyslinuxMirror struct {
	Version     string    `json:"version"`
	FileName    string    `json:"file_name"`
	DownloadURL string    `json:"download_url"`
	Size        int64     `json:"size"`
	ModifiedAt  time.Time `json:"modified_at"`
}

// GetDefaultConfig returns the default configuration for Syslinux
func GetDefaultConfig() SyslinuxConfig {
	return SyslinuxConfig{
		BaseURL:        "https://mirrors.kernel.org/pub/linux/utils/boot/syslinux/",
		TFTPDir:        "./public/tftp",
		BiosDir:        "boot-bios",
		EfiDir:         "boot-efi",
		TempDir:        "/tmp/syslinux-downloads",
		AutoExtract:    true,
		KeepArchive:    false,
		VerifyChecksum: true,
	}
}

// GetRequiredBiosFiles returns the list of required BIOS boot files
func GetRequiredBiosFiles() map[string]string {
	return map[string]string{
		"pxelinux.0":   "PXE boot loader for BIOS systems",
		"ldlinux.c32":  "Core library for PXELINUX",
		"libcom32.c32": "Common library for COM32 modules",
		"libutil.c32":  "Utility library for COM32 modules",
		"vesamenu.c32": "VESA graphical menu system",
		"menu.c32":     "Simple text menu system",
		"chain.c32":    "Chainloading module",
		"reboot.c32":   "System reboot module",
		"poweroff.c32": "System power off module",
	}
}

// GetRequiredEfiFiles returns the list of required EFI boot files
func GetRequiredEfiFiles() map[string]string {
	return map[string]string{
		"syslinux.efi": "EFI boot loader for UEFI systems",
		"ldlinux.e64":  "Core library for EFI SYSLINUX",
		"libcom32.c32": "Common library for COM32 modules",
		"libutil.c32":  "Utility library for COM32 modules",
		"vesamenu.c32": "VESA graphical menu system",
		"menu.c32":     "Simple text menu system",
		"chain.c32":    "Chainloading module",
		"reboot.c32":   "System reboot module",
		"poweroff.c32": "System power off module",
	}
}

// GetBootFileSourcePath returns the source path within the Syslinux archive
func GetBootFileSourcePath(bootType, fileName string) string {
	switch bootType {
	case "bios":
		switch fileName {
		case "pxelinux.0":
			return "bios/core/pxelinux.0"
		case "ldlinux.c32":
			return "bios/com32/elflink/ldlinux/ldlinux.c32"
		case "libcom32.c32":
			return "bios/com32/lib/libcom32.c32"
		case "libutil.c32":
			return "bios/com32/libutil/libutil.c32"
		case "vesamenu.c32":
			return "bios/com32/menu/vesamenu.c32"
		case "menu.c32":
			return "bios/com32/menu/menu.c32"
		case "chain.c32":
			return "bios/com32/modules/chain.c32"
		case "reboot.c32":
			return "bios/com32/modules/reboot.c32"
		case "poweroff.c32":
			return "bios/com32/modules/poweroff.c32"
		}
	case "efi":
		switch fileName {
		case "syslinux.efi":
			return "efi64/efi/syslinux.efi"
		case "ldlinux.e64":
			return "efi64/com32/elflink/ldlinux/ldlinux.e64"
		case "libcom32.c32":
			return "efi64/com32/lib/libcom32.c32"
		case "libutil.c32":
			return "efi64/com32/libutil/libutil.c32"
		case "vesamenu.c32":
			return "efi64/com32/menu/vesamenu.c32"
		case "menu.c32":
			return "efi64/com32/menu/menu.c32"
		case "chain.c32":
			return "efi64/com32/modules/chain.c32"
		case "reboot.c32":
			return "efi64/com32/modules/reboot.c32"
		case "poweroff.c32":
			return "efi64/com32/modules/poweroff.c32"
		}
	}
	return ""
}

// GetBootTypeFromVersion determines available boot types for a version.
// The comparison is numeric so that e.g. "10.00" is not considered older
// than "4.00" the way a lexicographic string comparison would.
func GetBootTypeFromVersion(version string) []string {
	// Most modern versions support both BIOS and EFI
	// Very old versions might only support BIOS
	if compareVersionStrings(version, "4.00") < 0 {
		return []string{"bios"}
	}
	return []string{"bios", "efi"}
}

// compareVersionStrings compares dot-separated version strings numerically,
// segment by segment, falling back to lexical comparison for non-numeric
// segments.
func compareVersionStrings(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		ai, aerr := parseVersionSegment(as[i])
		bi, berr := parseVersionSegment(bs[i])
		switch {
		case aerr == nil && berr == nil:
			if ai != bi {
				if ai < bi {
					return -1
				}
				return 1
			}
		default:
			if as[i] != bs[i] {
				if as[i] < bs[i] {
					return -1
				}
				return 1
			}
		}
	}
	switch {
	case len(as) < len(bs):
		return -1
	case len(as) > len(bs):
		return 1
	default:
		return 0
	}
}

// parseVersionSegment parses the leading numeric portion of a version
// segment ("03" -> 3, "pre1" -> error).
func parseVersionSegment(s string) (int, error) {
	digits := s
	for i, r := range s {
		if r < '0' || r > '9' {
			digits = s[:i]
			break
		}
	}
	if digits == "" {
		return 0, fmt.Errorf("no numeric prefix in %q", s)
	}
	n := 0
	for _, r := range digits {
		n = n*10 + int(r-'0')
	}
	return n, nil
}

// ParseVersionFromFilename extracts version from filename like "syslinux-6.03.tar.gz".
// It returns "" unless the filename has both the "syslinux-" prefix and the
// ".tar.gz" suffix.
func ParseVersionFromFilename(filename string) string {
	const prefix = "syslinux-"
	const suffix = ".tar.gz"
	if !strings.HasPrefix(filename, prefix) || !strings.HasSuffix(filename, suffix) {
		return ""
	}
	version := filename[len(prefix) : len(filename)-len(suffix)]
	if version == "" {
		return ""
	}
	return version
}
