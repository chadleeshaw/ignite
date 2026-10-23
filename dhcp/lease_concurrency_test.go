package dhcp

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ignite/db"

	"github.com/stretchr/testify/require"
)

// gateRepo is an in-memory lease repository. When armed, the next GetByMAC
// blocks until release is closed, so a test can prove a mutation still holds
// the allocation lock across the read.
type gateRepo struct {
	mu      sync.Mutex
	leases  map[string]*Lease
	byMAC   map[string]string
	arm     atomic.Bool
	entered chan struct{}
	release chan struct{}
	unblock sync.Once
}

func newGateRepo() *gateRepo {
	return &gateRepo{
		leases:  map[string]*Lease{},
		byMAC:   map[string]string{},
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (r *gateRepo) open() {
	r.unblock.Do(func() { close(r.release) })
}

func (r *gateRepo) clone(lease *Lease) *Lease {
	if lease == nil {
		return nil
	}
	cp := *lease
	if lease.IP != nil {
		cp.IP = append(net.IP(nil), lease.IP...)
	}
	if lease.StateHistory != nil {
		cp.StateHistory = append([]StateTransition(nil), lease.StateHistory...)
	}
	return &cp
}

func (r *gateRepo) Save(ctx context.Context, lease *Lease) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.leases[lease.ID] = r.clone(lease)
	r.byMAC[lease.MAC] = lease.ID
	return nil
}

func (r *gateRepo) Get(ctx context.Context, id string) (*Lease, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	lease, ok := r.leases[id]
	if !ok {
		return nil, db.ErrNotFound
	}
	return r.clone(lease), nil
}

func (r *gateRepo) GetByMAC(ctx context.Context, mac string) (*Lease, error) {
	if r.arm.CompareAndSwap(true, false) {
		close(r.entered)
		<-r.release
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.byMAC[mac]
	if !ok {
		return nil, db.ErrNotFound
	}
	return r.clone(r.leases[id]), nil
}

func (r *gateRepo) GetByServerID(ctx context.Context, serverID string) ([]*Lease, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*Lease
	for _, lease := range r.leases {
		if lease.ServerID == serverID {
			out = append(out, r.clone(lease))
		}
	}
	return out, nil
}

func (r *gateRepo) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if lease, ok := r.leases[id]; ok {
		delete(r.byMAC, lease.MAC)
		delete(r.leases, id)
	}
	return nil
}

func (r *gateRepo) DeleteByMAC(ctx context.Context, mac string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.byMAC[mac]
	if !ok {
		return db.ErrNotFound
	}
	delete(r.leases, id)
	delete(r.byMAC, mac)
	return nil
}

func (r *gateRepo) DeleteByServerID(ctx context.Context, serverID string) error { return nil }
func (r *gateRepo) GetExpired(ctx context.Context) ([]*Lease, error)            { return nil, nil }
func (r *gateRepo) CleanupExpired(ctx context.Context) error                    { return nil }

type memServers struct {
	server *Server
}

func (m memServers) Save(ctx context.Context, server *Server) error { return nil }
func (m memServers) Get(ctx context.Context, id string) (*Server, error) {
	if m.server == nil || m.server.ID != id {
		return nil, db.ErrNotFound
	}
	return m.server, nil
}
func (m memServers) GetAll(ctx context.Context) ([]*Server, error) { return []*Server{m.server}, nil }
func (m memServers) Delete(ctx context.Context, id string) error   { return nil }
func (m memServers) GetByIP(ctx context.Context, ip net.IP) (*Server, error) {
	return nil, db.ErrNotFound
}

func TestUpdateLeaseStateDoesNotClobberRenewal(t *testing.T) {
	ctx := context.Background()
	repo := newGateRepo()
	server := &Server{
		ID:            "srv",
		IP:            net.ParseIP("192.168.1.1"),
		IPStart:       net.ParseIP("192.168.1.100"),
		LeaseRange:    20,
		LeaseDuration: 2 * time.Hour,
	}
	svc := NewDHCPLeaseService(repo, memServers{server: server})

	mac := "aa:bb:cc:dd:ee:10"
	ip := net.ParseIP("192.168.1.100")
	first, err := svc.AssignLease(ctx, server.ID, mac, ip)
	require.NoError(t, err)
	originalExpiry := first.Expiry

	repo.arm.Store(true)
	defer repo.open()
	stateDone := make(chan error, 1)
	go func() {
		stateDone <- svc.UpdateLeaseState(ctx, mac, string(StateFailed), "provision")
	}()

	select {
	case <-repo.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("state update did not reach its lease read")
	}

	renewDone := make(chan error, 1)
	go func() {
		_, err := svc.AssignLease(ctx, server.ID, mac, ip)
		renewDone <- err
	}()

	select {
	case err := <-renewDone:
		t.Fatalf("renewal finished while the state update was inside its read: %v", err)
	case <-time.After(150 * time.Millisecond):
		// The renewal is waiting on the allocation lock. Release the read.
	}

	repo.open()

	require.NoError(t, <-stateDone)
	require.NoError(t, <-renewDone)

	got, err := svc.GetLeaseByMAC(ctx, mac)
	require.NoError(t, err)
	require.Equal(t, StateFailed, got.State, "provisioning state must survive the renewal")
	require.True(t, got.Expiry.After(originalExpiry), "renewal expiry must survive the state update, got %s not after %s", got.Expiry, originalExpiry)
}

func TestUpdateLeaseKeepsNewerExpiry(t *testing.T) {
	ctx := context.Background()
	repo := newGateRepo()
	server := &Server{
		ID:            "srv",
		IP:            net.ParseIP("192.168.1.1"),
		IPStart:       net.ParseIP("192.168.1.100"),
		LeaseRange:    20,
		LeaseDuration: 2 * time.Hour,
	}
	svc := NewDHCPLeaseService(repo, memServers{server: server})

	mac := "aa:bb:cc:dd:ee:11"
	ip := net.ParseIP("192.168.1.110")
	created, err := svc.AssignLease(ctx, server.ID, mac, ip)
	require.NoError(t, err)

	stale, err := svc.GetLeaseByMAC(ctx, mac)
	require.NoError(t, err)
	stale.Menu.Hostname = "build-box"

	// A renewal lands after the caller took its snapshot.
	renewedExpiry := created.Expiry.Add(time.Hour)
	stored, err := repo.Get(ctx, created.ID)
	require.NoError(t, err)
	stored.Expiry = renewedExpiry
	require.NoError(t, repo.Save(ctx, stored))

	require.NoError(t, svc.UpdateLease(ctx, stale))

	got, err := svc.GetLeaseByMAC(ctx, mac)
	require.NoError(t, err)
	require.Equal(t, "build-box", got.Menu.Hostname)
	require.True(t, got.Expiry.Equal(renewedExpiry), "stale save rolled the renewal back to %s", got.Expiry)
}

func TestRecordHeartbeatDoesNotRollBackExpiry(t *testing.T) {
	ctx := context.Background()
	repo := newGateRepo()
	server := &Server{
		ID:            "srv",
		IP:            net.ParseIP("192.168.1.1"),
		IPStart:       net.ParseIP("192.168.1.100"),
		LeaseRange:    20,
		LeaseDuration: 2 * time.Hour,
	}
	svc := NewDHCPLeaseService(repo, memServers{server: server})

	mac := "aa:bb:cc:dd:ee:12"
	ip := net.ParseIP("192.168.1.120")
	created, err := svc.AssignLease(ctx, server.ID, mac, ip)
	require.NoError(t, err)

	repo.arm.Store(true)
	defer repo.open()
	beatDone := make(chan error, 1)
	go func() {
		beatDone <- svc.RecordHeartbeat(ctx, mac)
	}()
	select {
	case <-repo.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("heartbeat did not reach its lease read")
	}

	renewDone := make(chan error, 1)
	go func() {
		_, err := svc.AssignLease(ctx, server.ID, mac, ip)
		renewDone <- err
	}()
	select {
	case err := <-renewDone:
		t.Fatalf("renewal finished while the heartbeat was inside its read: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	repo.open()
	require.NoError(t, <-beatDone)
	require.NoError(t, <-renewDone)

	got, err := svc.GetLeaseByMAC(ctx, mac)
	require.NoError(t, err)
	require.True(t, got.Expiry.After(created.Expiry), "heartbeat saved a stale expiry")
	require.False(t, got.LastSeen.IsZero())
}
