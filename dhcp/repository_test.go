package dhcp

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"ignite/config"
	"ignite/db"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestLeaseRepo(t *testing.T) (*BoltLeaseRepository, db.Database) {
	t.Helper()
	cfg, err := config.NewConfigBuilder().
		WithDBPath(t.TempDir()).
		WithDBFile("leases.db").
		WithBucket("test").
		Build()
	require.NoError(t, err)
	database, err := db.NewBoltDB(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	return NewBoltLeaseRepository(database, "leases"), database
}

func testLease(id, mac, ip string) *Lease {
	now := time.Now()
	return &Lease{
		ID:             id,
		MAC:            mac,
		IP:             net.ParseIP(ip).To4(),
		ServerID:       "server-1",
		Expiry:         now.Add(time.Hour),
		State:          StateAssigned,
		StateUpdatedAt: now,
		LastSeen:       now,
		StateHistory: []StateTransition{
			{ToState: StateAssigned, Timestamp: now, Source: "dhcp"},
		},
	}
}

// TestBoltLeaseRepository_MACIndex verifies the MAC -> ID secondary index:
// lookups hit the index, deletes maintain it, and a missing index (old DB)
// falls back to a full scan that repairs the index.
func TestBoltLeaseRepository_MACIndex(t *testing.T) {
	ctx := context.Background()
	repo, database := newTestLeaseRepo(t)

	l1 := testLease("id-1", "aa:bb:cc:dd:ee:01", "192.168.1.101")
	l2 := testLease("id-2", "aa:bb:cc:dd:ee:02", "192.168.1.102")
	require.NoError(t, repo.Save(ctx, l1))
	require.NoError(t, repo.Save(ctx, l2))

	// Index hit
	got, err := repo.GetByMAC(ctx, l1.MAC)
	require.NoError(t, err)
	assert.Equal(t, l1.ID, got.ID)

	// Missing MAC -> typed not-found
	_, err = repo.GetByMAC(ctx, "aa:bb:cc:dd:ee:ff")
	require.Error(t, err)
	assert.True(t, errors.Is(err, db.ErrNotFound))

	// Delete maintains the index: no stale entry afterwards
	require.NoError(t, repo.Delete(ctx, l1.ID))
	_, err = repo.GetByMAC(ctx, l1.MAC)
	assert.True(t, errors.Is(err, db.ErrNotFound))

	// Simulate a database written before the index existed: drop the index
	// bucket entirely. GetByMAC must fall back to a scan and repair the index.
	require.NoError(t, database.DeleteAllKV(ctx, "leases_by_mac"))
	got, err = repo.GetByMAC(ctx, l2.MAC)
	require.NoError(t, err)
	assert.Equal(t, l2.ID, got.ID)

	// The index was repaired: the bucket exists again with the entry.
	id, err := database.GetKV(ctx, "leases_by_mac", []byte(l2.MAC))
	require.NoError(t, err)
	assert.Equal(t, l2.ID, string(id))
}
