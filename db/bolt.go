package db

import (
	"fmt"
	"ignite/config"
	"log"
	"path/filepath"

	bolt "go.etcd.io/bbolt"
)

var KV *BoltKV

// BoltKV wraps bolt.DB to provide key-value operations with buckets.
type BoltKV struct {
	*bolt.DB
}

// Init initializes and returns a BoltDB instance, setting up the global KV pointer.
func Init() (*BoltKV, error) {
	conf := config.Defaults.DB
	path := filepath.Join(conf.DBPath, conf.DBFile)
	db, err := bolt.Open(path, 0744, nil)
	if err != nil {
		log.Fatalf("Fatal error opening database: %v", err)
	}

	KV = &BoltKV{db}
	_, err = KV.GetOrCreateBucket(conf.Bucket)
	if err != nil {
		return nil, fmt.Errorf("error with bucket: %v", err)
	}
	return KV, nil
}

// GetOrCreateBucket either creates a new bucket if it doesn't exist or returns the existing one.
func (b *BoltKV) GetOrCreateBucket(name string) (*bolt.Bucket, error) {
	var bucket *bolt.Bucket
	err := b.Update(func(tx *bolt.Tx) error {
		var err error
		bucket, err = tx.CreateBucketIfNotExists([]byte(name))
		return err
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get or create bucket %s: %v", name, err)
	}

	return bucket, nil
}

// GetKV retrieves the value associated with a key from a specified bucket.
func (b *BoltKV) GetKV(bucket string, key []byte) ([]byte, error) {
	var value []byte
	err := b.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket([]byte(bucket))
		if bkt == nil {
			return fmt.Errorf("bucket %q not found", bucket)
		}
		value = bkt.Get(key)
		return nil
	})
	return value, err
}

// PutKV stores a key-value pair in a specified bucket.
func (b *BoltKV) PutKV(bucket string, key, value []byte) error {
	return b.Update(func(tx *bolt.Tx) error {
		bkt, err := tx.CreateBucketIfNotExists([]byte(bucket))
		if err != nil {
			return fmt.Errorf("create bucket: %s", err)
		}
		return bkt.Put(key, value)
	})
}

// DeleteKV removes a key-value pair from a specified bucket.
func (b *BoltKV) DeleteKV(bucket string, key []byte) error {
	return b.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket([]byte(bucket))
		if bkt == nil {
			return fmt.Errorf("bucket %q not found", bucket)
		}
		return bkt.Delete(key)
	})
}

// GetAllKV retrieves all key-value pairs in a specified bucket.
func (b *BoltKV) GetAllKV(bucket string) (map[string][]byte, error) {
	result := make(map[string][]byte)
	err := b.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket([]byte(bucket))
		if bkt == nil {
			return fmt.Errorf("bucket %q not found", bucket)
		}

		return bkt.ForEach(func(k, v []byte) error {
			key := string(k)
			result[key] = append([]byte{}, v...)
			return nil
		})
	})
	return result, err
}

// DeleteAllKV removes all key-value pairs from a specified bucket.
func (b *BoltKV) DeleteAllKV(bucket string) error {
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
