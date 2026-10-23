package dhcp

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"ignite/db"

	"github.com/stretchr/testify/require"
)

func reviewRepo(t *testing.T) (*BoltLeaseRepository, db.Database) {
	t.Helper()
	repo, database := newTestLeaseRepo(t)
	// The test database creates test_leases, not the repository bucket.
	require.NoError(t, database.GetOrCreateBucket(context.Background(), "leases"))
	return repo, database
}

func reviewServer(duration time.Duration) *Server {
	return &Server{
		ID:            "srv",
		IP:            net.ParseIP("10.1.0.1").To4(),
		IPStart:       net.ParseIP("10.1.0.10").To4(),
		LeaseRange:    20,
		LeaseDuration: duration,
	}
}

func TestGetByMACRepairsLegacySpelling(t *testing.T) {
	ctx := context.Background()
	repo, database := reviewRepo(t)

	upper := testLease("id-upper", "AA:BB:CC:DD:EE:10", "10.1.0.10")
	hyphen := testLease("id-hyphen", "AA-BB-CC-DD-EE-11", "10.1.0.11")
	require.NoError(t, repo.Save(ctx, upper))
	require.NoError(t, repo.Save(ctx, hyphen))

	got, err := repo.GetByMAC(ctx, "aa:bb:cc:dd:ee:10")
	require.NoError(t, err)
	require.Equal(t, "aa:bb:cc:dd:ee:10", got.MAC)
	require.Equal(t, upper.ID, got.ID)

	old, err := database.GetKV(ctx, "leases_by_mac", []byte("AA:BB:CC:DD:EE:10"))
	require.NoError(t, err)
	require.Empty(t, old)
	indexed, err := database.GetKV(ctx, "leases_by_mac", []byte("aa:bb:cc:dd:ee:10"))
	require.NoError(t, err)
	require.Equal(t, upper.ID, string(indexed))

	got, err = repo.GetByMAC(ctx, "aa:bb:cc:dd:ee:11")
	require.NoError(t, err)
	require.Equal(t, "aa:bb:cc:dd:ee:11", got.MAC)
	require.Equal(t, hyphen.ID, got.ID)
}

func TestAssignLeaseFindsLegacyMAC(t *testing.T) {
	ctx := context.Background()
	repo, _ := reviewRepo(t)
	server := reviewServer(2 * time.Hour)
	legacy := testLease("id-legacy", "AA:BB:CC:DD:EE:10", "10.1.0.10")
	legacy.ServerID = server.ID
	legacy.Expiry = time.Now().Add(time.Hour)
	require.NoError(t, repo.Save(ctx, legacy))

	svc := NewDHCPLeaseService(repo, memServers{server: server})
	got, err := svc.AssignLease(ctx, server.ID, "aa:bb:cc:dd:ee:10", net.ParseIP("10.1.0.10"))
	require.NoError(t, err)
	require.Equal(t, legacy.ID, got.ID)
	require.True(t, got.IP.Equal(net.ParseIP("10.1.0.10")))

	all, err := repo.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)
}

func TestReleaseLeaseCanonicalizesMAC(t *testing.T) {
	ctx := context.Background()
	repo, _ := reviewRepo(t)
	server := reviewServer(2 * time.Hour)
	legacy := testLease("id-rel", "AA:BB:CC:DD:EE:12", "10.1.0.12")
	legacy.ServerID = server.ID
	require.NoError(t, repo.Save(ctx, legacy))

	svc := NewDHCPLeaseService(repo, memServers{server: server})
	require.NoError(t, svc.ReleaseLease(ctx, "aa:bb:cc:dd:ee:12"))
	_, err := repo.GetByMAC(ctx, "aa:bb:cc:dd:ee:12")
	require.ErrorIs(t, err, db.ErrNotFound)
}

func TestUpdateLeaseDoesNotRecreateDeletedRow(t *testing.T) {
	ctx := context.Background()
	repo, _ := reviewRepo(t)
	server := reviewServer(2 * time.Hour)
	svc := NewDHCPLeaseService(repo, memServers{server: server})

	lease, err := svc.AssignLease(ctx, server.ID, "aa:bb:cc:dd:ee:13", net.ParseIP("10.1.0.13"))
	require.NoError(t, err)
	require.NoError(t, repo.Delete(ctx, lease.ID))

	err = svc.UpdateLease(ctx, lease)
	require.ErrorIs(t, err, db.ErrNotFound)
	_, err = repo.Get(ctx, lease.ID)
	require.ErrorIs(t, err, db.ErrNotFound)
}

func TestMarkOfflineSkipsMidInstallAndQuietRenew(t *testing.T) {
	ctx := context.Background()
	repo, _ := reviewRepo(t)
	server := reviewServer(2 * time.Hour)
	svc := NewDHCPLeaseService(repo, memServers{server: server})

	imaging, err := svc.AssignLease(ctx, server.ID, "aa:bb:cc:dd:ee:14", net.ParseIP("10.1.0.14"))
	require.NoError(t, err)
	imaging, err = repo.Get(ctx, imaging.ID)
	require.NoError(t, err)
	imaging.State = StateImaging
	imaging.LastSeen = time.Now().Add(-30 * time.Minute)
	imaging.LastHeartbeat = imaging.LastSeen
	require.NoError(t, repo.Save(ctx, imaging))

	renewed, err := svc.AssignLease(ctx, server.ID, "aa:bb:cc:dd:ee:15", net.ParseIP("10.1.0.15"))
	require.NoError(t, err)
	renewed, err = repo.Get(ctx, renewed.ID)
	require.NoError(t, err)
	renewed.LastHeartbeat = time.Now().Add(-3 * time.Hour)
	renewed.LastSeen = time.Now().Add(-20 * time.Minute)
	require.NoError(t, repo.Save(ctx, renewed))

	quiet, err := svc.AssignLease(ctx, server.ID, "aa:bb:cc:dd:ee:16", net.ParseIP("10.1.0.16"))
	require.NoError(t, err)
	quiet, err = repo.Get(ctx, quiet.ID)
	require.NoError(t, err)
	quiet.LastHeartbeat = time.Time{}
	quiet.LastSeen = time.Now().Add(-30 * time.Minute)
	require.NoError(t, repo.Save(ctx, quiet))

	require.NoError(t, svc.MarkOfflineLeases(ctx, 10*time.Minute))

	got, err := repo.Get(ctx, imaging.ID)
	require.NoError(t, err)
	require.Equal(t, StateImaging, got.State)
	got, err = repo.Get(ctx, renewed.ID)
	require.NoError(t, err)
	require.Equal(t, StateAssigned, got.State)
	got, err = repo.Get(ctx, quiet.ID)
	require.NoError(t, err)
	require.Equal(t, StateAssigned, got.State)
}

func TestMarkOfflineAfterHeartbeatGap(t *testing.T) {
	ctx := context.Background()
	repo, _ := reviewRepo(t)
	server := reviewServer(2 * time.Hour)
	svc := NewDHCPLeaseService(repo, memServers{server: server})

	lease, err := svc.AssignLease(ctx, server.ID, "aa:bb:cc:dd:ee:17", net.ParseIP("10.1.0.17"))
	require.NoError(t, err)
	lease, err = repo.Get(ctx, lease.ID)
	require.NoError(t, err)
	lease.LastHeartbeat = time.Now().Add(-3 * time.Hour)
	lease.LastSeen = time.Now().Add(-90 * time.Minute)
	require.NoError(t, repo.Save(ctx, lease))

	require.NoError(t, svc.MarkOfflineLeases(ctx, 10*time.Minute))
	got, err := repo.Get(ctx, lease.ID)
	require.NoError(t, err)
	require.Equal(t, StateOffline, got.State)
}

func TestHeartbeatRestoresLastStage(t *testing.T) {
	ctx := context.Background()
	repo, _ := reviewRepo(t)
	server := reviewServer(2 * time.Hour)
	svc := NewDHCPLeaseService(repo, memServers{server: server})
	mac := "aa:bb:cc:dd:ee:18"

	_, err := svc.AssignLease(ctx, server.ID, mac, net.ParseIP("10.1.0.18"))
	require.NoError(t, err)
	require.NoError(t, svc.UpdateLeaseState(ctx, mac, string(StateImaging), "kickstart"))
	require.NoError(t, svc.UpdateLeaseState(ctx, mac, string(StateOffline), "timeout"))

	require.NoError(t, svc.RecordHeartbeat(ctx, mac))
	got, err := repo.GetByMAC(ctx, mac)
	require.NoError(t, err)
	require.Equal(t, StateImaging, got.State)
	require.False(t, got.LastHeartbeat.IsZero())
}

func TestRenewClearsOffline(t *testing.T) {
	ctx := context.Background()
	repo, _ := reviewRepo(t)
	server := reviewServer(2 * time.Hour)
	svc := NewDHCPLeaseService(repo, memServers{server: server})
	mac := "aa:bb:cc:dd:ee:19"

	_, err := svc.AssignLease(ctx, server.ID, mac, net.ParseIP("10.1.0.19"))
	require.NoError(t, err)
	require.NoError(t, svc.UpdateLeaseState(ctx, mac, string(StateImaging), "kickstart"))
	require.NoError(t, svc.UpdateLeaseState(ctx, mac, string(StateOffline), "timeout"))

	got, err := svc.AssignLease(ctx, server.ID, mac, net.ParseIP("10.1.0.19"))
	require.NoError(t, err)
	require.Equal(t, StateImaging, got.State)
	require.False(t, got.LastSeen.IsZero())
}

// renewOnReadRepo makes the re-read inside MarkOfflineLeases look like a
// DHCP renew landed after the scan and before the write.
type renewOnReadRepo struct {
	LeaseRepository
	renewMAC string
	once     sync.Once
}

func (r *renewOnReadRepo) GetByMAC(ctx context.Context, mac string) (*Lease, error) {
	lease, err := r.LeaseRepository.GetByMAC(ctx, mac)
	if err != nil || lease == nil || lease.MAC != r.renewMAC {
		return lease, err
	}
	r.once.Do(func() {
		lease.LastSeen = time.Now()
		_ = r.LeaseRepository.Save(ctx, lease)
	})
	return lease, nil
}

func TestMarkOfflineRechecksUnderLock(t *testing.T) {
	ctx := context.Background()
	repo, _ := reviewRepo(t)
	server := reviewServer(2 * time.Hour)
	svc := NewDHCPLeaseService(repo, memServers{server: server})
	mac := "aa:bb:cc:dd:ee:20"

	lease, err := svc.AssignLease(ctx, server.ID, mac, net.ParseIP("10.1.0.20"))
	require.NoError(t, err)
	lease, err = repo.Get(ctx, lease.ID)
	require.NoError(t, err)
	lease.LastHeartbeat = time.Now().Add(-3 * time.Hour)
	lease.LastSeen = time.Now().Add(-90 * time.Minute)
	require.NoError(t, repo.Save(ctx, lease))

	wrapped := NewDHCPLeaseService(&renewOnReadRepo{LeaseRepository: repo, renewMAC: mac}, memServers{server: server})
	require.NoError(t, wrapped.MarkOfflineLeases(ctx, 10*time.Minute))
	got, err := repo.Get(ctx, lease.ID)
	require.NoError(t, err)
	require.Equal(t, StateAssigned, got.State)
}

func TestLockLeaseHonorsContext(t *testing.T) {
	mac := "aa:bb:cc:dd:ee:21"
	unlock, err := lockLease(context.Background(), mac)
	require.NoError(t, err)
	released := false
	defer func() {
		if !released {
			unlock()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = lockLease(ctx, mac)
	require.ErrorIs(t, err, context.DeadlineExceeded)

	unlock()
	released = true
	unlock, err = lockLease(context.Background(), mac)
	require.NoError(t, err)
	unlock()
}

func TestAssignOtherMACWhileLeaseLocked(t *testing.T) {
	ctx := context.Background()
	repo, _ := reviewRepo(t)
	server := reviewServer(2 * time.Hour)
	svc := NewDHCPLeaseService(repo, memServers{server: server})

	held := "aa:bb:cc:dd:ee:30"
	unlock, err := lockLease(ctx, held)
	require.NoError(t, err)
	defer unlock()

	done := make(chan error, 1)
	go func() {
		_, err := svc.AssignLease(ctx, server.ID, "aa:bb:cc:dd:ee:31", net.ParseIP("10.1.0.31"))
		done <- err
	}()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("assign of a different MAC blocked on another lease lock")
	}
}

func TestMarkOfflineStopsWhenContextCancelled(t *testing.T) {
	repo, _ := reviewRepo(t)
	svc := NewDHCPLeaseService(repo, memServers{server: reviewServer(time.Hour)})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := svc.MarkOfflineLeases(ctx, time.Minute)
	require.ErrorIs(t, err, context.Canceled)
}

func TestCleanupStopsWhenContextCancelled(t *testing.T) {
	repo, _ := reviewRepo(t)
	svc := NewDHCPLeaseService(repo, memServers{server: reviewServer(time.Hour)})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := svc.CleanupExpiredLeases(ctx)
	require.ErrorIs(t, err, context.Canceled)
}

func TestCleanupExpiredHonorsContextBeforeDelete(t *testing.T) {
	ctx := context.Background()
	repo, _ := reviewRepo(t)
	lease := testLease("id-expired", "aa:bb:cc:dd:ee:22", "10.1.0.22")
	lease.Expiry = time.Now().Add(-time.Minute)
	require.NoError(t, repo.Save(ctx, lease))

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	err := repo.CleanupExpired(cancelled)
	require.ErrorIs(t, err, context.Canceled)
	_, err = repo.Get(ctx, lease.ID)
	require.NoError(t, err)
}

func TestDeleteServerTakesLeaseLock(t *testing.T) {
	ctx := context.Background()
	repo, _ := reviewRepo(t)
	server := reviewServer(2 * time.Hour)
	leases := NewDHCPLeaseService(repo, memServers{server: server})
	servers := NewDHCPServerService(memServers{server: server}, repo)
	mac := "aa:bb:cc:dd:ee:40"

	lease, err := leases.AssignLease(ctx, server.ID, mac, net.ParseIP("10.1.0.40"))
	require.NoError(t, err)

	unlock, err := lockLease(ctx, mac)
	require.NoError(t, err)
	released := false
	defer func() {
		if !released {
			unlock()
		}
	}()

	done := make(chan error, 1)
	go func() {
		done <- servers.DeleteServer(ctx, server.ID)
	}()
	select {
	case err := <-done:
		t.Fatalf("DeleteServer finished while the lease lock was held: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	unlock()
	released = true
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("DeleteServer did not finish after the lease lock was released")
	}
	_, err = repo.Get(ctx, lease.ID)
	require.ErrorIs(t, err, db.ErrNotFound)
}
