package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"ignite/osimage"
	"ignite/osimage/catalog"
	"net/http"
	"sort"
	"time"

	"github.com/gorilla/mux"
)

// OSImageHandlers handles OS image management requests
type OSImageHandlers struct {
	container *Container
}

// NewOSImageHandlers creates a new OSImageHandlers instance
func NewOSImageHandlers(container *Container) *OSImageHandlers {
	return &OSImageHandlers{container: container}
}

// OSImagesPage serves the OS images management page
func (h *OSImageHandlers) OSImagesPage(w http.ResponseWriter, r *http.Request) {
	if !requireConfig(w, r, h.container) {
		return
	}
	ctx := r.Context()

	// Get all OS images
	images, err := h.container.OSImageService.GetAllOSImages(ctx)
	if err != nil {
		HandleError(w, r, NewInternalError(fmt.Sprintf("Failed to get OS images: %v", err), "Unable to load OS images"))
		return
	}

	// Get active downloads
	downloads, err := h.container.OSImageService.GetActiveDownloads(ctx)
	if err != nil {
		downloads = []*osimage.DownloadStatus{} // Continue with empty downloads
	}

	supportedArchs := h.architectures(ctx)

	var kernelBytes, initrdBytes int64
	arches := map[string]struct{}{}
	for _, image := range images {
		if image == nil {
			continue
		}
		kernelBytes += image.KernelSize
		initrdBytes += image.InitrdSize
		if image.Architecture != "" {
			arches[image.Architecture] = struct{}{}
		}
	}
	archLabel := "—"
	if len(arches) == 1 {
		for arch := range arches {
			archLabel = arch
		}
	} else if len(arches) > 1 {
		archLabel = "mixed"
	}

	data := struct {
		Title         string
		Mode          string
		Page          string
		CSRFToken     string
		OverallStatus string
		OSImages      []*osimage.OSImage
		Downloads     []*osimage.DownloadStatus
		Architectures []string
		SupportedOS   []string
		KernelBytes   int64
		InitrdBytes   int64
		ArchLabel     string
		DownloadCount int
	}{
		Title:         "OS Images",
		Page:          "osimages",
		OSImages:      images,
		Downloads:     downloads,
		Architectures: supportedArchs,
		SupportedOS:   h.container.Config.OSImages.SupportedNames(),
		KernelBytes:   kernelBytes,
		InitrdBytes:   initrdBytes,
		ArchLabel:     archLabel,
		DownloadCount: len(downloads),
	}

	renderCachedTemplate(w, r, "osimages", data, "Unable to render the OS images page")
}

// DownloadOSImage starts downloading an OS image
func (h *OSImageHandlers) DownloadOSImage(w http.ResponseWriter, r *http.Request) {
	if !requireConfig(w, r, h.container) {
		return
	}
	ctx := r.Context()

	// Parse form data
	if err := r.ParseForm(); err != nil {
		HandleError(w, r, NewValidationError("Failed to parse form data", "The submitted form could not be parsed"))
		return
	}

	os := r.Form.Get("os")
	version := r.Form.Get("version")
	architecture := r.Form.Get("architecture")

	if os == "" || version == "" {
		HandleError(w, r, NewValidationError("OS and version are required", "OS and version are required"))
		return
	}

	if architecture == "" {
		architecture = "x86_64" // Default architecture
	}

	if err := h.validateOSImageRequest(ctx, os, version, architecture); err != nil {
		HandleError(w, r, NewValidationError(
			fmt.Sprintf("Invalid OS image request: %v", err),
			err.Error(),
		))
		return
	}

	// Create download config
	config := osimage.OSImageConfig{
		OS:           os,
		Version:      version,
		Architecture: architecture,
	}

	// Start download
	status, err := h.container.OSImageService.DownloadOSImage(ctx, config)
	if err != nil {
		HandleError(w, r, NewInternalError(fmt.Sprintf("Failed to start download: %v", err), "Unable to start the download"))
		return
	}

	// Return JSON response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      "started",
		"download_id": status.ID,
		"message":     fmt.Sprintf("Download started for %s %s", os, version),
	})
}

// validateOSImageRequest checks the catalog, including releases past EOL.
// An unknown OS, an unknown version, and a wrong architecture are distinct.
func (h *OSImageHandlers) validateOSImageRequest(ctx context.Context, osName, version, architecture string) error {
	cat := h.container.Catalog
	if cat == nil {
		return fmt.Errorf("unknown OS %q", osName)
	}
	if _, ok := cat.Lookup(ctx, osName, version, architecture); ok {
		return nil
	}
	snap, err := cat.Current(ctx)
	if err != nil {
		return fmt.Errorf("unknown OS %q", osName)
	}
	knownOS := osKnown(h.container, snap, osName)
	knownVersion := false
	for _, rel := range snap.Releases {
		if rel.OS == osName && rel.Version == version {
			knownVersion = true
			break
		}
	}
	if !knownOS {
		return fmt.Errorf("unknown OS %q", osName)
	}
	if !knownVersion {
		return fmt.Errorf("unknown version %q for OS %q", version, osName)
	}
	return fmt.Errorf("unsupported architecture %q for %s %s", architecture, osName, version)
}

func (h *OSImageHandlers) architectures(ctx context.Context) []string {
	seen := map[string]struct{}{}
	var out []string
	if h.container.Catalog != nil {
		snap, err := h.container.Catalog.Offerable(ctx)
		if err == nil {
			for _, rel := range snap.Releases {
				if rel.Arch == "" {
					continue
				}
				if _, ok := seen[rel.Arch]; ok {
					continue
				}
				seen[rel.Arch] = struct{}{}
				out = append(out, rel.Arch)
			}
		}
	}
	if len(out) == 0 {
		return []string{"x86_64"}
	}
	sort.Strings(out)
	return out
}

func osKnown(container *Container, snap catalog.Snapshot, osName string) bool {
	if container != nil && container.Config != nil {
		if _, ok := container.Config.OSImages.Sources[osName]; ok {
			return true
		}
	}
	for _, rel := range snap.Releases {
		if rel.OS == osName {
			return true
		}
	}
	return false
}

// GetDownloadStatus returns the status of a download
func (h *OSImageHandlers) GetDownloadStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	downloadID := vars["id"]

	if downloadID == "" {
		HandleError(w, r, NewValidationError("Download ID is required", "A download id is required"))
		return
	}

	status, err := h.container.OSImageService.GetDownloadStatus(ctx, downloadID)
	if err != nil {
		HandleError(w, r, NewNotFoundError(fmt.Sprintf("Failed to get download status: %v", err), "The requested download was not found"))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// SetDefaultVersion sets an OS image as the default for its OS type
func (h *OSImageHandlers) SetDefaultVersion(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	imageID := vars["id"]

	if imageID == "" {
		HandleError(w, r, NewValidationError("Image ID is required", "An image id is required"))
		return
	}

	if err := h.container.OSImageService.SetDefaultVersion(ctx, imageID); err != nil {
		HandleError(w, r, NewInternalError(fmt.Sprintf("Failed to set default version: %v", err), "Unable to set the default version"))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "Default version updated",
	})
}

// DeleteOSImage deletes an OS image
func (h *OSImageHandlers) DeleteOSImage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	imageID := vars["id"]

	if imageID == "" {
		HandleError(w, r, NewValidationError("Image ID is required", "An image id is required"))
		return
	}

	if err := h.container.OSImageService.DeleteOSImage(ctx, imageID); err != nil {
		HandleError(w, r, NewInternalError(fmt.Sprintf("Failed to delete OS image: %v", err), "Unable to delete the OS image"))
		return
	}

	// Redirect back to OS images page to show updated list
	w.Header().Set("HX-Redirect", "/osimages")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OS image deleted successfully"))
}

// GetAvailableVersions returns available versions for an OS
func (h *OSImageHandlers) GetAvailableVersions(w http.ResponseWriter, r *http.Request) {
	if !requireConfig(w, r, h.container) {
		return
	}
	ctx := r.Context()

	os := r.URL.Query().Get("os")
	if os == "" {
		HandleError(w, r, NewValidationError("OS parameter is required", "An OS is required"))
		return
	}

	cat := h.container.Catalog
	if cat == nil {
		HandleError(w, r, NewNotFoundError("catalog is not configured", "The requested OS is not configured"))
		return
	}
	snap, err := cat.Current(ctx)
	if err != nil {
		HandleError(w, r, NewInternalError(fmt.Sprintf("Failed to get versions: %v", err), "Unable to load available versions"))
		return
	}
	if !osKnown(h.container, snap, os) {
		HandleError(w, r, NewNotFoundError(fmt.Sprintf("OS configuration not found: %s", os), "The requested OS is not configured"))
		return
	}
	includeEOL := r.URL.Query().Get("include_eol") == "true"
	versions := catalog.Versions(os, snap.Releases, time.Now(), includeEOL)
	if versions == nil {
		versions = []catalog.Version{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(versions)
}

// GetOSImageInfo returns detailed information about an OS image
func (h *OSImageHandlers) GetOSImageInfo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	imageID := vars["id"]

	if imageID == "" {
		HandleError(w, r, NewValidationError("Image ID is required", "An image id is required"))
		return
	}

	image, err := h.container.OSImageService.GetOSImage(ctx, imageID)
	if err != nil {
		HandleError(w, r, NewNotFoundError(fmt.Sprintf("Failed to get OS image: %v", err), "The requested OS image was not found"))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(image)
}

// CancelDownload cancels an active download
func (h *OSImageHandlers) CancelDownload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	downloadID := vars["id"]

	if downloadID == "" {
		HandleError(w, r, NewValidationError("Download ID is required", "A download id is required"))
		return
	}

	if err := h.container.OSImageService.CancelDownload(ctx, downloadID); err != nil {
		HandleError(w, r, NewInternalError(fmt.Sprintf("Failed to cancel download: %v", err), "Unable to cancel the download"))
		return
	}

	// Redirect back to OS images page to show updated status
	w.Header().Set("HX-Redirect", "/osimages")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Download cancelled successfully"))
}
