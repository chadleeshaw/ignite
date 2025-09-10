package db

import (
	"context"
	"testing"
	"time"

	"ignite/config"

	"github.com/stretchr/testify/assert"
)

// TestBoltDB_Integration tests BoltDB with a real database file
func TestBoltDB_Integration(t *testing.T) {
	// Create test configuration
	cfg, err := config.NewConfigBuilder().
		WithDBPath("./testdata").
		WithDBFile("test.db").
		WithBucket("test").
		Build()

	assert.NoError(t, err)

	// Create database
	db, err := NewBoltDB(cfg)
	assert.NoError(t, err)
	defer db.Close()

	ctx := context.Background()
	bucket := cfg.DB.Bucket

	// Test PutKV and GetKV
	key := []byte("test-key")
	value := []byte("test-value")

	err = db.PutKV(ctx, bucket, key, value)
	assert.NoError(t, err)

	retrievedValue, err := db.GetKV(ctx, bucket, key)
	assert.NoError(t, err)
	assert.Equal(t, value, retrievedValue)

	// Test DeleteKV
	err = db.DeleteKV(ctx, bucket, key)
	assert.NoError(t, err)

	retrievedValue, err = db.GetKV(ctx, bucket, key)
	assert.NoError(t, err)
	assert.Nil(t, retrievedValue)
}

// TestGenericRepository tests the generic repository
func TestGenericRepository(t *testing.T) {
	// Create test configuration
	cfg, err := config.NewConfigBuilder().
		WithDBPath("./testdata").
		WithDBFile("test_repo.db").
		WithBucket("test").
		Build()

	assert.NoError(t, err)

	// Create database
	db, err := NewBoltDB(cfg)
	assert.NoError(t, err)
	defer db.Close()

	// Create repository
	type TestEntity struct {
		ID   string    `json:"id"`
		Name string    `json:"name"`
		Time time.Time `json:"time"`
	}

	repo := NewGenericRepository[*TestEntity](db, cfg.DB.Bucket)
	ctx := context.Background()

	// Test Save and Get
	entity := &TestEntity{
		ID:   "test-1",
		Name: "Test Entity",
		Time: time.Now(),
	}

	err = repo.Save(ctx, entity.ID, entity)
	assert.NoError(t, err)

	retrieved, err := repo.Get(ctx, entity.ID)
	assert.NoError(t, err)
	assert.Equal(t, entity.ID, retrieved.ID)
	assert.Equal(t, entity.Name, retrieved.Name)

	// Test GetAll
	entity2 := &TestEntity{
		ID:   "test-2",
		Name: "Test Entity 2",
		Time: time.Now(),
	}

	err = repo.Save(ctx, entity2.ID, entity2)
	assert.NoError(t, err)

	all, err := repo.GetAll(ctx)
	assert.NoError(t, err)
	assert.Len(t, all, 2)

	// Test Delete
	err = repo.Delete(ctx, entity.ID)
	assert.NoError(t, err)

	_, err = repo.Get(ctx, entity.ID)
	assert.Error(t, err)
}
