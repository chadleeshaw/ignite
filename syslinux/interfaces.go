package syslinux

import (
	"context"
	"io"
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

// Downloader interface for handling file downloads
type Downloader interface {
	Download(ctx context.Context, url, destination string, progress chan<- int) error
	GetFileSize(ctx context.Context, url string) (int64, error)
	VerifyChecksum(filePath, expectedChecksum string) error
}

// Extractor interface for handling archive extraction
type Extractor interface {
	Extract(ctx context.Context, archivePath, destination string, progress chan<- int) error
	ListContents(archivePath string) ([]string, error)
	ExtractFile(archivePath, fileName, destination string) error
}

// MirrorScanner interface for scanning the kernel.org mirror
type MirrorScanner interface {
	ScanVersions(ctx context.Context, baseURL string) ([]*SyslinuxMirror, error)
	GetFileInfo(ctx context.Context, url string) (*FileInfo, error)
}

// FileManager interface for file system operations
type FileManager interface {
	CreateDirectory(path string) error
	CopyFile(src, dst string) error
	MoveFile(src, dst string) error
	DeleteFile(path string) error
	FileExists(path string) bool
	GetFileSize(path string) (int64, error)
	GetFileChecksum(path string) (string, error)
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte) error
}

// Additional types for interface support

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

// FileInfo contains information about a file from mirror
type FileInfo struct {
	Name         string `json:"name"`
	Size         int64  `json:"size"`
	LastModified string `json:"last_modified"`
	URL          string `json:"url"`
}

// ProgressCallback is used for reporting download/extraction progress
type ProgressCallback func(percent int, message string)

// EventHandler interface for handling Syslinux events
type EventHandler interface {
	OnDownloadStarted(version string)
	OnDownloadProgress(version string, percent int)
	OnDownloadCompleted(version string)
	OnDownloadFailed(version string, err error)
	OnExtractionStarted(version string)
	OnExtractionCompleted(version string)
	OnInstallationCompleted(version, bootType string)
}

// Logger interface for structured logging
type Logger interface {
	Debug(msg string, fields ...interface{})
	Info(msg string, fields ...interface{})
	Warn(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
	WithField(key string, value interface{}) Logger
	WithFields(fields map[string]interface{}) Logger
}

// ConfigProvider interface for configuration management
type ConfigProvider interface {
	GetConfig() SyslinuxConfig
	SaveConfig(config SyslinuxConfig) error
	GetTFTPDir() string
	GetBiosDir() string
	GetEfiDir() string
	GetTempDir() string
}

// CacheManager interface for caching downloaded files and metadata
type CacheManager interface {
	Get(key string) (interface{}, bool)
	Set(key string, value interface{}, ttl int) error
	Delete(key string) error
	Clear() error
	GetSize() int64
}

// HTTPClient interface for making HTTP requests
type HTTPClient interface {
	Get(url string) (io.ReadCloser, error)
	GetWithContext(ctx context.Context, url string) (io.ReadCloser, error)
	Head(url string) (*HTTPResponse, error)
	HeadWithContext(ctx context.Context, url string) (*HTTPResponse, error)
}

// HTTPResponse represents an HTTP response
type HTTPResponse struct {
	StatusCode    int
	ContentLength int64
	LastModified  string
	Headers       map[string]string
}
