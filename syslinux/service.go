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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"ignite/dlstatus"

	"github.com/google/uuid"
	"golang.org/x/net/html"
)

type service struct {
	repo   Repository
	config SyslinuxConfig
	client *http.Client

	mu      sync.Mutex
	cancels map[string]context.CancelFunc // download ID -> cancel func
}

// NewService creates a new Syslinux service
func NewService(repo Repository, config SyslinuxConfig) Service {
	return &service{
		repo:    repo,
		config:  config,
		client:  &http.Client{Timeout: 30 * time.Second},
		cancels: make(map[string]context.CancelFunc),
	}
}

// removeCancel drops the cancel func for a download ID.
func (s *service) removeCancel(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cancels, id)
}

// updateStatus re-reads the download row and applies the new state only if it
// has not reached a terminal state. It runs under the service mutex (the same
// mutex CancelDownload uses), so a concurrent cancellation is never
// overwritten with stale worker data. It returns false when the row is
// missing or already terminal.
func (s *service) updateStatus(ctx context.Context, id string, status dlstatus.Status, progress int, errMsg string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, err := s.repo.GetDownloadStatus(ctx, id)
	if err != nil {
		return false
	}
	if current.Status.Terminal() {
		return false
	}
	current.Status = status
	current.Progress = progress
	if errMsg != "" {
		current.ErrorMessage = errMsg
	}
	if status.Terminal() {
		now := time.Now()
		current.CompletedAt = &now
	}
	if err := s.repo.SaveDownloadStatus(ctx, current); err != nil {
		log.Printf("syslinux: failed to save download status %s: %v", id, err)
		return false
	}
	return true
}

// failStatus records a terminal failure unless the row already went terminal.
func (s *service) failStatus(ctx context.Context, id, errMsg string) {
	if !s.updateStatus(ctx, id, dlstatus.StatusFailed, 0, errMsg) {
		log.Printf("syslinux: download %s failed (%s); status already terminal", id, errMsg)
		return
	}
	log.Printf("syslinux: download %s failed: %s", id, errMsg)
}

// ScanMirror scans the kernel.org mirror for available Syslinux versions
func (s *service) ScanMirror(ctx context.Context) ([]*SyslinuxMirror, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.config.BaseURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create mirror request: %w", err)
	}
	resp, err := s.client.Do(req)
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

// DownloadVersion downloads a specific version in the background.
// It returns a copy of the queued status; the worker only ever mutates
// re-read copies afterwards.
func (s *service) DownloadVersion(ctx context.Context, version string) (*DownloadStatus, error) {
	// Get version info
	sysVersion, err := s.repo.GetVersionByNumber(ctx, version)
	if err != nil {
		return nil, fmt.Errorf("version not found: %w", err)
	}

	// Create download status with a collision-resistant ID
	status := &DownloadStatus{
		ID:        uuid.New().String(),
		Version:   version,
		Status:    dlstatus.StatusQueued,
		Progress:  0,
		StartedAt: time.Now(),
	}

	if err := s.repo.SaveDownloadStatus(ctx, status); err != nil {
		return nil, fmt.Errorf("failed to save download status: %w", err)
	}

	dlCtx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.cancels[status.ID] = cancel
	s.mu.Unlock()

	// Start download in background
	go s.downloadAndInstall(dlCtx, status.ID, sysVersion)

	// Return a copy so the HTTP caller never sees worker-mutated state.
	statusCopy := *status
	return &statusCopy, nil
}

// downloadAndInstall runs the download pipeline in the background:
// download -> extract -> verify, and only then clean up the old active
// version and swap to the new one. A cancelled context aborts promptly and
// leaves the previously active version untouched.
func (s *service) downloadAndInstall(ctx context.Context, statusID string, sysVersion *SyslinuxVersion) {
	defer s.removeCancel(statusID)

	defer func() {
		if r := recover(); r != nil {
			s.failStatus(context.Background(), statusID, fmt.Sprintf("panic: %v", r))
		}
	}()

	bg := context.Background()

	if !s.updateStatus(ctx, statusID, dlstatus.StatusDownloading, 0, "") {
		return // cancelled while queued
	}

	// Step 1: download the archive (temp file + rename on success)
	if err := s.downloadFile(ctx, statusID, sysVersion); err != nil {
		if ctx.Err() != nil {
			s.markCancelled(bg, statusID)
		} else {
			s.failStatus(bg, statusID, err.Error())
		}
		return
	}

	if ctx.Err() != nil {
		s.markCancelled(bg, statusID)
		return
	}

	// Step 2: extract
	if !s.updateStatus(ctx, statusID, dlstatus.StatusExtracting, 90, "") {
		return
	}
	if err := s.ExtractBootFiles(ctx, sysVersion.Version); err != nil {
		if ctx.Err() != nil {
			s.markCancelled(bg, statusID)
		} else {
			s.failStatus(bg, statusID, fmt.Sprintf("failed to extract boot files: %v", err))
		}
		return
	}

	if ctx.Err() != nil {
		s.markCancelled(bg, statusID)
		return
	}

	// Step 3: verify the extracted files before touching the active version
	if err := s.verifyExtractedFiles(sysVersion.Version); err != nil {
		s.failStatus(bg, statusID, err.Error())
		return
	}

	if ctx.Err() != nil {
		s.markCancelled(bg, statusID)
		return
	}

	// Step 4: only now clean up the old active version and swap to the new one
	if err := s.cleanupActiveVersion(bg); err != nil {
		s.failStatus(bg, statusID, "failed to cleanup previous version: "+err.Error())
		return
	}

	now := time.Now()
	sysVersion.Downloaded = true
	sysVersion.Active = true
	sysVersion.DownloadedAt = &now
	if err := s.repo.SaveVersion(bg, sysVersion); err != nil {
		s.failStatus(bg, statusID, "failed to save version: "+err.Error())
		return
	}
	if err := s.repo.SetActiveVersion(bg, sysVersion.Version); err != nil {
		s.failStatus(bg, statusID, "failed to activate version: "+err.Error())
		return
	}

	s.updateStatus(bg, statusID, dlstatus.StatusCompleted, 100, "")
	log.Printf("syslinux: successfully downloaded and activated version %s", sysVersion.Version)
}

// markCancelled records cancellation unless the row already went terminal.
func (s *service) markCancelled(ctx context.Context, id string) {
	s.updateStatus(ctx, id, dlstatus.StatusCancelled, 0, "Download cancelled by user")
}

// throttledProgress forwards progress updates at most every 500ms or when the
// whole percentage changes, so a download doesn't write to the DB per chunk.
type throttledProgress struct {
	update   func(pct int)
	mu       sync.Mutex
	lastPct  int
	lastTime time.Time
}

func (t *throttledProgress) onProgress(current, total int64) {
	if total <= 0 {
		return
	}
	pct := int(float64(current) / float64(total) * 100)
	if pct > 100 {
		pct = 100
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if pct == t.lastPct && time.Since(t.lastTime) < 500*time.Millisecond {
		return
	}
	t.lastPct = pct
	t.lastTime = time.Now()
	t.update(pct)
}

// downloadFile downloads the version archive to the temp directory via a temp
// file that is renamed into place only on success.
func (s *service) downloadFile(ctx context.Context, statusID string, version *SyslinuxVersion) error {
	// Ensure temp directory exists
	if err := os.MkdirAll(s.config.TempDir, 0755); err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, version.DownloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create download request: %w", err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to start download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	tmp, err := os.CreateTemp(s.config.TempDir, ".download-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpName := tmp.Name()
	succeeded := false
	defer func() {
		if !succeeded {
			_ = os.Remove(tmpName)
		}
	}()

	// Copy with throttled progress tracking
	contentLength := resp.ContentLength
	if contentLength <= 0 {
		contentLength = version.Size
	}

	progress := &throttledProgress{
		update: func(pct int) {
			s.updateStatus(ctx, statusID, dlstatus.StatusDownloading, pct, "")
		},
	}

	reader := &progressReader{
		reader:     resp.Body,
		total:      contentLength,
		onProgress: progress.onProgress,
	}

	if _, err := io.Copy(tmp, reader); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to download file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	filePath := filepath.Join(s.config.TempDir, version.FileName)
	if err := os.Rename(tmpName, filePath); err != nil {
		return fmt.Errorf("failed to finalize download: %w", err)
	}
	succeeded = true
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
		if err := ctx.Err(); err != nil {
			return err
		}

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
			if err := s.extractSingleFile(ctx, tr, header, version); err != nil {
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

// extractSingleFile extracts a single file from the archive using the
// caller's context for repository writes.
func (s *service) extractSingleFile(ctx context.Context, tr *tar.Reader, header *tar.Header, version string) error {
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

	return s.repo.SaveBootFile(ctx, bootFile)
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

// GetDownloadStatus retrieves download status.
// The returned struct is a copy; the worker never shares its live struct.
func (s *service) GetDownloadStatus(ctx context.Context, id string) (*DownloadStatus, error) {
	status, err := s.repo.GetDownloadStatus(ctx, id)
	if err != nil {
		return nil, err
	}
	statusCopy := *status
	return &statusCopy, nil
}

// CancelDownload cancels an ongoing download: it signals the worker via the
// download's context and marks the row cancelled under the same mutex the
// worker uses for status updates, so the two can never overwrite each other.
func (s *service) CancelDownload(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	status, err := s.repo.GetDownloadStatus(ctx, id)
	if err != nil {
		return err
	}

	if !status.Status.Active() {
		return fmt.Errorf("cannot cancel download with status %s", status.Status)
	}

	if cancel, ok := s.cancels[id]; ok {
		delete(s.cancels, id)
		cancel()
	}

	now := time.Now()
	status.Status = dlstatus.StatusCancelled
	status.CompletedAt = &now

	return s.repo.SaveDownloadStatus(ctx, status)
}

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

// verifyExtractedFiles checks that every required BIOS and EFI boot file for
// the version exists in the configured TFTP directories.
func (s *service) verifyExtractedFiles(version string) error {
	bootTypes := map[string]map[string]string{
		"bios": GetRequiredBiosFiles(),
		"efi":  GetRequiredEfiFiles(),
	}
	for bootType, files := range bootTypes {
		var dir string
		switch bootType {
		case "bios":
			dir = filepath.Join(s.config.TFTPDir, s.config.BiosDir)
		case "efi":
			dir = filepath.Join(s.config.TFTPDir, s.config.EfiDir)
		}
		var missing []string
		for fileName := range files {
			if _, err := os.Stat(filepath.Join(dir, fileName)); err != nil {
				missing = append(missing, fileName)
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			return fmt.Errorf("missing %s boot files for version %s: %s",
				bootType, version, strings.Join(missing, ", "))
		}
	}
	return nil
}

// InstallBootFiles ensures the boot files for version/bootType are installed:
// every required file must exist in the configured TFTP boot directory, and
// its boot-file record is created or marked installed. It returns an error
// listing any missing files instead of silently succeeding.
func (s *service) InstallBootFiles(ctx context.Context, version, bootType string) error {
	var required map[string]string
	var targetDir string
	switch bootType {
	case "bios":
		required = GetRequiredBiosFiles()
		targetDir = filepath.Join(s.config.TFTPDir, s.config.BiosDir)
	case "efi":
		required = GetRequiredEfiFiles()
		targetDir = filepath.Join(s.config.TFTPDir, s.config.EfiDir)
	default:
		return fmt.Errorf("unsupported boot type: %s", bootType)
	}

	sysVersion, err := s.repo.GetVersionByNumber(ctx, version)
	if err != nil {
		return fmt.Errorf("version not found: %w", err)
	}
	if !sysVersion.Downloaded {
		return fmt.Errorf("version %s has not been downloaded", version)
	}

	var missing []string
	for fileName, description := range required {
		targetPath := filepath.Join(targetDir, fileName)
		fi, err := os.Stat(targetPath)
		if err != nil {
			missing = append(missing, fileName)
			continue
		}

		id := fmt.Sprintf("%s-%s-%s", version, bootType, fileName)
		bootFile, err := s.repo.GetBootFile(ctx, id)
		if err != nil {
			bootFile = &SyslinuxBootFile{
				ID:          id,
				Version:     version,
				BootType:    bootType,
				FileName:    fileName,
				Description: description,
				Required:    true,
				CreatedAt:   time.Now(),
			}
		}
		bootFile.FilePath = targetPath
		bootFile.Size = fi.Size()
		bootFile.Installed = true
		if err := s.repo.SaveBootFile(ctx, bootFile); err != nil {
			return fmt.Errorf("failed to record boot file %s: %w", fileName, err)
		}
	}

	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("missing %s boot files for version %s: %s",
			bootType, version, strings.Join(missing, ", "))
	}
	return nil
}

func (s *service) ListInstalledBootFiles(ctx context.Context, bootType string) ([]*SyslinuxBootFile, error) {
	return s.repo.ListBootFiles(ctx, "", bootType)
}

func (s *service) RemoveBootFiles(ctx context.Context, version, bootType string) error {
	// Get the appropriate boot directory from configuration
	var bootDir string
	var requiredFiles map[string]string

	switch bootType {
	case "bios":
		bootDir = filepath.Join(s.config.TFTPDir, s.config.BiosDir)
		requiredFiles = GetRequiredBiosFiles()
	case "efi":
		bootDir = filepath.Join(s.config.TFTPDir, s.config.EfiDir)
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

// cleanupActiveVersion removes any currently active version and cleans up files.
// It is only called after the new version has been fully downloaded,
// extracted, and verified.
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
