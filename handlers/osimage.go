package handlers

import (
	"encoding/json"
	"fmt"
	"ignite/config"
	"ignite/osimage"
	"net/http"

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

	// Get supported architectures from config (collect unique architectures from all versions)
	archMap := make(map[string]bool)
	for _, osSource := range h.container.Config.OSImages.Sources {
		for _, version := range osSource.Versions {
			for _, arch := range version.Architectures {
				archMap[arch] = true
			}
		}
	}

	var supportedArchs []string
	for arch := range archMap {
		supportedArchs = append(supportedArchs, arch)
	}

	// Fallback if no architectures found
	if len(supportedArchs) == 0 {
		supportedArchs = []string{"x86_64"}
	}

	data := struct {
		Title         string
		OSImages      []*osimage.OSImage
		Downloads     []*osimage.DownloadStatus
		Architectures []string
	}{
		Title:         "OS Images",
		OSImages:      images,
		Downloads:     downloads,
		Architectures: supportedArchs,
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

	// Validate the OS, version, and architecture against the configured
	// sources before starting any download.
	if err := validateOSImageRequest(h.container.Config.OSImages.Sources, os, version, architecture); err != nil {
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

// validateOSImageRequest checks that the requested OS, version, and
// architecture exist in the configured OS image sources.
func validateOSImageRequest(sources map[string]config.OSDefinition, os, version, architecture string) error {
	osDef, ok := sources[os]
	if !ok {
		return fmt.Errorf("unknown OS %q", os)
	}
	versionDef, ok := osDef.Versions[version]
	if !ok {
		return fmt.Errorf("unknown version %q for OS %q", version, os)
	}
	for _, arch := range versionDef.Architectures {
		if arch == architecture {
			return nil
		}
	}
	return fmt.Errorf("unsupported architecture %q for %s %s", architecture, os, version)
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

	// Use the service to get available versions
	versions, err := h.container.OSImageService.GetAvailableVersions(ctx, os)
	if err != nil {
		HandleError(w, r, NewInternalError(fmt.Sprintf("Failed to get versions: %v", err), "Unable to load available versions"))
		return
	}

	// Create a more detailed response with display names from config
	osDef, exists := h.container.Config.OSImages.Sources[os]
	if !exists {
		HandleError(w, r, NewNotFoundError(fmt.Sprintf("OS configuration not found: %s", os), "The requested OS is not configured"))
		return
	}

	type versionInfo struct {
		Value       string `json:"value"`
		DisplayName string `json:"display_name"`
	}

	var versionList []versionInfo
	for _, version := range versions {
		displayName := version // Default to version number
		if versionDef, ok := osDef.Versions[version]; ok {
			if versionDef.DisplayName != "" {
				displayName = versionDef.DisplayName
			}
		}
		versionList = append(versionList, versionInfo{
			Value:       version,
			DisplayName: displayName,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(versionList)
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
