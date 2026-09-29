package dhcp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"ignite/config"
	"ignite/db"

	"github.com/google/uuid"
)

// sharedAllocMu serializes IP allocation across the DHCP packet path
// (ProtocolHandler) and the service path (DHCPLeaseService). Both services
// are constructed independently, so the shared mutex lives at package scope;
// each service holds a pointer to it.
var sharedAllocMu sync.Mutex

// DHCPServerService implements the ServerService interface
type DHCPServerService struct {
	serverRepo ServerRepository
	leaseRepo  LeaseRepository
	handlers   map[string]*ProtocolHandler
	// handlersMu guards the handlers map: HTTP handlers run on concurrent
	// goroutines, and concurrent map access is a fatal runtime panic.
	handlersMu sync.RWMutex
	// allocMu is shared with every ProtocolHandler and with
	// DHCPLeaseService to close the check-then-act allocation race.
	allocMu *sync.Mutex
}

// NewDHCPServerService creates a new DHCP server service
func NewDHCPServerService(serverRepo ServerRepository, leaseRepo LeaseRepository) *DHCPServerService {
	return &DHCPServerService{
		serverRepo: serverRepo,
		leaseRepo:  leaseRepo,
		handlers:   make(map[string]*ProtocolHandler),
		allocMu:    &sharedAllocMu,
	}
}

// CreateServer creates a new DHCP server
func (s *DHCPServerService) CreateServer(ctx context.Context, config ServerConfig) (*Server, error) {
	// Validate configuration
	if err := s.validateServerConfig(config); err != nil {
		return nil, fmt.Errorf("invalid server configuration: %w", err)
	}

	// Check if server with this IP already exists.
	// Only a typed "not found" means "no such server" — any other error is a
	// real backend failure and must not be misread as absent.
	existing, err := s.serverRepo.GetByIP(ctx, config.IP)
	if err == nil && existing != nil {
		return nil, fmt.Errorf("server with IP %s already exists", config.IP)
	}
	if err != nil && !errors.Is(err, db.ErrNotFound) {
		return nil, fmt.Errorf("failed to check for existing server: %w", err)
	}

	server := &Server{
		ID:            uuid.New().String(),
		IP:            config.IP,
		IPStart:       config.StartIP,
		LeaseRange:    config.LeaseRange,
		LeaseDuration: config.LeaseDuration,
		Started:       false,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Options: DHCPOptions{
			SubnetMask: config.SubnetMask,
			Gateway:    config.Gateway,
			DNS:        config.DNS,
			TFTPServer: config.IP,
		},
	}

	if err := s.serverRepo.Save(ctx, server); err != nil {
		return nil, fmt.Errorf("failed to save server: %w", err)
	}

	return server, nil
}

// UpdateServer updates an existing DHCP server configuration
func (s *DHCPServerService) UpdateServer(ctx context.Context, serverID string, config ServerConfig) error {
	// Validate configuration
	if err := s.validateServerConfig(config); err != nil {
		return fmt.Errorf("invalid server configuration: %w", err)
	}

	// Get existing server
	server, err := s.serverRepo.Get(ctx, serverID)
	if err != nil {
		return fmt.Errorf("failed to get server: %w", err)
	}

	// The network IP is immutable on update: changing the bound address behind
	// the listener would orphan it. Reject it explicitly instead of silently
	// discarding the submitted value.
	if !config.IP.Equal(server.IP) {
		return fmt.Errorf("changing the server network IP on update is not allowed")
	}

	// If server is running, we need to stop and restart it
	wasRunning := server.Started
	if wasRunning {
		if err := s.StopServer(ctx, serverID); err != nil {
			return fmt.Errorf("failed to stop server for update: %w", err)
		}
	}

	// Update server configuration (the network IP is immutable on update)
	server.IPStart = config.StartIP
	server.LeaseRange = config.LeaseRange
	server.LeaseDuration = config.LeaseDuration
	server.UpdatedAt = time.Now()
	server.Options = DHCPOptions{
		SubnetMask: config.SubnetMask,
		Gateway:    config.Gateway,
		DNS:        config.DNS,
		TFTPServer: server.IP,
	}

	// Save updated server
	if err := s.serverRepo.Save(ctx, server); err != nil {
		return fmt.Errorf("failed to save updated server: %w", err)
	}

	// Restart server if it was running
	if wasRunning {
		if err := s.StartServer(ctx, serverID); err != nil {
			return fmt.Errorf("server configuration updated but failed to restart server: %w", err)
		}
	}

	return nil
}

// StartServer starts a DHCP server
func (s *DHCPServerService) StartServer(ctx context.Context, serverID string) error {
	server, err := s.serverRepo.Get(ctx, serverID)
	if err != nil {
		return fmt.Errorf("failed to get server: %w", err)
	}

	if server.Started {
		return fmt.Errorf("server is already running")
	}

	// Load the boot-file settings once at construction time so the packet
	// path never pays for config.LoadDefault() per packet.
	appCfg, err := config.LoadDefault()
	if err != nil {
		return fmt.Errorf("failed to load application config: %w", err)
	}
	bootFiles := ProtocolBootFiles{
		BiosFile: appCfg.DHCP.BiosFile,
		EFIFile:  appCfg.DHCP.EFIFile,
	}

	// Create and start protocol handler
	handler := NewProtocolHandler(server, s.leaseRepo,
		WithAllocationMutex(s.allocMu),
		WithBootFiles(bootFiles),
	)
	if err := handler.Start(); err != nil {
		return fmt.Errorf("failed to start DHCP handler: %w", err)
	}

	s.handlersMu.Lock()
	s.handlers[serverID] = handler
	s.handlersMu.Unlock()

	// Update server state
	server.Started = true
	server.UpdatedAt = time.Now()

	if err := s.serverRepo.Save(ctx, server); err != nil {
		// Try to stop the handler if we can't save the state
		_ = handler.Stop()
		s.handlersMu.Lock()
		delete(s.handlers, serverID)
		s.handlersMu.Unlock()
		return fmt.Errorf("failed to update server state: %w", err)
	}

	return nil
}

// StopServer stops a DHCP server
func (s *DHCPServerService) StopServer(ctx context.Context, serverID string) error {
	server, err := s.serverRepo.Get(ctx, serverID)
	if err != nil {
		return fmt.Errorf("failed to get server: %w", err)
	}

	if !server.Started {
		return fmt.Errorf("server is not running")
	}

	// Stop protocol handler
	s.handlersMu.Lock()
	if handler, exists := s.handlers[serverID]; exists {
		if err := handler.Stop(); err != nil {
			log.Printf("Error stopping DHCP handler: %v", err)
		}
		delete(s.handlers, serverID)
	}
	s.handlersMu.Unlock()

	// Update server state
	server.Started = false
	server.UpdatedAt = time.Now()

	if err := s.serverRepo.Save(ctx, server); err != nil {
		return fmt.Errorf("failed to update server state: %w", err)
	}

	return nil
}

// DeleteServer deletes a DHCP server and all its leases
func (s *DHCPServerService) DeleteServer(ctx context.Context, serverID string) error {
	// Stop server if running
	s.handlersMu.Lock()
	if handler, exists := s.handlers[serverID]; exists {
		_ = handler.Stop()
		delete(s.handlers, serverID)
	}
	s.handlersMu.Unlock()

	// Delete all leases for this server
	if err := s.leaseRepo.DeleteByServerID(ctx, serverID); err != nil {
		return fmt.Errorf("failed to delete server leases: %w", err)
	}

	// Delete server
	if err := s.serverRepo.Delete(ctx, serverID); err != nil {
		return fmt.Errorf("failed to delete server: %w", err)
	}

	return nil
}

// GetServer retrieves a server by ID
func (s *DHCPServerService) GetServer(ctx context.Context, serverID string) (*Server, error) {
	return s.serverRepo.Get(ctx, serverID)
}

// GetAllServers retrieves all servers
func (s *DHCPServerService) GetAllServers(ctx context.Context) ([]*Server, error) {
	return s.serverRepo.GetAll(ctx)
}

// validateServerConfig validates server configuration
func (s *DHCPServerService) validateServerConfig(config ServerConfig) error {
	if config.IP.To4() == nil {
		return fmt.Errorf("server IP must be a valid IPv4 address")
	}
	if config.SubnetMask.To4() == nil {
		return fmt.Errorf("subnet mask must be a valid IPv4 address")
	}
	if config.Gateway.To4() == nil {
		return fmt.Errorf("gateway must be a valid IPv4 address")
	}
	if config.DNS.To4() == nil {
		return fmt.Errorf("DNS must be a valid IPv4 address")
	}
	if config.StartIP.To4() == nil {
		return fmt.Errorf("start IP must be a valid IPv4 address")
	}
	if config.LeaseRange <= 0 {
		return fmt.Errorf("lease range must be positive")
	}
	if config.LeaseDuration <= 0 {
		return fmt.Errorf("lease duration must be positive")
	}

	// The netmask must be contiguous (e.g. 255.255.255.0).
	mask := net.IPMask(config.SubnetMask.To4())
	ones, bits := mask.Size()
	if bits != 32 || ones < 0 || ones > 32 {
		return fmt.Errorf("subnet mask %s is not a valid contiguous IPv4 netmask", config.SubnetMask)
	}

	// The lease range start must live inside the server's subnet.
	network := config.IP.Mask(mask)
	if !config.StartIP.Mask(mask).Equal(network) {
		return fmt.Errorf("start IP %s is not in the server subnet %s", config.StartIP, network)
	}

	// The whole lease range must fit inside IPv4 space.
	startInt, err := ipToInt(config.StartIP)
	if err != nil {
		return fmt.Errorf("start IP must be a valid IPv4 address")
	}
	if end := uint64(startInt) + uint64(config.LeaseRange); end > 0xFFFFFFFF {
		return fmt.Errorf("lease range overflows IPv4 address space")
	}

	return nil
}
