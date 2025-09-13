package dhcp

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/google/uuid"
)

// DHCPLeaseService implements the LeaseService interface
type DHCPLeaseService struct {
	leaseRepo  LeaseRepository
	serverRepo ServerRepository
}

// NewDHCPLeaseService creates a new lease service
func NewDHCPLeaseService(leaseRepo LeaseRepository, serverRepo ServerRepository) *DHCPLeaseService {
	return &DHCPLeaseService{
		leaseRepo:  leaseRepo,
		serverRepo: serverRepo,
	}
}

// AssignLease assigns an IP lease to a MAC address
func (s *DHCPLeaseService) AssignLease(ctx context.Context, serverID string, mac string, requestedIP net.IP) (*Lease, error) {
	server, err := s.serverRepo.Get(ctx, serverID)
	if err != nil {
		return nil, fmt.Errorf("failed to get server: %w", err)
	}

	// Check if MAC already has a lease
	existingLease, err := s.leaseRepo.GetByMAC(ctx, mac)
	if err == nil && existingLease != nil {
		if !existingLease.IsExpired() {
			// Extend existing lease if not expired
			existingLease.Extend(server.LeaseDuration)
			if err := s.leaseRepo.Save(ctx, existingLease); err != nil {
				return nil, fmt.Errorf("failed to extend existing lease: %w", err)
			}
			return existingLease, nil
		}
	}

	// Determine IP to assign
	var assignIP net.IP
	if requestedIP != nil && server.IsInRange(requestedIP) {
		// Check if requested IP is available
		if s.isIPAvailable(ctx, serverID, requestedIP, mac) {
			assignIP = requestedIP
		}
	}

	if assignIP == nil {
		// Find next available IP
		assignIP, err = s.findAvailableIP(ctx, serverID, mac)
		if err != nil {
			return nil, fmt.Errorf("no available IP addresses: %w", err)
		}
	}

	lease := &Lease{
		ID:             uuid.New().String(),
		IP:             assignIP,
		MAC:            mac,
		Expiry:         time.Now().Add(server.LeaseDuration),
		Reserved:       false,
		ServerID:       serverID,
		State:          StateAssigned,
		StateUpdatedAt: time.Now(),
		LastSeen:       time.Now(),
		StateHistory:   []StateTransition{},
	}

	// Record initial state
	lease.UpdateState(StateAssigned, "dhcp")

	if err := s.leaseRepo.Save(ctx, lease); err != nil {
		return nil, fmt.Errorf("failed to save lease: %w", err)
	}

	return lease, nil
}

// ReleaseLease releases a lease by MAC address
func (s *DHCPLeaseService) ReleaseLease(ctx context.Context, mac string) error {
	return s.leaseRepo.DeleteByMAC(ctx, mac)
}

// ReserveLease creates a reserved lease for a specific MAC and IP
func (s *DHCPLeaseService) ReserveLease(ctx context.Context, serverID string, mac string, ip net.IP) error {
	server, err := s.serverRepo.Get(ctx, serverID)
	if err != nil {
		return fmt.Errorf("failed to get server: %w", err)
	}

	if !server.IsInRange(ip) {
		return fmt.Errorf("IP %s is not in server range", ip)
	}

	if !s.isIPAvailable(ctx, serverID, ip, mac) {
		return fmt.Errorf("IP %s is already in use", ip)
	}

	// Remove any existing lease for this MAC
	s.leaseRepo.DeleteByMAC(ctx, mac)

	lease := &Lease{
		ID:             uuid.New().String(),
		IP:             ip,
		MAC:            mac,
		Expiry:         time.Now().Add(server.LeaseDuration),
		Reserved:       true,
		ServerID:       serverID,
		State:          StateAssigned,
		StateUpdatedAt: time.Now(),
		LastSeen:       time.Now(),
		StateHistory:   []StateTransition{},
	}

	// Record initial state for reserved lease
	lease.UpdateState(StateAssigned, "manual")

	return s.leaseRepo.Save(ctx, lease)
}

// UnreserveLease removes a reservation for a MAC address
func (s *DHCPLeaseService) UnreserveLease(ctx context.Context, mac string) error {
	lease, err := s.leaseRepo.GetByMAC(ctx, mac)
	if err != nil {
		return fmt.Errorf("lease not found for MAC %s: %w", mac, err)
	}

	if !lease.Reserved {
		return fmt.Errorf("lease for MAC %s is not reserved", mac)
	}

	lease.Reserved = false
	return s.leaseRepo.Save(ctx, lease)
}

// GetLeaseByMAC retrieves a lease by MAC address
func (s *DHCPLeaseService) GetLeaseByMAC(ctx context.Context, mac string) (*Lease, error) {
	return s.leaseRepo.GetByMAC(ctx, mac)
}

// GetLeasesByServer retrieves all leases for a server
func (s *DHCPLeaseService) GetLeasesByServer(ctx context.Context, serverID string) ([]*Lease, error) {
	return s.leaseRepo.GetByServerID(ctx, serverID)
}

// CleanupExpiredLeases removes all expired leases
func (s *DHCPLeaseService) CleanupExpiredLeases(ctx context.Context) error {
	return s.leaseRepo.CleanupExpired(ctx)
}

// UpdateLease updates an existing lease
func (s *DHCPLeaseService) UpdateLease(ctx context.Context, lease *Lease) error {
	return s.leaseRepo.Save(ctx, lease)
}

// isIPAvailable checks if an IP is available for assignment
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

	leases, err := s.leaseRepo.GetByServerID(ctx, serverID)
	if err != nil {
		return nil, fmt.Errorf("failed to get leases: %w", err)
	}

	// Create a map of used IPs
	usedIPs := make(map[string]bool)
	for _, lease := range leases {
		if !lease.IsExpired() && lease.MAC != excludeMAC {
			usedIPs[lease.IP.String()] = true
		}
	}

	// Find first available IP in range
	for i := 0; i < server.LeaseRange; i++ {
		candidate := incrementIP(server.IPStart, i)
		if !usedIPs[candidate.String()] {
			return candidate, nil
		}
	}

	return nil, fmt.Errorf("no available IP addresses in range")
}

// incrementIP increments an IP address by the given amount
func incrementIP(ip net.IP, increment int) net.IP {
	result := make(net.IP, len(ip))
	copy(result, ip)

	// Convert to IPv4 if needed
	if len(result) == 16 {
		result = result[12:16]
	}

	// Convert to uint32, add increment, convert back
	ipInt := uint32(result[0])<<24 + uint32(result[1])<<16 + uint32(result[2])<<8 + uint32(result[3])
	ipInt += uint32(increment)

	result[0] = byte(ipInt >> 24)
	result[1] = byte(ipInt >> 16)
	result[2] = byte(ipInt >> 8)
	result[3] = byte(ipInt)

	return result
}

// UpdateLeaseState updates the state of a lease and records the transition
func (s *DHCPLeaseService) UpdateLeaseState(ctx context.Context, mac string, newState string, source string) error {
	lease, err := s.leaseRepo.GetByMAC(ctx, mac)
	if err != nil {
		return fmt.Errorf("failed to get lease for MAC %s: %w", mac, err)
	}
	if lease == nil {
		return fmt.Errorf("lease not found for MAC %s", mac)
	}

	lease.UpdateState(newState, source)

	if err := s.leaseRepo.Save(ctx, lease); err != nil {
		return fmt.Errorf("failed to save lease state update: %w", err)
	}

	return nil
}

// RecordHeartbeat updates the last seen time for a lease
func (s *DHCPLeaseService) RecordHeartbeat(ctx context.Context, mac string) error {
	lease, err := s.leaseRepo.GetByMAC(ctx, mac)
	if err != nil {
		return fmt.Errorf("failed to get lease for MAC %s: %w", mac, err)
	}
	if lease == nil {
		return fmt.Errorf("lease not found for MAC %s", mac)
	}

	lease.LastSeen = time.Now()

	if err := s.leaseRepo.Save(ctx, lease); err != nil {
		return fmt.Errorf("failed to save heartbeat: %w", err)
	}

	return nil
}

// GetLeaseStateHistory returns the state history for a lease
func (s *DHCPLeaseService) GetLeaseStateHistory(ctx context.Context, mac string) ([]StateTransition, error) {
	lease, err := s.leaseRepo.GetByMAC(ctx, mac)
	if err != nil {
		return nil, fmt.Errorf("failed to get lease for MAC %s: %w", mac, err)
	}
	if lease == nil {
		return nil, fmt.Errorf("lease not found for MAC %s", mac)
	}

	return lease.StateHistory, nil
}

// GetLeasesByState returns all leases in a specific state
func (s *DHCPLeaseService) GetLeasesByState(ctx context.Context, state string) ([]*Lease, error) {
	// This would require extending the repository interface to support state filtering
	// For now, we'll get all leases and filter in memory
	servers, err := s.serverRepo.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get servers: %w", err)
	}

	var filteredLeases []*Lease
	for _, server := range servers {
		leases, err := s.leaseRepo.GetByServerID(ctx, server.ID)
		if err != nil {
			continue // Skip servers that can't be queried
		}

		for _, lease := range leases {
			if lease.State == state {
				filteredLeases = append(filteredLeases, lease)
			}
		}
	}

	return filteredLeases, nil
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
			continue // Skip servers that can't be queried
		}

		for _, lease := range leases {
			if lease.IsActive() && lease.LastSeen.Before(cutoffTime) {
				lease.UpdateState(StateOffline, "heartbeat")
				s.leaseRepo.Save(ctx, lease) // Ignore error and continue processing
			}
		}
	}

	return nil
}
