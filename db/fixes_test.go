package db

import (
	"context"
	"testing"

	"ignite/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestDB(t *testing.T, file string) Database {
	t.Helper()
	dir := t.TempDir()
	cfg, err := config.NewConfigBuilder().
		WithDBPath(dir).
		WithDBFile(file).
		WithBucket("test").
		Build()
	require.NoError(t, err)
	database, err := NewBoltDB(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	return database
}

// TestBoltDB_MissingBucketConsistent verifies that all KV operations agree:
// a missing bucket is an error, not an empty result.
func TestBoltDB_MissingBucketConsistent(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t, "missing.db")

	_, err := database.GetKV(ctx, "no-such-bucket", []byte("k"))
	assert.Error(t, err)

	_, err = database.GetAllKV(ctx, "no-such-bucket")
	assert.Error(t, err)

	assert.Error(t, database.DeleteKV(ctx, "no-such-bucket", []byte("k")))
	assert.Error(t, database.DeleteAllKV(ctx, "no-such-bucket"))
}

// TestBoltDB_DeleteAllKV verifies that deleting all keys works and does not
// mutate the bucket while iterating it.
func TestBoltDB_DeleteAllKV(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t, "deleteall.db")

	for _, k := range []string{"a", "b", "c", "d", "e"} {
		require.NoError(t, database.PutKV(ctx, "test", []byte(k), []byte("v-"+k)))
	}

	require.NoError(t, database.DeleteAllKV(ctx, "test"))

	all, err := database.GetAllKV(ctx, "test")
	require.NoError(t, err)
	assert.Empty(t, all)

	// Deleting again on the now-empty bucket still works.
	assert.NoError(t, database.DeleteAllKV(ctx, "test"))
}

// TestGenericRepository_GetNotFound verifies the typed not-found error.
func TestGenericRepository_GetNotFound(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t, "notfound.db")

	type Ent struct {
		ID string `json:"id"`
	}
	repo := NewGenericRepository[*Ent](database, "test")

	_, err := repo.Get(ctx, "missing")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

// TestGenericRepository_GetAllCorrupt verifies that a corrupt row fails the
// whole read instead of being silently skipped.
func TestGenericRepository_GetAllCorrupt(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t, "corrupt.db")

	type Ent struct {
		ID string `json:"id"`
	}
	repo := NewGenericRepository[*Ent](database, "test")

	require.NoError(t, repo.Save(ctx, "good", &Ent{ID: "good"}))
	require.NoError(t, database.PutKV(ctx, "test", []byte("bad"), []byte("{not json")))

	_, err := repo.GetAll(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"bad"`)
}
