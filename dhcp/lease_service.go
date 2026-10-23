package dhcp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"ignite/db"

	"github.com/google/uuid"
)

// DHCPLeaseService implements the LeaseService interface
type DHCPLeaseService struct {
	leaseRepo  LeaseRepository
	serverRepo ServerRepository
	// allocMu serializes IP allocation (scan -> select -> save) across the
	// service path and the DHCP packet path (ProtocolHandler).
	allocMu *sync.Mutex
}

// NewDHCPLeaseService creates a new lease service
func NewDHCPLeaseService(leaseRepo LeaseRepository, serverRepo ServerRepository) *DHCPLeaseService {
	return &DHCPLeaseService{
		leaseRepo:  leaseRepo,
		serverRepo: serverRepo,
		allocMu:    &sharedAllocMu,
	}
}

// AssignLease assigns an IP lease to a MAC address
func (s *DHCPLeaseService) AssignLease(ctx context.Context, serverID string, mac string, requestedIP net.IP) (*Lease, error) {
	// Hold the shared allocation lock across scan -> select -> save so the
	// packet path (ProtocolHandler) cannot interleave an allocation.
	s.allocMu.Lock()
	defer s.allocMu.Unlock()

	if canon, err := CanonicalMAC(mac); err == nil {
		mac = canon
	}

	server, err := s.serverRepo.Get(ctx, serverID)
	if err != nil {
		return nil, fmt.Errorf("failed to get server: %w", err)
	}

	unlock, err := lockLease(ctx, mac)
	if err != nil {
		return nil, err
	}
	defer unlock()

	// Check if MAC already has a lease
	existingLease, err := s.leaseByMAC(ctx, mac)
	if err == nil {
		// An existing lease may only be extended when it belongs to the
		// requested server; a lease from another server is not ours to renew.
		if existingLease.ServerID != serverID {
			return nil, fmt.Errorf("MAC %s already has a lease on another server", mac)
		}
		if !existingLease.IsExpired() {
			// Extend existing lease if not expired. A renew is activity:
			// it clears offline and restores the previous stage. Keep the
			// host token; fill one in when an older row lacks it.
			existingLease.MAC = mac
			existingLease.Extend(server.LeaseDuration)
			existingLease.NoteActivity("dhcp")
			if err := ensureProvisionToken(existingLease); err != nil {
				return nil, err
			}
			if err := s.leaseRepo.Save(ctx, existingLease); err != nil {
				return nil, fmt.Errorf("failed to extend existing lease: %w", err)
			}
			return existingLease, nil
		}
		// The old lease is expired: delete it before creating its replacement.
		// A not-found here just means someone else already removed it.
		if err := s.leaseRepo.DeleteByMAC(ctx, existingLease.MAC); err != nil && !errors.Is(err, db.ErrNotFound) {
			return nil, fmt.Errorf("failed to delete expired lease: %w", err)
		}
	} else if !errors.Is(err, db.ErrNotFound) {
		return nil, fmt.Errorf("failed to look up existing lease: %w", err)
	}

	// Determine IP to assign
	var assignIP net.IP
	if requestedIP.To4() != nil && server.IsInRange(requestedIP) {
		// Check if requested IP is available
		if s.isIPAvailable(ctx, serverID, requestedIP, mac) {
			assignIP = requestedIP.To4()
		}
	}

	if assignIP == nil {
		// Find next available IP
		assignIP, err = s.findAvailableIP(ctx, serverID, mac)
		if err != nil {
			return nil, fmt.Errorf("no available IP addresses: %w", err)
		}
	}

	// Create new lease
	now := time.Now()
	lease := &Lease{
		ID:             uuid.New().String(),
		MAC:            mac,
		IP:             assignIP,
		ServerID:       serverID,
		Expiry:         now.Add(server.LeaseDuration),
		Reserved:       false,
		State:          StateAssigned,
		StateUpdatedAt: now,
		LastSeen:       now,
		StateHistory: []StateTransition{
			{FromState: "", ToState: StateAssigned, Timestamp: now, Source: "dhcp"},
		},
	}
	if err := ensureProvisionToken(lease); err != nil {
		return nil, err
	}

	// Save lease
	if err := s.leaseRepo.Save(ctx, lease); err != nil {
		return nil, fmt.Errorf("failed to save lease: %w", err)
	}

	return lease, nil
}

// ReleaseLease releases a lease by MAC address.
// The MAC is canonicalized so a legacy spelling still finds the row.
func (s *DHCPLeaseService) ReleaseLease(ctx context.Context, mac string) error {
	if canon, err := CanonicalMAC(mac); err == nil {
		mac = canon
	}
	s.allocMu.Lock()
	defer s.allocMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	unlock, err := lockLease(ctx, mac)
	if err != nil {
		return err
	}
	defer unlock()

	if err := s.leaseRepo.DeleteByMAC(ctx, mac); err != nil {
		return fmt.Errorf("failed to release lease: %w", err)
	}
	return nil
}

// ReserveLease reserves a specific IP for a MAC address
func (s *DHCPLeaseService) ReserveLease(ctx context.Context, serverID string, mac string, ip net.IP) error {
	s.allocMu.Lock()
	defer s.allocMu.Unlock()

	if canon, err := CanonicalMAC(mac); err == nil {
		mac = canon
	}
	unlock, err := lockLease(ctx, mac)
	if err != nil {
		return err
	}
	defer unlock()

	server, err := s.serverRepo.Get(ctx, serverID)
	if err != nil {
		return fmt.Errorf("failed to get server: %w", err)
	}

	if !server.IsInRange(ip) {
		return fmt.Errorf("IP %s is not in server range", ip.String())
	}

	// Check if IP is already reserved
	if !s.isIPAvailable(ctx, serverID, ip, mac) {
		return fmt.Errorf("IP %s is already in use", ip.String())
	}

	// Remove any existing lease for this MAC first.
	// Only a genuine "not found" is fine; anything else is a real failure.
	if err := s.leaseRepo.DeleteByMAC(ctx, mac); err != nil && !errors.Is(err, db.ErrNotFound) {
		return fmt.Errorf("failed to remove existing lease for MAC %s: %w", mac, err)
	}

	// Create reserved lease
	now := time.Now()
	lease := &Lease{
		ID:             uuid.New().String(),
		MAC:            mac,
		IP:             ip.To4(),
		ServerID:       serverID,
		Expiry:         now.Add(server.LeaseDuration),
		Reserved:       true,
		State:          StateAssigned,
		StateUpdatedAt: now,
		LastSeen:       now,
		StateHistory: []StateTransition{
			{FromState: "", ToState: StateAssigned, Timestamp: now, Source: "manual"},
		},
	}
	if err := ensureProvisionToken(lease); err != nil {
		return err
	}

	if err := s.leaseRepo.Save(ctx, lease); err != nil {
		return fmt.Errorf("failed to save reserved lease: %w", err)
	}

	return nil
}

// UnreserveLease removes a reservation by MAC address.
// This is a single-lease update, so it takes the per-MAC lock and re-reads
// inside it. It does not hold the pool allocation lock.
func (s *DHCPLeaseService) UnreserveLease(ctx context.Context, mac string) error {
	unlock, err := lockLease(ctx, mac)
	if err != nil {
		return err
	}
	defer unlock()

	lease, err := s.leaseByMAC(ctx, mac)
	if err != nil {
		return fmt.Errorf("failed to get lease: %w", err)
	}

	lease.Reserved = false
	if err := s.leaseRepo.Save(ctx, lease); err != nil {
		return fmt.Errorf("failed to unreserve lease: %w", err)
	}

	return nil
}

// GetLeaseByMAC retrieves a lease by MAC address.
// The lookup accepts any conventional MAC spelling.
func (s *DHCPLeaseService) GetLeaseByMAC(ctx context.Context, mac string) (*Lease, error) {
	return s.leaseByMAC(ctx, mac)
}

// GetLeasesByServer retrieves all leases for a server
func (s *DHCPLeaseService) GetLeasesByServer(ctx context.Context, serverID string) ([]*Lease, error) {
	return s.leaseRepo.GetByServerID(ctx, serverID)
}

// CleanupExpiredLeases removes expired leases.
// The scan does not hold the allocation lock. Each delete takes the per-MAC
// lock, re-reads the row, and skips it when a renew extended it. A cancelled
// ctx stops the loop before the next delete.
func (s *DHCPLeaseService) CleanupExpiredLeases(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	expired, err := s.leaseRepo.GetExpired(ctx)
	if err != nil {
		return fmt.Errorf("failed to get expired leases: %w", err)
	}
	for _, lease := range expired {
		if err := s.deleteIfExpired(ctx, lease); err != nil {
			return err
		}
	}
	return nil
}

// deleteIfExpired removes lease when it is still expired under the per-MAC lock.
func (s *DHCPLeaseService) deleteIfExpired(ctx context.Context, lease *Lease) error {
	if lease == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	unlock, err := lockLease(ctx, lease.MAC)
	if err != nil {
		return err
	}
	defer unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	fresh, err := s.leaseRepo.Get(ctx, lease.ID)
	if errors.Is(err, db.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if fresh.Reserved || !time.Now().After(fresh.Expiry) {
		return nil
	}
	if err := s.leaseRepo.Delete(ctx, fresh.ID); err != nil {
		return fmt.Errorf("failed to delete expired lease %s: %w", fresh.ID, err)
	}
	return nil
}

// UpdateLease saves a caller-modified lease.
// The read of the stored row and the write share the per-MAC lock.
// DHCP-owned fields that moved forward after the caller read the lease
// (expiry, last seen, provisioning state) are kept, so a stale copy cannot
// roll back a concurrent renewal. A missing row is not recreated.
func (s *DHCPLeaseService) UpdateLease(ctx context.Context, lease *Lease) error {
	if lease == nil {
		return fmt.Errorf("lease is nil")
	}
	unlock, err := lockLease(ctx, lease.MAC)
	if err != nil {
		return err
	}
	defer unlock()

	fresh, err := s.leaseRepo.Get(ctx, lease.ID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return err
		}
		return fmt.Errorf("failed to read lease: %w", err)
	}
	preserveNewerDHCPFields(lease, fresh)
	if err := ensureProvisionToken(lease); err != nil {
		return err
	}
	if err := s.leaseRepo.Save(ctx, lease); err != nil {
		return fmt.Errorf("failed to update lease: %w", err)
	}
	return nil
}

// UpdateLeaseState updates the state of a lease with state tracking.
// The read-modify-write holds the per-MAC lock and re-reads inside it.
func (s *DHCPLeaseService) UpdateLeaseState(ctx context.Context, mac string, newState string, source string) error {
	unlock, err := lockLease(ctx, mac)
	if err != nil {
		return err
	}
	defer unlock()

	lease, err := s.leaseByMAC(ctx, mac)
	if err != nil {
		return fmt.Errorf("failed to get lease: %w", err)
	}

	lease.UpdateState(LeaseState(newState), source)
	if err := ensureProvisionToken(lease); err != nil {
		return err
	}
	if err := s.leaseRepo.Save(ctx, lease); err != nil {
		return fmt.Errorf("failed to save lease state: %w", err)
	}

	return nil
}

// RecordHeartbeat records a heartbeat from a machine, updating LastSeen
// and LastHeartbeat. An offline lease returns to its last non-offline
// stage. The read-modify-write holds the per-MAC lock so it cannot clobber
// a DHCP renewal that is extending the same row.
func (s *DHCPLeaseService) RecordHeartbeat(ctx context.Context, mac string) error {
	unlock, err := lockLease(ctx, mac)
	if err != nil {
		return err
	}
	defer unlock()

	lease, err := s.leaseByMAC(ctx, mac)
	if err != nil {
		return fmt.Errorf("failed to get lease: %w", err)
	}

	now := time.Now()
	lease.LastHeartbeat = now
	if lease.State == StateOffline {
		lease.RestoreFromOffline("heartbeat")
	} else {
		lease.LastSeen = now
	}
	if err := ensureProvisionToken(lease); err != nil {
		return err
	}

	if err := s.leaseRepo.Save(ctx, lease); err != nil {
		return fmt.Errorf("failed to save heartbeat: %w", err)
	}

	return nil
}

// GetLeaseStateHistory returns the state transition history for a lease
func (s *DHCPLeaseService) GetLeaseStateHistory(ctx context.Context, mac string) ([]StateTransition, error) {
	lease, err := s.leaseByMAC(ctx, mac)
	if err != nil {
		return nil, fmt.Errorf("failed to get lease: %w", err)
	}

	return lease.StateHistory, nil
}

// GetLeasesByState returns all leases in a specific state
func (s *DHCPLeaseService) GetLeasesByState(ctx context.Context, state string) ([]*Lease, error) {
	wanted := LeaseState(state)
	servers, err := s.serverRepo.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get servers: %w", err)
	}

	var result []*Lease
	for _, server := range servers {
		leases, err := s.leaseRepo.GetByServerID(ctx, server.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get leases for server %s: %w", server.ID, err)
		}
		for _, lease := range leases {
			if lease.State == wanted {
				result = append(result, lease)
			}
		}
	}

	return result, nil
}

// MarkOfflineLeases marks leases as offline when they have been quiet longer
// than their OfflineWindow. offlineThreshold is the heartbeat floor; the
// window also depends on the lease duration and whether the host has
// heartbeated or is mid-install. The scan does not hold the allocation
// lock. Each candidate is re-read under the per-MAC lock, and a cancelled
// ctx stops the loop before the next write.
func (s *DHCPLeaseService) MarkOfflineLeases(ctx context.Context, offlineThreshold time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	servers, err := s.serverRepo.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("failed to get servers: %w", err)
	}

	for _, server := range servers {
		if err := ctx.Err(); err != nil {
			return err
		}
		leases, err := s.leaseRepo.GetByServerID(ctx, server.ID)
		if err != nil {
			return fmt.Errorf("failed to get leases for server %s: %w", server.ID, err)
		}
		duration := server.LeaseDuration
		for _, lease := range leases {
			if err := s.markOneOffline(ctx, lease, duration, offlineThreshold); err != nil {
				return err
			}
		}
	}

	return nil
}

// markOneOffline re-checks one lease under its lock and marks it offline
// only when it is still quiet.
func (s *DHCPLeaseService) markOneOffline(ctx context.Context, lease *Lease, leaseDuration, offlineThreshold time.Duration) error {
	if lease == nil || !lease.IsActive() {
		return nil
	}
	if !lease.LastSeen.Before(time.Now().Add(-lease.OfflineWindow(leaseDuration, offlineThreshold))) {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	unlock, err := lockLease(ctx, lease.MAC)
	if err != nil {
		return err
	}
	defer unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	fresh, err := s.leaseByMAC(ctx, lease.MAC)
	if errors.Is(err, db.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !fresh.IsActive() || !fresh.LastSeen.Before(time.Now().Add(-fresh.OfflineWindow(leaseDuration, offlineThreshold))) {
		return nil
	}
	fresh.UpdateState(StateOffline, "heartbeat")
	if err := s.leaseRepo.Save(ctx, fresh); err != nil {
		return fmt.Errorf("failed to mark lease %s offline: %w", fresh.ID, err)
	}
	return nil
}

// isIPAvailable checks if an IP address is available for assignment
func (s *DHCPLeaseService) isIPAvailable(ctx context.Context, serverID string, ip net.IP, excludeMAC string) bool {
	leases, err := s.leaseRepo.GetByServerID(ctx, serverID)
	if err != nil {
		return false
	}

	now := time.Now()
	for _, lease := range leases {
		if lease.IP.Equal(ip) && lease.MAC != excludeMAC && lease.OccupiesAddress(now) {
			return false
		}
	}
	return true
}

// leaseByMAC loads a lease, accepting any conventional MAC spelling.
// A canonical hit wins. When the row was stored under a different spelling,
// the original string is tried so older rows remain reachable.
func (s *DHCPLeaseService) leaseByMAC(ctx context.Context, mac string) (*Lease, error) {
	if canon, err := CanonicalMAC(mac); err == nil {
		lease, lerr := s.leaseRepo.GetByMAC(ctx, canon)
		if lerr == nil {
			return lease, nil
		}
		if !errors.Is(lerr, db.ErrNotFound) {
			return nil, lerr
		}
		if canon == mac {
			return nil, lerr
		}
	}
	return s.leaseRepo.GetByMAC(ctx, mac)
}

// preserveNewerDHCPFields copies DHCP-owned fields from fresh onto incoming
// when fresh is strictly newer. The caller's other edits (boot menu, IPMI,
// reservation flag) stay in place.
func preserveNewerDHCPFields(incoming, fresh *Lease) {
	if incoming == nil || fresh == nil {
		return
	}
	if fresh.Expiry.After(incoming.Expiry) {
		incoming.Expiry = fresh.Expiry
	}
	if fresh.LastSeen.After(incoming.LastSeen) {
		incoming.LastSeen = fresh.LastSeen
	}
	if fresh.StateUpdatedAt.After(incoming.StateUpdatedAt) {
		incoming.State = fresh.State
		incoming.StateUpdatedAt = fresh.StateUpdatedAt
		incoming.StateHistory = append([]StateTransition(nil), fresh.StateHistory...)
	}
	if fresh.LastHeartbeat.After(incoming.LastHeartbeat) {
		incoming.LastHeartbeat = fresh.LastHeartbeat
	}
	if incoming.ProvisionToken == "" {
		incoming.ProvisionToken = fresh.ProvisionToken
	}
}

// findAvailableIP finds the next available IP in the server's range
func (s *DHCPLeaseService) findAvailableIP(ctx context.Context, serverID string, excludeMAC string) (net.IP, error) {
	server, err := s.serverRepo.Get(ctx, serverID)
	if err != nil {
		return nil, fmt.Errorf("failed to get server: %w", err)
	}

	for i := 0; i < server.LeaseRange; i++ {
		candidate, err := incrementIP(server.IPStart, i)
		if err != nil {
			return nil, err
		}
		if s.isIPAvailable(ctx, serverID, candidate, excludeMAC) {
			return candidate, nil
		}
	}

	return nil, fmt.Errorf("no available IP addresses in range")
}

// incrementIP increments an IPv4 address by the given amount.
// It rejects non-IPv4 input and refuses to wrap past 255.255.255.255.
func incrementIP(ip net.IP, increment int) (net.IP, error) {
	ip4 := ip.To4()
	if ip4 == nil {
		return nil, fmt.Errorf("incrementIP requires an IPv4 address")
	}
	if increment < 0 {
		return nil, fmt.Errorf("incrementIP requires a non-negative increment")
	}
	val, err := ipToInt(ip4)
	if err != nil {
		return nil, err
	}
	if newVal := uint64(val) + uint64(increment); newVal > 0xFFFFFFFF {
		return nil, fmt.Errorf("IP increment overflows IPv4 address space")
	} else {
		val = uint32(newVal)
	}
	return net.IPv4(byte(val>>24), byte(val>>16), byte(val>>8), byte(val)).To4(), nil
}
