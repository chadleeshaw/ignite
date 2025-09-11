package db

import (
	"context"
	"fmt"
	"ignite/config"
	"path/filepath"

	bolt "go.etcd.io/bbolt"
)

// BoltDB implements the Database interface
type BoltDB struct {
	*bolt.DB
	bucket string
}

// NewBoltDB creates a new BoltDB instance
func NewBoltDB(cfg *config.Config) (*BoltDB, error) {
	path := filepath.Join(cfg.DB.DBPath, cfg.DB.DBFile)
	db, err := bolt.Open(path, 0744, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	boltDB := &BoltDB{
		DB:     db,
		bucket: cfg.DB.Bucket,
	}

	// Ensure all required buckets exist
	requiredBuckets := []string{
		cfg.DB.Bucket,              // Base bucket
		cfg.DB.Bucket + "_servers", // DHCP servers bucket
		cfg.DB.Bucket + "_leases",  // DHCP leases bucket
	}

	for _, bucketName := range requiredBuckets {
		if err := boltDB.GetOrCreateBucket(context.Background(), bucketName); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to create bucket %s: %w", bucketName, err)
		}
	}

	return boltDB, nil
}

// GetOrCreateBucket creates a bucket if it doesn't exist
func (b *BoltDB) GetOrCreateBucket(ctx context.Context, name string) error {
	return b.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(name))
		return err
	})
}

// GetKV retrieves a value by key from the specified bucket
func (b *BoltDB) GetKV(ctx context.Context, bucket string, key []byte) ([]byte, error) {
	var value []byte
	err := b.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket([]byte(bucket))
		if bkt == nil {
			return fmt.Errorf("bucket %q not found", bucket)
		}

		data := bkt.Get(key)
		if data != nil {
			// Copy the data since it's only valid during the transaction
			value = make([]byte, len(data))
			copy(value, data)
		}
		return nil
	})
	return value, err
}

// PutKV stores a key-value pair in the specified bucket
func (b *BoltDB) PutKV(ctx context.Context, bucket string, key, value []byte) error {
	return b.Update(func(tx *bolt.Tx) error {
		bkt, err := tx.CreateBucketIfNotExists([]byte(bucket))
		if err != nil {
			return fmt.Errorf("create bucket: %w", err)
		}
		return bkt.Put(key, value)
	})
}

// DeleteKV removes a key-value pair from the specified bucket
func (b *BoltDB) DeleteKV(ctx context.Context, bucket string, key []byte) error {
	return b.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket([]byte(bucket))
		if bkt == nil {
			return fmt.Errorf("bucket %q not found", bucket)
		}
		return bkt.Delete(key)
	})
}

// GetAllKV retrieves all key-value pairs in the specified bucket
func (b *BoltDB) GetAllKV(ctx context.Context, bucket string) (map[string][]byte, error) {
	result := make(map[string][]byte)
	err := b.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket([]byte(bucket))
		if bkt == nil {
			// Return empty map if bucket doesn't exist
			return nil
		}

		return bkt.ForEach(func(k, v []byte) error {
			key := string(k)
			value := make([]byte, len(v))
			copy(value, v)
			result[key] = value
			return nil
		})
	})
	return result, err
}

// DeleteAllKV removes all key-value pairs from the specified bucket
func (b *BoltDB) DeleteAllKV(ctx context.Context, bucket string) error {
	return b.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket([]byte(bucket))
		if bkt == nil {
			return fmt.Errorf("bucket %q not found", bucket)
		}
		return bkt.ForEach(func(k, v []byte) error {
			return bkt.Delete(k)
		})
	})
}
