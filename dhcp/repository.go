// dhcp/repository.go - Repository implementations
package dhcp

import (
	"context"
	"fmt"
	"net"
	"time"

	"ignite/db"
)

// BoltServerRepository implements ServerRepository using BoltDB
type BoltServerRepository struct {
	repo *db.GenericRepository[*Server]
}

// NewBoltServerRepository creates a new BoltDB server repository
func NewBoltServerRepository(database db.Database, bucket string) *BoltServerRepository {
	return &BoltServerRepository{
		repo: db.NewGenericRepository[*Server](database, bucket),
	}
}

// Save saves a server to the repository
func (r *BoltServerRepository) Save(ctx context.Context, server *Server) error {
	server.UpdatedAt = time.Now()
	return r.repo.Save(ctx, server.ID, server)
}

// Get retrieves a server by ID
func (r *BoltServerRepository) Get(ctx context.Context, id string) (*Server, error) {
	return r.repo.Get(ctx, id)
}

// GetAll retrieves all servers
func (r *BoltServerRepository) GetAll(ctx context.Context) ([]*Server, error) {
	serverMap, err := r.repo.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	servers := make([]*Server, 0, len(serverMap))
	for _, server := range serverMap {
		servers = append(servers, server)
	}

	return servers, nil
}

// Delete removes a server from the repository
func (r *BoltServerRepository) Delete(ctx context.Context, id string) error {
	return r.repo.Delete(ctx, id)
}

// GetByIP retrieves a server by its IP address
func (r *BoltServerRepository) GetByIP(ctx context.Context, ip net.IP) (*Server, error) {
	servers, err := r.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	for _, server := range servers {
		if server.IP.Equal(ip) {
			return server, nil
		}
	}

	return nil, fmt.Errorf("%w: server with IP %s", db.ErrNotFound, ip.String())
}

// BoltLeaseRepository implements LeaseRepository using BoltDB
type BoltLeaseRepository struct {
	repo   *db.GenericRepository[*Lease]
	db     db.Database
	bucket string
}

// NewBoltLeaseRepository creates a new BoltDB lease repository
func NewBoltLeaseRepository(database db.Database, bucket string) *BoltLeaseRepository {
	return &BoltLeaseRepository{
		repo:   db.NewGenericRepository[*Lease](database, bucket),
		db:     database,
		bucket: bucket,
	}
}

// macIndexBucket returns the bucket holding the MAC -> lease ID secondary index.
// It is created on demand by PutKV, so readers must tolerate it being absent.
func (r *BoltLeaseRepository) macIndexBucket() string {
	return r.bucket + "_by_mac"
}

// Save saves a lease to the repository and maintains the MAC index
func (r *BoltLeaseRepository) Save(ctx context.Context, lease *Lease) error {
	// Drop any stale index entry if the lease's MAC changed since it was stored.
	if existing, err := r.Get(ctx, lease.ID); err == nil && existing != nil && existing.MAC != lease.MAC {
		_ = r.db.DeleteKV(ctx, r.macIndexBucket(), []byte(existing.MAC))
	}
	if err := r.repo.Save(ctx, lease.ID, lease); err != nil {
		return err
	}
	if err := r.db.PutKV(ctx, r.macIndexBucket(), []byte(lease.MAC), []byte(lease.ID)); err != nil {
		return fmt.Errorf("failed to update MAC index: %w", err)
	}
	return nil
}

// Get retrieves a lease by ID
func (r *BoltLeaseRepository) Get(ctx context.Context, id string) (*Lease, error) {
	return r.repo.Get(ctx, id)
}

// GetByMAC retrieves a lease by MAC address, using the secondary index.
// Falls back to a full scan (which repairs the index) when the index
// misses — e.g. for leases written before the index existed.
func (r *BoltLeaseRepository) GetByMAC(ctx context.Context, mac string) (*Lease, error) {
	if id, err := r.db.GetKV(ctx, r.macIndexBucket(), []byte(mac)); err == nil && len(id) > 0 {
		if lease, gerr := r.Get(ctx, string(id)); gerr == nil && lease != nil && lease.MAC == mac {
			return lease, nil
		}
		// Stale index entry: fall through to the full scan below.
	}

	leases, err := r.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	for _, lease := range leases {
		if lease.MAC == mac {
			// Repair the index for next time (best effort).
			_ = r.db.PutKV(ctx, r.macIndexBucket(), []byte(mac), []byte(lease.ID))
			return lease, nil
		}
	}

	return nil, fmt.Errorf("%w: lease for MAC %s", db.ErrNotFound, mac)
}

// GetByServerID retrieves all leases for a specific server
func (r *BoltLeaseRepository) GetByServerID(ctx context.Context, serverID string) ([]*Lease, error) {
	allLeases, err := r.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	var serverLeases []*Lease
	for _, lease := range allLeases {
		if lease.ServerID == serverID {
			serverLeases = append(serverLeases, lease)
		}
	}

	return serverLeases, nil
}

// GetAll retrieves all leases
func (r *BoltLeaseRepository) GetAll(ctx context.Context) ([]*Lease, error) {
	leaseMap, err := r.repo.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	leases := make([]*Lease, 0, len(leaseMap))
	for _, lease := range leaseMap {
		leases = append(leases, lease)
	}

	return leases, nil
}

// Delete removes a lease by ID and cleans up its MAC index entry
func (r *BoltLeaseRepository) Delete(ctx context.Context, id string) error {
	if existing, err := r.Get(ctx, id); err == nil && existing != nil {
		_ = r.db.DeleteKV(ctx, r.macIndexBucket(), []byte(existing.MAC))
	}
	return r.repo.Delete(ctx, id)
}

// DeleteByMAC removes a lease by MAC address
func (r *BoltLeaseRepository) DeleteByMAC(ctx context.Context, mac string) error {
	lease, err := r.GetByMAC(ctx, mac)
	if err != nil {
		return err
	}

	return r.Delete(ctx, lease.ID)
}

// DeleteByServerID removes all leases for a specific server
func (r *BoltLeaseRepository) DeleteByServerID(ctx context.Context, serverID string) error {
	leases, err := r.GetByServerID(ctx, serverID)
	if err != nil {
		return err
	}

	for _, lease := range leases {
		if err := r.Delete(ctx, lease.ID); err != nil {
			return fmt.Errorf("failed to delete lease %s: %w", lease.ID, err)
		}
	}

	return nil
}

// GetExpired retrieves all expired leases
func (r *BoltLeaseRepository) GetExpired(ctx context.Context) ([]*Lease, error) {
	allLeases, err := r.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	var expiredLeases []*Lease
	now := time.Now()

	for _, lease := range allLeases {
		if now.After(lease.Expiry) && !lease.Reserved {
			expiredLeases = append(expiredLeases, lease)
		}
	}

	return expiredLeases, nil
}

// CleanupExpired removes all expired leases
func (r *BoltLeaseRepository) CleanupExpired(ctx context.Context) error {
	expiredLeases, err := r.GetExpired(ctx)
	if err != nil {
		return fmt.Errorf("failed to get expired leases: %w", err)
	}

	for _, lease := range expiredLeases {
		if err := r.Delete(ctx, lease.ID); err != nil {
			return fmt.Errorf("failed to delete expired lease %s: %w", lease.ID, err)
		}
	}

	return nil
}
