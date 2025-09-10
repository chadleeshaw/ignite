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

	return nil, fmt.Errorf("server with IP %s not found", ip.String())
}

// BoltLeaseRepository implements LeaseRepository using BoltDB
type BoltLeaseRepository struct {
	repo *db.GenericRepository[*Lease]
}

// NewBoltLeaseRepository creates a new BoltDB lease repository
func NewBoltLeaseRepository(database db.Database, bucket string) *BoltLeaseRepository {
	return &BoltLeaseRepository{
		repo: db.NewGenericRepository[*Lease](database, bucket),
	}
}

// Save saves a lease to the repository
func (r *BoltLeaseRepository) Save(ctx context.Context, lease *Lease) error {
	return r.repo.Save(ctx, lease.ID, lease)
}

// Get retrieves a lease by ID
func (r *BoltLeaseRepository) Get(ctx context.Context, id string) (*Lease, error) {
	return r.repo.Get(ctx, id)
}

// GetByMAC retrieves a lease by MAC address
func (r *BoltLeaseRepository) GetByMAC(ctx context.Context, mac string) (*Lease, error) {
	leases, err := r.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	for _, lease := range leases {
		if lease.MAC == mac {
			return lease, nil
		}
	}

	return nil, fmt.Errorf("lease for MAC %s not found", mac)
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

// Delete removes a lease by ID
func (r *BoltLeaseRepository) Delete(ctx context.Context, id string) error {
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
