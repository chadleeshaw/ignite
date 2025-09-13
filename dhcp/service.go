package dhcp

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

// DHCPServerService implements the ServerService interface
type DHCPServerService struct {
	serverRepo ServerRepository
	leaseRepo  LeaseRepository
	handlers   map[string]*ProtocolHandler
}

// NewDHCPServerService creates a new DHCP server service
func NewDHCPServerService(serverRepo ServerRepository, leaseRepo LeaseRepository) *DHCPServerService {
	return &DHCPServerService{
		serverRepo: serverRepo,
		leaseRepo:  leaseRepo,
		handlers:   make(map[string]*ProtocolHandler),
	}
}

// CreateServer creates a new DHCP server
func (s *DHCPServerService) CreateServer(ctx context.Context, config ServerConfig) (*Server, error) {
	// Validate configuration
	if err := s.validateServerConfig(config); err != nil {
		return nil, fmt.Errorf("invalid server configuration: %w", err)
	}

	// Check if server with this IP already exists
	existing, err := s.serverRepo.GetByIP(ctx, config.IP)
	if err == nil && existing != nil {
		return nil, fmt.Errorf("server with IP %s already exists", config.IP)
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

	// For updates, we don't allow changing the network IP to prevent conflicts
	// The UI should prevent this, but we enforce it here as well
	config.IP = server.IP

	// If server is running, we need to stop and restart it
	wasRunning := server.Started
	if wasRunning {
		if err := s.StopServer(ctx, serverID); err != nil {
			return fmt.Errorf("failed to stop server for update: %w", err)
		}
	}

	// Update server configuration
	server.IP = config.IP
	server.IPStart = config.StartIP
	server.LeaseRange = config.LeaseRange
	server.LeaseDuration = config.LeaseDuration
	server.UpdatedAt = time.Now()
	server.Options = DHCPOptions{
		SubnetMask: config.SubnetMask,
		Gateway:    config.Gateway,
		DNS:        config.DNS,
		TFTPServer: config.IP,
	}

	// Save updated server
	if err := s.serverRepo.Save(ctx, server); err != nil {
		return fmt.Errorf("failed to save updated server: %w", err)
	}

	// Restart server if it was running
	if wasRunning {
		if err := s.StartServer(ctx, serverID); err != nil {
			log.Printf("Warning: failed to restart server after update: %v", err)
			// Don't return error here, the update was successful
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

	// Create and start protocol handler
	handler := NewProtocolHandler(server, s.leaseRepo)
	if err := handler.Start(); err != nil {
		return fmt.Errorf("failed to start DHCP handler: %w", err)
	}

	s.handlers[serverID] = handler

	// Update server state
	server.Started = true
	server.UpdatedAt = time.Now()

	if err := s.serverRepo.Save(ctx, server); err != nil {
		// Try to stop the handler if we can't save the state
		handler.Stop()
		delete(s.handlers, serverID)
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
	if handler, exists := s.handlers[serverID]; exists {
		if err := handler.Stop(); err != nil {
			log.Printf("Error stopping DHCP handler: %v", err)
		}
		delete(s.handlers, serverID)
	}

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
	if handler, exists := s.handlers[serverID]; exists {
		handler.Stop()
		delete(s.handlers, serverID)
	}

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
	if config.IP == nil {
		return fmt.Errorf("server IP cannot be nil")
	}
	if config.SubnetMask == nil {
		return fmt.Errorf("subnet mask cannot be nil")
	}
	if config.Gateway == nil {
		return fmt.Errorf("gateway cannot be nil")
	}
	if config.DNS == nil {
		return fmt.Errorf("DNS cannot be nil")
	}
	if config.StartIP == nil {
		return fmt.Errorf("start IP cannot be nil")
	}
	if config.LeaseRange <= 0 {
		return fmt.Errorf("lease range must be positive")
	}
	if config.LeaseDuration <= 0 {
		return fmt.Errorf("lease duration must be positive")
	}

	return nil
}
