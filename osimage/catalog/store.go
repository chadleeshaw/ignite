package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"ignite/db"
)

const (
	// Bucket is the bbolt bucket that holds the last catalog snapshot.
	Bucket = "osimage_catalog"

	// SnapshotKey is the only key in Bucket.
	SnapshotKey = "snapshot"
)

// BoltStore persists the catalog snapshot as one JSON document.
type BoltStore struct {
	db db.Database
}

// NewBoltStore creates the catalog bucket and returns a store over database.
func NewBoltStore(database db.Database) (*BoltStore, error) {
	if database == nil {
		return nil, fmt.Errorf("database is nil")
	}
	if err := database.GetOrCreateBucket(context.Background(), Bucket); err != nil {
		return nil, fmt.Errorf("create %s bucket: %w", Bucket, err)
	}
	return &BoltStore{db: database}, nil
}

// Load reads the snapshot. A missing key is db.ErrNotFound.
func (s *BoltStore) Load(ctx context.Context) (Snapshot, error) {
	raw, err := s.db.GetKV(ctx, Bucket, []byte(SnapshotKey))
	if err != nil {
		return Snapshot{}, err
	}
	if len(raw) == 0 {
		return Snapshot{}, db.ErrNotFound
	}
	var snap Snapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return Snapshot{}, fmt.Errorf("decode catalog snapshot: %w", err)
	}
	return snap, nil
}

// Save writes snap under the snapshot key. Other buckets are left alone,
// so a refresh cannot delete downloaded images.
func (s *BoltStore) Save(ctx context.Context, snap Snapshot) error {
	raw, err := json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("encode catalog snapshot: %w", err)
	}
	if err := s.db.PutKV(ctx, Bucket, []byte(SnapshotKey), raw); err != nil {
		return fmt.Errorf("save catalog snapshot: %w", err)
	}
	return nil
}

// IsNotFound reports whether err is a missing snapshot.
func IsNotFound(err error) bool {
	return errors.Is(err, db.ErrNotFound)
}
