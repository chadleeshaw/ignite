package osimage

import (
	"time"
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
	Checksum     string    `json:"checksum"`     // SHA256 verification
	Active       bool      `json:"active"`       // Default version for OS
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

// DownloadStatus represents the status of an OS image download
type DownloadStatus struct {
	ID           string     `json:"id"`
	OS           string     `json:"os"`
	Version      string     `json:"version"`
	Status       string     `json:"status"`   // downloading, completed, failed
	Progress     int        `json:"progress"` // Percentage 0-100
	ErrorMessage string     `json:"error_message,omitempty"`
	StartedAt    time.Time  `json:"started_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}

