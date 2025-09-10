package db

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
)

// GenericRepository provides a generic repository implementation
type GenericRepository[T any] struct {
	db     Database
	bucket string
}

// NewGenericRepository creates a new generic repository
func NewGenericRepository[T any](db Database, bucket string) *GenericRepository[T] {
	return &GenericRepository[T]{
		db:     db,
		bucket: bucket,
	}
}

// Save stores an entity with the given key
func (r *GenericRepository[T]) Save(ctx context.Context, key string, entity T) error {
	data, err := json.Marshal(entity)
	if err != nil {
		return fmt.Errorf("failed to marshal entity: %w", err)
	}

	return r.db.PutKV(ctx, r.bucket, []byte(key), data)
}

// Get retrieves an entity by key
func (r *GenericRepository[T]) Get(ctx context.Context, key string) (T, error) {
	var entity T

	data, err := r.db.GetKV(ctx, r.bucket, []byte(key))
	if err != nil {
		return entity, fmt.Errorf("failed to get entity: %w", err)
	}

	if data == nil {
		return entity, fmt.Errorf("entity not found")
	}

	if err := json.Unmarshal(data, &entity); err != nil {
		return entity, fmt.Errorf("failed to unmarshal entity: %w", err)
	}

	return entity, nil
}

// GetAll retrieves all entities
func (r *GenericRepository[T]) GetAll(ctx context.Context) (map[string]T, error) {
	data, err := r.db.GetAllKV(ctx, r.bucket)
	if err != nil {
		return nil, fmt.Errorf("failed to get all entities: %w", err)
	}

	result := make(map[string]T)
	for key, value := range data {
		var entity T
		if err := json.Unmarshal(value, &entity); err != nil {
			log.Printf("Failed to unmarshal entity for key %s: %v", key, err)
			continue
		}
		result[key] = entity
	}

	return result, nil
}

// Delete removes an entity by key
func (r *GenericRepository[T]) Delete(ctx context.Context, key string) error {
	return r.db.DeleteKV(ctx, r.bucket, []byte(key))
}

// DeleteAll removes all entities
func (r *GenericRepository[T]) DeleteAll(ctx context.Context) error {
	return r.db.DeleteAllKV(ctx, r.bucket)
}
