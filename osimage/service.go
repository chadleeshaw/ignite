package osimage

import (
	"context"
	"crypto/sha256"
	"fmt"
	"ignite/config"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// OSImageServiceImpl provides business logic for OS image management
type OSImageServiceImpl struct {
	repo            OSImageRepository
	downloadRepo    DownloadStatusRepository
	config          *config.Config
	downloadChan    chan OSImageConfig
	activeDownloads map[string]*DownloadStatus
}

// NewOSImageService creates a new OS image service
func NewOSImageService(repo OSImageRepository, downloadRepo DownloadStatusRepository, cfg *config.Config) OSImageService {
	service := &OSImageServiceImpl{
		repo:            repo,
		downloadRepo:    downloadRepo,
		config:          cfg,
		downloadChan:    make(chan OSImageConfig, 10),
		activeDownloads: make(map[string]*DownloadStatus),
	}

	// Start background download worker
	go service.downloadWorker()

	return service
}

// GetAllOSImages retrieves all OS images
func (s *OSImageServiceImpl) GetAllOSImages(ctx context.Context) ([]*OSImage, error) {
	return s.repo.GetAll(ctx)
}

// GetOSImagesByOS retrieves all images for a specific operating system
func (s *OSImageServiceImpl) GetOSImagesByOS(ctx context.Context, os string) ([]*OSImage, error) {
	return s.repo.GetByOS(ctx, os)
}

// GetOSImage retrieves an OS image by ID
func (s *OSImageServiceImpl) GetOSImage(ctx context.Context, id string) (*OSImage, error) {
	return s.repo.Get(ctx, id)
}

// GetDefaultVersion retrieves the default version for an OS
func (s *OSImageServiceImpl) GetDefaultVersion(ctx context.Context, os string) (*OSImage, error) {
	return s.repo.GetDefaultVersion(ctx, os)
}

// SetDefaultVersion sets an OS image as the default for its OS type
func (s *OSImageServiceImpl) SetDefaultVersion(ctx context.Context, id string) error {
	return s.repo.SetDefault(ctx, id)
}

// DeleteOSImage removes an OS image and its files
func (s *OSImageServiceImpl) DeleteOSImage(ctx context.Context, id string) error {
	// Get the image to find file paths
	image, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}

	// Remove files from filesystem
	kernelPath := filepath.Join(s.config.TFTP.Dir, image.KernelPath)
	initrdPath := filepath.Join(s.config.TFTP.Dir, image.InitrdPath)

	if err := os.Remove(kernelPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove kernel file: %w", err)
	}

	if err := os.Remove(initrdPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove initrd file: %w", err)
	}

	// Remove from database
	return s.repo.Delete(ctx, id)
}

// DownloadOSImage starts downloading an OS image
func (s *OSImageServiceImpl) DownloadOSImage(ctx context.Context, osConfig OSImageConfig) (*DownloadStatus, error) {
	// Validate OS and version
	if !s.isValidOSVersion(osConfig.OS, osConfig.Version) {
		return nil, fmt.Errorf("unsupported OS/version combination: %s %s", osConfig.OS, osConfig.Version)
	}

	// Check if already exists
	existing, err := s.repo.GetByOSAndVersion(ctx, osConfig.OS, osConfig.Version)
	if err == nil && existing != nil {
		return nil, fmt.Errorf("OS image already exists: %s %s", osConfig.OS, osConfig.Version)
	}

	// Create download status
	status := &DownloadStatus{
		ID:        uuid.New().String(),
		OS:        osConfig.OS,
		Version:   osConfig.Version,
		Status:    "queued",
		Progress:  0,
		StartedAt: time.Now(),
	}

	if err := s.downloadRepo.Save(ctx, status); err != nil {
		return nil, err
	}

	// Update status to downloading BEFORE queuing to prevent race condition
	status.Status = "downloading"
	if err := s.downloadRepo.Save(ctx, status); err != nil {
		return nil, err
	}

	// Queue for download
	select {
	case s.downloadChan <- osConfig:
		// Successfully queued
	default:
		status.Status = "failed"
		status.ErrorMessage = "download queue is full"
		s.downloadRepo.Save(ctx, status)
		return status, fmt.Errorf("download queue is full")
	}

	return status, nil
}

// GetDownloadStatus retrieves the status of a download
func (s *OSImageServiceImpl) GetDownloadStatus(ctx context.Context, id string) (*DownloadStatus, error) {
	return s.downloadRepo.Get(ctx, id)
}

// GetActiveDownloads retrieves all active downloads
func (s *OSImageServiceImpl) GetActiveDownloads(ctx context.Context) ([]*DownloadStatus, error) {
	return s.downloadRepo.GetActive(ctx)
}

// CancelDownload cancels an active download and cleans up partial files
func (s *OSImageServiceImpl) CancelDownload(ctx context.Context, id string) error {
	// Get the download status
	status, err := s.downloadRepo.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get download status: %w", err)
	}

	// Only allow canceling downloads that are still in progress
	if status.Status != "downloading" && status.Status != "queued" {
		return fmt.Errorf("cannot cancel download with status: %s", status.Status)
	}

	// Mark as cancelled
	now := time.Now()
	status.Status = "cancelled"
	status.Progress = 0
	status.ErrorMessage = "Download cancelled by user"
	status.CompletedAt = &now

	if err := s.downloadRepo.Save(ctx, status); err != nil {
		return fmt.Errorf("failed to save cancelled status: %w", err)
	}

	// Clean up any partially downloaded files
	osDir := filepath.Join(s.config.TFTP.Dir, status.OS, status.Version)
	kernelPath := filepath.Join(osDir, "vmlinuz")
	initrdPath := filepath.Join(osDir, "initrd.img")

	// Remove partial files (ignore errors as files might not exist)
	os.Remove(kernelPath)
	os.Remove(initrdPath)

	// Try to remove the directory if it's empty
	os.Remove(osDir)

	return nil
}

// isValidOSVersion checks if the OS and version combination is supported
func (s *OSImageServiceImpl) isValidOSVersion(os, version string) bool {
	osDef, exists := s.config.OSImages.Sources[os]
	if !exists {
		return false
	}

	_, versionExists := osDef.Versions[version]
	return versionExists
}

// downloadWorker processes download requests in the background
func (s *OSImageServiceImpl) downloadWorker() {
	for osConfig := range s.downloadChan {
		s.processDownload(osConfig)
	}
}

// processDownload handles the actual download process
func (s *OSImageServiceImpl) processDownload(osConfig OSImageConfig) {
	ctx := context.Background()

	// Find download status
	var status *DownloadStatus
	downloads, err := s.downloadRepo.GetActive(ctx)
	if err != nil {
		return
	}

	for _, d := range downloads {
		if d.OS == osConfig.OS && d.Version == osConfig.Version {
			status = d
			break
		}
	}

	if status == nil {
		return // Status not found
	}

	// Get OS definition from config
	osDef, exists := s.config.OSImages.Sources[osConfig.OS]
	if !exists {
		s.markDownloadFailed(ctx, status, "unsupported OS")
		return
	}

	// Get version info from config
	versionInfo, versionExists := osDef.Versions[osConfig.Version]
	if !versionExists {
		s.markDownloadFailed(ctx, status, "unsupported version for this OS")
		return
	}

	baseURL := versionInfo.BaseURL
	if baseURL == "" {
		s.markDownloadFailed(ctx, status, "no download URL found for this version")
		return
	}

	// Create directory structure
	osDir := filepath.Join(s.config.TFTP.Dir, osConfig.OS, osConfig.Version)
	if err := os.MkdirAll(osDir, 0755); err != nil {
		s.markDownloadFailed(ctx, status, fmt.Sprintf("failed to create directory: %v", err))
		return
	}

	// Get filenames from config
	kernelFile := osDef.KernelFile
	initrdFile := osDef.InitrdFile

	kernelURL := strings.TrimSuffix(baseURL, "/") + "/" + kernelFile
	initrdURL := strings.TrimSuffix(baseURL, "/") + "/" + initrdFile

	kernelPath := filepath.Join(osDir, "vmlinuz")
	initrdPath := filepath.Join(osDir, "initrd.img")

	// Download kernel
	status.Progress = 10
	s.downloadRepo.Save(ctx, status)

	// Check if cancelled before starting kernel download
	if status, err := s.downloadRepo.Get(ctx, status.ID); err != nil || status.Status == "cancelled" {
		return
	}

	kernelSize, kernelChecksum, err := s.downloadFile(kernelURL, kernelPath)
	if err != nil {
		s.markDownloadFailed(ctx, status, fmt.Sprintf("failed to download kernel: %v", err))
		return
	}

	// Download initrd
	status.Progress = 60
	s.downloadRepo.Save(ctx, status)

	// Check if cancelled before starting initrd download
	if status, err := s.downloadRepo.Get(ctx, status.ID); err != nil || status.Status == "cancelled" {
		return
	}

	initrdSize, initrdChecksum, err := s.downloadFile(initrdURL, initrdPath)
	if err != nil {
		s.markDownloadFailed(ctx, status, fmt.Sprintf("failed to download initrd: %v", err))
		return
	}

	// Create OS image record
	status.Progress = 90
	s.downloadRepo.Save(ctx, status)

	osImage := &OSImage{
		OS:           osConfig.OS,
		Version:      osConfig.Version,
		Architecture: "x86_64", // Default to x86_64
		KernelPath:   fmt.Sprintf("%s/%s/vmlinuz", osConfig.OS, osConfig.Version),
		InitrdPath:   fmt.Sprintf("%s/%s/initrd.img", osConfig.OS, osConfig.Version),
		KernelSize:   kernelSize,
		InitrdSize:   initrdSize,
		Checksum:     kernelChecksum + ":" + initrdChecksum,
		Active:       false,   // Don't automatically set as default
		DownloadURL:  baseURL, // Store the base URL where files were downloaded from
	}

	if err := s.repo.Save(ctx, osImage); err != nil {
		s.markDownloadFailed(ctx, status, fmt.Sprintf("failed to save OS image: %v", err))
		return
	}

	// Mark download as completed
	now := time.Now()
	status.Status = "completed"
	status.Progress = 100
	status.CompletedAt = &now
	s.downloadRepo.Save(ctx, status)
}

// downloadFile downloads a file and returns its size and checksum
func (s *OSImageServiceImpl) downloadFile(url, filepath string) (int64, string, error) {
	resp, err := http.Get(url)
	if err != nil {
		log.Printf("HTTP GET failed for %s: %v", url, err)
		return 0, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Download failed with status %s for %s", resp.Status, url)
		return 0, "", fmt.Errorf("download failed: %s", resp.Status)
	}

	file, err := os.Create(filepath)
	if err != nil {
		return 0, "", err
	}
	defer file.Close()

	// Create hash writer
	hash := sha256.New()

	// Copy with hash calculation
	size, err := io.Copy(io.MultiWriter(file, hash), resp.Body)
	if err != nil {
		return 0, "", err
	}

	checksum := fmt.Sprintf("%x", hash.Sum(nil))
	return size, checksum, nil
}

// markDownloadFailed marks a download as failed with an error message
func (s *OSImageServiceImpl) markDownloadFailed(ctx context.Context, status *DownloadStatus, errorMsg string) {
	status.Status = "failed"
	status.ErrorMessage = errorMsg
	now := time.Now()
	status.CompletedAt = &now
	s.downloadRepo.Save(ctx, status)
}

// GetAvailableVersions returns available versions for a given OS
func (s *OSImageServiceImpl) GetAvailableVersions(ctx context.Context, os string) ([]string, error) {
	osDef, exists := s.config.OSImages.Sources[os]
	if !exists {
		return nil, fmt.Errorf("unsupported OS: %s", os)
	}

	var versions []string
	for version := range osDef.Versions {
		versions = append(versions, version)
	}

	sort.Strings(versions)
	return versions, nil
}
