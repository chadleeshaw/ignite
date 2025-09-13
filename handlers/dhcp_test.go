package handlers

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ignite/config"
	"ignite/dhcp"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockServerService is a mock implementation of dhcp.ServerService
type MockServerService struct {
	mock.Mock
}

func (m *MockServerService) CreateServer(ctx context.Context, config dhcp.ServerConfig) (*dhcp.Server, error) {
	args := m.Called(ctx, config)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dhcp.Server), args.Error(1)
}

func (m *MockServerService) GetServer(ctx context.Context, id string) (*dhcp.Server, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dhcp.Server), args.Error(1)
}

func (m *MockServerService) GetAllServers(ctx context.Context) ([]*dhcp.Server, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*dhcp.Server), args.Error(1)
}

func (m *MockServerService) UpdateServer(ctx context.Context, serverID string, config dhcp.ServerConfig) error {
	args := m.Called(ctx, serverID, config)
	return args.Error(0)
}

func (m *MockServerService) DeleteServer(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockServerService) StartServer(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockServerService) StopServer(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockServerService) GetServerStatus(ctx context.Context, id string) (bool, error) {
	args := m.Called(ctx, id)
	return args.Bool(0), args.Error(1)
}

// MockLeaseService is a mock implementation of dhcp.LeaseService
type MockLeaseService struct {
	mock.Mock
}

func (m *MockLeaseService) AssignLease(ctx context.Context, serverID, mac string, requestedIP net.IP) (*dhcp.Lease, error) {
	args := m.Called(ctx, serverID, mac, requestedIP)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dhcp.Lease), args.Error(1)
}

func (m *MockLeaseService) GetLease(ctx context.Context, id string) (*dhcp.Lease, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dhcp.Lease), args.Error(1)
}

func (m *MockLeaseService) GetLeasesByServer(ctx context.Context, serverID string) ([]*dhcp.Lease, error) {
	args := m.Called(ctx, serverID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*dhcp.Lease), args.Error(1)
}

func (m *MockLeaseService) ReserveLease(ctx context.Context, serverID, mac string, ip net.IP) error {
	args := m.Called(ctx, serverID, mac, ip)
	return args.Error(0)
}

func (m *MockLeaseService) UnreserveLease(ctx context.Context, mac string) error {
	args := m.Called(ctx, mac)
	return args.Error(0)
}

func (m *MockLeaseService) UpdateLease(ctx context.Context, lease *dhcp.Lease) error {
	args := m.Called(ctx, lease)
	return args.Error(0)
}

func (m *MockLeaseService) DeleteLease(ctx context.Context, mac string) error {
	args := m.Called(ctx, mac)
	return args.Error(0)
}

func (m *MockLeaseService) ReleaseLease(ctx context.Context, mac string) error {
	args := m.Called(ctx, mac)
	return args.Error(0)
}

func (m *MockLeaseService) GetLeaseByMAC(ctx context.Context, mac string) (*dhcp.Lease, error) {
	args := m.Called(ctx, mac)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dhcp.Lease), args.Error(1)
}

func (m *MockLeaseService) CleanupExpiredLeases(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockLeaseService) UpdateLeaseState(ctx context.Context, mac string, newState string, source string) error {
	args := m.Called(ctx, mac, newState, source)
	return args.Error(0)
}

func (m *MockLeaseService) RecordHeartbeat(ctx context.Context, mac string) error {
	args := m.Called(ctx, mac)
	return args.Error(0)
}

func (m *MockLeaseService) GetLeaseStateHistory(ctx context.Context, mac string) ([]dhcp.StateTransition, error) {
	args := m.Called(ctx, mac)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]dhcp.StateTransition), args.Error(1)
}

func (m *MockLeaseService) GetLeasesByState(ctx context.Context, state string) ([]*dhcp.Lease, error) {
	args := m.Called(ctx, state)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*dhcp.Lease), args.Error(1)
}

func (m *MockLeaseService) MarkOfflineLeases(ctx context.Context, offlineThreshold time.Duration) error {
	args := m.Called(ctx, offlineThreshold)
	return args.Error(0)
}

// Helper function to create test container
func createTestContainer() *Container {
	return &Container{
		Config: &config.Config{
			TFTP: config.TFTPConfig{
				Dir: "/tmp/test-tftp",
			},
		},
	}
}

// Test DHCPHandlers creation
func TestNewDHCPHandlers(t *testing.T) {
	container := createTestContainer()
	mockServerService := &MockServerService{}
	mockLeaseService := &MockLeaseService{}

	container.ServerService = mockServerService
	container.LeaseService = mockLeaseService

	handlers := NewDHCPHandlers(container)

	assert.NotNil(t, handlers)
	assert.Equal(t, mockServerService, handlers.serverService)
	assert.Equal(t, mockLeaseService, handlers.leaseService)
	assert.Equal(t, container.Config, handlers.config)
}

// Test GetDHCPServers success case
func TestDHCPHandlers_GetDHCPServers_Success(t *testing.T) {
	mockServerService := &MockServerService{}
	mockLeaseService := &MockLeaseService{}

	handlers := &DHCPHandlers{
		serverService: mockServerService,
		leaseService:  mockLeaseService,
		config:        createTestContainer().Config,
	}

	// Create test data
	servers := []*dhcp.Server{
		{
			ID:            "server-1",
			IP:            net.ParseIP("192.168.1.1"),
			IPStart:       net.ParseIP("192.168.1.100"),
			LeaseRange:    50,
			LeaseDuration: 2 * time.Hour,
			Started:       true,
		},
		{
			ID:            "server-2",
			IP:            net.ParseIP("192.168.2.1"),
			IPStart:       net.ParseIP("192.168.2.100"),
			LeaseRange:    30,
			LeaseDuration: 1 * time.Hour,
			Started:       false,
		},
	}

	leases := []*dhcp.Lease{
		{
			ID:       "lease-1",
			ServerID: "server-1",
			MAC:      "aa:bb:cc:dd:ee:ff",
			IP:       net.ParseIP("192.168.1.100"),
			State:    "bound",
		},
	}

	mockServerService.On("GetAllServers", mock.Anything).Return(servers, nil)
	mockLeaseService.On("GetLeasesByServer", mock.Anything, "server-1").Return(leases, nil)
	mockLeaseService.On("GetLeasesByServer", mock.Anything, "server-2").Return([]*dhcp.Lease{}, nil)

	// Create request
	req := httptest.NewRequest("GET", "/dhcp/servers", nil)
	w := httptest.NewRecorder()

	// Execute
	handlers.GetDHCPServers(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/html")

	mockServerService.AssertExpectations(t)
	mockLeaseService.AssertExpectations(t)
}

// Test GetDHCPServers error case
func TestDHCPHandlers_GetDHCPServers_Error(t *testing.T) {
	mockServerService := &MockServerService{}
	mockLeaseService := &MockLeaseService{}

	handlers := &DHCPHandlers{
		serverService: mockServerService,
		leaseService:  mockLeaseService,
		config:        createTestContainer().Config,
	}

	expectedError := errors.New("database error")
	mockServerService.On("GetAllServers", mock.Anything).Return(nil, expectedError)

	req := httptest.NewRequest("GET", "/dhcp/servers", nil)
	w := httptest.NewRecorder()

	handlers.GetDHCPServers(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	mockServerService.AssertExpectations(t)
}

// Test StartDHCPServer success
func TestDHCPHandlers_StartDHCPServer_Success(t *testing.T) {
	mockServerService := &MockServerService{}
	mockLeaseService := &MockLeaseService{}

	handlers := &DHCPHandlers{
		serverService: mockServerService,
		leaseService:  mockLeaseService,
		config:        createTestContainer().Config,
	}

	serverID := "test-server"
	mockServerService.On("StartServer", mock.Anything, serverID).Return(nil)

	req := httptest.NewRequest("POST", "/dhcp/start?server_id="+serverID, nil)
	w := httptest.NewRecorder()

	handlers.StartDHCPServer(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	mockServerService.AssertExpectations(t)
}

// Test StartDHCPServer missing server_id
func TestDHCPHandlers_StartDHCPServer_MissingID(t *testing.T) {
	mockServerService := &MockServerService{}
	mockLeaseService := &MockLeaseService{}

	handlers := &DHCPHandlers{
		serverService: mockServerService,
		leaseService:  mockLeaseService,
		config:        createTestContainer().Config,
	}

	req := httptest.NewRequest("POST", "/dhcp/start", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handlers.StartDHCPServer(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Server ID is required")
}

// Test StopDHCPServer success
func TestDHCPHandlers_StopDHCPServer_Success(t *testing.T) {
	mockServerService := &MockServerService{}
	mockLeaseService := &MockLeaseService{}

	handlers := &DHCPHandlers{
		serverService: mockServerService,
		leaseService:  mockLeaseService,
		config:        createTestContainer().Config,
	}

	serverID := "test-server"
	mockServerService.On("StopServer", mock.Anything, serverID).Return(nil)

	req := httptest.NewRequest("POST", "/dhcp/stop?server_id="+serverID, nil)
	w := httptest.NewRecorder()

	handlers.StopDHCPServer(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	mockServerService.AssertExpectations(t)
}

// Test DeleteDHCPServer success
func TestDHCPHandlers_DeleteDHCPServer_Success(t *testing.T) {
	mockServerService := &MockServerService{}
	mockLeaseService := &MockLeaseService{}

	handlers := &DHCPHandlers{
		serverService: mockServerService,
		leaseService:  mockLeaseService,
		config:        createTestContainer().Config,
	}

	serverID := "test-server"
	mockServerService.On("DeleteServer", mock.Anything, serverID).Return(nil)

	req := httptest.NewRequest("POST", "/dhcp/delete?server_id="+serverID, nil)
	w := httptest.NewRecorder()

	handlers.DeleteDHCPServer(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	mockServerService.AssertExpectations(t)
}

// Test ReserveLease success
func TestDHCPHandlers_ReserveLease_Success(t *testing.T) {
	mockServerService := &MockServerService{}
	mockLeaseService := &MockLeaseService{}

	handlers := &DHCPHandlers{
		serverService: mockServerService,
		leaseService:  mockLeaseService,
		config:        createTestContainer().Config,
	}

	serverID := "test-server"
	mac := "aa:bb:cc:dd:ee:ff"
	ip := net.ParseIP("192.168.1.100")

	mockLeaseService.On("ReserveLease", mock.Anything, serverID, mac, ip).Return(nil)

	req := httptest.NewRequest("POST", "/dhcp/submit_reserve?server_id="+serverID+"&mac="+mac+"&ip="+ip.String(), nil)
	w := httptest.NewRecorder()

	handlers.ReserveLease(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	mockLeaseService.AssertExpectations(t)
}

// Test ReserveLease with invalid MAC address
func TestDHCPHandlers_ReserveLease_InvalidMAC(t *testing.T) {
	mockServerService := &MockServerService{}
	mockLeaseService := &MockLeaseService{}

	handlers := &DHCPHandlers{
		serverService: mockServerService,
		leaseService:  mockLeaseService,
		config:        createTestContainer().Config,
	}

	req := httptest.NewRequest("POST", "/dhcp/submit_reserve?mac=aa:bb:cc:dd:ee:ff&ip=192.168.1.100", nil)
	w := httptest.NewRecorder()

	handlers.ReserveLease(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Server ID, MAC, and IP are required")
}

// Test UnreserveLease success
func TestDHCPHandlers_UnreserveLease_Success(t *testing.T) {
	mockServerService := &MockServerService{}
	mockLeaseService := &MockLeaseService{}

	handlers := &DHCPHandlers{
		serverService: mockServerService,
		leaseService:  mockLeaseService,
		config:        createTestContainer().Config,
	}

	mac := "aa:bb:cc:dd:ee:ff"
	mockLeaseService.On("UnreserveLease", mock.Anything, mac).Return(nil)

	req := httptest.NewRequest("POST", "/dhcp/remove_reserve?mac="+mac, nil)
	w := httptest.NewRecorder()

	handlers.UnreserveLease(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	mockLeaseService.AssertExpectations(t)
}

// Test DeleteLease success
func TestDHCPHandlers_DeleteLease_Success(t *testing.T) {
	mockServerService := &MockServerService{}
	mockLeaseService := &MockLeaseService{}

	handlers := &DHCPHandlers{
		serverService: mockServerService,
		leaseService:  mockLeaseService,
		config:        createTestContainer().Config,
	}

	mac := "aa:bb:cc:dd:ee:ff"
	mockLeaseService.On("ReleaseLease", mock.Anything, mac).Return(nil)

	req := httptest.NewRequest("POST", "/dhcp/delete_lease?mac="+mac, nil)
	w := httptest.NewRecorder()

	handlers.DeleteLease(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	mockLeaseService.AssertExpectations(t)
}

// Test GetLeaseStateHistory success
func TestDHCPHandlers_GetLeaseStateHistory_Success(t *testing.T) {
	mockServerService := &MockServerService{}
	mockLeaseService := &MockLeaseService{}

	handlers := &DHCPHandlers{
		serverService: mockServerService,
		leaseService:  mockLeaseService,
		config:        createTestContainer().Config,
	}

	mac := "aa:bb:cc:dd:ee:ff"
	history := []dhcp.StateTransition{
		{
			FromState: "",
			ToState:   "assigned",
			Timestamp: time.Now(),
			Source:    "dhcp",
		},
		{
			FromState: "assigned",
			ToState:   "pxe_requested",
			Timestamp: time.Now().Add(1 * time.Second),
			Source:    "pxe",
		},
	}

	mockLeaseService.On("GetLeaseStateHistory", mock.Anything, mac).Return(history, nil)

	req := httptest.NewRequest("GET", "/dhcp/lease_history?mac="+mac, nil)
	w := httptest.NewRecorder()

	handlers.GetLeaseStateHistory(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json")

	mockLeaseService.AssertExpectations(t)
}

// Test utility functions
func TestDHCPHandlers_getServerStatusBadge(t *testing.T) {
	handlers := &DHCPHandlers{}

	// Test started server
	badge := handlers.getServerStatusBadge(true)
	assert.Equal(t, "badge-success", badge)

	// Test stopped server
	badge = handlers.getServerStatusBadge(false)
	assert.Equal(t, "badge-error", badge)
}
