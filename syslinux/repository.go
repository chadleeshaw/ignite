package syslinux

import (
	"context"
	"encoding/json"
	"fmt"
	"ignite/db"
	"ignite/dlstatus"
	"time"
)

// Repository bucket names
const (
	VersionsBucket       = "syslinux_versions"
	BootFilesBucket      = "syslinux_boot_files"
	DownloadStatusBucket = "syslinux_download_status"
	ConfigBucket         = "syslinux_config"
)

type boltRepository struct {
	db db.Database
}

// NewBoltRepository creates a new Bolt-based repository backed by the shared
// db.Database abstraction (no direct *bbolt.DB dependency).
func NewBoltRepository(database db.Database) (Repository, error) {
	repo := &boltRepository{db: database}

	// Initialize buckets
	for _, bucket := range []string{
		VersionsBucket,
		BootFilesBucket,
		DownloadStatusBucket,
		ConfigBucket,
	} {
		if err := database.GetOrCreateBucket(context.Background(), bucket); err != nil {
			return nil, fmt.Errorf("failed to create bucket %s: %w", bucket, err)
		}
	}

	return repo, nil
}

// putJSON marshals value and stores it under key in bucket.
func (r *boltRepository) putJSON(ctx context.Context, bucket, key string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}
	return r.db.PutKV(ctx, bucket, []byte(key), data)
}

// getJSON fetches key from bucket and unmarshals it into value.
func (r *boltRepository) getJSON(ctx context.Context, bucket, key string, value interface{}) error {
	data, err := r.db.GetKV(ctx, bucket, []byte(key))
	if err != nil {
		return err
	}
	if data == nil {
		return fmt.Errorf("key %q not found in bucket %q", key, bucket)
	}
	return json.Unmarshal(data, value)
}

// allJSON returns every value in bucket unmarshalled into a slice.
func (r *boltRepository) allJSON(ctx context.Context, bucket string, newValue func() interface{}) ([]interface{}, error) {
	raw, err := r.db.GetAllKV(ctx, bucket)
	if err != nil {
		return nil, err
	}
	values := make([]interface{}, 0, len(raw))
	for _, data := range raw {
		v := newValue()
		if err := json.Unmarshal(data, v); err != nil {
			continue
		}
		values = append(values, v)
	}
	return values, nil
}

// Version management

func (r *boltRepository) SaveVersion(ctx context.Context, version *SyslinuxVersion) error {
	// Copy before stamping so the caller's struct is never mutated.
	versionCopy := *version
	versionCopy.UpdatedAt = time.Now()
	return r.putJSON(ctx, VersionsBucket, versionCopy.ID, &versionCopy)
}

func (r *boltRepository) GetVersion(ctx context.Context, id string) (*SyslinuxVersion, error) {
	version := &SyslinuxVersion{}
	if err := r.getJSON(ctx, VersionsBucket, id, version); err != nil {
		return nil, fmt.Errorf("version not found: %w", err)
	}
	return version, nil
}

func (r *boltRepository) GetVersionByNumber(ctx context.Context, versionNumber string) (*SyslinuxVersion, error) {
	versions, err := r.ListVersions(ctx)
	if err != nil {
		return nil, err
	}
	for _, version := range versions {
		if version.Version == versionNumber {
			return version, nil
		}
	}
	return nil, fmt.Errorf("version %s not found", versionNumber)
}

func (r *boltRepository) ListVersions(ctx context.Context) ([]*SyslinuxVersion, error) {
	raw, err := r.allJSON(ctx, VersionsBucket, func() interface{} { return &SyslinuxVersion{} })
	if err != nil {
		return nil, err
	}
	versions := make([]*SyslinuxVersion, 0, len(raw))
	for _, v := range raw {
		versions = append(versions, v.(*SyslinuxVersion))
	}
	return versions, nil
}

func (r *boltRepository) DeleteVersion(ctx context.Context, id string) error {
	return r.db.DeleteKV(ctx, VersionsBucket, []byte(id))
}

func (r *boltRepository) SetActiveVersion(ctx context.Context, version string) error {
	versions, err := r.ListVersions(ctx)
	if err != nil {
		return err
	}

	now := time.Now()
	updates := make(map[string][]byte)
	found := false
	for _, ver := range versions {
		updated := *ver
		changed := false
		if updated.Active {
			updated.Active = false
			updated.UpdatedAt = now
			changed = true
		}
		if updated.Version == version {
			updated.Active = true
			updated.UpdatedAt = now
			changed = true
			found = true
		}
		if changed {
			data, err := json.Marshal(&updated)
			if err != nil {
				return fmt.Errorf("failed to marshal version %q: %w", updated.ID, err)
			}
			updates[updated.ID] = data
		}
	}

	if !found {
		return fmt.Errorf("version %s not found", version)
	}

	// Apply the deactivation/activation atomically in a single batch when the
	// database supports it; otherwise fall back to sequential writes.
	if bp, ok := r.db.(batchPutter); ok {
		return bp.BatchPut(ctx, VersionsBucket, updates)
	}
	for key, data := range updates {
		if err := r.db.PutKV(ctx, VersionsBucket, []byte(key), data); err != nil {
			return fmt.Errorf("failed to save version %q: %w", key, err)
		}
	}
	return nil
}

// batchPutter is implemented by *db.BoltDB (see db/bolt.go). The repository
// asserts it via this local interface so it keeps depending only on
// db.Database while still getting atomic batch writes when available.
type batchPutter interface {
	BatchPut(ctx context.Context, bucket string, kvs map[string][]byte) error
}

func (r *boltRepository) GetActiveVersion(ctx context.Context) (*SyslinuxVersion, error) {
	versions, err := r.ListVersions(ctx)
	if err != nil {
		return nil, err
	}
	for _, version := range versions {
		if version.Active {
			return version, nil
		}
	}
	return nil, fmt.Errorf("no active version found")
}

// Boot file management

func (r *boltRepository) SaveBootFile(ctx context.Context, bootFile *SyslinuxBootFile) error {
	// Copy before stamping so the caller's struct is never mutated.
	bootFileCopy := *bootFile
	bootFileCopy.UpdatedAt = time.Now()
	return r.putJSON(ctx, BootFilesBucket, bootFileCopy.ID, &bootFileCopy)
}

func (r *boltRepository) GetBootFile(ctx context.Context, id string) (*SyslinuxBootFile, error) {
	bootFile := &SyslinuxBootFile{}
	if err := r.getJSON(ctx, BootFilesBucket, id, bootFile); err != nil {
		return nil, fmt.Errorf("boot file not found: %w", err)
	}
	return bootFile, nil
}

func (r *boltRepository) ListBootFiles(ctx context.Context, version, bootType string) ([]*SyslinuxBootFile, error) {
	raw, err := r.allJSON(ctx, BootFilesBucket, func() interface{} { return &SyslinuxBootFile{} })
	if err != nil {
		return nil, err
	}
	var bootFiles []*SyslinuxBootFile
	for _, v := range raw {
		bootFile := v.(*SyslinuxBootFile)
		// Filter by version and/or boot type if specified
		if version != "" && bootFile.Version != version {
			continue
		}
		if bootType != "" && bootFile.BootType != bootType {
			continue
		}
		bootFiles = append(bootFiles, bootFile)
	}
	return bootFiles, nil
}

func (r *boltRepository) UpdateBootFileStatus(ctx context.Context, id string, installed bool) error {
	bootFile, err := r.GetBootFile(ctx, id)
	if err != nil {
		return err
	}
	bootFile.Installed = installed
	return r.SaveBootFile(ctx, bootFile)
}

func (r *boltRepository) DeleteBootFile(ctx context.Context, id string) error {
	return r.db.DeleteKV(ctx, BootFilesBucket, []byte(id))
}

// Download status tracking

func (r *boltRepository) SaveDownloadStatus(ctx context.Context, status *DownloadStatus) error {
	return r.putJSON(ctx, DownloadStatusBucket, status.ID, status)
}

func (r *boltRepository) GetDownloadStatus(ctx context.Context, id string) (*DownloadStatus, error) {
	status := &DownloadStatus{}
	if err := r.getJSON(ctx, DownloadStatusBucket, id, status); err != nil {
		return nil, fmt.Errorf("download status not found: %w", err)
	}
	return status, nil
}

func (r *boltRepository) ListDownloadStatuses(ctx context.Context) ([]*DownloadStatus, error) {
	raw, err := r.allJSON(ctx, DownloadStatusBucket, func() interface{} { return &DownloadStatus{} })
	if err != nil {
		return nil, err
	}
	statuses := make([]*DownloadStatus, 0, len(raw))
	for _, v := range raw {
		statuses = append(statuses, v.(*DownloadStatus))
	}
	return statuses, nil
}

func (r *boltRepository) DeleteDownloadStatus(ctx context.Context, id string) error {
	return r.db.DeleteKV(ctx, DownloadStatusBucket, []byte(id))
}

// Configuration management

func (r *boltRepository) SaveConfig(ctx context.Context, config *SyslinuxConfig) error {
	return r.putJSON(ctx, ConfigBucket, "current", config)
}

func (r *boltRepository) GetConfig(ctx context.Context) (*SyslinuxConfig, error) {
	config := &SyslinuxConfig{}
	if err := r.getJSON(ctx, ConfigBucket, "current", config); err != nil {
		// Return default config if none exists
		defaultConfig := GetDefaultConfig()
		return &defaultConfig, nil
	}
	return config, nil
}

// Cleanup old download statuses
func (r *boltRepository) CleanupOldDownloadStatuses(ctx context.Context, olderThan time.Duration) error {
	cutoff := time.Now().Add(-olderThan)

	statuses, err := r.ListDownloadStatuses(ctx)
	if err != nil {
		return err
	}

	for _, status := range statuses {
		// Delete completed or failed downloads older than cutoff
		if status.CompletedAt != nil && status.CompletedAt.Before(cutoff) {
			if err := r.DeleteDownloadStatus(ctx, status.ID); err != nil {
				return err
			}
		}
	}

	return nil
}

// Helper methods for statistics and maintenance

func (r *boltRepository) GetStatistics(ctx context.Context) (*RepositoryStatistics, error) {
	var stats RepositoryStatistics

	versions, err := r.ListVersions(ctx)
	if err != nil {
		return nil, err
	}
	stats.TotalVersions = len(versions)

	bootFiles, err := r.ListBootFiles(ctx, "", "")
	if err != nil {
		return nil, err
	}
	stats.TotalBootFiles = len(bootFiles)

	statuses, err := r.ListDownloadStatuses(ctx)
	if err != nil {
		return nil, err
	}
	for _, status := range statuses {
		switch {
		case status.Status.Active():
			stats.ActiveDownloads++
		case status.Status == dlstatus.StatusCompleted:
			stats.CompletedDownloads++
		case status.Status.Terminal():
			stats.FailedDownloads++
		}
	}

	return &stats, nil
}

// RepositoryStatistics provides repository usage statistics
type RepositoryStatistics struct {
	TotalVersions      int `json:"total_versions"`
	TotalBootFiles     int `json:"total_boot_files"`
	ActiveDownloads    int `json:"active_downloads"`
	CompletedDownloads int `json:"completed_downloads"`
	FailedDownloads    int `json:"failed_downloads"`
}
