package dhcp

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockServerRepository is a mock implementation of ServerRepository
type MockServerRepository struct {
	mock.Mock
}

func (m *MockServerRepository) Save(ctx context.Context, server *Server) error {
	args := m.Called(ctx, server)
	return args.Error(0)
}

func (m *MockServerRepository) Get(ctx context.Context, id string) (*Server, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(*Server), args.Error(1)
}

func (m *MockServerRepository) GetAll(ctx context.Context) ([]*Server, error) {
	args := m.Called(ctx)
	return args.Get(0).([]*Server), args.Error(1)
}

func (m *MockServerRepository) Delete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockServerRepository) GetByIP(ctx context.Context, ip net.IP) (*Server, error) {
	args := m.Called(ctx, ip)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Server), args.Error(1)
}

// MockLeaseRepository is a mock implementation of LeaseRepository
type MockLeaseRepository struct {
	mock.Mock
}

func (m *MockLeaseRepository) Save(ctx context.Context, lease *Lease) error {
	args := m.Called(ctx, lease)
	return args.Error(0)
}

func (m *MockLeaseRepository) Get(ctx context.Context, id string) (*Lease, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(*Lease), args.Error(1)
}

func (m *MockLeaseRepository) GetByMAC(ctx context.Context, mac string) (*Lease, error) {
	args := m.Called(ctx, mac)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Lease), args.Error(1)
}

func (m *MockLeaseRepository) GetByServerID(ctx context.Context, serverID string) ([]*Lease, error) {
	args := m.Called(ctx, serverID)
	return args.Get(0).([]*Lease), args.Error(1)
}

func (m *MockLeaseRepository) Delete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockLeaseRepository) DeleteByMAC(ctx context.Context, mac string) error {
	args := m.Called(ctx, mac)
	return args.Error(0)
}

func (m *MockLeaseRepository) DeleteByServerID(ctx context.Context, serverID string) error {
	args := m.Called(ctx, serverID)
	return args.Error(0)
}

func (m *MockLeaseRepository) GetExpired(ctx context.Context) ([]*Lease, error) {
	args := m.Called(ctx)
	return args.Get(0).([]*Lease), args.Error(1)
}

func (m *MockLeaseRepository) CleanupExpired(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// TestDHCPServerService_CreateServer tests server creation
func TestDHCPServerService_CreateServer(t *testing.T) {
	ctx := context.Background()
	mockServerRepo := &MockServerRepository{}
	mockLeaseRepo := &MockLeaseRepository{}

	service := NewDHCPServerService(mockServerRepo, mockLeaseRepo)

	config := ServerConfig{
		IP:            net.ParseIP("192.168.1.10"),
		SubnetMask:    net.ParseIP("255.255.255.0"),
		Gateway:       net.ParseIP("192.168.1.1"),
		DNS:           net.ParseIP("8.8.8.8"),
		StartIP:       net.ParseIP("192.168.1.100"),
		LeaseRange:    50,
		LeaseDuration: 2 * time.Hour,
	}

	// Mock expectations
	mockServerRepo.On("GetByIP", ctx, config.IP).Return(nil, assert.AnError)
	mockServerRepo.On("Save", ctx, mock.AnythingOfType("*dhcp.Server")).Return(nil)

	// Execute
	server, err := service.CreateServer(ctx, config)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, server)
	assert.Equal(t, config.IP, server.IP)
	assert.Equal(t, config.LeaseRange, server.LeaseRange)
	assert.False(t, server.Started)

	mockServerRepo.AssertExpectations(t)
}

// TestDHCPServerService_CreateServer_DuplicateIP tests duplicate IP handling
func TestDHCPServerService_CreateServer_DuplicateIP(t *testing.T) {
	ctx := context.Background()
	mockServerRepo := &MockServerRepository{}
	mockLeaseRepo := &MockLeaseRepository{}

	service := NewDHCPServerService(mockServerRepo, mockLeaseRepo)

	config := ServerConfig{
		IP:            net.ParseIP("192.168.1.10"),
		SubnetMask:    net.ParseIP("255.255.255.0"),
		Gateway:       net.ParseIP("192.168.1.1"),
		DNS:           net.ParseIP("8.8.8.8"),
		StartIP:       net.ParseIP("192.168.1.100"),
		LeaseRange:    50,
		LeaseDuration: 2 * time.Hour,
	}

	existingServer := &Server{
		ID: "existing-server",
		IP: config.IP,
	}

	// Mock expectations
	mockServerRepo.On("GetByIP", ctx, config.IP).Return(existingServer, nil)

	// Execute
	server, err := service.CreateServer(ctx, config)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, server)
	assert.Contains(t, err.Error(), "already exists")

	mockServerRepo.AssertExpectations(t)
}

// TestDHCPLeaseService_AssignLease tests lease assignment
func TestDHCPLeaseService_AssignLease(t *testing.T) {
	ctx := context.Background()
	mockServerRepo := &MockServerRepository{}
	mockLeaseRepo := &MockLeaseRepository{}

	service := NewDHCPLeaseService(mockLeaseRepo, mockServerRepo)

	serverID := "test-server"
	mac := "00:11:22:33:44:55"
	requestedIP := net.ParseIP("192.168.1.100")

	server := &Server{
		ID:            serverID,
		IP:            net.ParseIP("192.168.1.1"),
		IPStart:       net.ParseIP("192.168.1.100"),
		LeaseRange:    50,
		LeaseDuration: 2 * time.Hour,
	}

	// Mock expectations
	mockServerRepo.On("Get", ctx, serverID).Return(server, nil)
	mockLeaseRepo.On("GetByMAC", ctx, mac).Return(nil, assert.AnError)       // No existing lease
	mockLeaseRepo.On("GetByServerID", ctx, serverID).Return([]*Lease{}, nil) // No existing leases
	mockLeaseRepo.On("Save", ctx, mock.AnythingOfType("*dhcp.Lease")).Return(nil)

	// Execute
	lease, err := service.AssignLease(ctx, serverID, mac, requestedIP)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, lease)
	assert.Equal(t, mac, lease.MAC)
	assert.Equal(t, requestedIP, lease.IP)
	assert.Equal(t, serverID, lease.ServerID)
	assert.False(t, lease.Reserved)

	mockServerRepo.AssertExpectations(t)
	mockLeaseRepo.AssertExpectations(t)
}
