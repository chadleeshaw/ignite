package interfaces

import (
	"context"
	"time"
)

// ServiceStatus represents the status of a service
type ServiceStatus string

const (
	StatusStopped  ServiceStatus = "stopped"
	StatusStarting ServiceStatus = "starting"
	StatusRunning  ServiceStatus = "running"
	StatusStopping ServiceStatus = "stopping"
	StatusError    ServiceStatus = "error"
	StatusUnknown  ServiceStatus = "unknown"
)

// ServiceInfo contains information about a service
type ServiceInfo struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Status      ServiceStatus          `json:"status"`
	Port        int                    `json:"port,omitempty"`
	StartTime   *time.Time             `json:"start_time,omitempty"`
	Config      map[string]interface{} `json:"config,omitempty"`
	Metrics     map[string]interface{} `json:"metrics,omitempty"`
	Health      *HealthCheck           `json:"health,omitempty"`
}

// HealthCheck represents service health information
type HealthCheck struct {
	Status     string            `json:"status"`
	LastCheck  time.Time         `json:"last_check"`
	Message    string            `json:"message,omitempty"`
	Details    map[string]string `json:"details,omitempty"`
	CheckCount int               `json:"check_count"`
	FailCount  int               `json:"fail_count"`
	Uptime     time.Duration     `json:"uptime"`
}

// ServiceOptions contains options for service configuration
type ServiceOptions struct {
	AutoStart     bool                   `json:"auto_start"`
	RestartOnFail bool                   `json:"restart_on_fail"`
	MaxRestarts   int                    `json:"max_restarts"`
	Config        map[string]interface{} `json:"config"`
	Environment   map[string]string      `json:"environment"`
	Dependencies  []string               `json:"dependencies"`
}

// Service defines the standard interface for all services
type Service interface {
	// Basic service lifecycle
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Restart(ctx context.Context) error

	// Service information
	GetInfo() ServiceInfo
	GetStatus() ServiceStatus
	IsRunning() bool

	// Health and monitoring
	HealthCheck() *HealthCheck
	GetMetrics() map[string]interface{}

	// Configuration
	Configure(options ServiceOptions) error
	GetConfig() map[string]interface{}
	ValidateConfig(config map[string]interface{}) error
}

// ManagedService extends Service with management capabilities
type ManagedService interface {
	Service

	// Management operations
	Enable() error
	Disable() error
	IsEnabled() bool

	// Logging and monitoring
	GetLogs(ctx context.Context, lines int) ([]string, error)
	Subscribe() <-chan ServiceEvent
	Unsubscribe()
}

// ServiceEvent represents events that can occur in a service
type ServiceEvent struct {
	Type      string                 `json:"type"`
	Service   string                 `json:"service"`
	Timestamp time.Time              `json:"timestamp"`
	Message   string                 `json:"message"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Level     string                 `json:"level"` // info, warning, error
}

// ServiceManager manages multiple services
type ServiceManager interface {
	// Service management
	RegisterService(name string, service Service) error
	UnregisterService(name string) error
	GetService(name string) (Service, error)
	ListServices() []ServiceInfo

	// Bulk operations
	StartAll(ctx context.Context) error
	StopAll(ctx context.Context) error
	RestartAll(ctx context.Context) error

	// Health monitoring
	HealthCheckAll() map[string]*HealthCheck
	GetOverallHealth() *HealthCheck

	// Events
	Subscribe() <-chan ServiceEvent
	Unsubscribe()
}

// ConfigurableService defines services that can be reconfigured
type ConfigurableService interface {
	Service

	// Configuration management
	UpdateConfig(config map[string]interface{}) error
	ReloadConfig() error
	GetConfigSchema() map[string]interface{}
	ExportConfig() (map[string]interface{}, error)
	ImportConfig(config map[string]interface{}) error
}

// NetworkService defines services that use network resources
type NetworkService interface {
	Service

	// Network information
	GetListenAddresses() []string
	GetPort() int
	IsPortInUse(port int) bool

	// Network configuration
	BindToAddress(address string) error
	ChangePort(port int) error
}

// StorageService defines services that manage storage
type StorageService interface {
	Service

	// Storage operations
	GetStoragePath() string
	GetStorageUsage() (used, available int64, err error)
	CleanupStorage() error
	BackupData(destination string) error
	RestoreData(source string) error
}

// DHCPService defines DHCP-specific operations
type DHCPService interface {
	NetworkService
	ConfigurableService

	// DHCP operations
	CreateServer(config map[string]interface{}) (string, error)
	DeleteServer(id string) error
	GetServers() ([]map[string]interface{}, error)

	// Lease management
	GetLeases(serverID string) ([]map[string]interface{}, error)
	ReserveLease(serverID, ip, mac string) error
	ReleaseLease(serverID, ip string) error

	// Configuration
	UpdateServerConfig(id string, config map[string]interface{}) error
}

// TFTPService defines TFTP-specific operations
type TFTPService interface {
	NetworkService
	StorageService

	// File operations
	ListFiles(path string) ([]FileInfo, error)
	GetFile(path string) ([]byte, error)
	PutFile(path string, data []byte) error
	DeleteFile(path string) error

	// Directory operations
	CreateDirectory(path string) error
	DeleteDirectory(path string) error
	GetDirectorySize(path string) (int64, error)
}

// OSImageService defines OS image management operations
type OSImageService interface {
	StorageService

	// Image management
	ListImages() ([]ImageInfo, error)
	DownloadImage(url, destination string) (string, error)
	DeleteImage(id string) error
	GetImageInfo(id string) (*ImageInfo, error)

	// Image operations
	ExtractImage(id, destination string) error
	VerifyImage(id string) error
	SetDefaultImage(id string) error
}

// ProvisionService defines provisioning operations
type ProvisionService interface {
	ConfigurableService

	// Template management
	ListTemplates() ([]TemplateInfo, error)
	GetTemplate(name string) (*TemplateInfo, error)
	SaveTemplate(name string, content []byte) error
	DeleteTemplate(name string) error

	// Provisioning operations
	GenerateConfig(template string, variables map[string]interface{}) ([]byte, error)
	ValidateTemplate(content []byte) error
}

// FileInfo represents file information
type FileInfo struct {
	Name        string            `json:"name"`
	Path        string            `json:"path"`
	Size        int64             `json:"size"`
	IsDirectory bool              `json:"is_directory"`
	ModTime     time.Time         `json:"mod_time"`
	Permissions string            `json:"permissions"`
	Owner       string            `json:"owner,omitempty"`
	Group       string            `json:"group,omitempty"`
	ContentType string            `json:"content_type,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// ImageInfo represents OS image information
type ImageInfo struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Architecture string            `json:"architecture"`
	Type         string            `json:"type"`
	Size         int64             `json:"size"`
	Checksum     string            `json:"checksum"`
	Downloaded   bool              `json:"downloaded"`
	IsDefault    bool              `json:"is_default"`
	Metadata     map[string]string `json:"metadata"`
	DownloadURL  string            `json:"download_url,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

// Template type constants
const (
	TemplateTypeKickstart = "kickstart"  // Red Hat/CentOS/Fedora
	TemplateTypePreseed   = "preseed"    // Debian/Ubuntu
	TemplateTypeAutoYaST  = "autoyast"   // SUSE/openSUSE
	TemplateTypeCloudInit = "cloud-init" // Modern cloud images
	TemplateTypeIPXE      = "ipxe"       // Custom boot scripts
)

// TemplateInfo represents template information
type TemplateInfo struct {
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	Description string            `json:"description"`
	Content     string            `json:"content"`
	Variables   []VariableInfo    `json:"variables"`
	Metadata    map[string]string `json:"metadata"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// VariableInfo represents template variable information
type VariableInfo struct {
	Name         string      `json:"name"`
	Type         string      `json:"type"`
	Description  string      `json:"description"`
	Required     bool        `json:"required"`
	DefaultValue interface{} `json:"default_value,omitempty"`
	Options      []string    `json:"options,omitempty"`
}

// ServiceEvent types
const (
	EventTypeStarted     = "started"
	EventTypeStopped     = "stopped"
	EventTypeRestarted   = "restarted"
	EventTypeConfigured  = "configured"
	EventTypeError       = "error"
	EventTypeHealthCheck = "health_check"
	EventTypeMetrics     = "metrics"
)

// Event levels
const (
	LevelInfo    = "info"
	LevelWarning = "warning"
	LevelError   = "error"
	LevelDebug   = "debug"
)
