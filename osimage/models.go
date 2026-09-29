package osimage

import (
	"time"

	"ignite/dlstatus"
)

// OSImage represents a bootable OS kernel and initrd combination
type OSImage struct {
	ID           string    `json:"id"`
	OS           string    `json:"os"`           // ubuntu, centos, nixos
	Version      string    `json:"version"`      // 22.04, 8, 23.11
	Architecture string    `json:"architecture"` // x86_64, arm64
	KernelPath   string    `json:"kernel_path"`  // ubuntu/22.04/vmlinuz
	InitrdPath   string    `json:"initrd_path"`  // ubuntu/22.04/initrd.img
	KernelSize   int64     `json:"kernel_size"`  // Size in bytes
	InitrdSize   int64     `json:"initrd_size"`  // Size in bytes
	Checksum     string    `json:"checksum"`     // SHA256 fingerprints of the downloaded files (kernel:initrd); verified against config only when expected checksums are provided
	Active       bool      `json:"active"`       // Default version for OS
	DownloadURL  string    `json:"download_url"` // Original download URL
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// OSImageConfig holds configuration for downloading OS images
type OSImageConfig struct {
	OS           string `json:"os"`
	Version      string `json:"version"`
	Architecture string `json:"architecture"`
	Source       string `json:"source"` // Download URL
}

// DownloadStatus is the shared download-status type (see ignite/dlstatus).
// It is aliased so existing references keep compiling.
type DownloadStatus = dlstatus.DownloadStatus
