package db

import (
	"context"
)

// Database defines the interface for database operations
type Database interface {
	Close() error
	GetKV(ctx context.Context, bucket string, key []byte) ([]byte, error)
	PutKV(ctx context.Context, bucket string, key, value []byte) error
	DeleteKV(ctx context.Context, bucket string, key []byte) error
	GetAllKV(ctx context.Context, bucket string) (map[string][]byte, error)
	DeleteAllKV(ctx context.Context, bucket string) error
	GetOrCreateBucket(ctx context.Context, name string) error
}

// Repository provides a generic repository interface
type Repository[T any] interface {
	Save(ctx context.Context, key string, entity T) error
	Get(ctx context.Context, key string) (T, error)
	GetAll(ctx context.Context) (map[string]T, error)
	Delete(ctx context.Context, key string) error
	DeleteAll(ctx context.Context) error
}
