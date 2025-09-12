package osimage

import "context"

// OSImageRepository defines the interface for OS image data persistence
type OSImageRepository interface {
	// Save stores an OS image record
	Save(ctx context.Context, image *OSImage) error

	// Get retrieves an OS image by ID
	Get(ctx context.Context, id string) (*OSImage, error)

	// GetAll retrieves all OS images
	GetAll(ctx context.Context) ([]*OSImage, error)

	// GetByOS retrieves all images for a specific operating system
	GetByOS(ctx context.Context, os string) ([]*OSImage, error)

	// GetByOSAndVersion retrieves a specific OS image by OS and version
	GetByOSAndVersion(ctx context.Context, os, version string) (*OSImage, error)

	// GetDefaultVersion retrieves the default (active) version for an OS
	GetDefaultVersion(ctx context.Context, os string) (*OSImage, error)

	// Delete removes an OS image by ID
	Delete(ctx context.Context, id string) error

	// SetDefault sets an OS image as the default for its OS type
	SetDefault(ctx context.Context, id string) error
}

// DownloadStatusRepository defines the interface for download status tracking
type DownloadStatusRepository interface {
	// Save stores a download status record
	Save(ctx context.Context, status *DownloadStatus) error

	// Get retrieves a download status by ID
	Get(ctx context.Context, id string) (*DownloadStatus, error)

	// GetActive retrieves all active downloads
	GetActive(ctx context.Context) ([]*DownloadStatus, error)

	// Delete removes a download status record
	Delete(ctx context.Context, id string) error
}

// OSImageService defines business logic for OS images
type OSImageService interface {
	GetAllOSImages(ctx context.Context) ([]*OSImage, error)
	GetOSImagesByOS(ctx context.Context, os string) ([]*OSImage, error)
	GetOSImage(ctx context.Context, id string) (*OSImage, error)
	GetDefaultVersion(ctx context.Context, os string) (*OSImage, error)
	SetDefaultVersion(ctx context.Context, id string) error
	DeleteOSImage(ctx context.Context, id string) error
	DownloadOSImage(ctx context.Context, osConfig OSImageConfig) (*DownloadStatus, error)
	GetDownloadStatus(ctx context.Context, id string) (*DownloadStatus, error)
	GetActiveDownloads(ctx context.Context) ([]*DownloadStatus, error)
	CancelDownload(ctx context.Context, id string) error
	GetAvailableVersions(ctx context.Context, os string) ([]string, error)
}
