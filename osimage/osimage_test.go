package osimage

import (
	"context"
	"errors"
	"testing"
	"time"

	"ignite/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockOSImageRepository is a mock implementation of OSImageRepository
type MockOSImageRepository struct {
	mock.Mock
}

func (m *MockOSImageRepository) Save(ctx context.Context, image *OSImage) error {
	args := m.Called(ctx, image)
	return args.Error(0)
}

func (m *MockOSImageRepository) Get(ctx context.Context, id string) (*OSImage, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*OSImage), args.Error(1)
}

func (m *MockOSImageRepository) GetAll(ctx context.Context) ([]*OSImage, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*OSImage), args.Error(1)
}

func (m *MockOSImageRepository) GetByOS(ctx context.Context, os string) ([]*OSImage, error) {
	args := m.Called(ctx, os)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*OSImage), args.Error(1)
}

func (m *MockOSImageRepository) GetByOSAndVersion(ctx context.Context, os, version string) (*OSImage, error) {
	args := m.Called(ctx, os, version)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*OSImage), args.Error(1)
}

func (m *MockOSImageRepository) GetDefaultVersion(ctx context.Context, os string) (*OSImage, error) {
	args := m.Called(ctx, os)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*OSImage), args.Error(1)
}

func (m *MockOSImageRepository) Delete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockOSImageRepository) SetDefault(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// MockDownloadStatusRepository is a mock implementation of DownloadStatusRepository
type MockDownloadStatusRepository struct {
	mock.Mock
}

func (m *MockDownloadStatusRepository) Save(ctx context.Context, status *DownloadStatus) error {
	args := m.Called(ctx, status)
	return args.Error(0)
}

func (m *MockDownloadStatusRepository) Get(ctx context.Context, id string) (*DownloadStatus, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*DownloadStatus), args.Error(1)
}

func (m *MockDownloadStatusRepository) GetActive(ctx context.Context) ([]*DownloadStatus, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*DownloadStatus), args.Error(1)
}

func (m *MockDownloadStatusRepository) Delete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// Helper function to create a test config
func createTestConfig() *config.Config {
	return &config.Config{
		TFTP: config.TFTPConfig{
			Dir: "/tmp/test-tftp",
		},
		OSImages: config.OSImageConfig{
			Sources: map[string]config.OSDefinition{
				"ubuntu": {
					DisplayName: "Ubuntu",
					Versions: map[string]config.OSVersion{
						"22.04": {
							DisplayName:   "Ubuntu 22.04 LTS",
							BaseURL:       "http://releases.ubuntu.com/22.04/",
							Architectures: []string{"x86_64"},
						},
						"20.04": {
							DisplayName:   "Ubuntu 20.04 LTS",
							BaseURL:       "http://releases.ubuntu.com/20.04/",
							Architectures: []string{"x86_64"},
						},
					},
				},
				"centos": {
					DisplayName: "CentOS",
					Versions: map[string]config.OSVersion{
						"8": {
							DisplayName:   "CentOS 8",
							BaseURL:       "http://mirror.centos.org/centos/8/",
							Architectures: []string{"x86_64"},
						},
					},
				},
			},
		},
	}
}

// Helper function to create a service without starting the background worker
func createTestService(mockRepo *MockOSImageRepository, mockDownloadRepo *MockDownloadStatusRepository, config *config.Config) *OSImageServiceImpl {
	service := &OSImageServiceImpl{
		repo:            mockRepo,
		downloadRepo:    mockDownloadRepo,
		config:          config,
		downloadChan:    make(chan OSImageConfig, 10),
		activeDownloads: make(map[string]*DownloadStatus),
	}
	// Don't start the background worker for tests
	return service
}

// Test OSImageServiceImpl creation
func TestNewOSImageService(t *testing.T) {
	mockRepo := &MockOSImageRepository{}
	mockDownloadRepo := &MockDownloadStatusRepository{}
	config := createTestConfig()

	// For this test we'll accept the background worker starting
	// but we need to mock the calls it will make
	mockDownloadRepo.On("GetActive", mock.Anything).Return([]*DownloadStatus{}, nil).Maybe()

	service := NewOSImageService(mockRepo, mockDownloadRepo, config)

	assert.NotNil(t, service)

	// Cast to implementation to verify internal structure
	impl, ok := service.(*OSImageServiceImpl)
	assert.True(t, ok)
	assert.Equal(t, mockRepo, impl.repo)
	assert.Equal(t, mockDownloadRepo, impl.downloadRepo)
	assert.Equal(t, config, impl.config)
	assert.NotNil(t, impl.downloadChan)
	assert.NotNil(t, impl.activeDownloads)
}

// Test GetAllOSImages
func TestGetAllOSImages(t *testing.T) {
	mockRepo := &MockOSImageRepository{}
	mockDownloadRepo := &MockDownloadStatusRepository{}
	config := createTestConfig()
	service := createTestService(mockRepo, mockDownloadRepo, config)

	ctx := context.Background()
	expectedImages := []*OSImage{
		{
			ID:           "1",
			OS:           "ubuntu",
			Version:      "22.04",
			Architecture: "x86_64",
		},
		{
			ID:           "2",
			OS:           "centos",
			Version:      "8",
			Architecture: "x86_64",
		},
	}

	mockRepo.On("GetAll", ctx).Return(expectedImages, nil)

	images, err := service.GetAllOSImages(ctx)

	assert.NoError(t, err)
	assert.Equal(t, expectedImages, images)
	mockRepo.AssertExpectations(t)
}

// Test GetAllOSImages with error
func TestGetAllOSImages_Error(t *testing.T) {
	mockRepo := &MockOSImageRepository{}
	mockDownloadRepo := &MockDownloadStatusRepository{}
	config := createTestConfig()
	service := createTestService(mockRepo, mockDownloadRepo, config)

	ctx := context.Background()
	expectedError := errors.New("database error")

	mockRepo.On("GetAll", ctx).Return(nil, expectedError)

	images, err := service.GetAllOSImages(ctx)

	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
	assert.Nil(t, images)
	mockRepo.AssertExpectations(t)
}

// Test GetOSImagesByOS
func TestGetOSImagesByOS(t *testing.T) {
	mockRepo := &MockOSImageRepository{}
	mockDownloadRepo := &MockDownloadStatusRepository{}
	config := createTestConfig()
	service := createTestService(mockRepo, mockDownloadRepo, config)

	ctx := context.Background()
	os := "ubuntu"
	expectedImages := []*OSImage{
		{
			ID:           "1",
			OS:           "ubuntu",
			Version:      "22.04",
			Architecture: "x86_64",
		},
		{
			ID:           "3",
			OS:           "ubuntu",
			Version:      "20.04",
			Architecture: "x86_64",
		},
	}

	mockRepo.On("GetByOS", ctx, os).Return(expectedImages, nil)

	images, err := service.GetOSImagesByOS(ctx, os)

	assert.NoError(t, err)
	assert.Equal(t, expectedImages, images)
	mockRepo.AssertExpectations(t)
}

// Test GetOSImage
func TestGetOSImage(t *testing.T) {
	mockRepo := &MockOSImageRepository{}
	mockDownloadRepo := &MockDownloadStatusRepository{}
	config := createTestConfig()
	service := createTestService(mockRepo, mockDownloadRepo, config)

	ctx := context.Background()
	id := "test-id"
	expectedImage := &OSImage{
		ID:           id,
		OS:           "ubuntu",
		Version:      "22.04",
		Architecture: "x86_64",
	}

	mockRepo.On("Get", ctx, id).Return(expectedImage, nil)

	image, err := service.GetOSImage(ctx, id)

	assert.NoError(t, err)
	assert.Equal(t, expectedImage, image)
	mockRepo.AssertExpectations(t)
}

// Test GetDefaultVersion
func TestGetDefaultVersion(t *testing.T) {
	mockRepo := &MockOSImageRepository{}
	mockDownloadRepo := &MockDownloadStatusRepository{}
	config := createTestConfig()
	service := createTestService(mockRepo, mockDownloadRepo, config)

	ctx := context.Background()
	os := "ubuntu"
	expectedImage := &OSImage{
		ID:           "1",
		OS:           "ubuntu",
		Version:      "22.04",
		Architecture: "x86_64",
		Active:       true,
	}

	mockRepo.On("GetDefaultVersion", ctx, os).Return(expectedImage, nil)

	image, err := service.GetDefaultVersion(ctx, os)

	assert.NoError(t, err)
	assert.Equal(t, expectedImage, image)
	assert.True(t, image.Active)
	mockRepo.AssertExpectations(t)
}

// Test SetDefaultVersion
func TestSetDefaultVersion(t *testing.T) {
	mockRepo := &MockOSImageRepository{}
	mockDownloadRepo := &MockDownloadStatusRepository{}
	config := createTestConfig()
	service := createTestService(mockRepo, mockDownloadRepo, config)

	ctx := context.Background()
	id := "test-id"

	mockRepo.On("SetDefault", ctx, id).Return(nil)

	err := service.SetDefaultVersion(ctx, id)

	assert.NoError(t, err)
	mockRepo.AssertExpectations(t)
}

// Test DownloadOSImage with valid configuration
func TestDownloadOSImage_Valid(t *testing.T) {
	mockRepo := &MockOSImageRepository{}
	mockDownloadRepo := &MockDownloadStatusRepository{}
	config := createTestConfig()
	service := createTestService(mockRepo, mockDownloadRepo, config)

	ctx := context.Background()
	osConfig := OSImageConfig{
		OS:           "ubuntu",
		Version:      "22.04",
		Architecture: "x86_64",
	}

	// Mock that the OS/version doesn't exist yet
	mockRepo.On("GetByOSAndVersion", ctx, osConfig.OS, osConfig.Version).Return(nil, errors.New("not found"))

	// Mock saving the download status twice (queued, then downloading)
	mockDownloadRepo.On("Save", ctx, mock.AnythingOfType("*osimage.DownloadStatus")).Return(nil).Twice()

	status, err := service.DownloadOSImage(ctx, osConfig)

	assert.NoError(t, err)
	assert.NotNil(t, status)
	assert.Equal(t, osConfig.OS, status.OS)
	assert.Equal(t, osConfig.Version, status.Version)
	assert.Equal(t, "downloading", status.Status)
	assert.False(t, status.StartedAt.IsZero())

	mockRepo.AssertExpectations(t)
	mockDownloadRepo.AssertExpectations(t)
}

// Test DownloadOSImage with invalid OS/version
func TestDownloadOSImage_InvalidOS(t *testing.T) {
	mockRepo := &MockOSImageRepository{}
	mockDownloadRepo := &MockDownloadStatusRepository{}
	config := createTestConfig()
	service := createTestService(mockRepo, mockDownloadRepo, config)

	ctx := context.Background()
	osConfig := OSImageConfig{
		OS:           "invalid-os",
		Version:      "1.0",
		Architecture: "x86_64",
	}

	status, err := service.DownloadOSImage(ctx, osConfig)

	assert.Error(t, err)
	assert.Nil(t, status)
	assert.Contains(t, err.Error(), "unsupported OS/version combination")

	// No mock expectations should be called
	mockRepo.AssertExpectations(t)
	mockDownloadRepo.AssertExpectations(t)
}

// Test DownloadOSImage with existing image
func TestDownloadOSImage_AlreadyExists(t *testing.T) {
	mockRepo := &MockOSImageRepository{}
	mockDownloadRepo := &MockDownloadStatusRepository{}
	config := createTestConfig()
	service := createTestService(mockRepo, mockDownloadRepo, config)

	ctx := context.Background()
	osConfig := OSImageConfig{
		OS:           "ubuntu",
		Version:      "22.04",
		Architecture: "x86_64",
	}

	existingImage := &OSImage{
		ID:      "existing-id",
		OS:      osConfig.OS,
		Version: osConfig.Version,
	}

	mockRepo.On("GetByOSAndVersion", ctx, osConfig.OS, osConfig.Version).Return(existingImage, nil)

	status, err := service.DownloadOSImage(ctx, osConfig)

	assert.Error(t, err)
	assert.Nil(t, status)
	assert.Contains(t, err.Error(), "already exists")

	mockRepo.AssertExpectations(t)
	mockDownloadRepo.AssertExpectations(t)
}

// Test GetAvailableVersions
func TestGetAvailableVersions(t *testing.T) {
	mockRepo := &MockOSImageRepository{}
	mockDownloadRepo := &MockDownloadStatusRepository{}
	config := createTestConfig()
	service := createTestService(mockRepo, mockDownloadRepo, config)

	ctx := context.Background()
	os := "ubuntu"

	versions, err := service.GetAvailableVersions(ctx, os)

	assert.NoError(t, err)
	assert.Contains(t, versions, "22.04")
	assert.Contains(t, versions, "20.04")
	assert.Len(t, versions, 2)
}

// Test GetAvailableVersions with invalid OS
func TestGetAvailableVersions_InvalidOS(t *testing.T) {
	mockRepo := &MockOSImageRepository{}
	mockDownloadRepo := &MockDownloadStatusRepository{}
	config := createTestConfig()
	service := createTestService(mockRepo, mockDownloadRepo, config)

	ctx := context.Background()
	os := "invalid-os"

	versions, err := service.GetAvailableVersions(ctx, os)

	assert.Error(t, err)
	assert.Nil(t, versions)
	assert.Contains(t, err.Error(), "unsupported OS")
}

// Test GetDownloadStatus
func TestGetDownloadStatus(t *testing.T) {
	mockRepo := &MockOSImageRepository{}
	mockDownloadRepo := &MockDownloadStatusRepository{}
	config := createTestConfig()
	service := createTestService(mockRepo, mockDownloadRepo, config)

	ctx := context.Background()
	id := "download-id"
	expectedStatus := &DownloadStatus{
		ID:        id,
		OS:        "ubuntu",
		Version:   "22.04",
		Status:    "downloading",
		Progress:  50,
		StartedAt: time.Now(),
	}

	mockDownloadRepo.On("Get", ctx, id).Return(expectedStatus, nil)

	status, err := service.GetDownloadStatus(ctx, id)

	assert.NoError(t, err)
	assert.Equal(t, expectedStatus, status)
	mockDownloadRepo.AssertExpectations(t)
}

// Test GetActiveDownloads
func TestGetActiveDownloads(t *testing.T) {
	mockRepo := &MockOSImageRepository{}
	mockDownloadRepo := &MockDownloadStatusRepository{}
	config := createTestConfig()
	service := createTestService(mockRepo, mockDownloadRepo, config)

	ctx := context.Background()
	expectedDownloads := []*DownloadStatus{
		{
			ID:      "download-1",
			Status:  "downloading",
			OS:      "ubuntu",
			Version: "22.04",
		},
		{
			ID:      "download-2",
			Status:  "queued",
			OS:      "centos",
			Version: "8",
		},
	}

	mockDownloadRepo.On("GetActive", ctx).Return(expectedDownloads, nil)

	downloads, err := service.GetActiveDownloads(ctx)

	assert.NoError(t, err)
	assert.Equal(t, expectedDownloads, downloads)
	mockDownloadRepo.AssertExpectations(t)
}

// Test CancelDownload
func TestCancelDownload(t *testing.T) {
	mockRepo := &MockOSImageRepository{}
	mockDownloadRepo := &MockDownloadStatusRepository{}
	config := createTestConfig()
	service := createTestService(mockRepo, mockDownloadRepo, config)

	ctx := context.Background()
	id := "download-id"
	downloadStatus := &DownloadStatus{
		ID:        id,
		OS:        "ubuntu",
		Version:   "22.04",
		Status:    "downloading",
		Progress:  30,
		StartedAt: time.Now(),
	}

	mockDownloadRepo.On("Get", ctx, id).Return(downloadStatus, nil)
	mockDownloadRepo.On("Save", ctx, mock.MatchedBy(func(status *DownloadStatus) bool {
		return status.ID == id &&
			status.Status == "cancelled" &&
			status.Progress == 0 &&
			status.ErrorMessage == "Download cancelled by user" &&
			status.CompletedAt != nil
	})).Return(nil)

	err := service.CancelDownload(ctx, id)

	assert.NoError(t, err)
	mockDownloadRepo.AssertExpectations(t)
}

// Test CancelDownload with invalid status
func TestCancelDownload_InvalidStatus(t *testing.T) {
	mockRepo := &MockOSImageRepository{}
	mockDownloadRepo := &MockDownloadStatusRepository{}
	config := createTestConfig()
	service := createTestService(mockRepo, mockDownloadRepo, config)

	ctx := context.Background()
	id := "download-id"
	downloadStatus := &DownloadStatus{
		ID:     id,
		Status: "completed", // Cannot cancel completed download
	}

	mockDownloadRepo.On("Get", ctx, id).Return(downloadStatus, nil)

	err := service.CancelDownload(ctx, id)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot cancel download with status")
	mockDownloadRepo.AssertExpectations(t)
}

// Test model validation
func TestOSImageModel(t *testing.T) {
	image := &OSImage{
		ID:           "test-id",
		OS:           "ubuntu",
		Version:      "22.04",
		Architecture: "x86_64",
		KernelPath:   "ubuntu/22.04/vmlinuz",
		InitrdPath:   "ubuntu/22.04/initrd.img",
		KernelSize:   5120000,
		InitrdSize:   15360000,
		Checksum:     "abc123def456",
		Active:       true,
		DownloadURL:  "http://example.com/ubuntu-22.04.iso",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	assert.Equal(t, "test-id", image.ID)
	assert.Equal(t, "ubuntu", image.OS)
	assert.Equal(t, "22.04", image.Version)
	assert.Equal(t, "x86_64", image.Architecture)
	assert.True(t, image.Active)
	assert.Greater(t, image.KernelSize, int64(0))
	assert.Greater(t, image.InitrdSize, int64(0))
	assert.NotEmpty(t, image.Checksum)
}

func TestOSImageConfig(t *testing.T) {
	config := OSImageConfig{
		OS:           "centos",
		Version:      "8",
		Architecture: "x86_64",
		Source:       "http://mirror.centos.org/centos/8/",
	}

	assert.Equal(t, "centos", config.OS)
	assert.Equal(t, "8", config.Version)
	assert.Equal(t, "x86_64", config.Architecture)
	assert.NotEmpty(t, config.Source)
}

func TestDownloadStatus(t *testing.T) {
	now := time.Now()
	completed := now.Add(5 * time.Minute)

	status := DownloadStatus{
		ID:           "download-123",
		OS:           "ubuntu",
		Version:      "22.04",
		Status:       "completed",
		Progress:     100,
		ErrorMessage: "",
		StartedAt:    now,
		CompletedAt:  &completed,
	}

	assert.Equal(t, "download-123", status.ID)
	assert.Equal(t, "ubuntu", status.OS)
	assert.Equal(t, "completed", status.Status)
	assert.Equal(t, 100, status.Progress)
	assert.Empty(t, status.ErrorMessage)
	assert.NotNil(t, status.CompletedAt)
	assert.True(t, status.CompletedAt.After(status.StartedAt))
}
