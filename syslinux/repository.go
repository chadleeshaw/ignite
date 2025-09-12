package syslinux

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.etcd.io/bbolt"
)

type boltRepository struct {
	db *bbolt.DB
}

// Repository bucket names
const (
	VersionsBucket        = "syslinux_versions"
	BootFilesBucket       = "syslinux_boot_files"
	DownloadStatusBucket  = "syslinux_download_status"
	ConfigBucket          = "syslinux_config"
)

// NewBoltRepository creates a new Bolt-based repository
func NewBoltRepository(db *bbolt.DB) (Repository, error) {
	repo := &boltRepository{db: db}
	
	// Initialize buckets
	err := db.Update(func(tx *bbolt.Tx) error {
		buckets := []string{
			VersionsBucket,
			BootFilesBucket,
			DownloadStatusBucket,
			ConfigBucket,
		}
		
		for _, bucket := range buckets {
			if _, err := tx.CreateBucketIfNotExists([]byte(bucket)); err != nil {
				return fmt.Errorf("failed to create bucket %s: %w", bucket, err)
			}
		}
		return nil
	})
	
	return repo, err
}

// Version management

func (r *boltRepository) SaveVersion(ctx context.Context, version *SyslinuxVersion) error {
	return r.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(VersionsBucket))
		version.UpdatedAt = time.Now()
		
		data, err := json.Marshal(version)
		if err != nil {
			return fmt.Errorf("failed to marshal version: %w", err)
		}
		
		return b.Put([]byte(version.ID), data)
	})
}

func (r *boltRepository) GetVersion(ctx context.Context, id string) (*SyslinuxVersion, error) {
	var version *SyslinuxVersion
	
	err := r.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(VersionsBucket))
		data := b.Get([]byte(id))
		
		if data == nil {
			return fmt.Errorf("version not found")
		}
		
		version = &SyslinuxVersion{}
		return json.Unmarshal(data, version)
	})
	
	return version, err
}

func (r *boltRepository) GetVersionByNumber(ctx context.Context, versionNumber string) (*SyslinuxVersion, error) {
	var foundVersion *SyslinuxVersion
	
	err := r.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(VersionsBucket))
		c := b.Cursor()
		
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var version SyslinuxVersion
			if err := json.Unmarshal(v, &version); err != nil {
				continue
			}
			
			if version.Version == versionNumber {
				foundVersion = &version
				break
			}
		}
		
		if foundVersion == nil {
			return fmt.Errorf("version %s not found", versionNumber)
		}
		
		return nil
	})
	
	return foundVersion, err
}

func (r *boltRepository) ListVersions(ctx context.Context) ([]*SyslinuxVersion, error) {
	var versions []*SyslinuxVersion
	
	err := r.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(VersionsBucket))
		c := b.Cursor()
		
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var version SyslinuxVersion
			if err := json.Unmarshal(v, &version); err != nil {
				continue
			}
			versions = append(versions, &version)
		}
		
		return nil
	})
	
	return versions, err
}

func (r *boltRepository) DeleteVersion(ctx context.Context, id string) error {
	return r.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(VersionsBucket))
		return b.Delete([]byte(id))
	})
}

func (r *boltRepository) SetActiveVersion(ctx context.Context, version string) error {
	return r.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(VersionsBucket))
		c := b.Cursor()
		
		// First, set all versions to inactive
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var ver SyslinuxVersion
			if err := json.Unmarshal(v, &ver); err != nil {
				continue
			}
			
			if ver.Active {
				ver.Active = false
				ver.UpdatedAt = time.Now()
				data, _ := json.Marshal(&ver)
				b.Put(k, data)
			}
		}
		
		// Then set the specified version to active
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var ver SyslinuxVersion
			if err := json.Unmarshal(v, &ver); err != nil {
				continue
			}
			
			if ver.Version == version {
				ver.Active = true
				ver.UpdatedAt = time.Now()
				data, _ := json.Marshal(&ver)
				b.Put(k, data)
			}
		}
		
		return nil
	})
}

func (r *boltRepository) GetActiveVersion(ctx context.Context) (*SyslinuxVersion, error) {
	var activeVersion *SyslinuxVersion
	
	err := r.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(VersionsBucket))
		c := b.Cursor()
		
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var version SyslinuxVersion
			if err := json.Unmarshal(v, &version); err != nil {
				continue
			}
			
			if version.Active {
				activeVersion = &version
				break
			}
		}
		
		return nil
	})
	
	if activeVersion == nil {
		return nil, fmt.Errorf("no active version found")
	}
	
	return activeVersion, err
}

// Boot file management

func (r *boltRepository) SaveBootFile(ctx context.Context, bootFile *SyslinuxBootFile) error {
	return r.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(BootFilesBucket))
		bootFile.UpdatedAt = time.Now()
		
		data, err := json.Marshal(bootFile)
		if err != nil {
			return fmt.Errorf("failed to marshal boot file: %w", err)
		}
		
		return b.Put([]byte(bootFile.ID), data)
	})
}

func (r *boltRepository) GetBootFile(ctx context.Context, id string) (*SyslinuxBootFile, error) {
	var bootFile *SyslinuxBootFile
	
	err := r.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(BootFilesBucket))
		data := b.Get([]byte(id))
		
		if data == nil {
			return fmt.Errorf("boot file not found")
		}
		
		bootFile = &SyslinuxBootFile{}
		return json.Unmarshal(data, bootFile)
	})
	
	return bootFile, err
}

func (r *boltRepository) ListBootFiles(ctx context.Context, version, bootType string) ([]*SyslinuxBootFile, error) {
	var bootFiles []*SyslinuxBootFile
	
	err := r.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(BootFilesBucket))
		c := b.Cursor()
		
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var bootFile SyslinuxBootFile
			if err := json.Unmarshal(v, &bootFile); err != nil {
				continue
			}
			
			// Filter by version and/or boot type if specified
			if version != "" && bootFile.Version != version {
				continue
			}
			if bootType != "" && bootFile.BootType != bootType {
				continue
			}
			
			bootFiles = append(bootFiles, &bootFile)
		}
		
		return nil
	})
	
	return bootFiles, err
}

func (r *boltRepository) UpdateBootFileStatus(ctx context.Context, id string, installed bool) error {
	return r.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(BootFilesBucket))
		data := b.Get([]byte(id))
		
		if data == nil {
			return fmt.Errorf("boot file not found")
		}
		
		var bootFile SyslinuxBootFile
		if err := json.Unmarshal(data, &bootFile); err != nil {
			return err
		}
		
		bootFile.Installed = installed
		bootFile.UpdatedAt = time.Now()
		
		updatedData, err := json.Marshal(&bootFile)
		if err != nil {
			return err
		}
		
		return b.Put([]byte(id), updatedData)
	})
}

func (r *boltRepository) DeleteBootFile(ctx context.Context, id string) error {
	return r.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(BootFilesBucket))
		return b.Delete([]byte(id))
	})
}

// Download status tracking

func (r *boltRepository) SaveDownloadStatus(ctx context.Context, status *DownloadStatus) error {
	return r.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(DownloadStatusBucket))
		
		data, err := json.Marshal(status)
		if err != nil {
			return fmt.Errorf("failed to marshal download status: %w", err)
		}
		
		return b.Put([]byte(status.ID), data)
	})
}

func (r *boltRepository) GetDownloadStatus(ctx context.Context, id string) (*DownloadStatus, error) {
	var status *DownloadStatus
	
	err := r.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(DownloadStatusBucket))
		data := b.Get([]byte(id))
		
		if data == nil {
			return fmt.Errorf("download status not found")
		}
		
		status = &DownloadStatus{}
		return json.Unmarshal(data, status)
	})
	
	return status, err
}

func (r *boltRepository) ListDownloadStatuses(ctx context.Context) ([]*DownloadStatus, error) {
	var statuses []*DownloadStatus
	
	err := r.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(DownloadStatusBucket))
		c := b.Cursor()
		
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var status DownloadStatus
			if err := json.Unmarshal(v, &status); err != nil {
				continue
			}
			statuses = append(statuses, &status)
		}
		
		return nil
	})
	
	return statuses, err
}

func (r *boltRepository) DeleteDownloadStatus(ctx context.Context, id string) error {
	return r.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(DownloadStatusBucket))
		return b.Delete([]byte(id))
	})
}

// Configuration management

func (r *boltRepository) SaveConfig(ctx context.Context, config *SyslinuxConfig) error {
	return r.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(ConfigBucket))
		
		data, err := json.Marshal(config)
		if err != nil {
			return fmt.Errorf("failed to marshal config: %w", err)
		}
		
		return b.Put([]byte("current"), data)
	})
}

func (r *boltRepository) GetConfig(ctx context.Context) (*SyslinuxConfig, error) {
	var config *SyslinuxConfig
	
	err := r.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(ConfigBucket))
		data := b.Get([]byte("current"))
		
		if data == nil {
			// Return default config if none exists
			defaultConfig := GetDefaultConfig()
			config = &defaultConfig
			return nil
		}
		
		config = &SyslinuxConfig{}
		return json.Unmarshal(data, config)
	})
	
	return config, err
}

// Cleanup old download statuses
func (r *boltRepository) CleanupOldDownloadStatuses(ctx context.Context, olderThan time.Duration) error {
	cutoff := time.Now().Add(-olderThan)
	
	return r.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(DownloadStatusBucket))
		c := b.Cursor()
		
		var toDelete [][]byte
		
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var status DownloadStatus
			if err := json.Unmarshal(v, &status); err != nil {
				continue
			}
			
			// Delete completed or failed downloads older than cutoff
			if status.CompletedAt != nil && status.CompletedAt.Before(cutoff) {
				toDelete = append(toDelete, append([]byte(nil), k...))
			}
		}
		
		for _, key := range toDelete {
			b.Delete(key)
		}
		
		return nil
	})
}

// Helper methods for statistics and maintenance

func (r *boltRepository) GetStatistics(ctx context.Context) (*RepositoryStatistics, error) {
	var stats RepositoryStatistics
	
	err := r.db.View(func(tx *bbolt.Tx) error {
		// Count versions
		versionsBucket := tx.Bucket([]byte(VersionsBucket))
		stats.TotalVersions = versionsBucket.Stats().KeyN
		
		// Count boot files
		bootFilesBucket := tx.Bucket([]byte(BootFilesBucket))
		stats.TotalBootFiles = bootFilesBucket.Stats().KeyN
		
		// Count active downloads
		downloadsBucket := tx.Bucket([]byte(DownloadStatusBucket))
		c := downloadsBucket.Cursor()
		
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var status DownloadStatus
			if err := json.Unmarshal(v, &status); err != nil {
				continue
			}
			
			switch status.Status {
			case "downloading", "extracting":
				stats.ActiveDownloads++
			case "completed":
				stats.CompletedDownloads++
			case "failed", "cancelled":
				stats.FailedDownloads++
			}
		}
		
		return nil
	})
	
	return &stats, err
}

// RepositoryStatistics provides repository usage statistics
type RepositoryStatistics struct {
	TotalVersions      int `json:"total_versions"`
	TotalBootFiles     int `json:"total_boot_files"`
	ActiveDownloads    int `json:"active_downloads"`
	CompletedDownloads int `json:"completed_downloads"`
	FailedDownloads    int `json:"failed_downloads"`
}