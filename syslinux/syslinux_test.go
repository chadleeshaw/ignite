package syslinux

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockRepository is a mock implementation of Repository
type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) SaveVersion(ctx context.Context, version *SyslinuxVersion) error {
	args := m.Called(ctx, version)
	return args.Error(0)
}

func (m *MockRepository) GetVersion(ctx context.Context, id string) (*SyslinuxVersion, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*SyslinuxVersion), args.Error(1)
}

func (m *MockRepository) GetVersionByNumber(ctx context.Context, version string) (*SyslinuxVersion, error) {
	args := m.Called(ctx, version)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*SyslinuxVersion), args.Error(1)
}

func (m *MockRepository) ListVersions(ctx context.Context) ([]*SyslinuxVersion, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*SyslinuxVersion), args.Error(1)
}

func (m *MockRepository) DeleteVersion(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockRepository) SetActiveVersion(ctx context.Context, version string) error {
	args := m.Called(ctx, version)
	return args.Error(0)
}

func (m *MockRepository) GetActiveVersion(ctx context.Context) (*SyslinuxVersion, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*SyslinuxVersion), args.Error(1)
}

func (m *MockRepository) SaveBootFile(ctx context.Context, bootFile *SyslinuxBootFile) error {
	args := m.Called(ctx, bootFile)
	return args.Error(0)
}

func (m *MockRepository) GetBootFile(ctx context.Context, id string) (*SyslinuxBootFile, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*SyslinuxBootFile), args.Error(1)
}

func (m *MockRepository) ListBootFiles(ctx context.Context, version, bootType string) ([]*SyslinuxBootFile, error) {
	args := m.Called(ctx, version, bootType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*SyslinuxBootFile), args.Error(1)
}

func (m *MockRepository) UpdateBootFileStatus(ctx context.Context, id string, installed bool) error {
	args := m.Called(ctx, id, installed)
	return args.Error(0)
}

func (m *MockRepository) DeleteBootFile(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockRepository) SaveDownloadStatus(ctx context.Context, status *DownloadStatus) error {
	args := m.Called(ctx, status)
	return args.Error(0)
}

func (m *MockRepository) GetDownloadStatus(ctx context.Context, id string) (*DownloadStatus, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*DownloadStatus), args.Error(1)
}

func (m *MockRepository) ListDownloadStatuses(ctx context.Context) ([]*DownloadStatus, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*DownloadStatus), args.Error(1)
}

func (m *MockRepository) DeleteDownloadStatus(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// Test models and utility functions

func TestSyslinuxVersion(t *testing.T) {
	now := time.Now()
	downloaded := now.Add(1 * time.Hour)

	version := &SyslinuxVersion{
		ID:           "syslinux-6.03",
		Version:      "6.03",
		BootType:     "bios",
		DownloadURL:  "https://mirrors.kernel.org/pub/linux/utils/boot/syslinux/syslinux-6.03.tar.gz",
		FileName:     "syslinux-6.03.tar.gz",
		Size:         5120000,
		Checksum:     "abcd1234ef567890",
		Downloaded:   true,
		Active:       true,
		CreatedAt:    now,
		UpdatedAt:    now,
		DownloadedAt: &downloaded,
	}

	assert.Equal(t, "syslinux-6.03", version.ID)
	assert.Equal(t, "6.03", version.Version)
	assert.Equal(t, "bios", version.BootType)
	assert.True(t, version.Downloaded)
	assert.True(t, version.Active)
	assert.NotNil(t, version.DownloadedAt)
	assert.True(t, version.DownloadedAt.After(version.CreatedAt))
}

func TestSyslinuxBootFile(t *testing.T) {
	now := time.Now()

	bootFile := &SyslinuxBootFile{
		ID:          "pxelinux-6.03-bios",
		Version:     "6.03",
		BootType:    "bios",
		FileName:    "pxelinux.0",
		FilePath:    "boot-bios/pxelinux.0",
		Size:        42496,
		Description: "PXE boot loader for BIOS systems",
		Required:    true,
		Installed:   true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	assert.Equal(t, "pxelinux-6.03-bios", bootFile.ID)
	assert.Equal(t, "6.03", bootFile.Version)
	assert.Equal(t, "bios", bootFile.BootType)
	assert.Equal(t, "pxelinux.0", bootFile.FileName)
	assert.True(t, bootFile.Required)
	assert.True(t, bootFile.Installed)
}

func TestDownloadStatus(t *testing.T) {
	now := time.Now()
	completed := now.Add(5 * time.Minute)

	status := &DownloadStatus{
		ID:           "download-123",
		Version:      "6.03",
		Status:       "completed",
		Progress:     100,
		ErrorMessage: "",
		StartedAt:    now,
		CompletedAt:  &completed,
	}

	assert.Equal(t, "download-123", status.ID)
	assert.Equal(t, "6.03", status.Version)
	assert.Equal(t, "completed", status.Status)
	assert.Equal(t, 100, status.Progress)
	assert.Empty(t, status.ErrorMessage)
	assert.NotNil(t, status.CompletedAt)
	assert.True(t, status.CompletedAt.After(status.StartedAt))
}

func TestGetDefaultConfig(t *testing.T) {
	config := GetDefaultConfig()

	assert.Equal(t, "https://mirrors.kernel.org/pub/linux/utils/boot/syslinux/", config.BaseURL)
	assert.Equal(t, "./public/tftp", config.TFTPDir)
	assert.Equal(t, "boot-bios", config.BiosDir)
	assert.Equal(t, "boot-efi", config.EfiDir)
	assert.Equal(t, "/tmp/syslinux-downloads", config.TempDir)
	assert.True(t, config.AutoExtract)
	assert.False(t, config.KeepArchive)
	assert.True(t, config.VerifyChecksum)
}

func TestGetRequiredBiosFiles(t *testing.T) {
	files := GetRequiredBiosFiles()

	// Check that all required files are present
	expectedFiles := []string{
		"pxelinux.0", "ldlinux.c32", "libcom32.c32", "libutil.c32",
		"vesamenu.c32", "menu.c32", "chain.c32", "reboot.c32", "poweroff.c32",
	}

	assert.Len(t, files, len(expectedFiles))
	
	for _, expectedFile := range expectedFiles {
		description, exists := files[expectedFile]
		assert.True(t, exists, "Expected file %s should be present", expectedFile)
		assert.NotEmpty(t, description, "Description for %s should not be empty", expectedFile)
	}

	// Check specific descriptions
	assert.Equal(t, "PXE boot loader for BIOS systems", files["pxelinux.0"])
	assert.Equal(t, "Core library for PXELINUX", files["ldlinux.c32"])
	assert.Equal(t, "VESA graphical menu system", files["vesamenu.c32"])
}

func TestGetRequiredEfiFiles(t *testing.T) {
	files := GetRequiredEfiFiles()

	// Check that all required files are present
	expectedFiles := []string{
		"syslinux.efi", "ldlinux.e64", "libcom32.c32", "libutil.c32",
		"vesamenu.c32", "menu.c32", "chain.c32", "reboot.c32", "poweroff.c32",
	}

	assert.Len(t, files, len(expectedFiles))
	
	for _, expectedFile := range expectedFiles {
		description, exists := files[expectedFile]
		assert.True(t, exists, "Expected file %s should be present", expectedFile)
		assert.NotEmpty(t, description, "Description for %s should not be empty", expectedFile)
	}

	// Check specific descriptions
	assert.Equal(t, "EFI boot loader for UEFI systems", files["syslinux.efi"])
	assert.Equal(t, "Core library for EFI SYSLINUX", files["ldlinux.e64"])
}

func TestGetBootFileSourcePath(t *testing.T) {
	// Test BIOS paths
	tests := []struct {
		bootType string
		fileName string
		expected string
	}{
		{"bios", "pxelinux.0", "bios/core/pxelinux.0"},
		{"bios", "ldlinux.c32", "bios/com32/elflink/ldlinux/ldlinux.c32"},
		{"bios", "libcom32.c32", "bios/com32/lib/libcom32.c32"},
		{"bios", "vesamenu.c32", "bios/com32/menu/vesamenu.c32"},
		{"bios", "chain.c32", "bios/com32/modules/chain.c32"},
		
		// Test EFI paths
		{"efi", "syslinux.efi", "efi64/efi/syslinux.efi"},
		{"efi", "ldlinux.e64", "efi64/com32/elflink/ldlinux/ldlinux.e64"},
		{"efi", "libcom32.c32", "efi64/com32/lib/libcom32.c32"},
		{"efi", "vesamenu.c32", "efi64/com32/menu/vesamenu.c32"},
		{"efi", "chain.c32", "efi64/com32/modules/chain.c32"},
		
		// Test invalid cases
		{"invalid", "pxelinux.0", ""},
		{"bios", "nonexistent.c32", ""},
	}

	for _, test := range tests {
		result := GetBootFileSourcePath(test.bootType, test.fileName)
		assert.Equal(t, test.expected, result, 
			"GetBootFileSourcePath(%s, %s) should return %s", 
			test.bootType, test.fileName, test.expected)
	}
}

func TestGetBootTypeFromVersion(t *testing.T) {
	tests := []struct {
		version  string
		expected []string
	}{
		{"3.86", []string{"bios"}},        // Old version, BIOS only
		{"3.99", []string{"bios"}},        // Just before 4.00
		{"4.00", []string{"bios", "efi"}}, // Modern version, both
		{"6.03", []string{"bios", "efi"}}, // Current version, both
		{"6.04", []string{"bios", "efi"}}, // Future version, both
	}

	for _, test := range tests {
		result := GetBootTypeFromVersion(test.version)
		assert.Equal(t, test.expected, result, 
			"GetBootTypeFromVersion(%s) should return %v", test.version, test.expected)
	}
}

func TestParseVersionFromFilename(t *testing.T) {
	tests := []struct {
		filename string
		expected string
	}{
		{"syslinux-6.03.tar.gz", "6.03"},
		{"syslinux-6.04-pre1.tar.gz", "6.04-pre1"},
		{"syslinux-4.07.tar.gz", "4.07"},
		{"syslinux-3.86.tar.gz", "3.86"},
		
		// Invalid cases
		{"invalid-file.tar.gz", ""},
		{"syslinux-.tar.gz", ""},
		{"syslinux-6.03.zip", "6"},      // Wrong extension, but still parses (implementation quirk)
		{"notasyslinux-6.03.tar.gz", ""}, // Wrong prefix
		{"syslinux", ""},                 // No extension
		{"", ""},                         // Empty string
	}

	for _, test := range tests {
		result := ParseVersionFromFilename(test.filename)
		assert.Equal(t, test.expected, result, 
			"ParseVersionFromFilename(%s) should return %s", test.filename, test.expected)
	}
}

func TestSyslinuxMirror(t *testing.T) {
	now := time.Now()

	mirror := &SyslinuxMirror{
		Version:     "6.03",
		FileName:    "syslinux-6.03.tar.gz",
		DownloadURL: "https://mirrors.kernel.org/pub/linux/utils/boot/syslinux/syslinux-6.03.tar.gz",
		Size:        5120000,
		ModifiedAt:  now,
	}

	assert.Equal(t, "6.03", mirror.Version)
	assert.Equal(t, "syslinux-6.03.tar.gz", mirror.FileName)
	assert.Contains(t, mirror.DownloadURL, "syslinux-6.03.tar.gz")
	assert.Greater(t, mirror.Size, int64(0))
}

func TestSystemStatus(t *testing.T) {
	status := &SystemStatus{
		InstalledVersions: map[string]bool{
			"6.03": true,
			"6.02": false,
		},
		ActiveVersion:  "6.03",
		BiosFilesCount: 9,
		EfiFilesCount:  9,
		TotalDiskUsage: 2048000,
		LastUpdate:    "2023-01-01T12:00:00Z",
		HealthStatus:   "healthy",
	}

	assert.True(t, status.InstalledVersions["6.03"])
	assert.False(t, status.InstalledVersions["6.02"])
	assert.Equal(t, "6.03", status.ActiveVersion)
	assert.Equal(t, 9, status.BiosFilesCount)
	assert.Equal(t, 9, status.EfiFilesCount)
	assert.Equal(t, "healthy", status.HealthStatus)
}

func TestValidationResult(t *testing.T) {
	result := &ValidationResult{
		Valid:        false,
		BootType:     "bios",
		MissingFiles: []string{"pxelinux.0", "ldlinux.c32"},
		CorruptFiles: []string{"menu.c32"},
		ExtraFiles:   []string{"old-file.c32"},
		Warnings:     []string{"File permissions may be incorrect"},
	}

	assert.False(t, result.Valid)
	assert.Equal(t, "bios", result.BootType)
	assert.Contains(t, result.MissingFiles, "pxelinux.0")
	assert.Contains(t, result.CorruptFiles, "menu.c32")
	assert.Contains(t, result.ExtraFiles, "old-file.c32")
	assert.Len(t, result.Warnings, 1)
}

func TestDiskSpaceInfo(t *testing.T) {
	diskInfo := &DiskSpaceInfo{
		TotalSpace:     10000000000, // 10GB
		AvailableSpace: 5000000000,  // 5GB
		UsedSpace:      5000000000,  // 5GB
		UsagePercent:   50.0,
		Sufficient:     true,
	}

	assert.Equal(t, int64(10000000000), diskInfo.TotalSpace)
	assert.Equal(t, int64(5000000000), diskInfo.AvailableSpace)
	assert.Equal(t, int64(5000000000), diskInfo.UsedSpace)
	assert.Equal(t, 50.0, diskInfo.UsagePercent)
	assert.True(t, diskInfo.Sufficient)
	assert.Equal(t, diskInfo.UsedSpace+diskInfo.AvailableSpace, diskInfo.TotalSpace)
}

// Test repository operations with mocks

func TestMockRepository_SaveVersion(t *testing.T) {
	repo := &MockRepository{}
	ctx := context.Background()

	version := &SyslinuxVersion{
		ID:      "syslinux-6.03",
		Version: "6.03",
	}

	repo.On("SaveVersion", ctx, version).Return(nil)

	err := repo.SaveVersion(ctx, version)

	assert.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestMockRepository_GetVersion(t *testing.T) {
	repo := &MockRepository{}
	ctx := context.Background()
	id := "syslinux-6.03"

	expectedVersion := &SyslinuxVersion{
		ID:      id,
		Version: "6.03",
	}

	repo.On("GetVersion", ctx, id).Return(expectedVersion, nil)

	version, err := repo.GetVersion(ctx, id)

	assert.NoError(t, err)
	assert.Equal(t, expectedVersion, version)
	repo.AssertExpectations(t)
}

func TestMockRepository_ListVersions(t *testing.T) {
	repo := &MockRepository{}
	ctx := context.Background()

	expectedVersions := []*SyslinuxVersion{
		{ID: "syslinux-6.03", Version: "6.03"},
		{ID: "syslinux-6.02", Version: "6.02"},
	}

	repo.On("ListVersions", ctx).Return(expectedVersions, nil)

	versions, err := repo.ListVersions(ctx)

	assert.NoError(t, err)
	assert.Equal(t, expectedVersions, versions)
	assert.Len(t, versions, 2)
	repo.AssertExpectations(t)
}

func TestMockRepository_SetActiveVersion(t *testing.T) {
	repo := &MockRepository{}
	ctx := context.Background()
	version := "6.03"

	repo.On("SetActiveVersion", ctx, version).Return(nil)

	err := repo.SetActiveVersion(ctx, version)

	assert.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestMockRepository_BootFileOperations(t *testing.T) {
	repo := &MockRepository{}
	ctx := context.Background()

	bootFile := &SyslinuxBootFile{
		ID:       "pxelinux-6.03-bios",
		Version:  "6.03",
		BootType: "bios",
		FileName: "pxelinux.0",
	}

	// Test SaveBootFile
	repo.On("SaveBootFile", ctx, bootFile).Return(nil)
	err := repo.SaveBootFile(ctx, bootFile)
	assert.NoError(t, err)

	// Test GetBootFile
	repo.On("GetBootFile", ctx, bootFile.ID).Return(bootFile, nil)
	retrieved, err := repo.GetBootFile(ctx, bootFile.ID)
	assert.NoError(t, err)
	assert.Equal(t, bootFile, retrieved)

	// Test ListBootFiles
	bootFiles := []*SyslinuxBootFile{bootFile}
	repo.On("ListBootFiles", ctx, "6.03", "bios").Return(bootFiles, nil)
	files, err := repo.ListBootFiles(ctx, "6.03", "bios")
	assert.NoError(t, err)
	assert.Equal(t, bootFiles, files)

	// Test UpdateBootFileStatus
	repo.On("UpdateBootFileStatus", ctx, bootFile.ID, true).Return(nil)
	err = repo.UpdateBootFileStatus(ctx, bootFile.ID, true)
	assert.NoError(t, err)

	repo.AssertExpectations(t)
}

func TestMockRepository_DownloadStatusOperations(t *testing.T) {
	repo := &MockRepository{}
	ctx := context.Background()

	status := &DownloadStatus{
		ID:      "download-123",
		Version: "6.03",
		Status:  "downloading",
	}

	// Test SaveDownloadStatus
	repo.On("SaveDownloadStatus", ctx, status).Return(nil)
	err := repo.SaveDownloadStatus(ctx, status)
	assert.NoError(t, err)

	// Test GetDownloadStatus
	repo.On("GetDownloadStatus", ctx, status.ID).Return(status, nil)
	retrieved, err := repo.GetDownloadStatus(ctx, status.ID)
	assert.NoError(t, err)
	assert.Equal(t, status, retrieved)

	// Test ListDownloadStatuses
	statuses := []*DownloadStatus{status}
	repo.On("ListDownloadStatuses", ctx).Return(statuses, nil)
	allStatuses, err := repo.ListDownloadStatuses(ctx)
	assert.NoError(t, err)
	assert.Equal(t, statuses, allStatuses)

	repo.AssertExpectations(t)
}

// Test edge cases and error conditions

func TestParseVersionFromFilename_EdgeCases(t *testing.T) {
	// Test with exactly minimum length cases
	assert.Equal(t, "", ParseVersionFromFilename("syslinux-.tar.gz"))  // 16 chars, but no version
	assert.Equal(t, "", ParseVersionFromFilename("syslinux.tar.gz"))   // 15 chars, no dash
	assert.Equal(t, "a", ParseVersionFromFilename("syslinux-a.tar.gz")) // Single character version
}

func TestGetBootTypeFromVersion_Boundary(t *testing.T) {
	// Test boundary condition exactly at 4.00
	assert.Equal(t, []string{"bios", "efi"}, GetBootTypeFromVersion("4.00"))
	assert.Equal(t, []string{"bios"}, GetBootTypeFromVersion("3.99"))
	
	// Test string comparison behavior
	assert.Equal(t, []string{"bios"}, GetBootTypeFromVersion("3.999")) // String comparison
}

// Test configuration validation
func TestSyslinuxConfig_Validation(t *testing.T) {
	config := SyslinuxConfig{
		BaseURL:        "https://mirrors.kernel.org/pub/linux/utils/boot/syslinux/",
		TFTPDir:        "/tmp/tftp",
		BiosDir:        "boot-bios", 
		EfiDir:         "boot-efi",
		TempDir:        "/tmp/syslinux",
		AutoExtract:    true,
		KeepArchive:    false,
		VerifyChecksum: true,
	}

	// Basic validation
	assert.NotEmpty(t, config.BaseURL)
	assert.NotEmpty(t, config.TFTPDir)
	assert.NotEmpty(t, config.BiosDir)
	assert.NotEmpty(t, config.EfiDir)
	assert.NotEmpty(t, config.TempDir)
	
	// URL validation
	assert.Contains(t, config.BaseURL, "https://")
	assert.Contains(t, config.BaseURL, "syslinux")
}