package dhcp

import (
	"context"
	"net"
	"time"

	d4 "github.com/krolaw/dhcp4"
)

// ServerRepository defines the interface for DHCP server persistence
type ServerRepository interface {
	Save(ctx context.Context, server *Server) error
	Get(ctx context.Context, id string) (*Server, error)
	GetAll(ctx context.Context) ([]*Server, error)
	Delete(ctx context.Context, id string) error
	GetByIP(ctx context.Context, ip net.IP) (*Server, error)
}

// LeaseRepository defines the interface for lease persistence
type LeaseRepository interface {
	Save(ctx context.Context, lease *Lease) error
	Get(ctx context.Context, id string) (*Lease, error)
	GetByMAC(ctx context.Context, mac string) (*Lease, error)
	GetByServerID(ctx context.Context, serverID string) ([]*Lease, error)
	Delete(ctx context.Context, id string) error
	DeleteByMAC(ctx context.Context, mac string) error
	DeleteByServerID(ctx context.Context, serverID string) error
	GetExpired(ctx context.Context) ([]*Lease, error)
	CleanupExpired(ctx context.Context) error
}

// ServerService defines the interface for DHCP server management
type ServerService interface {
	CreateServer(ctx context.Context, config ServerConfig) (*Server, error)
	UpdateServer(ctx context.Context, serverID string, config ServerConfig) error
	StartServer(ctx context.Context, serverID string) error
	StopServer(ctx context.Context, serverID string) error
	DeleteServer(ctx context.Context, serverID string) error
	GetServer(ctx context.Context, serverID string) (*Server, error)
	GetAllServers(ctx context.Context) ([]*Server, error)
}

// LeaseService defines the interface for lease management
type LeaseService interface {
	AssignLease(ctx context.Context, serverID string, mac string, requestedIP net.IP) (*Lease, error)
	ReleaseLease(ctx context.Context, mac string) error
	ReserveLease(ctx context.Context, serverID string, mac string, ip net.IP) error
	UnreserveLease(ctx context.Context, mac string) error
	GetLeaseByMAC(ctx context.Context, mac string) (*Lease, error)
	GetLeasesByServer(ctx context.Context, serverID string) ([]*Lease, error)
	CleanupExpiredLeases(ctx context.Context) error
	UpdateLease(ctx context.Context, lease *Lease) error

	// State management methods
	UpdateLeaseState(ctx context.Context, mac string, newState string, source string) error
	RecordHeartbeat(ctx context.Context, mac string) error
	GetLeaseStateHistory(ctx context.Context, mac string) ([]StateTransition, error)
	GetLeasesByState(ctx context.Context, state string) ([]*Lease, error)
	MarkOfflineLeases(ctx context.Context, offlineThreshold time.Duration) error
}

// DHCPHandler defines the interface for handling DHCP packets
type DHCPHandler interface {
	ServeDHCP(p d4.Packet, msgType d4.MessageType, options d4.Options) d4.Packet
}

// ServerConfig represents configuration for creating a new DHCP server
type ServerConfig struct {
	IP            net.IP
	SubnetMask    net.IP
	Gateway       net.IP
	DNS           net.IP
	StartIP       net.IP
	LeaseRange    int
	LeaseDuration time.Duration
}
