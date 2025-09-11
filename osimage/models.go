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

// OSImageSources contains download sources for different operating systems
type OSImageSources struct {
	Ubuntu map[string]string `json:"ubuntu"`
	CentOS map[string]string `json:"centos"`
	NixOS  map[string]string `json:"nixos"`
}

// GetDefaultSources returns the default download sources for OS images
func GetDefaultSources() OSImageSources {
	return OSImageSources{
		Ubuntu: map[string]string{
			"20.04": "https://github.com/netbootxyz/ubuntu-squash/releases/download/20.04.6-c92baa25/",
			"22.04": "https://github.com/netbootxyz/ubuntu-squash/releases/download/22.04.5-b0159fca/",
			"24.04": "https://github.com/netbootxyz/ubuntu-squash/releases/download/24.04.3-8efa196d/",
		},
		CentOS: map[string]string{
			"9":  "http://mirror.stream.centos.org/9-stream/BaseOS/x86_64/os/images/pxeboot/",
			"10": "http://mirror.stream.centos.org/9-stream/BaseOS/x86_64/os/images/pxeboot/",
		},
		NixOS: map[string]string{
			"23.05": "https://github.com/nix-community/nixos-images/releases/download/nixos-23.05/",
			"23.11": "https://github.com/nix-community/nixos-images/releases/download/nixos-23.11/",
			"24.05": "https://github.com/nix-community/nixos-images/releases/download/nixos-24.05/",
		},
	}
}

// GetKernelFileName returns the expected kernel filename for an OS
func GetKernelFileName(os string) string {
	switch os {
	case "ubuntu":
		return "vmlinuz"
	case "centos":
		return "vmlinuz"
	case "nixos":
		return "bzImage-x86_64-linux"
	default:
		return "vmlinuz"
	}
}

// GetInitrdFileName returns the expected initrd filename for an OS
func GetInitrdFileName(os string) string {
	switch os {
	case "ubuntu":
		return "initrd"
	case "centos":
		return "initrd.img"
	case "nixos":
		return "initrd-x86_64-linux"
	default:
		return "initrd.img"
	}
}
