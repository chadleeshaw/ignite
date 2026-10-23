package osimage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"ignite/config"
	"ignite/dlstatus"
	"ignite/osimage/catalog"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// errDownloadTerminal is returned when a worker tries to persist progress for
// a download whose stored row has already reached a terminal state.
var errDownloadTerminal = errors.New("download already in terminal state")

// downloadRequest is a unit of work for the background download worker.
type downloadRequest struct {
	ctx      context.Context
	id       string
	osConfig OSImageConfig
	release  catalog.Release
}

// osImageServiceImpl provides business logic for OS image management.
// The struct is unexported; callers use the OSImageService interface.
type osImageServiceImpl struct {
	repo         OSImageRepository
	downloadRepo DownloadStatusRepository
	config       *config.Config
	catalog      catalog.Catalog
	httpClient   *http.Client
	downloadChan chan downloadRequest

	mu      sync.Mutex
	cancels map[string]context.CancelFunc // download ID -> cancel func
}

// NewOSImageService creates a new OS image service. cat supplies release
// URLs and the versions a download is allowed to request, including releases
// that are past end of life so an existing image can still be resolved.
func NewOSImageService(repo OSImageRepository, downloadRepo DownloadStatusRepository, cfg *config.Config, cat catalog.Catalog) OSImageService {
	service := &osImageServiceImpl{
		repo:         repo,
		downloadRepo: downloadRepo,
		config:       cfg,
		catalog:      cat,
		httpClient: &http.Client{
			Timeout: 30 * time.Minute,
		},
		downloadChan: make(chan downloadRequest, 10),
		cancels:      make(map[string]context.CancelFunc),
	}

	// Start background download worker
	go service.downloadWorker()

	return service
}

// GetAllOSImages retrieves all OS images
func (s *osImageServiceImpl) GetAllOSImages(ctx context.Context) ([]*OSImage, error) {
	return s.repo.GetAll(ctx)
}

// GetOSImagesByOS retrieves all images for a specific operating system
func (s *osImageServiceImpl) GetOSImagesByOS(ctx context.Context, osName string) ([]*OSImage, error) {
	return s.repo.GetByOS(ctx, osName)
}

// GetOSImage retrieves an OS image by ID
func (s *osImageServiceImpl) GetOSImage(ctx context.Context, id string) (*OSImage, error) {
	return s.repo.Get(ctx, id)
}

// GetDefaultVersion retrieves the default version for an OS
func (s *osImageServiceImpl) GetDefaultVersion(ctx context.Context, osName string) (*OSImage, error) {
	return s.repo.GetDefaultVersion(ctx, osName)
}

// SetDefaultVersion sets the default version for an OS
func (s *osImageServiceImpl) SetDefaultVersion(ctx context.Context, id string) error {
	return s.repo.SetDefault(ctx, id)
}

// DeleteOSImage deletes an OS image
func (s *osImageServiceImpl) DeleteOSImage(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

// GetDownloadStatus retrieves the status of a download.
// The returned struct is a copy; the worker never shares its live struct.
func (s *osImageServiceImpl) GetDownloadStatus(ctx context.Context, id string) (*DownloadStatus, error) {
	status, err := s.downloadRepo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	statusCopy := *status
	return &statusCopy, nil
}

// GetActiveDownloads retrieves all active downloads
func (s *osImageServiceImpl) GetActiveDownloads(ctx context.Context) ([]*DownloadStatus, error) {
	return s.downloadRepo.GetActive(ctx)
}

// removeCancel drops the cancel func for a download ID. It is called when a
// download finishes, fails, is cancelled, or is dequeued by the worker.
func (s *osImageServiceImpl) removeCancel(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cancels, id)
}

// DownloadOSImage initiates a download of an OS image.
// It returns a copy of the queued status; the worker mutates only its own
// re-read copy afterwards.
func (s *osImageServiceImpl) DownloadOSImage(ctx context.Context, osConfig OSImageConfig) (*DownloadStatus, error) {
	release, ok := s.lookupRelease(ctx, osConfig)
	if !ok {
		return nil, fmt.Errorf("unsupported OS/version combination: %s %s", osConfig.OS, osConfig.Version)
	}

	// Check if image already exists
	existing, err := s.repo.GetByOSAndVersion(ctx, osConfig.OS, osConfig.Version)
	if err == nil && existing != nil {
		return nil, fmt.Errorf("OS image already exists: %s %s", osConfig.OS, osConfig.Version)
	}

	status := &DownloadStatus{
		ID:        uuid.New().String(),
		OS:        osConfig.OS,
		Version:   osConfig.Version,
		Status:    dlstatus.StatusQueued,
		Progress:  0,
		StartedAt: time.Now(),
	}

	// Each download gets its own cancellable context, tracked by download ID.
	dlCtx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.cancels[status.ID] = cancel
	s.mu.Unlock()

	req := downloadRequest{
		ctx:      dlCtx,
		id:       status.ID,
		osConfig: osConfig,
		release:  release,
	}

	// Persist the queued status BEFORE enqueueing so the worker can never
	// observe a request without its row.
	if err := s.downloadRepo.Save(ctx, status); err != nil {
		cancel()
		s.removeCancel(status.ID)
		return nil, fmt.Errorf("failed to save download status: %w", err)
	}

	select {
	case s.downloadChan <- req:
	default:
		cancel()
		s.removeCancel(status.ID)
		status.Status = dlstatus.StatusFailed
		status.ErrorMessage = "download queue is full, try again later"
		now := time.Now()
		status.CompletedAt = &now
		if err := s.downloadRepo.Save(ctx, status); err != nil {
			return nil, fmt.Errorf("download queue is full (failed to record status: %w)", err)
		}
		return nil, fmt.Errorf("download queue is full, try again later")
	}

	// Return a copy so HTTP callers never observe worker-mutated state.
	statusCopy := *status
	return &statusCopy, nil
}

// CancelDownload cancels an active download. It signals the worker via the
// download's context and marks the row cancelled in the same critical
// section used by worker saves, so the two can never overwrite each other.
func (s *osImageServiceImpl) CancelDownload(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	status, err := s.downloadRepo.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("download not found: %s", id)
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
	status.Progress = 0
	status.ErrorMessage = "Download cancelled by user"
	status.CompletedAt = &now

	if err := s.downloadRepo.Save(ctx, status); err != nil {
		return fmt.Errorf("failed to save cancelled status: %w", err)
	}
	return nil
}

// downloadWorker processes download requests sequentially
func (s *osImageServiceImpl) downloadWorker() {
	for req := range s.downloadChan {
		s.processDownload(req)
	}
}

// saveStatusChecked persists status only if the stored row has not reached a
// terminal state. It re-reads the row under the service mutex (the same
// mutex CancelDownload uses), so a concurrent cancellation is never
// overwritten with stale worker data.
func (s *osImageServiceImpl) saveStatusChecked(ctx context.Context, status *DownloadStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, err := s.downloadRepo.Get(ctx, status.ID)
	if err != nil {
		return err
	}
	if current.Status.Terminal() {
		return errDownloadTerminal
	}
	return s.downloadRepo.Save(ctx, status)
}

// failDownload records a terminal failure for the download unless the row was
// already cancelled or failed concurrently.
func (s *osImageServiceImpl) failDownload(ctx context.Context, status *DownloadStatus, err error) {
	status.Status = dlstatus.StatusFailed
	status.ErrorMessage = err.Error()
	now := time.Now()
	status.CompletedAt = &now
	if saveErr := s.saveStatusChecked(ctx, status); saveErr != nil {
		log.Printf("Download %s failed (%v); could not persist failure: %v", status.ID, err, saveErr)
		return
	}
	log.Printf("Download %s failed: %v", status.ID, err)
}

// processDownload handles the actual download of OS image files
func (s *osImageServiceImpl) processDownload(req downloadRequest) {
	defer s.removeCancel(req.id)

	ctx := req.ctx

	// Re-read the row: the download may have been cancelled while queued.
	status, err := s.downloadRepo.Get(ctx, req.id)
	if err != nil {
		log.Printf("Download %s: status not found, aborting: %v", req.id, err)
		return
	}
	if status.Status.Terminal() {
		return
	}

	status.Status = dlstatus.StatusDownloading
	status.Progress = 5
	if err := s.saveStatusChecked(ctx, status); err != nil {
		return
	}

	osConfig := req.osConfig
	release := req.release

	arch := osConfig.Architecture
	if arch == "" {
		arch = release.Arch
	}
	if arch == "" {
		arch = "x86_64"
	}
	kernelURL := release.KernelURL
	initrdURL := release.InitrdURL
	if kernelURL == "" || initrdURL == "" {
		s.failDownload(ctx, status, fmt.Errorf("release %s %s is missing a boot file URL", osConfig.OS, osConfig.Version))
		return
	}

	kernelRel, err := RelPath(osConfig.OS, osConfig.Version, KernelName)
	if err != nil {
		s.failDownload(ctx, status, err)
		return
	}
	initrdRel, err := RelPath(osConfig.OS, osConfig.Version, InitrdName)
	if err != nil {
		s.failDownload(ctx, status, err)
		return
	}
	kernelPath := filepath.Join(s.config.TFTP.Dir, filepath.FromSlash(kernelRel))
	initrdPath := filepath.Join(s.config.TFTP.Dir, filepath.FromSlash(initrdRel))
	if err := os.MkdirAll(filepath.Dir(kernelPath), 0755); err != nil {
		s.failDownload(ctx, status, fmt.Errorf("failed to create download directory: %w", err))
		return
	}

	if err := ctx.Err(); err != nil {
		return // cancelled; CancelDownload already marked the row
	}

	kernelSize, kernelChecksum, err := s.downloadFile(ctx, kernelURL, kernelPath)
	if err != nil {
		s.failDownload(ctx, status, fmt.Errorf("failed to download kernel: %w", err))
		return
	}

	status.Progress = 50
	if err := s.saveStatusChecked(ctx, status); err != nil {
		_ = os.Remove(kernelPath)
		return
	}

	if err := ctx.Err(); err != nil {
		_ = os.Remove(kernelPath)
		return
	}

	initrdSize, initrdChecksum, err := s.downloadFile(ctx, initrdURL, initrdPath)
	if err != nil {
		_ = os.Remove(kernelPath)
		s.failDownload(ctx, status, fmt.Errorf("failed to download initrd: %w", err))
		return
	}

	if err := ctx.Err(); err != nil {
		_ = os.Remove(kernelPath)
		_ = os.Remove(initrdPath)
		return
	}

	// Create OS image record
	now := time.Now()
	image := &OSImage{
		ID:           uuid.New().String(),
		OS:           osConfig.OS,
		Version:      osConfig.Version,
		Architecture: arch,
		KernelPath:   kernelPath,
		InitrdPath:   initrdPath,
		KernelSize:   kernelSize,
		InitrdSize:   initrdSize,
		Checksum:     kernelChecksum + ":" + initrdChecksum,
		Active:       false,
		DownloadURL:  kernelURL,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.repo.Save(ctx, image); err != nil {
		_ = os.Remove(kernelPath)
		_ = os.Remove(initrdPath)
		s.failDownload(ctx, status, fmt.Errorf("failed to save OS image record: %w", err))
		return
	}

	// Mark download as completed
	status.Status = dlstatus.StatusCompleted
	status.Progress = 100
	completed := time.Now()
	status.CompletedAt = &completed
	if err := s.saveStatusChecked(ctx, status); err != nil {
		// The row went terminal concurrently (e.g. cancelled): roll back the
		// image record and files rather than leaving an orphan.
		_ = s.repo.Delete(ctx, image.ID)
		_ = os.Remove(kernelPath)
		_ = os.Remove(initrdPath)
		return
	}

	log.Printf("Successfully downloaded OS image: %s %s", osConfig.OS, osConfig.Version)
}

// downloadFile downloads a file from URL to destPath. The data is streamed to
// a temp file in the destination directory and renamed into place only on
// success, so a failed or cancelled download never leaves a partial file at
// destPath. It returns the size and SHA256 hex digest of the downloaded file.
func (s *osImageServiceImpl) downloadFile(ctx context.Context, url, destPath string) (int64, string, error) {
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return 0, "", fmt.Errorf("failed to create destination directory: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(destPath), ".download-*")
	if err != nil {
		return 0, "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpName := tmp.Name()
	succeeded := false
	defer func() {
		if !succeeded {
			_ = os.Remove(tmpName)
		}
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		_ = tmp.Close()
		return 0, "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		_ = tmp.Close()
		return 0, "", fmt.Errorf("failed to download %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_ = tmp.Close()
		return 0, "", fmt.Errorf("failed to download %s: HTTP %s", url, resp.Status)
	}

	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, hash), resp.Body)
	closeErr := tmp.Close()
	if err != nil {
		return 0, "", fmt.Errorf("failed to write download %s: %w", url, err)
	}
	if closeErr != nil {
		return 0, "", fmt.Errorf("failed to close temp file for %s: %w", url, closeErr)
	}

	checksum := hex.EncodeToString(hash.Sum(nil))
	if err := os.Rename(tmpName, destPath); err != nil {
		return 0, "", fmt.Errorf("failed to finalize %s: %w", destPath, err)
	}
	succeeded = true
	return size, checksum, nil
}

// GetAvailableVersions returns non-EOL versions for an OS, newest first.
func (s *osImageServiceImpl) GetAvailableVersions(ctx context.Context, osName string) ([]string, error) {
	if s.catalog == nil {
		return nil, fmt.Errorf("unsupported OS: %s", osName)
	}
	snap, err := s.catalog.Current(ctx)
	if err != nil {
		return nil, fmt.Errorf("unsupported OS: %s", osName)
	}
	known := false
	for _, rel := range snap.Releases {
		if rel.OS == osName {
			known = true
			break
		}
	}
	if !known {
		return nil, fmt.Errorf("unsupported OS: %s", osName)
	}
	views := catalog.Versions(osName, snap.Releases, time.Now(), false)
	versions := make([]string, len(views))
	for i, view := range views {
		versions[i] = view.Value
	}
	return versions, nil
}

// GetSupportedOSes returns list of supported operating systems
func (s *osImageServiceImpl) GetSupportedOSes() []string {
	var oses []string
	for osName := range s.config.OSImages.Sources {
		oses = append(oses, osName)
	}
	sort.Strings(oses)
	return oses
}

// lookupRelease resolves a download against the catalog, including releases
// past end of life. An empty architecture matches the catalog's arch.
func (s *osImageServiceImpl) lookupRelease(ctx context.Context, osConfig OSImageConfig) (catalog.Release, bool) {
	if s.catalog == nil {
		return catalog.Release{}, false
	}
	arch := osConfig.Architecture
	if arch == "" {
		arch = "x86_64"
	}
	return s.catalog.Lookup(ctx, osConfig.OS, osConfig.Version, arch)
}
