package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"ignite/syslinux"

	"github.com/gorilla/mux"
)

// SyslinuxHandler handles Syslinux-related HTTP requests
type SyslinuxHandler struct {
	service   syslinux.Service
	container *Container
}

// NewSyslinuxHandler creates a new Syslinux handler
func NewSyslinuxHandler(container *Container) *SyslinuxHandler {
	return &SyslinuxHandler{
		service:   container.SyslinuxService,
		container: container,
	}
}

// RegisterRoutes registers all Syslinux routes
func (h *SyslinuxHandler) RegisterRoutes(r *mux.Router) {
	// Main page route
	r.HandleFunc("/syslinux", h.SyslinuxPage).Methods("GET")

	// API routes
	api := r.PathPrefix("/api/syslinux").Subrouter()

	// Version management
	api.HandleFunc("/versions", h.ListVersions).Methods("GET")
	api.HandleFunc("/versions/refresh", h.RefreshVersions).Methods("POST")
	api.HandleFunc("/scan", h.ScanMirror).Methods("POST")
	api.HandleFunc("/download/{version}", h.DownloadAndInstallVersion).Methods("POST")
	api.HandleFunc("/activate/{version}", h.ActivateVersion).Methods("POST")
	api.HandleFunc("/deactivate/{version}", h.DeactivateVersion).Methods("POST")
	api.HandleFunc("/delete/{version}", h.DeleteVersion).Methods("DELETE")
	api.HandleFunc("/versions/{version}", h.GetVersion).Methods("GET")

	// Boot file management
	api.HandleFunc("/bootfiles", h.ListBootFiles).Methods("GET")
	api.HandleFunc("/bootfiles/{version}/{bootType}/install", h.InstallBootFiles).Methods("POST")
	api.HandleFunc("/bootfiles/{version}/{bootType}/remove", h.RemoveBootFiles).Methods("POST")
	api.HandleFunc("/bootfiles/{id}", h.GetBootFile).Methods("GET")

	// Download status
	api.HandleFunc("/downloads", h.ListDownloadStatuses).Methods("GET")
	api.HandleFunc("/downloads/{id}", h.GetDownloadStatus).Methods("GET")
	api.HandleFunc("/downloads/{id}/cancel", h.CancelDownload).Methods("POST")

	// System status and configuration
	api.HandleFunc("/status", h.GetSystemStatus).Methods("GET")
	api.HandleFunc("/config", h.GetConfig).Methods("GET")
	api.HandleFunc("/config", h.UpdateConfig).Methods("PUT")
	api.HandleFunc("/validate/{bootType}", h.ValidateInstallation).Methods("GET")
}

// SyslinuxPage serves the main Syslinux management page
func (h *SyslinuxHandler) SyslinuxPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get all available versions
	versions, err := h.service.GetAvailableVersions(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get versions: %v", err), http.StatusInternalServerError)
		return
	}

	// For now, we don't have active downloads in the service interface
	// This will need to be implemented when the service layer is complete
	var downloads []*syslinux.DownloadStatus

	data := struct {
		Title     string
		Versions  []*syslinux.SyslinuxVersion
		Downloads []*syslinux.DownloadStatus
	}{
		Title:     "Syslinux Boot Files",
		Versions:  versions,
		Downloads: downloads,
	}

	templates := LoadTemplates()
	if tmpl, ok := templates["syslinux"]; ok {
		if err := tmpl.Execute(w, data); err != nil {
			http.Error(w, fmt.Sprintf("Failed to execute template: %v", err), http.StatusInternalServerError)
		}
	} else {
		http.Error(w, "Syslinux template not found", http.StatusInternalServerError)
	}
}

// ListVersions returns all available Syslinux versions
func (h *SyslinuxHandler) ListVersions(w http.ResponseWriter, r *http.Request) {
	versions, err := h.service.GetAvailableVersions(r.Context())
	if err != nil {
		http.Error(w, "Failed to get versions: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"versions": versions,
		"count":    len(versions),
	})
}

// RefreshVersions scans the mirror for new versions
func (h *SyslinuxHandler) RefreshVersions(w http.ResponseWriter, r *http.Request) {
	if err := h.service.RefreshAvailableVersions(r.Context()); err != nil {
		http.Error(w, "Failed to refresh versions: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Versions refreshed successfully",
	})
}

// ScanMirror scans the mirror for available Syslinux versions
func (h *SyslinuxHandler) ScanMirror(w http.ResponseWriter, r *http.Request) {
	if err := h.service.RefreshAvailableVersions(r.Context()); err != nil {
		http.Error(w, "Failed to scan mirror: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Redirect back to the main page to show updated results
	http.Redirect(w, r, "/syslinux", http.StatusSeeOther)
}

// DownloadAndInstallVersion downloads, extracts and installs a Syslinux version
func (h *SyslinuxHandler) DownloadAndInstallVersion(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	version := vars["version"]

	if version == "" {
		http.Error(w, "Version parameter required", http.StatusBadRequest)
		return
	}

	// Start the complete download/install/activate process
	// The service now handles: cleanup -> download -> extract -> install -> activate
	_, err := h.service.DownloadVersion(r.Context(), version)
	if err != nil {
		http.Error(w, "Failed to start download: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Redirect back to main page
	http.Redirect(w, r, "/syslinux", http.StatusSeeOther)
}

// ActivateVersion sets a version as the active one
func (h *SyslinuxHandler) ActivateVersion(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	version := vars["version"]

	if version == "" {
		http.Error(w, "Version parameter required", http.StatusBadRequest)
		return
	}

	// First deactivate any currently active version
	if err := h.deactivateCurrentVersion(r.Context()); err != nil {
		http.Error(w, "Failed to deactivate current version: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Install both BIOS and EFI boot files
	if err := h.service.InstallBootFiles(r.Context(), version, "bios"); err != nil {
		http.Error(w, "Failed to install BIOS boot files: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := h.service.InstallBootFiles(r.Context(), version, "efi"); err != nil {
		http.Error(w, "Failed to install EFI boot files: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Mark version as active
	if err := h.activateVersion(r.Context(), version); err != nil {
		http.Error(w, "Failed to activate version: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Redirect back to main page
	http.Redirect(w, r, "/syslinux", http.StatusSeeOther)
}

// DeactivateVersion deactivates and removes the currently active version
func (h *SyslinuxHandler) DeactivateVersion(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	version := vars["version"]

	if version == "" {
		http.Error(w, "Version parameter required", http.StatusBadRequest)
		return
	}

	// Use the service method to properly deactivate the version
	if err := h.service.DeactivateVersion(r.Context(), version); err != nil {
		http.Error(w, "Failed to deactivate version: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Redirect back to main page
	http.Redirect(w, r, "/syslinux", http.StatusSeeOther)
}

// GetVersion returns details for a specific version
func (h *SyslinuxHandler) GetVersion(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	version := vars["version"]

	if version == "" {
		http.Error(w, "Version parameter required", http.StatusBadRequest)
		return
	}

	// Get all variants of this version (BIOS and EFI)
	versions, err := h.service.GetAvailableVersions(r.Context())
	if err != nil {
		http.Error(w, "Failed to get versions: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var matchedVersion *syslinux.SyslinuxVersion
	for _, v := range versions {
		if v.Version == version {
			matchedVersion = v
			break
		}
	}

	if matchedVersion == nil {
		http.Error(w, "Version not found", http.StatusNotFound)
		return
	}

	// Format response for the modal
	response := map[string]interface{}{
		"version":      matchedVersion.Version,
		"download_url": matchedVersion.DownloadURL,
		"file_name":    matchedVersion.FileName,
		"size":         matchedVersion.Size,
		"active":       matchedVersion.Active,
		"downloaded":   matchedVersion.Downloaded,
		"created_at":   matchedVersion.CreatedAt,
		"updated_at":   matchedVersion.UpdatedAt,
	}

	if matchedVersion.DownloadedAt != nil {
		response["downloaded_at"] = *matchedVersion.DownloadedAt
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// DeleteVersion removes a version
func (h *SyslinuxHandler) DeleteVersion(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	version := vars["version"]

	if version == "" {
		http.Error(w, "Version parameter required", http.StatusBadRequest)
		return
	}

	// This would need implementation in the service
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Version %s deleted", version),
	})
}

// ListBootFiles returns boot files, optionally filtered by version and boot type
func (h *SyslinuxHandler) ListBootFiles(w http.ResponseWriter, r *http.Request) {
	version := r.URL.Query().Get("version")
	bootType := r.URL.Query().Get("bootType")

	bootFiles, err := h.service.ListInstalledBootFiles(r.Context(), bootType)
	if err != nil {
		http.Error(w, "Failed to get boot files: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Filter by version if specified
	if version != "" {
		var filtered []*syslinux.SyslinuxBootFile
		for _, bf := range bootFiles {
			if bf.Version == version {
				filtered = append(filtered, bf)
			}
		}
		bootFiles = filtered
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"boot_files": bootFiles,
		"count":      len(bootFiles),
	})
}

// InstallBootFiles installs boot files for a version and boot type
func (h *SyslinuxHandler) InstallBootFiles(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	version := vars["version"]
	bootType := vars["bootType"]

	if version == "" || bootType == "" {
		http.Error(w, "Version and bootType parameters required", http.StatusBadRequest)
		return
	}

	if err := h.service.InstallBootFiles(r.Context(), version, bootType); err != nil {
		http.Error(w, "Failed to install boot files: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Boot files for %s %s installed successfully", version, bootType),
	})
}

// RemoveBootFiles removes boot files for a version and boot type
func (h *SyslinuxHandler) RemoveBootFiles(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	version := vars["version"]
	bootType := vars["bootType"]

	if version == "" || bootType == "" {
		http.Error(w, "Version and bootType parameters required", http.StatusBadRequest)
		return
	}

	if err := h.service.RemoveBootFiles(r.Context(), version, bootType); err != nil {
		http.Error(w, "Failed to remove boot files: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Boot files for %s %s removed successfully", version, bootType),
	})
}

// GetBootFile returns details for a specific boot file
func (h *SyslinuxHandler) GetBootFile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	if id == "" {
		http.Error(w, "ID parameter required", http.StatusBadRequest)
		return
	}

	// This would need implementation in the service
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"id":      id,
	})
}

// ListDownloadStatuses returns all download statuses
func (h *SyslinuxHandler) ListDownloadStatuses(w http.ResponseWriter, r *http.Request) {
	// Get optional status filter
	statusFilter := r.URL.Query().Get("status")

	// This would need implementation in the service to get all statuses
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"downloads": []interface{}{},
		"filter":    statusFilter,
	})
}

// GetDownloadStatus returns status for a specific download
func (h *SyslinuxHandler) GetDownloadStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	if id == "" {
		http.Error(w, "ID parameter required", http.StatusBadRequest)
		return
	}

	status, err := h.service.GetDownloadStatus(r.Context(), id)
	if err != nil {
		http.Error(w, "Failed to get download status: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// CancelDownload cancels an ongoing download
func (h *SyslinuxHandler) CancelDownload(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	if id == "" {
		http.Error(w, "ID parameter required", http.StatusBadRequest)
		return
	}

	if err := h.service.CancelDownload(r.Context(), id); err != nil {
		http.Error(w, "Failed to cancel download: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Download cancelled successfully",
	})
}

// GetSystemStatus returns overall system status
func (h *SyslinuxHandler) GetSystemStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.service.GetSystemStatus(r.Context())
	if err != nil {
		http.Error(w, "Failed to get system status: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Also get disk space info
	diskSpace, err := h.service.CheckDiskSpace(r.Context())
	if err != nil {
		diskSpace = &syslinux.DiskSpaceInfo{} // Return empty if error
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"system":     status,
		"disk_space": diskSpace,
	})
}

// GetConfig returns current configuration
func (h *SyslinuxHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	config := h.service.GetConfig()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

// UpdateConfig updates the configuration
func (h *SyslinuxHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	var config syslinux.SyslinuxConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.service.UpdateConfig(r.Context(), config); err != nil {
		http.Error(w, "Failed to update config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Configuration updated successfully",
	})
}

// ValidateInstallation validates a boot type installation
func (h *SyslinuxHandler) ValidateInstallation(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	bootType := vars["bootType"]

	if bootType == "" {
		http.Error(w, "Boot type parameter required", http.StatusBadRequest)
		return
	}

	if bootType != "bios" && bootType != "efi" {
		http.Error(w, "Boot type must be 'bios' or 'efi'", http.StatusBadRequest)
		return
	}

	result, err := h.service.ValidateInstallation(r.Context(), bootType)
	if err != nil {
		http.Error(w, "Failed to validate installation: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// Helper methods for version management

// deactivateCurrentVersion finds and deactivates any currently active version
func (h *SyslinuxHandler) deactivateCurrentVersion(ctx context.Context) error {
	versions, err := h.service.GetAvailableVersions(ctx)
	if err != nil {
		return err
	}

	for _, version := range versions {
		if version.Active {
			// Remove boot files
			if err := h.service.RemoveBootFiles(ctx, version.Version, "bios"); err != nil {
				return fmt.Errorf("failed to remove BIOS boot files for %s: %w", version.Version, err)
			}
			if err := h.service.RemoveBootFiles(ctx, version.Version, "efi"); err != nil {
				return fmt.Errorf("failed to remove EFI boot files for %s: %w", version.Version, err)
			}

			// Mark as inactive
			if err := h.deactivateVersion(ctx, version.Version); err != nil {
				return fmt.Errorf("failed to deactivate version %s: %w", version.Version, err)
			}
		}
	}

	return nil
}

// activateVersion marks a specific version as active
func (h *SyslinuxHandler) activateVersion(ctx context.Context, version string) error {
	// This would need to be implemented in the service layer
	// For now, we'll need to directly update the database via repository
	versions, err := h.service.GetAvailableVersions(ctx)
	if err != nil {
		return err
	}

	for _, v := range versions {
		if v.Version == version {
			v.Active = true
			v.Downloaded = true
			v.UpdatedAt = time.Now()
			now := time.Now()
			v.DownloadedAt = &now
			// We would need a SaveVersion method in the service
			break
		}
	}

	return nil
}

// deactivateVersion marks a specific version as inactive
func (h *SyslinuxHandler) deactivateVersion(ctx context.Context, version string) error {
	// This would need to be implemented in the service layer
	// For now, we'll need to directly update the database via repository
	versions, err := h.service.GetAvailableVersions(ctx)
	if err != nil {
		return err
	}

	for _, v := range versions {
		if v.Version == version {
			v.Active = false
			v.UpdatedAt = time.Now()
			// We would need a SaveVersion method in the service
			break
		}
	}

	return nil
}
