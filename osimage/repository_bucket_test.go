package osimage

import (
	"context"
	"errors"
	"testing"

	"ignite/config"
	"ignite/db"

	"github.com/stretchr/testify/require"
)

type failBucketDB struct {
	db.Database
}

func (failBucketDB) GetOrCreateBucket(context.Context, string) error {
	return errors.New("disk full")
}

func TestNewOSImageRepositoryBucketFailure(t *testing.T) {
	repo, err := NewOSImageRepository(failBucketDB{})
	require.Nil(t, repo)
	require.Error(t, err)
	require.ErrorContains(t, err, "osimages")
	require.ErrorContains(t, err, "disk full")
}

func TestNewDownloadStatusRepositoryBucketFailure(t *testing.T) {
	repo, err := NewDownloadStatusRepository(failBucketDB{})
	require.Nil(t, repo)
	require.Error(t, err)
	require.ErrorContains(t, err, "download_status")
	require.ErrorContains(t, err, "disk full")
}

func TestNewOSImageRepositoryCreatesBucket(t *testing.T) {
	cfg, err := config.NewConfigBuilder().
		WithDBPath(t.TempDir()).
		WithDBFile("osimages.db").
		WithBucket("test").
		Build()
	require.NoError(t, err)
	database, err := db.NewBoltDB(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })

	repo, err := NewOSImageRepository(database)
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, repo.Save(ctx, &OSImage{
		OS:           "ubuntu",
		Version:      "24.04",
		Architecture: "x86_64",
	}))
	all, err := repo.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)
}
