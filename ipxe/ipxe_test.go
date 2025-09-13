package ipxe

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ignite/config"
	"ignite/osimage"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockOSImageService is a mock implementation of osimage.OSImageService
type MockOSImageService struct {
	mock.Mock
}

func (m *MockOSImageService) GetAllOSImages(ctx context.Context) ([]*osimage.OSImage, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*osimage.OSImage), args.Error(1)
}

func (m *MockOSImageService) GetOSImagesByOS(ctx context.Context, os string) ([]*osimage.OSImage, error) {
	args := m.Called(ctx, os)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*osimage.OSImage), args.Error(1)
}

func (m *MockOSImageService) GetOSImage(ctx context.Context, id string) (*osimage.OSImage, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*osimage.OSImage), args.Error(1)
}

func (m *MockOSImageService) GetDefaultVersion(ctx context.Context, os string) (*osimage.OSImage, error) {
	args := m.Called(ctx, os)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*osimage.OSImage), args.Error(1)
}

func (m *MockOSImageService) SetDefaultVersion(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockOSImageService) DownloadOSImage(ctx context.Context, osConfig osimage.OSImageConfig) (*osimage.DownloadStatus, error) {
	args := m.Called(ctx, osConfig)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*osimage.DownloadStatus), args.Error(1)
}

func (m *MockOSImageService) DeleteOSImage(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockOSImageService) GetAvailableVersions(ctx context.Context, os string) ([]string, error) {
	args := m.Called(ctx, os)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockOSImageService) GetDownloadStatus(ctx context.Context, id string) (*osimage.DownloadStatus, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*osimage.DownloadStatus), args.Error(1)
}

func (m *MockOSImageService) GetActiveDownloads(ctx context.Context) ([]*osimage.DownloadStatus, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*osimage.DownloadStatus), args.Error(1)
}

func (m *MockOSImageService) CancelDownload(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// Helper function to create test config
func createTestConfig() *config.Config {
	return &config.Config{
		HTTP: config.HTTPConfig{
			Port: "8080",
		},
		TFTP: config.TFTPConfig{
			Dir: "/tmp/test-tftp",
		},
		OSImages: config.OSImageConfig{
			Sources: map[string]config.OSDefinition{
				"ubuntu": {
					KernelFile: "vmlinuz",
					InitrdFile: "initrd",
				},
				"centos": {
					KernelFile: "vmlinuz",
					InitrdFile: "initrd.img",
				},
				"nixos": {
					KernelFile: "bzImage-x86_64-linux",
					InitrdFile: "initrd-x86_64-linux",
				},
			},
		},
	}
}

// Helper function to create test OS images
func createTestOSImages() []*osimage.OSImage {
	return []*osimage.OSImage{
		{
			ID:      "ubuntu-22.04",
			OS:      "ubuntu",
			Version: "22.04",
		},
		{
			ID:      "centos-stream-9",
			OS:      "centos",
			Version: "stream-9",
		},
		{
			ID:      "nixos-23.11",
			OS:      "nixos",
			Version: "23.11",
		},
	}
}

// Test NewService
func TestNewService(t *testing.T) {
	cfg := createTestConfig()
	mockOSService := &MockOSImageService{}

	service := NewService(cfg, mockOSService)

	assert.NotNil(t, service)
	assert.Equal(t, cfg, service.config)
	assert.Equal(t, mockOSService, service.osImageService)
}

// Test GenerateConfig with successful OS images
func TestGenerateConfig_Success(t *testing.T) {
	ctx := context.Background()
	cfg := createTestConfig()
	mockOSService := &MockOSImageService{}
	service := NewService(cfg, mockOSService)

	osImages := createTestOSImages()
	mockOSService.On("GetAllOSImages", ctx).Return(osImages, nil)

	config, err := service.GenerateConfig(ctx)

	assert.NoError(t, err)
	assert.NotEmpty(t, config)

	// Check for iPXE script header
	assert.Contains(t, config, "#!ipxe")
	assert.Contains(t, config, "dhcp")

	// Check for OS image entries
	assert.Contains(t, config, "Ubuntu 22.04")
	assert.Contains(t, config, "CentOS Stream stream-9")
	assert.Contains(t, config, "NixOS 23.11")

	// Check for kernel and initrd paths
	assert.Contains(t, config, "ubuntu/22.04/vmlinuz")
	assert.Contains(t, config, "ubuntu/22.04/initrd")
	assert.Contains(t, config, "centos/stream-9/vmlinuz")
	assert.Contains(t, config, "centos/stream-9/initrd.img")
	assert.Contains(t, config, "nixos/23.11/bzImage-x86_64-linux")
	assert.Contains(t, config, "nixos/23.11/initrd-x86_64-linux")

	// Check for menu structure
	assert.Contains(t, config, ":menu")
	assert.Contains(t, config, "choose selected")
	assert.Contains(t, config, ":ubuntu")
	assert.Contains(t, config, ":centos")
	assert.Contains(t, config, ":nixos")

	mockOSService.AssertExpectations(t)
}

// Test GenerateConfig with OSImageService error
func TestGenerateConfig_OSImageServiceError(t *testing.T) {
	ctx := context.Background()
	cfg := createTestConfig()
	mockOSService := &MockOSImageService{}
	service := NewService(cfg, mockOSService)

	mockOSService.On("GetAllOSImages", ctx).Return(nil, errors.New("database error"))

	config, err := service.GenerateConfig(ctx)

	assert.Error(t, err)
	assert.Empty(t, config)
	assert.Contains(t, err.Error(), "failed to get OS images")

	mockOSService.AssertExpectations(t)
}

// Test GenerateConfig with no OS images
func TestGenerateConfig_NoOSImages(t *testing.T) {
	ctx := context.Background()
	cfg := createTestConfig()
	mockOSService := &MockOSImageService{}
	service := NewService(cfg, mockOSService)

	mockOSService.On("GetAllOSImages", ctx).Return([]*osimage.OSImage{}, nil)

	config, err := service.GenerateConfig(ctx)

	assert.NoError(t, err)
	assert.NotEmpty(t, config)

	// Should still have basic iPXE structure
	assert.Contains(t, config, "#!ipxe")
	assert.Contains(t, config, ":menu")

	// But no OS-specific entries
	assert.NotContains(t, config, ":ubuntu")
	assert.NotContains(t, config, ":centos")

	mockOSService.AssertExpectations(t)
}

// Test getDisplayName
func TestGetDisplayName(t *testing.T) {
	cfg := createTestConfig()
	service := NewService(cfg, nil)

	tests := []struct {
		os       string
		version  string
		expected string
	}{
		{"ubuntu", "22.04", "Ubuntu 22.04"},
		{"centos", "stream-9", "CentOS Stream stream-9"},
		{"nixos", "23.11", "NixOS 23.11"},
		{"debian", "12", "Debian 12"},
		{"fedora", "39", "Fedora 39"},
	}

	for _, test := range tests {
		result := service.getDisplayName(test.os, test.version)
		assert.Equal(t, test.expected, result, "OS: %s, Version: %s", test.os, test.version)
	}
}

// Test getKernelFilename
func TestGetKernelFilename(t *testing.T) {
	cfg := createTestConfig()
	service := NewService(cfg, nil)

	tests := []struct {
		os       string
		expected string
	}{
		{"ubuntu", "vmlinuz"},
		{"centos", "vmlinuz"},
		{"nixos", "bzImage-x86_64-linux"},
		{"unknown", "vmlinuz"}, // fallback
	}

	for _, test := range tests {
		result := service.getKernelFilename(test.os)
		assert.Equal(t, test.expected, result, "OS: %s", test.os)
	}
}

// Test getInitrdFilename
func TestGetInitrdFilename(t *testing.T) {
	cfg := createTestConfig()
	service := NewService(cfg, nil)

	tests := []struct {
		os       string
		expected string
	}{
		{"ubuntu", "initrd"},
		{"centos", "initrd.img"},
		{"nixos", "initrd-x86_64-linux"},
		{"unknown", "initrd.img"}, // fallback
	}

	for _, test := range tests {
		result := service.getInitrdFilename(test.os)
		assert.Equal(t, test.expected, result, "OS: %s", test.os)
	}
}

// Test getKernelArgs
func TestGetKernelArgs(t *testing.T) {
	cfg := createTestConfig()
	service := NewService(cfg, nil)
	serverIP := "192.168.1.100"

	tests := []struct {
		os       string
		contains []string
	}{
		{"ubuntu", []string{"boot=casper", "netboot=url", "fetch=http://192.168.1.100:8080/ubuntu/"}},
		{"centos", []string{"inst.repo=http://192.168.1.100:8080/centos/", "quiet"}},
		{"nixos", []string{"init=/nix/store", "boot.shell_on_fail", "console=ttyS0"}},
		{"unknown", []string{"quiet"}},
	}

	for _, test := range tests {
		result := service.getKernelArgs(test.os, serverIP)
		for _, expected := range test.contains {
			assert.Contains(t, result, expected, "OS: %s should contain: %s", test.os, expected)
		}
	}
}

// Test getServerIP
func TestGetServerIP(t *testing.T) {
	cfg := createTestConfig()
	service := NewService(cfg, nil)

	// This test is a bit tricky since it depends on actual network interfaces
	// We'll just test that it doesn't panic and returns something
	ip, err := service.getServerIP()

	// It might succeed or fail depending on the system
	if err == nil {
		// If it succeeds, should be a valid IP
		parsedIP := net.ParseIP(ip)
		assert.NotNil(t, parsedIP, "Should return a valid IP address")
		assert.False(t, parsedIP.IsLoopback(), "Should not return loopback address")
	} else {
		// If it fails, should have a meaningful error
		assert.Contains(t, err.Error(), "no suitable network interface found")
	}
}

// Test renderTemplate
func TestRenderTemplate(t *testing.T) {
	cfg := createTestConfig()
	service := NewService(cfg, nil)

	data := iPXEConfig{
		ServerIP: "192.168.1.100",
		HTTPPort: "8080",
		BaseURL:  "http://192.168.1.100:8080",
		OSImages: []OSImageEntry{
			{
				ID:          "ubuntu",
				Name:        "ubuntu",
				DisplayName: "Ubuntu 22.04",
				KernelPath:  "ubuntu/22.04/vmlinuz",
				InitrdPath:  "ubuntu/22.04/initrd",
				KernelArgs:  "boot=casper netboot=url",
			},
		},
	}

	result, err := service.renderTemplate(data)

	assert.NoError(t, err)
	assert.NotEmpty(t, result)

	// Check template rendering
	assert.Contains(t, result, "#!ipxe")
	assert.Contains(t, result, "set server-ip 192.168.1.100")
	assert.Contains(t, result, "set base-url http://192.168.1.100:8080")
	assert.Contains(t, result, "item ubuntu Ubuntu 22.04")
	assert.Contains(t, result, ":ubuntu")
	assert.Contains(t, result, "kernel ${base-url}/ubuntu/22.04/vmlinuz")
	assert.Contains(t, result, "initrd ${base-url}/ubuntu/22.04/initrd")
}

// Test WriteConfigToFile
func TestWriteConfigToFile(t *testing.T) {
	ctx := context.Background()

	// Create temporary directory for test
	tempDir := t.TempDir()

	cfg := createTestConfig()
	cfg.TFTP.Dir = tempDir

	mockOSService := &MockOSImageService{}
	service := NewService(cfg, mockOSService)

	osImages := createTestOSImages()
	mockOSService.On("GetAllOSImages", ctx).Return(osImages, nil)

	err := service.WriteConfigToFile(ctx)

	assert.NoError(t, err)

	// Check that file was created
	filePath := filepath.Join(tempDir, "boot.ipxe")
	assert.FileExists(t, filePath)

	// Check file contents
	content, err := os.ReadFile(filePath)
	assert.NoError(t, err)
	assert.Contains(t, string(content), "#!ipxe")
	assert.Contains(t, string(content), "Ubuntu 22.04")

	mockOSService.AssertExpectations(t)
}

// Test WriteConfigToFile with GenerateConfig error
func TestWriteConfigToFile_GenerateConfigError(t *testing.T) {
	ctx := context.Background()

	tempDir := t.TempDir()
	cfg := createTestConfig()
	cfg.TFTP.Dir = tempDir

	mockOSService := &MockOSImageService{}
	service := NewService(cfg, mockOSService)

	mockOSService.On("GetAllOSImages", ctx).Return(nil, errors.New("service error"))

	err := service.WriteConfigToFile(ctx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to generate iPXE config")

	// File should not exist
	filePath := filepath.Join(tempDir, "boot.ipxe")
	assert.NoFileExists(t, filePath)

	mockOSService.AssertExpectations(t)
}

// Test WriteConfigToFile with invalid directory
func TestWriteConfigToFile_InvalidDirectory(t *testing.T) {
	ctx := context.Background()

	cfg := createTestConfig()
	cfg.TFTP.Dir = "/nonexistent/directory"

	mockOSService := &MockOSImageService{}
	service := NewService(cfg, mockOSService)

	osImages := createTestOSImages()
	mockOSService.On("GetAllOSImages", ctx).Return(osImages, nil)

	err := service.WriteConfigToFile(ctx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write iPXE config")

	mockOSService.AssertExpectations(t)
}

// Test OSImageEntry struct
func TestOSImageEntry(t *testing.T) {
	entry := OSImageEntry{
		ID:          "test-id",
		Name:        "test-name",
		DisplayName: "Test Display Name",
		KernelPath:  "test/kernel/path",
		InitrdPath:  "test/initrd/path",
		KernelArgs:  "test=args quiet",
	}

	assert.Equal(t, "test-id", entry.ID)
	assert.Equal(t, "test-name", entry.Name)
	assert.Equal(t, "Test Display Name", entry.DisplayName)
	assert.Equal(t, "test/kernel/path", entry.KernelPath)
	assert.Equal(t, "test/initrd/path", entry.InitrdPath)
	assert.Equal(t, "test=args quiet", entry.KernelArgs)
}

// Test iPXEConfig struct
func TestIPXEConfig(t *testing.T) {
	config := iPXEConfig{
		ServerIP: "192.168.1.100",
		HTTPPort: "8080",
		BaseURL:  "http://192.168.1.100:8080",
		OSImages: []OSImageEntry{
			{ID: "ubuntu", Name: "ubuntu"},
			{ID: "centos", Name: "centos"},
		},
	}

	assert.Equal(t, "192.168.1.100", config.ServerIP)
	assert.Equal(t, "8080", config.HTTPPort)
	assert.Equal(t, "http://192.168.1.100:8080", config.BaseURL)
	assert.Len(t, config.OSImages, 2)
	assert.Equal(t, "ubuntu", config.OSImages[0].ID)
	assert.Equal(t, "centos", config.OSImages[1].ID)
}

// Test template output structure and completeness
func TestTemplateStructure(t *testing.T) {
	cfg := createTestConfig()
	service := NewService(cfg, nil)

	data := iPXEConfig{
		ServerIP: "192.168.1.100",
		HTTPPort: "8080",
		BaseURL:  "http://192.168.1.100:8080",
		OSImages: []OSImageEntry{
			{
				ID:          "ubuntu",
				Name:        "ubuntu",
				DisplayName: "Ubuntu 22.04",
				KernelPath:  "ubuntu/22.04/vmlinuz",
				InitrdPath:  "ubuntu/22.04/initrd",
				KernelArgs:  "boot=casper netboot=url",
			},
		},
	}

	result, err := service.renderTemplate(data)
	assert.NoError(t, err)

	// Check essential iPXE structure
	lines := strings.Split(result, "\n")

	// Should start with shebang
	assert.True(t, strings.HasPrefix(lines[0], "#!ipxe"))

	// Should contain key iPXE commands
	assert.Contains(t, result, "dhcp")
	assert.Contains(t, result, "menu ")
	assert.Contains(t, result, "item ")
	assert.Contains(t, result, "choose ")
	assert.Contains(t, result, "kernel ")
	assert.Contains(t, result, "initrd ")
	assert.Contains(t, result, "boot")

	// Should contain menu options
	assert.Contains(t, result, "item local Boot from local disk")
	assert.Contains(t, result, "item shell iPXE Shell")
	assert.Contains(t, result, "item exit Exit to BIOS")
	assert.Contains(t, result, "item memtest Memory Test")

	// Should contain labels
	assert.Contains(t, result, ":menu")
	assert.Contains(t, result, ":local")
	assert.Contains(t, result, ":shell")
	assert.Contains(t, result, ":exit")
	assert.Contains(t, result, ":memtest")
}
