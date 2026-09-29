package syslinux

import (
	"context"
)

// Repository interface for Syslinux data persistence
type Repository interface {
	// Version management
	SaveVersion(ctx context.Context, version *SyslinuxVersion) error
	GetVersion(ctx context.Context, id string) (*SyslinuxVersion, error)
	GetVersionByNumber(ctx context.Context, version string) (*SyslinuxVersion, error)
	ListVersions(ctx context.Context) ([]*SyslinuxVersion, error)
	DeleteVersion(ctx context.Context, id string) error
	SetActiveVersion(ctx context.Context, version string) error
	GetActiveVersion(ctx context.Context) (*SyslinuxVersion, error)

	// Boot file management
	SaveBootFile(ctx context.Context, bootFile *SyslinuxBootFile) error
	GetBootFile(ctx context.Context, id string) (*SyslinuxBootFile, error)
	ListBootFiles(ctx context.Context, version, bootType string) ([]*SyslinuxBootFile, error)
	UpdateBootFileStatus(ctx context.Context, id string, installed bool) error
	DeleteBootFile(ctx context.Context, id string) error

	// Download status tracking
	SaveDownloadStatus(ctx context.Context, status *DownloadStatus) error
	GetDownloadStatus(ctx context.Context, id string) (*DownloadStatus, error)
	ListDownloadStatuses(ctx context.Context) ([]*DownloadStatus, error)
	DeleteDownloadStatus(ctx context.Context, id string) error
}

// Service interface for Syslinux business logic
type Service interface {
	// Mirror scanning and version discovery
	ScanMirror(ctx context.Context) ([]*SyslinuxMirror, error)
	RefreshAvailableVersions(ctx context.Context) error
	GetAvailableVersions(ctx context.Context) ([]*SyslinuxVersion, error)

	// Download and installation
	DownloadVersion(ctx context.Context, version string) (*DownloadStatus, error)
	GetDownloadStatus(ctx context.Context, id string) (*DownloadStatus, error)
	CancelDownload(ctx context.Context, id string) error

	// Version activation/deactivation
	DeactivateVersion(ctx context.Context, version string) error

	// Boot file management
	ExtractBootFiles(ctx context.Context, version string) error
	InstallBootFiles(ctx context.Context, version, bootType string) error
	ListInstalledBootFiles(ctx context.Context, bootType string) ([]*SyslinuxBootFile, error)
	RemoveBootFiles(ctx context.Context, version, bootType string) error

	// Configuration and status
	GetConfig() SyslinuxConfig
	UpdateConfig(ctx context.Context, config SyslinuxConfig) error
	GetSystemStatus(ctx context.Context) (*SystemStatus, error)

	// Validation and health checks
	ValidateInstallation(ctx context.Context, bootType string) (*ValidationResult, error)
	CheckDiskSpace(ctx context.Context) (*DiskSpaceInfo, error)
}

// SystemStatus provides overview of the Syslinux system
type SystemStatus struct {
	InstalledVersions map[string]bool `json:"installed_versions"` // version -> installed
	ActiveVersion     string          `json:"active_version"`
	BiosFilesCount    int             `json:"bios_files_count"`
	EfiFilesCount     int             `json:"efi_files_count"`
	TotalDiskUsage    int64           `json:"total_disk_usage"`
	LastUpdate        string          `json:"last_update"`
	HealthStatus      string          `json:"health_status"` // healthy, warning, error
}

// ValidationResult contains validation results for boot files
type ValidationResult struct {
	Valid        bool     `json:"valid"`
	BootType     string   `json:"boot_type"`
	MissingFiles []string `json:"missing_files"`
	CorruptFiles []string `json:"corrupt_files"`
	ExtraFiles   []string `json:"extra_files"`
	Warnings     []string `json:"warnings"`
}

// DiskSpaceInfo provides disk space information
type DiskSpaceInfo struct {
	TotalSpace     int64   `json:"total_space"`
	AvailableSpace int64   `json:"available_space"`
	UsedSpace      int64   `json:"used_space"`
	UsagePercent   float64 `json:"usage_percent"`
	Sufficient     bool    `json:"sufficient"` // Whether space is sufficient for operations
}
