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

	server, err := s.serverRepo.Get(ctx, serverID)
	if err != nil {
		return nil, fmt.Errorf("failed to get server: %w", err)
	}

	// Check if MAC already has a lease
	existingLease, err := s.leaseRepo.GetByMAC(ctx, mac)
	if err == nil {
		// An existing lease may only be extended when it belongs to the
		// requested server; a lease from another server is not ours to renew.
		if existingLease.ServerID != serverID {
			return nil, fmt.Errorf("MAC %s already has a lease on another server", mac)
		}
		if !existingLease.IsExpired() {
			// Extend existing lease if not expired
			existingLease.Extend(server.LeaseDuration)
			if err := s.leaseRepo.Save(ctx, existingLease); err != nil {
				return nil, fmt.Errorf("failed to extend existing lease: %w", err)
			}
			return existingLease, nil
		}
		// The old lease is expired: delete it before creating its replacement.
		// A not-found here just means someone else already removed it.
		if err := s.leaseRepo.DeleteByMAC(ctx, mac); err != nil && !errors.Is(err, db.ErrNotFound) {
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

	// Save lease
	if err := s.leaseRepo.Save(ctx, lease); err != nil {
		return nil, fmt.Errorf("failed to save lease: %w", err)
	}

	return lease, nil
}

// ReleaseLease releases a lease by MAC address
func (s *DHCPLeaseService) ReleaseLease(ctx context.Context, mac string) error {
	if err := s.leaseRepo.DeleteByMAC(ctx, mac); err != nil {
		return fmt.Errorf("failed to release lease: %w", err)
	}
	return nil
}

// ReserveLease reserves a specific IP for a MAC address
func (s *DHCPLeaseService) ReserveLease(ctx context.Context, serverID string, mac string, ip net.IP) error {
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

	if err := s.leaseRepo.Save(ctx, lease); err != nil {
		return fmt.Errorf("failed to save reserved lease: %w", err)
	}

	return nil
}

// UnreserveLease removes a reservation by MAC address
func (s *DHCPLeaseService) UnreserveLease(ctx context.Context, mac string) error {
	lease, err := s.leaseRepo.GetByMAC(ctx, mac)
	if err != nil {
		return fmt.Errorf("failed to get lease: %w", err)
	}

	lease.Reserved = false
	if err := s.leaseRepo.Save(ctx, lease); err != nil {
		return fmt.Errorf("failed to unreserve lease: %w", err)
	}

	return nil
}

// GetLeaseByMAC retrieves a lease by MAC address
func (s *DHCPLeaseService) GetLeaseByMAC(ctx context.Context, mac string) (*Lease, error) {
	return s.leaseRepo.GetByMAC(ctx, mac)
}

// GetLeasesByServer retrieves all leases for a server
func (s *DHCPLeaseService) GetLeasesByServer(ctx context.Context, serverID string) ([]*Lease, error) {
	return s.leaseRepo.GetByServerID(ctx, serverID)
}

// CleanupExpiredLeases removes expired leases
func (s *DHCPLeaseService) CleanupExpiredLeases(ctx context.Context) error {
	return s.leaseRepo.CleanupExpired(ctx)
}

// UpdateLease updates an existing lease
func (s *DHCPLeaseService) UpdateLease(ctx context.Context, lease *Lease) error {
	if err := s.leaseRepo.Save(ctx, lease); err != nil {
		return fmt.Errorf("failed to update lease: %w", err)
	}
	return nil
}

// UpdateLeaseState updates the state of a lease with state tracking
func (s *DHCPLeaseService) UpdateLeaseState(ctx context.Context, mac string, newState string, source string) error {
	lease, err := s.leaseRepo.GetByMAC(ctx, mac)
	if err != nil {
		return fmt.Errorf("failed to get lease: %w", err)
	}

	lease.UpdateState(LeaseState(newState), source)
	if err := s.leaseRepo.Save(ctx, lease); err != nil {
		return fmt.Errorf("failed to save lease state: %w", err)
	}

	return nil
}

// RecordHeartbeat records a heartbeat from a machine, updating LastSeen
func (s *DHCPLeaseService) RecordHeartbeat(ctx context.Context, mac string) error {
	lease, err := s.leaseRepo.GetByMAC(ctx, mac)
	if err != nil {
		return fmt.Errorf("failed to get lease: %w", err)
	}

	lease.LastSeen = time.Now()
	// If lease was offline and now checking in, mark it as needing attention
	if lease.State == StateOffline {
		lease.UpdateState(StateAssigned, "heartbeat")
	}

	if err := s.leaseRepo.Save(ctx, lease); err != nil {
		return fmt.Errorf("failed to save heartbeat: %w", err)
	}

	return nil
}

// GetLeaseStateHistory returns the state transition history for a lease
func (s *DHCPLeaseService) GetLeaseStateHistory(ctx context.Context, mac string) ([]StateTransition, error) {
	lease, err := s.leaseRepo.GetByMAC(ctx, mac)
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

// MarkOfflineLeases marks leases as offline if they haven't been seen recently
func (s *DHCPLeaseService) MarkOfflineLeases(ctx context.Context, offlineThreshold time.Duration) error {
	servers, err := s.serverRepo.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("failed to get servers: %w", err)
	}

	cutoffTime := time.Now().Add(-offlineThreshold)

	for _, server := range servers {
		leases, err := s.leaseRepo.GetByServerID(ctx, server.ID)
		if err != nil {
			return fmt.Errorf("failed to get leases for server %s: %w", server.ID, err)
		}

		for _, lease := range leases {
			if lease.IsActive() && lease.LastSeen.Before(cutoffTime) {
				lease.UpdateState(StateOffline, "heartbeat")
				if err := s.leaseRepo.Save(ctx, lease); err != nil {
					return fmt.Errorf("failed to mark lease %s offline: %w", lease.ID, err)
				}
			}
		}
	}

	return nil
}

// isIPAvailable checks if an IP address is available for assignment
func (s *DHCPLeaseService) isIPAvailable(ctx context.Context, serverID string, ip net.IP, excludeMAC string) bool {
	leases, err := s.leaseRepo.GetByServerID(ctx, serverID)
	if err != nil {
		return false
	}

	for _, lease := range leases {
		if lease.IP.Equal(ip) && lease.MAC != excludeMAC && !lease.IsExpired() {
			return false
		}
	}
	return true
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
