package db

import (
	"context"
	"errors"
)

// ErrNotFound is returned when a requested entity does not exist.
// Use errors.Is to distinguish it from real backend failures.
var ErrNotFound = errors.New("entity not found")

// ErrStopIteration stops Tx.ForEach without failing or rolling back the
// surrounding transaction.
var ErrStopIteration = errors.New("stop iteration")

// Tx is a single read-write transaction. Values returned by Get are copies
// and stay valid after the transaction ends. Put creates a missing bucket.
// Delete of a missing bucket or key is a no-op.
type Tx interface {
	Get(bucket string, key []byte) ([]byte, error)
	Put(bucket string, key, value []byte) error
	Delete(bucket string, key []byte) error
	// ForEach visits every key in bucket. A missing bucket is an empty
	// range, not an error. Returning ErrStopIteration from fn stops the
	// walk and ForEach itself returns nil.
	ForEach(bucket string, fn func(k, v []byte) error) error
}

// Database defines the interface for database operations
type Database interface {
	Close() error
	GetKV(ctx context.Context, bucket string, key []byte) ([]byte, error)
	PutKV(ctx context.Context, bucket string, key, value []byte) error
	DeleteKV(ctx context.Context, bucket string, key []byte) error
	GetAllKV(ctx context.Context, bucket string) (map[string][]byte, error)
	DeleteAllKV(ctx context.Context, bucket string) error
	GetOrCreateBucket(ctx context.Context, name string) error
	// Update runs fn inside one read-write transaction. Returning an error
	// from fn rolls the whole transaction back.
	Update(ctx context.Context, fn func(Tx) error) error
}

// Repository provides a generic repository interface
type Repository[T any] interface {
	Save(ctx context.Context, key string, entity T) error
	Get(ctx context.Context, key string) (T, error)
	GetAll(ctx context.Context) (map[string]T, error)
	Delete(ctx context.Context, key string) error
	DeleteAll(ctx context.Context) error
}
