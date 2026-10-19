// dhcp/repository.go - Repository implementations
package dhcp

import (
	"context"
	"encoding/json"
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

// Save saves a lease and its MAC index in one transaction. A failure rolls
// both writes back, so the index cannot point at a lease row that was not
// committed (and a MAC change cannot leave the old index entry behind).
func (r *BoltLeaseRepository) Save(ctx context.Context, lease *Lease) error {
	if lease == nil || lease.ID == "" {
		return fmt.Errorf("lease id is required")
	}
	if lease.MAC == "" {
		return fmt.Errorf("lease MAC is required")
	}
	data, err := json.Marshal(lease)
	if err != nil {
		return fmt.Errorf("failed to marshal lease: %w", err)
	}
	return r.db.Update(ctx, func(tx db.Tx) error {
		return writeLease(tx, r.bucket, r.macIndexBucket(), lease, data)
	})
}

// writeLease writes the lease row and the MAC index. When the stored MAC
// changed, the previous index entry is removed in the same transaction.
func writeLease(tx db.Tx, bucket, indexBucket string, lease *Lease, data []byte) error {
	existing, err := tx.Get(bucket, []byte(lease.ID))
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		var prev Lease
		if err := json.Unmarshal(existing, &prev); err == nil && prev.MAC != "" && prev.MAC != lease.MAC {
			if err := tx.Delete(indexBucket, []byte(prev.MAC)); err != nil {
				return err
			}
		}
	}
	if err := tx.Put(bucket, []byte(lease.ID), data); err != nil {
		return err
	}
	if err := tx.Put(indexBucket, []byte(lease.MAC), []byte(lease.ID)); err != nil {
		return fmt.Errorf("failed to update MAC index: %w", err)
	}
	return nil
}

// Get retrieves a lease by ID
func (r *BoltLeaseRepository) Get(ctx context.Context, id string) (*Lease, error) {
	return r.repo.Get(ctx, id)
}

// GetByMAC retrieves a lease by MAC address, using the secondary index.
// A canonical miss scans inside one write transaction and, when the stored
// spelling differs, rewrites the row and the index before returning. The
// repair is not a separate PutKV: it commits with the scan or not at all.
// Callers that already hold lockLease must not take it again here; the
// bbolt write transaction is the lock for the index update.
func (r *BoltLeaseRepository) GetByMAC(ctx context.Context, mac string) (*Lease, error) {
	canon, canonErr := CanonicalMAC(mac)
	keys := []string{mac}
	if canonErr == nil {
		keys = []string{canon}
		if canon != mac {
			keys = append(keys, mac)
		}
	}
	for _, key := range keys {
		if lease, ok := r.leaseByIndex(ctx, key, canon); ok {
			return lease, nil
		}
	}

	var found *Lease
	err := r.db.Update(ctx, func(tx db.Tx) error {
		for _, key := range keys {
			id, gerr := tx.Get(r.macIndexBucket(), []byte(key))
			if gerr != nil || len(id) == 0 {
				continue
			}
			raw, gerr := tx.Get(r.bucket, id)
			if gerr != nil || len(raw) == 0 {
				continue
			}
			var lease Lease
			if json.Unmarshal(raw, &lease) != nil {
				continue
			}
			if macMatches(&lease, mac, canon) && (canonErr != nil || lease.MAC == canon) {
				cp := lease
				found = &cp
				return nil
			}
		}

		var match *Lease
		err := tx.ForEach(r.bucket, func(k, v []byte) error {
			var lease Lease
			if json.Unmarshal(v, &lease) != nil {
				return nil
			}
			if !macMatches(&lease, mac, canon) {
				return nil
			}
			cp := lease
			match = &cp
			return db.ErrStopIteration
		})
		if err != nil {
			return err
		}
		if match == nil {
			return fmt.Errorf("%w: lease for MAC %s", db.ErrNotFound, mac)
		}
		// Rewrite after ForEach. bbolt forbids mutating the bucket being walked.
		if canonErr == nil && match.MAC != canon {
			match.MAC = canon
		}
		data, err := json.Marshal(match)
		if err != nil {
			return fmt.Errorf("failed to marshal lease: %w", err)
		}
		if err := writeLease(tx, r.bucket, r.macIndexBucket(), match, data); err != nil {
			return err
		}
		found = match
		return nil
	})
	if err != nil {
		return nil, err
	}
	if found == nil {
		return nil, fmt.Errorf("%w: lease for MAC %s", db.ErrNotFound, mac)
	}
	return found, nil
}

// leaseByIndex returns the lease when the index key points at a row whose
// MAC is already the canonical spelling. A legacy spelling falls through so
// the write transaction can repair it.
func (r *BoltLeaseRepository) leaseByIndex(ctx context.Context, key, canon string) (*Lease, bool) {
	id, err := r.db.GetKV(ctx, r.macIndexBucket(), []byte(key))
	if err != nil || len(id) == 0 {
		return nil, false
	}
	lease, err := r.Get(ctx, string(id))
	if err != nil || lease == nil || !macMatches(lease, key, canon) {
		return nil, false
	}
	if canon != "" && lease.MAC != canon {
		return nil, false
	}
	return lease, true
}

// macMatches reports whether lease.MAC is the requested spelling or the
// same 48-bit address under another conventional spelling.
func macMatches(lease *Lease, raw, canon string) bool {
	if lease == nil || lease.MAC == "" {
		return false
	}
	if lease.MAC == raw || (canon != "" && lease.MAC == canon) {
		return true
	}
	got, err := CanonicalMAC(lease.MAC)
	return err == nil && canon != "" && got == canon
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

// Delete removes a lease and its MAC index entry in one transaction.
func (r *BoltLeaseRepository) Delete(ctx context.Context, id string) error {
	return r.db.Update(ctx, func(tx db.Tx) error {
		existing, err := tx.Get(r.bucket, []byte(id))
		if err != nil {
			return err
		}
		if len(existing) > 0 {
			var prev Lease
			if json.Unmarshal(existing, &prev) == nil && prev.MAC != "" {
				if err := tx.Delete(r.macIndexBucket(), []byte(prev.MAC)); err != nil {
					return err
				}
			}
		}
		return tx.Delete(r.bucket, []byte(id))
	})
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
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := r.Delete(ctx, lease.ID); err != nil {
			return fmt.Errorf("failed to delete expired lease %s: %w", lease.ID, err)
		}
	}

	return nil
}
