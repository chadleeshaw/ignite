package osimage

import (
	"context"
	"fmt"
	"ignite/db"
	"time"

	"github.com/google/uuid"
)

// OSImageRepositoryImpl implements OSImageRepository using BoltDB
type OSImageRepositoryImpl struct {
	*db.GenericRepository[OSImage]
}

// NewOSImageRepository creates a new OS image repository
func NewOSImageRepository(database db.Database) OSImageRepository {
	return &OSImageRepositoryImpl{
		GenericRepository: db.NewGenericRepository[OSImage](database, "osimages"),
	}
}

// Save stores an OS image record
func (r *OSImageRepositoryImpl) Save(ctx context.Context, image *OSImage) error {
	if image.ID == "" {
		image.ID = uuid.New().String()
	}

	now := time.Now()
	if image.CreatedAt.IsZero() {
		image.CreatedAt = now
	}
	image.UpdatedAt = now

	return r.GenericRepository.Save(ctx, image.ID, *image)
}

// Get retrieves an OS image by ID
func (r *OSImageRepositoryImpl) Get(ctx context.Context, id string) (*OSImage, error) {
	image, err := r.GenericRepository.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return &image, nil
}

// GetAll retrieves all OS images
func (r *OSImageRepositoryImpl) GetAll(ctx context.Context) ([]*OSImage, error) {
	imagesMap, err := r.GenericRepository.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	var images []*OSImage
	for _, image := range imagesMap {
		imageCopy := image
		images = append(images, &imageCopy)
	}

	return images, nil
}

// GetByOS retrieves all images for a specific operating system
func (r *OSImageRepositoryImpl) GetByOS(ctx context.Context, os string) ([]*OSImage, error) {
	allImages, err := r.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	var filtered []*OSImage
	for _, image := range allImages {
		if image.OS == os {
			filtered = append(filtered, image)
		}
	}

	return filtered, nil
}

// GetByOSAndVersion retrieves a specific OS image by OS and version
func (r *OSImageRepositoryImpl) GetByOSAndVersion(ctx context.Context, os, version string) (*OSImage, error) {
	allImages, err := r.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	for _, image := range allImages {
		if image.OS == os && image.Version == version {
			return image, nil
		}
	}

	return nil, fmt.Errorf("OS image not found: %s %s", os, version)
}

// GetDefaultVersion retrieves the default (active) version for an OS
func (r *OSImageRepositoryImpl) GetDefaultVersion(ctx context.Context, os string) (*OSImage, error) {
	allImages, err := r.GetByOS(ctx, os)
	if err != nil {
		return nil, err
	}

	for _, image := range allImages {
		if image.Active {
			return image, nil
		}
	}

	return nil, fmt.Errorf("no default version found for OS: %s", os)
}

// Delete removes an OS image by ID
func (r *OSImageRepositoryImpl) Delete(ctx context.Context, id string) error {
	return r.GenericRepository.Delete(ctx, id)
}

// SetDefault sets an OS image as the default for its OS type
func (r *OSImageRepositoryImpl) SetDefault(ctx context.Context, id string) error {
	// Get the image to be set as default
	image, err := r.Get(ctx, id)
	if err != nil {
		return err
	}

	// First, unset all other images for this OS as default
	allImages, err := r.GetByOS(ctx, image.OS)
	if err != nil {
		return err
	}

	for _, img := range allImages {
		if img.Active {
			img.Active = false
			if err := r.Save(ctx, img); err != nil {
				return err
			}
		}
	}

	// Set the target image as default
	image.Active = true
	return r.Save(ctx, image)
}

// DownloadStatusRepositoryImpl implements DownloadStatusRepository using BoltDB
type DownloadStatusRepositoryImpl struct {
	*db.GenericRepository[DownloadStatus]
}

// NewDownloadStatusRepository creates a new download status repository
func NewDownloadStatusRepository(database db.Database) DownloadStatusRepository {
	return &DownloadStatusRepositoryImpl{
		GenericRepository: db.NewGenericRepository[DownloadStatus](database, "download_status"),
	}
}

// Save stores a download status record
func (r *DownloadStatusRepositoryImpl) Save(ctx context.Context, status *DownloadStatus) error {
	if status.ID == "" {
		status.ID = uuid.New().String()
	}

	if status.StartedAt.IsZero() {
		status.StartedAt = time.Now()
	}

	return r.GenericRepository.Save(ctx, status.ID, *status)
}

// Get retrieves a download status by ID
func (r *DownloadStatusRepositoryImpl) Get(ctx context.Context, id string) (*DownloadStatus, error) {
	status, err := r.GenericRepository.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return &status, nil
}

// GetActive retrieves all active downloads
func (r *DownloadStatusRepositoryImpl) GetActive(ctx context.Context) ([]*DownloadStatus, error) {
	allStatusMap, err := r.GenericRepository.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	var active []*DownloadStatus
	for _, status := range allStatusMap {
		if status.Status == "downloading" || status.Status == "queued" {
			statusCopy := status
			active = append(active, &statusCopy)
		}
	}

	return active, nil
}

// Delete removes a download status record
func (r *DownloadStatusRepositoryImpl) Delete(ctx context.Context, id string) error {
	return r.GenericRepository.Delete(ctx, id)
}
