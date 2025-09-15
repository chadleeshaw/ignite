package interfaces

import (
	"context"
	"testing"
	"time"
)

// MockService implements the Service interface for testing
type MockService struct {
	id          string
	name        string
	status      ServiceStatus
	running     bool
	config      map[string]interface{}
	metrics     map[string]interface{}
	health      *HealthCheck
	startError  error
	stopError   error
	configError error
}

func NewMockService(id, name string) *MockService {
	return &MockService{
		id:      id,
		name:    name,
		status:  StatusStopped,
		running: false,
		config:  make(map[string]interface{}),
		metrics: make(map[string]interface{}),
		health: &HealthCheck{
			Status:    "ok",
			LastCheck: time.Now(),
			Message:   "Service is healthy",
		},
	}
}

func (m *MockService) Start(ctx context.Context) error {
	if m.startError != nil {
		return m.startError
	}
	m.status = StatusRunning
	m.running = true
	return nil
}

func (m *MockService) Stop(ctx context.Context) error {
	if m.stopError != nil {
		return m.stopError
	}
	m.status = StatusStopped
	m.running = false
	return nil
}

func (m *MockService) Restart(ctx context.Context) error {
	if err := m.Stop(ctx); err != nil {
		return err
	}
	return m.Start(ctx)
}

func (m *MockService) GetInfo() ServiceInfo {
	startTime := time.Now()
	return ServiceInfo{
		ID:          m.id,
		Name:        m.name,
		Description: "Mock service for testing",
		Status:      m.status,
		StartTime:   &startTime,
		Config:      m.config,
		Metrics:     m.metrics,
		Health:      m.health,
	}
}

func (m *MockService) GetStatus() ServiceStatus {
	return m.status
}

func (m *MockService) IsRunning() bool {
	return m.running
}

func (m *MockService) HealthCheck() *HealthCheck {
	return m.health
}

func (m *MockService) GetMetrics() map[string]interface{} {
	return m.metrics
}

func (m *MockService) Configure(options ServiceOptions) error {
	if m.configError != nil {
		return m.configError
	}
	m.config = options.Config
	return nil
}

func (m *MockService) GetConfig() map[string]interface{} {
	return m.config
}

func (m *MockService) ValidateConfig(config map[string]interface{}) error {
	if m.configError != nil {
		return m.configError
	}
	return nil
}

// Test Service interface implementation
func TestMockService_ImplementsService(t *testing.T) {
	var _ Service = &MockService{}
}

func TestMockService_Lifecycle(t *testing.T) {
	service := NewMockService("test-1", "Test Service")
	ctx := context.Background()

	// Initial state
	if service.GetStatus() != StatusStopped {
		t.Errorf("Expected initial status to be %s, got %s", StatusStopped, service.GetStatus())
	}

	if service.IsRunning() {
		t.Error("Expected service to not be running initially")
	}

	// Start service
	err := service.Start(ctx)
	if err != nil {
		t.Errorf("Failed to start service: %v", err)
	}

	if service.GetStatus() != StatusRunning {
		t.Errorf("Expected status to be %s after start, got %s", StatusRunning, service.GetStatus())
	}

	if !service.IsRunning() {
		t.Error("Expected service to be running after start")
	}

	// Stop service
	err = service.Stop(ctx)
	if err != nil {
		t.Errorf("Failed to stop service: %v", err)
	}

	if service.GetStatus() != StatusStopped {
		t.Errorf("Expected status to be %s after stop, got %s", StatusStopped, service.GetStatus())
	}

	if service.IsRunning() {
		t.Error("Expected service to not be running after stop")
	}

	// Restart service
	err = service.Restart(ctx)
	if err != nil {
		t.Errorf("Failed to restart service: %v", err)
	}

	if service.GetStatus() != StatusRunning {
		t.Errorf("Expected status to be %s after restart, got %s", StatusRunning, service.GetStatus())
	}
}

func TestMockService_Configuration(t *testing.T) {
	service := NewMockService("test-2", "Test Service 2")

	// Initial config should be empty
	config := service.GetConfig()
	if len(config) != 0 {
		t.Errorf("Expected empty initial config, got %d items", len(config))
	}

	// Configure service
	options := ServiceOptions{
		Config: map[string]interface{}{
			"timeout": 30,
			"retries": 3,
		},
	}

	err := service.Configure(options)
	if err != nil {
		t.Errorf("Failed to configure service: %v", err)
	}

	// Check config was applied
	newConfig := service.GetConfig()
	if len(newConfig) != 2 {
		t.Errorf("Expected 2 config items, got %d", len(newConfig))
	}

	if newConfig["timeout"] != 30 {
		t.Errorf("Expected timeout to be 30, got %v", newConfig["timeout"])
	}

	if newConfig["retries"] != 3 {
		t.Errorf("Expected retries to be 3, got %v", newConfig["retries"])
	}

	// Validate config
	err = service.ValidateConfig(newConfig)
	if err != nil {
		t.Errorf("Config validation failed: %v", err)
	}
}

func TestMockService_HealthAndMetrics(t *testing.T) {
	service := NewMockService("test-3", "Test Service 3")

	// Check health
	health := service.HealthCheck()
	if health == nil {
		t.Fatal("Expected health check to return non-nil")
	}

	if health.Status != "ok" {
		t.Errorf("Expected health status to be 'ok', got %s", health.Status)
	}

	// Check metrics
	metrics := service.GetMetrics()
	if metrics == nil {
		t.Fatal("Expected metrics to return non-nil map")
	}
}

func TestMockService_Info(t *testing.T) {
	service := NewMockService("test-4", "Test Service 4")

	info := service.GetInfo()

	if info.ID != "test-4" {
		t.Errorf("Expected ID to be 'test-4', got %s", info.ID)
	}

	if info.Name != "Test Service 4" {
		t.Errorf("Expected name to be 'Test Service 4', got %s", info.Name)
	}

	if info.Status != StatusStopped {
		t.Errorf("Expected status to be %s, got %s", StatusStopped, info.Status)
	}

	if info.StartTime == nil {
		t.Error("Expected start time to be set")
	}

	if info.Config == nil {
		t.Error("Expected config to be non-nil")
	}

	if info.Metrics == nil {
		t.Error("Expected metrics to be non-nil")
	}

	if info.Health == nil {
		t.Error("Expected health to be non-nil")
	}
}

// Test ServiceStatus constants
func TestServiceStatus_Constants(t *testing.T) {
	statuses := []ServiceStatus{
		StatusStopped,
		StatusStarting,
		StatusRunning,
		StatusStopping,
		StatusError,
		StatusUnknown,
	}

	expectedValues := []string{
		"stopped",
		"starting",
		"running",
		"stopping",
		"error",
		"unknown",
	}

	for i, status := range statuses {
		if string(status) != expectedValues[i] {
			t.Errorf("Expected status %d to be %s, got %s", i, expectedValues[i], string(status))
		}
	}
}

// Test HealthCheck structure
func TestHealthCheck_Structure(t *testing.T) {
	health := &HealthCheck{
		Status:     "healthy",
		LastCheck:  time.Now(),
		Message:    "All systems operational",
		Details:    map[string]string{"cpu": "50%", "memory": "60%"},
		CheckCount: 100,
		FailCount:  2,
		Uptime:     time.Hour * 24,
	}

	if health.Status != "healthy" {
		t.Error("Health status not set correctly")
	}

	if health.Message == "" {
		t.Error("Health message should not be empty")
	}

	if health.Details == nil {
		t.Error("Health details should not be nil")
	}

	if len(health.Details) != 2 {
		t.Errorf("Expected 2 health details, got %d", len(health.Details))
	}

	if health.CheckCount != 100 {
		t.Errorf("Expected check count to be 100, got %d", health.CheckCount)
	}

	if health.FailCount != 2 {
		t.Errorf("Expected fail count to be 2, got %d", health.FailCount)
	}
}

// Test ServiceOptions structure
func TestServiceOptions_Structure(t *testing.T) {
	options := ServiceOptions{
		AutoStart:     true,
		RestartOnFail: true,
		MaxRestarts:   5,
		Config: map[string]interface{}{
			"port": 8080,
			"host": "localhost",
		},
		Environment: map[string]string{
			"ENV":   "test",
			"DEBUG": "true",
		},
		Dependencies: []string{"database", "cache"},
	}

	if !options.AutoStart {
		t.Error("AutoStart should be true")
	}

	if !options.RestartOnFail {
		t.Error("RestartOnFail should be true")
	}

	if options.MaxRestarts != 5 {
		t.Errorf("Expected MaxRestarts to be 5, got %d", options.MaxRestarts)
	}

	if len(options.Config) != 2 {
		t.Errorf("Expected 2 config items, got %d", len(options.Config))
	}

	if len(options.Environment) != 2 {
		t.Errorf("Expected 2 environment variables, got %d", len(options.Environment))
	}

	if len(options.Dependencies) != 2 {
		t.Errorf("Expected 2 dependencies, got %d", len(options.Dependencies))
	}
}

// Test ServiceEvent structure
func TestServiceEvent_Structure(t *testing.T) {
	event := ServiceEvent{
		Type:      EventTypeStarted,
		Service:   "test-service",
		Timestamp: time.Now(),
		Message:   "Service started successfully",
		Data: map[string]interface{}{
			"port": 8080,
		},
		Level: LevelInfo,
	}

	if event.Type != EventTypeStarted {
		t.Errorf("Expected event type to be %s, got %s", EventTypeStarted, event.Type)
	}

	if event.Service != "test-service" {
		t.Errorf("Expected service to be 'test-service', got %s", event.Service)
	}

	if event.Message == "" {
		t.Error("Event message should not be empty")
	}

	if event.Level != LevelInfo {
		t.Errorf("Expected level to be %s, got %s", LevelInfo, event.Level)
	}
}

// Test event type constants
func TestEventType_Constants(t *testing.T) {
	events := []string{
		EventTypeStarted,
		EventTypeStopped,
		EventTypeRestarted,
		EventTypeConfigured,
		EventTypeError,
		EventTypeHealthCheck,
		EventTypeMetrics,
	}

	expectedValues := []string{
		"started",
		"stopped",
		"restarted",
		"configured",
		"error",
		"health_check",
		"metrics",
	}

	for i, event := range events {
		if event != expectedValues[i] {
			t.Errorf("Expected event %d to be %s, got %s", i, expectedValues[i], event)
		}
	}
}

// Test log level constants
func TestLogLevel_Constants(t *testing.T) {
	levels := []string{
		LevelInfo,
		LevelWarning,
		LevelError,
		LevelDebug,
	}

	expectedValues := []string{
		"info",
		"warning",
		"error",
		"debug",
	}

	for i, level := range levels {
		if level != expectedValues[i] {
			t.Errorf("Expected level %d to be %s, got %s", i, expectedValues[i], level)
		}
	}
}

// Test FileInfo structure
func TestFileInfo_Structure(t *testing.T) {
	fileInfo := FileInfo{
		Name:        "test.txt",
		Path:        "/path/to/test.txt",
		Size:        1024,
		IsDirectory: false,
		ModTime:     time.Now(),
		Permissions: "644",
		Owner:       "user",
		Group:       "group",
		ContentType: "text/plain",
		Metadata: map[string]string{
			"encoding": "utf-8",
		},
	}

	if fileInfo.Name != "test.txt" {
		t.Errorf("Expected name to be 'test.txt', got %s", fileInfo.Name)
	}

	if fileInfo.Size != 1024 {
		t.Errorf("Expected size to be 1024, got %d", fileInfo.Size)
	}

	if fileInfo.IsDirectory {
		t.Error("Expected IsDirectory to be false")
	}

	if fileInfo.Permissions != "644" {
		t.Errorf("Expected permissions to be '644', got %s", fileInfo.Permissions)
	}
}

// Test ImageInfo structure
func TestImageInfo_Structure(t *testing.T) {
	imageInfo := ImageInfo{
		ID:           "ubuntu-22.04",
		Name:         "Ubuntu 22.04 LTS",
		Version:      "22.04",
		Architecture: "amd64",
		Type:         "iso",
		Size:         3221225472, // 3GB
		Checksum:     "sha256:abc123",
		Downloaded:   true,
		IsDefault:    false,
		Metadata: map[string]string{
			"release_date": "2022-04-21",
		},
		DownloadURL: "https://releases.ubuntu.com/22.04/ubuntu-22.04-desktop-amd64.iso",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if imageInfo.ID != "ubuntu-22.04" {
		t.Errorf("Expected ID to be 'ubuntu-22.04', got %s", imageInfo.ID)
	}

	if imageInfo.Architecture != "amd64" {
		t.Errorf("Expected architecture to be 'amd64', got %s", imageInfo.Architecture)
	}

	if !imageInfo.Downloaded {
		t.Error("Expected Downloaded to be true")
	}

	if imageInfo.IsDefault {
		t.Error("Expected IsDefault to be false")
	}
}

// Test TemplateInfo structure
func TestTemplateInfo_Structure(t *testing.T) {
	templateInfo := TemplateInfo{
		Name:        "ubuntu-kickstart",
		Type:        "kickstart",
		Description: "Ubuntu kickstart template",
		Content:     "#Ubuntu kickstart configuration",
		Variables: []VariableInfo{
			{
				Name:         "hostname",
				Type:         "string",
				Description:  "System hostname",
				Required:     true,
				DefaultValue: "ubuntu-server",
			},
		},
		Metadata: map[string]string{
			"author": "system",
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if templateInfo.Name != "ubuntu-kickstart" {
		t.Errorf("Expected name to be 'ubuntu-kickstart', got %s", templateInfo.Name)
	}

	if templateInfo.Type != "kickstart" {
		t.Errorf("Expected type to be 'kickstart', got %s", templateInfo.Type)
	}

	if len(templateInfo.Variables) != 1 {
		t.Errorf("Expected 1 variable, got %d", len(templateInfo.Variables))
	}

	variable := templateInfo.Variables[0]
	if variable.Name != "hostname" {
		t.Errorf("Expected variable name to be 'hostname', got %s", variable.Name)
	}

	if !variable.Required {
		t.Error("Expected variable to be required")
	}
}

// Test VariableInfo structure
func TestVariableInfo_Structure(t *testing.T) {
	variable := VariableInfo{
		Name:         "disk_size",
		Type:         "integer",
		Description:  "Disk size in GB",
		Required:     true,
		DefaultValue: 20,
		Options:      []string{"10", "20", "50", "100"},
	}

	if variable.Name != "disk_size" {
		t.Errorf("Expected name to be 'disk_size', got %s", variable.Name)
	}

	if variable.Type != "integer" {
		t.Errorf("Expected type to be 'integer', got %s", variable.Type)
	}

	if !variable.Required {
		t.Error("Expected variable to be required")
	}

	if variable.DefaultValue != 20 {
		t.Errorf("Expected default value to be 20, got %v", variable.DefaultValue)
	}

	if len(variable.Options) != 4 {
		t.Errorf("Expected 4 options, got %d", len(variable.Options))
	}
}
