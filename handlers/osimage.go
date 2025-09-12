package handlers

import (
	"encoding/json"
	"fmt"
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
	ctx := r.Context()
	
	// Get all OS images
	images, err := h.container.OSImageService.GetAllOSImages(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get OS images: %v", err), http.StatusInternalServerError)
		return
	}
	
	// Get active downloads
	downloads, err := h.container.OSImageService.GetActiveDownloads(ctx)
	if err != nil {
		downloads = []*osimage.DownloadStatus{} // Continue with empty downloads
	}
	
	data := struct {
		Title     string
		OSImages  []*osimage.OSImage
		Downloads []*osimage.DownloadStatus
	}{
		Title:     "OS Images",
		OSImages:  images,
		Downloads: downloads,
	}
	
	templates := LoadTemplates()
	if err := templates["osimages"].Execute(w, data); err != nil {
		http.Error(w, fmt.Sprintf("Template error: %v", err), http.StatusInternalServerError)
	}
}

// ListOSImages returns all OS images as JSON
func (h *OSImageHandlers) ListOSImages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	
	images, err := h.container.OSImageService.GetAllOSImages(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get OS images: %v", err), http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(images)
}

// DownloadOSImage starts downloading an OS image
func (h *OSImageHandlers) DownloadOSImage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	
	// Parse form data
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}
	
	os := r.Form.Get("os")
	version := r.Form.Get("version")
	architecture := r.Form.Get("architecture")
	
	if os == "" || version == "" {
		http.Error(w, "OS and version are required", http.StatusBadRequest)
		return
	}
	
	if architecture == "" {
		architecture = "x86_64" // Default architecture
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
		http.Error(w, fmt.Sprintf("Failed to start download: %v", err), http.StatusInternalServerError)
		return
	}
	
	// Return JSON response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     "started",
		"download_id": status.ID,
		"message":    fmt.Sprintf("Download started for %s %s", os, version),
	})
}

// GetDownloadStatus returns the status of a download
func (h *OSImageHandlers) GetDownloadStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	downloadID := vars["id"]
	
	if downloadID == "" {
		http.Error(w, "Download ID is required", http.StatusBadRequest)
		return
	}
	
	status, err := h.container.OSImageService.GetDownloadStatus(ctx, downloadID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get download status: %v", err), http.StatusNotFound)
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
		http.Error(w, "Image ID is required", http.StatusBadRequest)
		return
	}
	
	if err := h.container.OSImageService.SetDefaultVersion(ctx, imageID); err != nil {
		http.Error(w, fmt.Sprintf("Failed to set default version: %v", err), http.StatusInternalServerError)
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
		http.Error(w, "Image ID is required", http.StatusBadRequest)
		return
	}
	
	if err := h.container.OSImageService.DeleteOSImage(ctx, imageID); err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete OS image: %v", err), http.StatusInternalServerError)
		return
	}
	
	// Redirect back to OS images page to show updated list
	w.Header().Set("HX-Redirect", "/osimages")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OS image deleted successfully"))
}

// GetAvailableVersions returns available versions for an OS
func (h *OSImageHandlers) GetAvailableVersions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	
	os := r.URL.Query().Get("os")
	if os == "" {
		http.Error(w, "OS parameter is required", http.StatusBadRequest)
		return
	}
	
	// Use the service to get available versions
	versions, err := h.container.OSImageService.GetAvailableVersions(ctx, os)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get versions: %v", err), http.StatusInternalServerError)
		return
	}
	
	// Create a more detailed response with display names from config
	osDef, exists := h.container.Config.OSImages.Sources[os]
	if !exists {
		http.Error(w, fmt.Sprintf("OS configuration not found: %s", os), http.StatusInternalServerError)
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
		http.Error(w, "Image ID is required", http.StatusBadRequest)
		return
	}
	
	image, err := h.container.OSImageService.GetOSImage(ctx, imageID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get OS image: %v", err), http.StatusNotFound)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(image)
}

// GetOSImagesByOS returns all images for a specific OS
func (h *OSImageHandlers) GetOSImagesByOS(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	os := r.URL.Query().Get("os")
	
	if os == "" {
		http.Error(w, "OS parameter is required", http.StatusBadRequest)
		return
	}
	
	images, err := h.container.OSImageService.GetOSImagesByOS(ctx, os)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get OS images: %v", err), http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(images)
}

// CancelDownload cancels an active download
func (h *OSImageHandlers) CancelDownload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	downloadID := vars["id"]
	
	if downloadID == "" {
		http.Error(w, "Download ID is required", http.StatusBadRequest)
		return
	}
	
	if err := h.container.OSImageService.CancelDownload(ctx, downloadID); err != nil {
		http.Error(w, fmt.Sprintf("Failed to cancel download: %v", err), http.StatusInternalServerError)
		return
	}
	
	// Redirect back to OS images page to show updated status
	w.Header().Set("HX-Redirect", "/osimages")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Download cancelled successfully"))
}

// RegisterOSImageHandlers registers all OS image related routes
func RegisterOSImageHandlers(r *mux.Router, service osimage.OSImageService) {
	r.HandleFunc("/osimages", GetAllOSImages(service)).Methods("GET")
	r.HandleFunc("/osimages/{os}", GetOSImagesByOS(service)).Methods("GET")
	r.HandleFunc("/osimages/{id}", GetOSImage(service)).Methods("GET")
	r.HandleFunc("/osimages/default/{os}", GetDefaultVersion(service)).Methods("GET")
	r.HandleFunc("/osimages/set-default/{id}", SetDefaultVersion(service)).Methods("POST")
	r.HandleFunc("/osimages/delete/{id}", DeleteOSImage(service)).Methods("DELETE")
	r.HandleFunc("/osimages/download", DownloadOSImage(service)).Methods("POST")
	r.HandleFunc("/osimages/download/status/{id}", GetDownloadStatus(service)).Methods("GET")
	r.HandleFunc("/osimages/download/active", GetActiveDownloads(service)).Methods("GET")
	r.HandleFunc("/osimages/download/cancel/{id}", CancelDownload(service)).Methods("POST")
	r.HandleFunc("/osimages/available-versions/{os}", GetAvailableVersions(service)).Methods("GET")
}

func GetAllOSImages(service osimage.OSImageService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		images, err := service.GetAllOSImages(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(images)
	}
}

func GetOSImagesByOS(service osimage.OSImageService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		os := vars["os"]
		images, err := service.GetOSImagesByOS(r.Context(), os)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(images)
	}
}

func GetOSImage(service osimage.OSImageService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id := vars["id"]
		image, err := service.GetOSImage(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(image)
	}
}

func GetDefaultVersion(service osimage.OSImageService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		os := vars["os"]
		image, err := service.GetDefaultVersion(r.Context(), os)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(image)
	}
}

func SetDefaultVersion(service osimage.OSImageService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id := vars["id"]
		err := service.SetDefaultVersion(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func DeleteOSImage(service osimage.OSImageService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id := vars["id"]
		err := service.DeleteOSImage(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func DownloadOSImage(service osimage.OSImageService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var osConfig osimage.OSImageConfig
		if err := json.NewDecoder(r.Body).Decode(&osConfig); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		status, err := service.DownloadOSImage(r.Context(), osConfig)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(status)
	}
}

func GetDownloadStatus(service osimage.OSImageService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id := vars["id"]
		status, err := service.GetDownloadStatus(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(status)
	}
}

func GetActiveDownloads(service osimage.OSImageService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		downloads, err := service.GetActiveDownloads(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(downloads)
	}
}

func CancelDownload(service osimage.OSImageService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id := vars["id"]
		err := service.CancelDownload(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func GetAvailableVersions(service osimage.OSImageService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		osName := vars["os"]
		
		versions, err := service.GetAvailableVersions(r.Context(), osName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		
		json.NewEncoder(w).Encode(versions)
	}
}