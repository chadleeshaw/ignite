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

// OSImageService defines the interface for OS image business logic
type OSImageService interface {
	// GetAllOSImages retrieves all OS images
	GetAllOSImages(ctx context.Context) ([]*OSImage, error)
	
	// GetOSImagesByOS retrieves all images for a specific operating system
	GetOSImagesByOS(ctx context.Context, os string) ([]*OSImage, error)
	
	// GetOSImage retrieves an OS image by ID
	GetOSImage(ctx context.Context, id string) (*OSImage, error)
	
	// GetDefaultVersion retrieves the default version for an OS
	GetDefaultVersion(ctx context.Context, os string) (*OSImage, error)
	
	// SetDefaultVersion sets an OS image as the default for its OS type
	SetDefaultVersion(ctx context.Context, id string) error
	
	// DeleteOSImage removes an OS image and its files
	DeleteOSImage(ctx context.Context, id string) error
	
	// DownloadOSImage starts downloading an OS image
	DownloadOSImage(ctx context.Context, osConfig OSImageConfig) (*DownloadStatus, error)
	
	// GetDownloadStatus retrieves the status of a download
	GetDownloadStatus(ctx context.Context, id string) (*DownloadStatus, error)
	
	// GetActiveDownloads retrieves all active downloads
	GetActiveDownloads(ctx context.Context) ([]*DownloadStatus, error)
	
	// CancelDownload cancels an active download
	CancelDownload(ctx context.Context, id string) error
}