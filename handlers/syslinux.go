package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

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
		HandleError(w, r, NewInternalError(fmt.Sprintf("Failed to get versions: %v", err), "Unable to load syslinux versions"))
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
			HandleError(w, r, NewInternalError(fmt.Sprintf("Failed to execute template: %v", err), "Unable to render the syslinux page"))
		}
	} else {
		HandleError(w, r, NewInternalError("Syslinux template not found", "Unable to render the syslinux page"))
	}
}

// ListVersions returns all available Syslinux versions
func (h *SyslinuxHandler) ListVersions(w http.ResponseWriter, r *http.Request) {
	versions, err := h.service.GetAvailableVersions(r.Context())
	if err != nil {
		HandleError(w, r, NewInternalError("Failed to get versions: "+err.Error(), "Unable to load syslinux versions"))
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
		HandleError(w, r, NewInternalError("Failed to refresh versions: "+err.Error(), "Unable to refresh syslinux versions"))
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
		HandleError(w, r, NewInternalError("Failed to scan mirror: "+err.Error(), "Unable to scan the syslinux mirror"))
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
		HandleError(w, r, NewValidationError("Version parameter required", "A version is required"))
		return
	}

	// Start the complete download/install/activate process
	// The service now handles: cleanup -> download -> extract -> install -> activate
	_, err := h.service.DownloadVersion(r.Context(), version)
	if err != nil {
		HandleError(w, r, NewInternalError("Failed to start download: "+err.Error(), "Unable to start the download"))
		return
	}

	// Redirect back to main page
	http.Redirect(w, r, "/syslinux", http.StatusSeeOther)
}

// ActivateVersion marks an already-downloaded version as the active one.
// The service layer exposes no standalone activation for an already
// downloaded version (activation only happens inside the download flow),
// so this honestly reports 501 instead of faking it.
func (h *SyslinuxHandler) ActivateVersion(w http.ResponseWriter, r *http.Request) {
	HandleError(w, r, NewAppError(
		ErrorTypeServiceUnavail,
		"syslinux version activation is not implemented in the service layer",
		"Activating an already-downloaded version is not supported by this server",
		http.StatusNotImplemented,
	))
}

// DeactivateVersion deactivates and removes the currently active version
func (h *SyslinuxHandler) DeactivateVersion(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	version := vars["version"]

	if version == "" {
		HandleError(w, r, NewValidationError("Version parameter required", "A version is required"))
		return
	}

	// Use the service method to properly deactivate the version
	if err := h.service.DeactivateVersion(r.Context(), version); err != nil {
		HandleError(w, r, NewInternalError("Failed to deactivate version: "+err.Error(), "Unable to deactivate the version"))
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
		HandleError(w, r, NewValidationError("Version parameter required", "A version is required"))
		return
	}

	// Get all variants of this version (BIOS and EFI)
	versions, err := h.service.GetAvailableVersions(r.Context())
	if err != nil {
		HandleError(w, r, NewInternalError("Failed to get versions: "+err.Error(), "Unable to load syslinux versions"))
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
		HandleError(w, r, NewNotFoundError("Version not found", "The requested syslinux version does not exist"))
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

// DeleteVersion handles DELETE /api/syslinux/versions/{version}.
// The service layer exposes no version deletion, so this honestly reports
// 501 instead of pretending to delete.
func (h *SyslinuxHandler) DeleteVersion(w http.ResponseWriter, r *http.Request) {
	HandleError(w, r, NewAppError(
		ErrorTypeServiceUnavail,
		"syslinux version deletion is not implemented in the service layer",
		"Deleting syslinux versions is not supported by this server",
		http.StatusNotImplemented,
	))
}

// ListBootFiles returns boot files, optionally filtered by version and boot type
func (h *SyslinuxHandler) ListBootFiles(w http.ResponseWriter, r *http.Request) {
	version := r.URL.Query().Get("version")
	bootType := r.URL.Query().Get("bootType")

	bootFiles, err := h.service.ListInstalledBootFiles(r.Context(), bootType)
	if err != nil {
		HandleError(w, r, NewInternalError("Failed to get boot files: "+err.Error(), "Unable to load boot files"))
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
		HandleError(w, r, NewValidationError("Version and bootType parameters required", "A version and boot type are required"))
		return
	}

	if err := h.service.InstallBootFiles(r.Context(), version, bootType); err != nil {
		HandleError(w, r, NewInternalError("Failed to install boot files: "+err.Error(), "Unable to install the boot files"))
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
		HandleError(w, r, NewValidationError("Version and bootType parameters required", "A version and boot type are required"))
		return
	}

	if err := h.service.RemoveBootFiles(r.Context(), version, bootType); err != nil {
		HandleError(w, r, NewInternalError("Failed to remove boot files: "+err.Error(), "Unable to remove the boot files"))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Boot files for %s %s removed successfully", version, bootType),
	})
}

// GetBootFile returns details for a specific boot file.
// The service layer exposes no single-boot-file lookup, so this honestly
// reports 501 instead of echoing the id back.
func (h *SyslinuxHandler) GetBootFile(w http.ResponseWriter, r *http.Request) {
	HandleError(w, r, NewAppError(
		ErrorTypeServiceUnavail,
		"syslinux boot file lookup is not implemented in the service layer",
		"Looking up individual boot files is not supported by this server",
		http.StatusNotImplemented,
	))
}

// ListDownloadStatuses returns all download statuses.
// The service layer only supports lookup by download id, so this honestly
// reports 501 instead of returning an empty list.
func (h *SyslinuxHandler) ListDownloadStatuses(w http.ResponseWriter, r *http.Request) {
	HandleError(w, r, NewAppError(
		ErrorTypeServiceUnavail,
		"syslinux download listing is not implemented in the service layer",
		"Listing all downloads is not supported by this server",
		http.StatusNotImplemented,
	))
}

// GetDownloadStatus returns status for a specific download
func (h *SyslinuxHandler) GetDownloadStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	if id == "" {
		HandleError(w, r, NewValidationError("ID parameter required", "An id is required"))
		return
	}

	status, err := h.service.GetDownloadStatus(r.Context(), id)
	if err != nil {
		HandleError(w, r, NewInternalError("Failed to get download status: "+err.Error(), "Unable to load the download status"))
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
		HandleError(w, r, NewValidationError("ID parameter required", "An id is required"))
		return
	}

	if err := h.service.CancelDownload(r.Context(), id); err != nil {
		HandleError(w, r, NewInternalError("Failed to cancel download: "+err.Error(), "Unable to cancel the download"))
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
		HandleError(w, r, NewInternalError("Failed to get system status: "+err.Error(), "Unable to load the system status"))
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
		HandleError(w, r, NewValidationError("Invalid JSON: "+err.Error(), "The request body is not valid JSON"))
		return
	}

	if err := h.service.UpdateConfig(r.Context(), config); err != nil {
		HandleError(w, r, NewInternalError("Failed to update config: "+err.Error(), "Unable to update the configuration"))
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
		HandleError(w, r, NewValidationError("Boot type parameter required", "A boot type is required"))
		return
	}

	if bootType != "bios" && bootType != "efi" {
		HandleError(w, r, NewValidationError("Boot type must be 'bios' or 'efi'", "Boot type must be 'bios' or 'efi'"))
		return
	}

	result, err := h.service.ValidateInstallation(r.Context(), bootType)
	if err != nil {
		HandleError(w, r, NewInternalError("Failed to validate installation: "+err.Error(), "Unable to validate the installation"))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
