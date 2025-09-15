package interfaces

import (
	"context"
	"testing"
	"time"
)

// MockManagedService implements ManagedService for testing
type MockManagedService struct {
	*MockService
	enabled     bool
	subscribers []chan ServiceEvent
}

func NewMockManagedService(id, name string) *MockManagedService {
	return &MockManagedService{
		MockService: NewMockService(id, name),
		enabled:     false,
		subscribers: make([]chan ServiceEvent, 0),
	}
}

func (m *MockManagedService) Enable() error {
	m.enabled = true
	return nil
}

func (m *MockManagedService) Disable() error {
	m.enabled = false
	return nil
}

func (m *MockManagedService) IsEnabled() bool {
	return m.enabled
}

func (m *MockManagedService) GetLogs(ctx context.Context, lines int) ([]string, error) {
	logs := make([]string, lines)
	for i := 0; i < lines; i++ {
		logs[i] = "Mock log line " + string(rune('0'+i))
	}
	return logs, nil
}

func (m *MockManagedService) Subscribe() <-chan ServiceEvent {
	ch := make(chan ServiceEvent, 10)
	m.subscribers = append(m.subscribers, ch)
	return ch
}

func (m *MockManagedService) Unsubscribe() {
	for _, ch := range m.subscribers {
		close(ch)
	}
	m.subscribers = make([]chan ServiceEvent, 0)
}

// MockNetworkService implements NetworkService for testing
type MockNetworkService struct {
	*MockService
	addresses []string
	port      int
}

func NewMockNetworkService(id, name string, port int) *MockNetworkService {
	return &MockNetworkService{
		MockService: NewMockService(id, name),
		addresses:   []string{"127.0.0.1", "0.0.0.0"},
		port:        port,
	}
}

func (m *MockNetworkService) GetListenAddresses() []string {
	return m.addresses
}

func (m *MockNetworkService) GetPort() int {
	return m.port
}

func (m *MockNetworkService) IsPortInUse(port int) bool {
	return port == m.port
}

func (m *MockNetworkService) BindToAddress(address string) error {
	m.addresses = []string{address}
	return nil
}

func (m *MockNetworkService) ChangePort(port int) error {
	m.port = port
	return nil
}

// MockStorageService implements StorageService for testing
type MockStorageService struct {
	*MockService
	storagePath string
	used        int64
	available   int64
}

func NewMockStorageService(id, name, path string) *MockStorageService {
	return &MockStorageService{
		MockService: NewMockService(id, name),
		storagePath: path,
		used:        1024 * 1024 * 100, // 100MB used
		available:   1024 * 1024 * 900, // 900MB available
	}
}

func (m *MockStorageService) GetStoragePath() string {
	return m.storagePath
}

func (m *MockStorageService) GetStorageUsage() (used, available int64, err error) {
	return m.used, m.available, nil
}

func (m *MockStorageService) CleanupStorage() error {
	m.used = 0
	return nil
}

func (m *MockStorageService) BackupData(destination string) error {
	// Mock backup operation
	return nil
}

func (m *MockStorageService) RestoreData(source string) error {
	// Mock restore operation
	return nil
}

// MockServiceManager implements ServiceManager for testing
type MockServiceManager struct {
	services    map[string]Service
	subscribers []chan ServiceEvent
}

func NewMockServiceManager() *MockServiceManager {
	return &MockServiceManager{
		services:    make(map[string]Service),
		subscribers: make([]chan ServiceEvent, 0),
	}
}

func (m *MockServiceManager) RegisterService(name string, service Service) error {
	m.services[name] = service
	return nil
}

func (m *MockServiceManager) UnregisterService(name string) error {
	delete(m.services, name)
	return nil
}

func (m *MockServiceManager) GetService(name string) (Service, error) {
	if service, exists := m.services[name]; exists {
		return service, nil
	}
	return nil, ErrServiceNotFound
}

func (m *MockServiceManager) ListServices() []ServiceInfo {
	infos := make([]ServiceInfo, 0, len(m.services))
	for _, service := range m.services {
		infos = append(infos, service.GetInfo())
	}
	return infos
}

func (m *MockServiceManager) StartAll(ctx context.Context) error {
	for _, service := range m.services {
		if err := service.Start(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (m *MockServiceManager) StopAll(ctx context.Context) error {
	for _, service := range m.services {
		if err := service.Stop(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (m *MockServiceManager) RestartAll(ctx context.Context) error {
	for _, service := range m.services {
		if err := service.Restart(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (m *MockServiceManager) HealthCheckAll() map[string]*HealthCheck {
	health := make(map[string]*HealthCheck)
	for name, service := range m.services {
		health[name] = service.HealthCheck()
	}
	return health
}

func (m *MockServiceManager) GetOverallHealth() *HealthCheck {
	return &HealthCheck{
		Status:    "healthy",
		LastCheck: time.Now(),
		Message:   "All services are healthy",
	}
}

func (m *MockServiceManager) Subscribe() <-chan ServiceEvent {
	ch := make(chan ServiceEvent, 10)
	m.subscribers = append(m.subscribers, ch)
	return ch
}

func (m *MockServiceManager) Unsubscribe() {
	for _, ch := range m.subscribers {
		close(ch)
	}
	m.subscribers = make([]chan ServiceEvent, 0)
}

// Custom error for testing
var ErrServiceNotFound = &ServiceError{
	Code:    "SERVICE_NOT_FOUND",
	Message: "Service not found",
}

type ServiceError struct {
	Code    string
	Message string
}

func (e *ServiceError) Error() string {
	return e.Message
}

// Test interface compliance
func TestInterfaceCompliance(t *testing.T) {
	// Test Service interface
	var _ Service = &MockService{}

	// Test ManagedService interface
	var _ ManagedService = &MockManagedService{}
	var _ Service = &MockManagedService{} // ManagedService should also implement Service

	// Test NetworkService interface
	var _ NetworkService = &MockNetworkService{}
	var _ Service = &MockNetworkService{} // NetworkService should also implement Service

	// Test StorageService interface
	var _ StorageService = &MockStorageService{}
	var _ Service = &MockStorageService{} // StorageService should also implement Service

	// Test ServiceManager interface
	var _ ServiceManager = &MockServiceManager{}
}

func TestManagedService_EnableDisable(t *testing.T) {
	service := NewMockManagedService("managed-1", "Managed Service")

	// Initially disabled
	if service.IsEnabled() {
		t.Error("Expected service to be disabled initially")
	}

	// Enable service
	err := service.Enable()
	if err != nil {
		t.Errorf("Failed to enable service: %v", err)
	}

	if !service.IsEnabled() {
		t.Error("Expected service to be enabled")
	}

	// Disable service
	err = service.Disable()
	if err != nil {
		t.Errorf("Failed to disable service: %v", err)
	}

	if service.IsEnabled() {
		t.Error("Expected service to be disabled")
	}
}

func TestManagedService_Logs(t *testing.T) {
	service := NewMockManagedService("managed-2", "Managed Service 2")
	ctx := context.Background()

	logs, err := service.GetLogs(ctx, 5)
	if err != nil {
		t.Errorf("Failed to get logs: %v", err)
	}

	if len(logs) != 5 {
		t.Errorf("Expected 5 log lines, got %d", len(logs))
	}

	for i, log := range logs {
		expected := "Mock log line " + string(rune('0'+i))
		if log != expected {
			t.Errorf("Expected log line %d to be '%s', got '%s'", i, expected, log)
		}
	}
}

func TestManagedService_Subscribe(t *testing.T) {
	service := NewMockManagedService("managed-3", "Managed Service 3")

	// Subscribe to events
	eventChan := service.Subscribe()
	if eventChan == nil {
		t.Fatal("Expected non-nil event channel")
	}

	// Test that channel is ready to receive
	select {
	case <-eventChan:
		// Channel received something (unexpected for this test)
	default:
		// Channel is ready but no events (expected)
	}

	// Unsubscribe
	service.Unsubscribe()

	// After unsubscribe, channel should be closed
	select {
	case _, open := <-eventChan:
		if open {
			t.Error("Expected channel to be closed after unsubscribe")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Channel should have been closed immediately")
	}
}

func TestNetworkService_AddressManagement(t *testing.T) {
	service := NewMockNetworkService("network-1", "Network Service", 8080)

	// Check initial addresses
	addresses := service.GetListenAddresses()
	if len(addresses) != 2 {
		t.Errorf("Expected 2 initial addresses, got %d", len(addresses))
	}

	// Check initial port
	if service.GetPort() != 8080 {
		t.Errorf("Expected initial port to be 8080, got %d", service.GetPort())
	}

	// Check port in use
	if !service.IsPortInUse(8080) {
		t.Error("Expected port 8080 to be in use")
	}

	if service.IsPortInUse(8081) {
		t.Error("Expected port 8081 to not be in use")
	}

	// Bind to new address
	err := service.BindToAddress("192.168.1.100")
	if err != nil {
		t.Errorf("Failed to bind to address: %v", err)
	}

	newAddresses := service.GetListenAddresses()
	if len(newAddresses) != 1 || newAddresses[0] != "192.168.1.100" {
		t.Errorf("Expected single address '192.168.1.100', got %v", newAddresses)
	}

	// Change port
	err = service.ChangePort(9090)
	if err != nil {
		t.Errorf("Failed to change port: %v", err)
	}

	if service.GetPort() != 9090 {
		t.Errorf("Expected port to be 9090, got %d", service.GetPort())
	}
}

func TestStorageService_StorageManagement(t *testing.T) {
	service := NewMockStorageService("storage-1", "Storage Service", "/data")

	// Check storage path
	if service.GetStoragePath() != "/data" {
		t.Errorf("Expected storage path to be '/data', got %s", service.GetStoragePath())
	}

	// Check storage usage
	used, available, err := service.GetStorageUsage()
	if err != nil {
		t.Errorf("Failed to get storage usage: %v", err)
	}

	expectedUsed := int64(1024 * 1024 * 100) // 100MB
	if used != expectedUsed {
		t.Errorf("Expected used storage to be %d, got %d", expectedUsed, used)
	}

	expectedAvailable := int64(1024 * 1024 * 900) // 900MB
	if available != expectedAvailable {
		t.Errorf("Expected available storage to be %d, got %d", expectedAvailable, available)
	}

	// Cleanup storage
	err = service.CleanupStorage()
	if err != nil {
		t.Errorf("Failed to cleanup storage: %v", err)
	}

	// Check that used storage is now 0
	used, _, err = service.GetStorageUsage()
	if err != nil {
		t.Errorf("Failed to get storage usage after cleanup: %v", err)
	}

	if used != 0 {
		t.Errorf("Expected used storage to be 0 after cleanup, got %d", used)
	}

	// Test backup and restore
	err = service.BackupData("/backup/location")
	if err != nil {
		t.Errorf("Failed to backup data: %v", err)
	}

	err = service.RestoreData("/backup/location")
	if err != nil {
		t.Errorf("Failed to restore data: %v", err)
	}
}

func TestServiceManager_ServiceManagement(t *testing.T) {
	manager := NewMockServiceManager()
	ctx := context.Background()

	// Initially no services
	services := manager.ListServices()
	if len(services) != 0 {
		t.Errorf("Expected 0 initial services, got %d", len(services))
	}

	// Register services
	service1 := NewMockService("svc1", "Service 1")
	service2 := NewMockService("svc2", "Service 2")

	err := manager.RegisterService("service1", service1)
	if err != nil {
		t.Errorf("Failed to register service1: %v", err)
	}

	err = manager.RegisterService("service2", service2)
	if err != nil {
		t.Errorf("Failed to register service2: %v", err)
	}

	// Check services are registered
	services = manager.ListServices()
	if len(services) != 2 {
		t.Errorf("Expected 2 services after registration, got %d", len(services))
	}

	// Get specific service
	retrievedService, err := manager.GetService("service1")
	if err != nil {
		t.Errorf("Failed to get service1: %v", err)
	}

	if retrievedService.GetInfo().ID != "svc1" {
		t.Errorf("Expected retrieved service ID to be 'svc1', got %s", retrievedService.GetInfo().ID)
	}

	// Get non-existent service
	_, err = manager.GetService("nonexistent")
	if err == nil {
		t.Error("Expected error when getting non-existent service")
	}

	// Start all services
	err = manager.StartAll(ctx)
	if err != nil {
		t.Errorf("Failed to start all services: %v", err)
	}

	// Check services are running
	if !service1.IsRunning() {
		t.Error("Expected service1 to be running")
	}
	if !service2.IsRunning() {
		t.Error("Expected service2 to be running")
	}

	// Stop all services
	err = manager.StopAll(ctx)
	if err != nil {
		t.Errorf("Failed to stop all services: %v", err)
	}

	// Check services are stopped
	if service1.IsRunning() {
		t.Error("Expected service1 to be stopped")
	}
	if service2.IsRunning() {
		t.Error("Expected service2 to be stopped")
	}

	// Restart all services
	err = manager.RestartAll(ctx)
	if err != nil {
		t.Errorf("Failed to restart all services: %v", err)
	}

	// Check services are running again
	if !service1.IsRunning() {
		t.Error("Expected service1 to be running after restart")
	}
	if !service2.IsRunning() {
		t.Error("Expected service2 to be running after restart")
	}

	// Health check all
	healthMap := manager.HealthCheckAll()
	if len(healthMap) != 2 {
		t.Errorf("Expected 2 health checks, got %d", len(healthMap))
	}

	// Overall health
	overallHealth := manager.GetOverallHealth()
	if overallHealth == nil {
		t.Fatal("Expected non-nil overall health")
	}

	if overallHealth.Status != "healthy" {
		t.Errorf("Expected overall health to be 'healthy', got %s", overallHealth.Status)
	}

	// Unregister service
	err = manager.UnregisterService("service1")
	if err != nil {
		t.Errorf("Failed to unregister service1: %v", err)
	}

	services = manager.ListServices()
	if len(services) != 1 {
		t.Errorf("Expected 1 service after unregistration, got %d", len(services))
	}
}

func TestServiceManager_Events(t *testing.T) {
	manager := NewMockServiceManager()

	// Subscribe to events
	eventChan := manager.Subscribe()
	if eventChan == nil {
		t.Fatal("Expected non-nil event channel")
	}

	// Unsubscribe
	manager.Unsubscribe()

	// Check channel is closed
	select {
	case _, open := <-eventChan:
		if open {
			t.Error("Expected channel to be closed after unsubscribe")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Channel should have been closed immediately")
	}
}

// Test template type constants
func TestTemplateType_Constants(t *testing.T) {
	expectedTypes := map[string]string{
		TemplateTypeKickstart: "kickstart",
		TemplateTypePreseed:   "preseed",
		TemplateTypeAutoYaST:  "autoyast",
		TemplateTypeCloudInit: "cloud-init",
		TemplateTypeIPXE:      "ipxe",
	}

	for constant, expected := range expectedTypes {
		if constant != expected {
			t.Errorf("Expected template type constant to be '%s', got '%s'", expected, constant)
		}
	}
}
