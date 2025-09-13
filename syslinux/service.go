package syslinux

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

type service struct {
	repo   Repository
	config SyslinuxConfig
	client *http.Client
}

// NewService creates a new Syslinux service
func NewService(repo Repository, config SyslinuxConfig) Service {
	return &service{
		repo:   repo,
		config: config,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// ScanMirror scans the kernel.org mirror for available Syslinux versions
func (s *service) ScanMirror(ctx context.Context) ([]*SyslinuxMirror, error) {
	resp, err := s.client.Get(s.config.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch mirror page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mirror returned status %d", resp.StatusCode)
	}

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	var mirrors []*SyslinuxMirror
	var extractLinks func(*html.Node)

	// Regular expression to match syslinux tar files
	syslinuxRegex := regexp.MustCompile(`syslinux-([0-9]+\.[0-9]+(?:-[a-z0-9]+)?(?:\.[a-z0-9]+)?)\.tar\.gz`)

	extractLinks = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, attr := range n.Attr {
				if attr.Key == "href" {
					if matches := syslinuxRegex.FindStringSubmatch(attr.Val); matches != nil {
						version := matches[1]
						fileName := attr.Val
						downloadURL := s.config.BaseURL + fileName

						// Get file size and modification time from the page
						size, modTime := s.extractFileInfo(n)

						mirror := &SyslinuxMirror{
							Version:     version,
							FileName:    fileName,
							DownloadURL: downloadURL,
							Size:        size,
							ModifiedAt:  modTime,
						}

						mirrors = append(mirrors, mirror)
					}
				}
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			extractLinks(c)
		}
	}

	extractLinks(doc)
	return mirrors, nil
}

// extractFileInfo extracts file size and modification time from HTML context
func (s *service) extractFileInfo(node *html.Node) (int64, time.Time) {
	// Look for size and date information in the parent row
	parent := node.Parent
	if parent == nil {
		return 0, time.Time{}
	}

	// Find all text nodes in the row
	var texts []string
	var extractText func(*html.Node)
	extractText = func(n *html.Node) {
		if n.Type == html.TextNode {
			text := strings.TrimSpace(n.Data)
			if text != "" {
				texts = append(texts, text)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			extractText(c)
		}
	}

	extractText(parent)

	var size int64
	var modTime time.Time

	// Parse texts for size and date
	for _, text := range texts {
		// Try to parse as size (e.g., "5.2M", "1.1K", "123")
		if sizeBytes := s.parseSize(text); sizeBytes > 0 {
			size = sizeBytes
		}

		// Try to parse as date (various formats)
		if t := s.parseDate(text); !t.IsZero() {
			modTime = t
		}
	}

	return size, modTime
}

// parseSize parses size strings like "5.2M", "1.1K", "123"
func (s *service) parseSize(sizeStr string) int64 {
	sizeStr = strings.TrimSpace(sizeStr)
	if sizeStr == "" {
		return 0
	}

	// Extract number and unit
	var numStr string
	var unit string

	for i, r := range sizeStr {
		if r >= '0' && r <= '9' || r == '.' {
			numStr += string(r)
		} else {
			unit = sizeStr[i:]
			break
		}
	}

	if numStr == "" {
		return 0
	}

	num, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0
	}

	multiplier := int64(1)
	switch strings.ToUpper(unit) {
	case "K", "KB":
		multiplier = 1024
	case "M", "MB":
		multiplier = 1024 * 1024
	case "G", "GB":
		multiplier = 1024 * 1024 * 1024
	}

	return int64(num * float64(multiplier))
}

// parseDate parses various date formats commonly used on mirror sites
func (s *service) parseDate(dateStr string) time.Time {
	dateStr = strings.TrimSpace(dateStr)
	if dateStr == "" {
		return time.Time{}
	}

	// Common date formats
	formats := []string{
		"2006-01-02 15:04",
		"02-Jan-2006 15:04",
		"2006-01-02",
		"02/01/2006",
		"01/02/2006",
		time.RFC3339,
		time.RFC822,
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t
		}
	}

	return time.Time{}
}

// RefreshAvailableVersions scans the mirror and updates the database
func (s *service) RefreshAvailableVersions(ctx context.Context) error {
	// First, consolidate any duplicate entries from old system
	if err := s.consolidateDuplicateVersions(ctx); err != nil {
		return fmt.Errorf("failed to consolidate duplicate versions: %w", err)
	}

	mirrors, err := s.ScanMirror(ctx)
	if err != nil {
		return fmt.Errorf("failed to scan mirror: %w", err)
	}

	for _, mirror := range mirrors {
		// Check if version already exists
		existing, err := s.repo.GetVersionByNumber(ctx, mirror.Version)
		if err == nil && existing != nil {
			// Update existing version
			existing.DownloadURL = mirror.DownloadURL
			existing.FileName = mirror.FileName
			existing.Size = mirror.Size
			existing.UpdatedAt = time.Now()
			if err := s.repo.SaveVersion(ctx, existing); err != nil {
				return fmt.Errorf("failed to update version %s: %w", mirror.Version, err)
			}
		} else {
			// Create a single version entry (not per boot type)
			version := &SyslinuxVersion{
				ID:          mirror.Version,
				Version:     mirror.Version,
				BootType:    "", // Empty - supports both bios and efi
				DownloadURL: mirror.DownloadURL,
				FileName:    mirror.FileName,
				Size:        mirror.Size,
				Downloaded:  false,
				Active:      false,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}

			if err := s.repo.SaveVersion(ctx, version); err != nil {
				return fmt.Errorf("failed to save version %s: %w", mirror.Version, err)
			}
		}
	}

	return nil
}

// GetAvailableVersions returns all available versions from the database
func (s *service) GetAvailableVersions(ctx context.Context) ([]*SyslinuxVersion, error) {
	return s.repo.ListVersions(ctx)
}

// DownloadVersion downloads a specific version
func (s *service) DownloadVersion(ctx context.Context, version string) (*DownloadStatus, error) {
	// Get version info
	sysVersion, err := s.repo.GetVersionByNumber(ctx, version)
	if err != nil {
		return nil, fmt.Errorf("version not found: %w", err)
	}

	// Create download status
	status := &DownloadStatus{
		ID:        fmt.Sprintf("download-%s-%d", version, time.Now().Unix()),
		Version:   version,
		Status:    "downloading",
		Progress:  0,
		StartedAt: time.Now(),
	}

	if err := s.repo.SaveDownloadStatus(ctx, status); err != nil {
		return nil, fmt.Errorf("failed to save download status: %w", err)
	}

	// Start download in background
	go func() {
		defer func() {
			if r := recover(); r != nil {
				status.Status = "failed"
				status.ErrorMessage = fmt.Sprintf("panic: %v", r)
				now := time.Now()
				status.CompletedAt = &now
				s.repo.SaveDownloadStatus(ctx, status)
			}
		}()

		// Step 1: Clean up any existing active version
		if err := s.cleanupActiveVersion(ctx); err != nil {
			status.Status = "failed"
			status.ErrorMessage = "Failed to cleanup previous version: " + err.Error()
		} else if err := s.downloadFile(ctx, sysVersion, status); err != nil {
			status.Status = "failed"
			status.ErrorMessage = err.Error()
		} else {
			// Step 2: Install boot files automatically after extraction
			status.Status = "installing"
			s.repo.SaveDownloadStatus(ctx, status)

			// Install both BIOS and EFI boot files
			if err := s.InstallBootFiles(ctx, sysVersion.Version, "bios"); err != nil {
				status.Status = "failed"
				status.ErrorMessage = "Failed to install BIOS boot files: " + err.Error()
			} else if err := s.InstallBootFiles(ctx, sysVersion.Version, "efi"); err != nil {
				status.Status = "failed"
				status.ErrorMessage = "Failed to install EFI boot files: " + err.Error()
			} else {
				// Step 3: Mark as downloaded and active
				status.Status = "completed"
				status.Progress = 100

				sysVersion.Downloaded = true
				sysVersion.Active = true
				now := time.Now()
				sysVersion.DownloadedAt = &now
				sysVersion.UpdatedAt = now
				s.repo.SaveVersion(ctx, sysVersion)
			}
		}

		now := time.Now()
		status.CompletedAt = &now
		s.repo.SaveDownloadStatus(ctx, status)
	}()

	return status, nil
}

// downloadFile handles the actual file download
func (s *service) downloadFile(ctx context.Context, version *SyslinuxVersion, status *DownloadStatus) error {
	// Ensure temp directory exists
	if err := os.MkdirAll(s.config.TempDir, 0755); err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Download file
	resp, err := s.client.Get(version.DownloadURL)
	if err != nil {
		return fmt.Errorf("failed to start download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Create destination file
	filePath := filepath.Join(s.config.TempDir, version.FileName)
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Copy with progress tracking
	contentLength := resp.ContentLength
	if contentLength <= 0 {
		contentLength = version.Size
	}

	reader := &progressReader{
		reader: resp.Body,
		total:  contentLength,
		onProgress: func(current, total int64) {
			if total > 0 {
				progress := int(float64(current) / float64(total) * 100)
				status.Progress = progress
				s.repo.SaveDownloadStatus(ctx, status)
			}
		},
	}

	_, err = io.Copy(file, reader)
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}

	// Auto-extract if configured
	if s.config.AutoExtract {
		status.Status = "extracting"
		s.repo.SaveDownloadStatus(ctx, status)

		if err := s.ExtractBootFiles(ctx, version.Version); err != nil {
			return fmt.Errorf("failed to extract boot files: %w", err)
		}
	}

	return nil
}

// ExtractBootFiles extracts boot files from downloaded archive
func (s *service) ExtractBootFiles(ctx context.Context, version string) error {
	archivePath := filepath.Join(s.config.TempDir, fmt.Sprintf("syslinux-%s.tar.gz", version))

	// Open archive
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open archive: %w", err)
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	// Extract relevant files
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar entry: %w", err)
		}

		if header.Typeflag != tar.TypeReg {
			continue
		}

		// Check if this is a boot file we need
		if s.shouldExtractFile(header.Name, version) {
			if err := s.extractSingleFile(tr, header, version); err != nil {
				return fmt.Errorf("failed to extract %s: %w", header.Name, err)
			}
		}
	}

	// Clean up archive if not keeping
	if !s.config.KeepArchive {
		os.Remove(archivePath)
	}

	return nil
}

// shouldExtractFile determines if a file should be extracted
func (s *service) shouldExtractFile(filePath, version string) bool {
	// Remove version prefix (e.g., "syslinux-6.03/")
	parts := strings.Split(filePath, "/")
	if len(parts) < 2 {
		return false
	}

	// Skip the version directory
	relativePath := strings.Join(parts[1:], "/")

	// Check against required files for both BIOS and EFI
	biosFiles := GetRequiredBiosFiles()
	efiFiles := GetRequiredEfiFiles()

	// Check BIOS files
	for fileName := range biosFiles {
		if expectedPath := GetBootFileSourcePath("bios", fileName); expectedPath == relativePath {
			return true
		}
	}

	// Check EFI files
	for fileName := range efiFiles {
		if expectedPath := GetBootFileSourcePath("efi", fileName); expectedPath == relativePath {
			return true
		}
	}

	return false
}

// extractSingleFile extracts a single file from the archive
func (s *service) extractSingleFile(tr *tar.Reader, header *tar.Header, version string) error {
	// Determine boot type and target directory
	var bootType, targetDir string

	if strings.Contains(header.Name, "/bios/") {
		bootType = "bios"
		targetDir = filepath.Join(s.config.TFTPDir, s.config.BiosDir)
	} else if strings.Contains(header.Name, "/efi") {
		bootType = "efi"
		targetDir = filepath.Join(s.config.TFTPDir, s.config.EfiDir)
	} else {
		return fmt.Errorf("unknown boot type for file %s", header.Name)
	}

	// Ensure target directory exists
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	// Get filename from path
	fileName := filepath.Base(header.Name)
	targetPath := filepath.Join(targetDir, fileName)

	// Create target file
	outFile, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("failed to create target file: %w", err)
	}
	defer outFile.Close()

	// Copy file data
	if _, err := io.Copy(outFile, tr); err != nil {
		return fmt.Errorf("failed to copy file data: %w", err)
	}

	// Set executable permissions for certain files
	if fileName == "pxelinux.0" || fileName == "syslinux.efi" {
		if err := os.Chmod(targetPath, 0755); err != nil {
			return fmt.Errorf("failed to set permissions: %w", err)
		}
	}

	// Save boot file record
	bootFile := &SyslinuxBootFile{
		ID:          fmt.Sprintf("%s-%s-%s", version, bootType, fileName),
		Version:     version,
		BootType:    bootType,
		FileName:    fileName,
		FilePath:    targetPath,
		Size:        header.Size,
		Description: s.getFileDescription(bootType, fileName),
		Required:    s.isRequiredFile(bootType, fileName),
		Installed:   true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	return s.repo.SaveBootFile(context.Background(), bootFile)
}

// getFileDescription returns a description for a boot file
func (s *service) getFileDescription(bootType, fileName string) string {
	if bootType == "bios" {
		if desc, ok := GetRequiredBiosFiles()[fileName]; ok {
			return desc
		}
	} else if bootType == "efi" {
		if desc, ok := GetRequiredEfiFiles()[fileName]; ok {
			return desc
		}
	}
	return "Boot file"
}

// isRequiredFile checks if a file is required
func (s *service) isRequiredFile(bootType, fileName string) bool {
	if bootType == "bios" {
		_, ok := GetRequiredBiosFiles()[fileName]
		return ok
	} else if bootType == "efi" {
		_, ok := GetRequiredEfiFiles()[fileName]
		return ok
	}
	return false
}

// GetDownloadStatus retrieves download status
func (s *service) GetDownloadStatus(ctx context.Context, id string) (*DownloadStatus, error) {
	return s.repo.GetDownloadStatus(ctx, id)
}

// CancelDownload cancels an ongoing download
func (s *service) CancelDownload(ctx context.Context, id string) error {
	status, err := s.repo.GetDownloadStatus(ctx, id)
	if err != nil {
		return err
	}

	status.Status = "cancelled"
	now := time.Now()
	status.CompletedAt = &now

	return s.repo.SaveDownloadStatus(ctx, status)
}

// Additional service methods would continue here...
// (InstallBootFiles, ListInstalledBootFiles, RemoveBootFiles, etc.)

// progressReader tracks download progress
type progressReader struct {
	reader     io.Reader
	total      int64
	current    int64
	onProgress func(current, total int64)
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	pr.current += int64(n)
	if pr.onProgress != nil {
		pr.onProgress(pr.current, pr.total)
	}
	return n, err
}

// Implement remaining Service interface methods...
func (s *service) InstallBootFiles(ctx context.Context, version, bootType string) error {
	// Implementation for installing boot files
	return nil
}

func (s *service) ListInstalledBootFiles(ctx context.Context, bootType string) ([]*SyslinuxBootFile, error) {
	return s.repo.ListBootFiles(ctx, "", bootType)
}

func (s *service) RemoveBootFiles(ctx context.Context, version, bootType string) error {
	// Get the appropriate boot directory
	var bootDir string
	var requiredFiles map[string]string

	switch bootType {
	case "bios":
		bootDir = filepath.Join(s.config.TFTPDir, "boot-bios")
		requiredFiles = GetRequiredBiosFiles()
	case "efi":
		bootDir = filepath.Join(s.config.TFTPDir, "boot-efi")
		requiredFiles = GetRequiredEfiFiles()
	default:
		return fmt.Errorf("unsupported boot type: %s", bootType)
	}

	// Remove each required file
	for fileName := range requiredFiles {
		filePath := filepath.Join(bootDir, fileName)

		// Check if file exists before trying to remove
		if _, err := os.Stat(filePath); err == nil {
			if err := os.Remove(filePath); err != nil {
				log.Printf("Warning: failed to remove %s boot file %s: %v", bootType, fileName, err)
				// Continue with other files even if one fails
			} else {
				log.Printf("Removed %s boot file: %s", bootType, fileName)
			}
		}
	}

	return nil
}

func (s *service) GetConfig() SyslinuxConfig {
	return s.config
}

func (s *service) UpdateConfig(ctx context.Context, config SyslinuxConfig) error {
	s.config = config
	return nil
}

func (s *service) GetSystemStatus(ctx context.Context) (*SystemStatus, error) {
	// Implementation for getting system status
	return &SystemStatus{}, nil
}

func (s *service) ValidateInstallation(ctx context.Context, bootType string) (*ValidationResult, error) {
	// Implementation for validating installation
	return &ValidationResult{}, nil
}

func (s *service) CheckDiskSpace(ctx context.Context) (*DiskSpaceInfo, error) {
	// Implementation for checking disk space
	return &DiskSpaceInfo{}, nil
}

// cleanupActiveVersion removes any currently active version and cleans up files
func (s *service) cleanupActiveVersion(ctx context.Context) error {
	versions, err := s.repo.ListVersions(ctx)
	if err != nil {
		return fmt.Errorf("failed to list versions: %w", err)
	}

	for _, version := range versions {
		if version.Downloaded || version.Active {
			// Remove boot files
			s.RemoveBootFiles(ctx, version.Version, "bios")
			s.RemoveBootFiles(ctx, version.Version, "efi")

			// Remove downloaded archive
			archivePath := filepath.Join(s.config.TempDir, fmt.Sprintf("syslinux-%s.tar.gz", version.Version))
			os.Remove(archivePath)

			// Mark as not downloaded and not active
			version.Downloaded = false
			version.Active = false
			version.DownloadedAt = nil
			s.repo.SaveVersion(ctx, version)
		}
	}

	return nil
}

// DeactivateVersion deactivates and removes the currently active version
func (s *service) DeactivateVersion(ctx context.Context, version string) error {
	// Get the version to deactivate
	sysVersion, err := s.repo.GetVersionByNumber(ctx, version)
	if err != nil {
		return fmt.Errorf("version not found: %w", err)
	}

	if !sysVersion.Active {
		return fmt.Errorf("version %s is not active", version)
	}

	// Remove boot files
	if err := s.RemoveBootFiles(ctx, version, "bios"); err != nil {
		return fmt.Errorf("failed to remove BIOS boot files: %w", err)
	}

	if err := s.RemoveBootFiles(ctx, version, "efi"); err != nil {
		return fmt.Errorf("failed to remove EFI boot files: %w", err)
	}

	// Mark as not downloaded and not active (completely remove)
	sysVersion.Downloaded = false
	sysVersion.Active = false
	sysVersion.DownloadedAt = nil

	// Save the updated version back to repository
	return s.repo.SaveVersion(ctx, sysVersion)
}

// consolidateDuplicateVersions removes old boot-type-specific entries and keeps only single version entries
func (s *service) consolidateDuplicateVersions(ctx context.Context) error {
	versions, err := s.repo.ListVersions(ctx)
	if err != nil {
		return fmt.Errorf("failed to list versions: %w", err)
	}

	// Group versions by version number
	versionGroups := make(map[string][]*SyslinuxVersion)
	for _, version := range versions {
		versionGroups[version.Version] = append(versionGroups[version.Version], version)
	}

	// For each version number, consolidate duplicates
	for _, versionList := range versionGroups {
		if len(versionList) > 1 {
			// Find the best entry to keep (prefer one with simple ID = version number)
			var keeper *SyslinuxVersion
			var toDelete []*SyslinuxVersion

			for _, v := range versionList {
				if v.ID == v.Version {
					// This is the new format (ID = version number)
					keeper = v
				} else {
					// This is old format (ID = version-boottype)
					toDelete = append(toDelete, v)
				}
			}

			// If no new format found, convert the first old one to new format
			if keeper == nil && len(versionList) > 0 {
				keeper = versionList[0]
				keeper.ID = keeper.Version
				keeper.BootType = "" // Clear boot type for single-version approach
				toDelete = versionList[1:]
			}

			// Delete old format entries
			for _, v := range toDelete {
				s.repo.DeleteVersion(ctx, v.ID)
			}

			// Save the consolidated version
			if keeper != nil {
				s.repo.SaveVersion(ctx, keeper)
			}
		}
	}

	return nil
}
