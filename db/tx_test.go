package db

import (
	"context"
	"errors"
	"testing"

	"ignite/config"

	"github.com/stretchr/testify/require"
)

func TestBoltDB_UpdateRollsBack(t *testing.T) {
	cfg, err := config.NewConfigBuilder().
		WithDBPath(t.TempDir()).
		WithDBFile("tx.db").
		WithBucket("test").
		Build()
	require.NoError(t, err)
	database, err := NewBoltDB(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })

	ctx := context.Background()
	boom := errors.New("boom")
	err = database.Update(ctx, func(tx Tx) error {
		if err := tx.Put("letters", []byte("a"), []byte("1")); err != nil {
			return err
		}
		return boom
	})
	require.ErrorIs(t, err, boom)

	got, err := database.GetKV(ctx, "letters", []byte("a"))
	if err != nil {
		require.Contains(t, err.Error(), "not found")
		return
	}
	require.Nil(t, got, "a failed transaction must not leave the key behind")
}

func TestBoltDB_UpdateCommitsBothBuckets(t *testing.T) {
	cfg, err := config.NewConfigBuilder().
		WithDBPath(t.TempDir()).
		WithDBFile("tx.db").
		WithBucket("test").
		Build()
	require.NoError(t, err)
	database, err := NewBoltDB(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })

	ctx := context.Background()
	err = database.Update(ctx, func(tx Tx) error {
		if err := tx.Put("rows", []byte("id"), []byte("row")); err != nil {
			return err
		}
		return tx.Put("index", []byte("mac"), []byte("id"))
	})
	require.NoError(t, err)

	row, err := database.GetKV(ctx, "rows", []byte("id"))
	require.NoError(t, err)
	require.Equal(t, "row", string(row))
	id, err := database.GetKV(ctx, "index", []byte("mac"))
	require.NoError(t, err)
	require.Equal(t, "id", string(id))
}
