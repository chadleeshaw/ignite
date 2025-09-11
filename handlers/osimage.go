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
		Sources   osimage.OSImageSources
	}{
		Title:     "OS Images",
		OSImages:  images,
		Downloads: downloads,
		Sources:   osimage.GetDefaultSources(),
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
	os := r.URL.Query().Get("os")
	if os == "" {
		http.Error(w, "OS parameter is required", http.StatusBadRequest)
		return
	}
	
	sources := osimage.GetDefaultSources()
	var versions []string
	
	switch os {
	case "ubuntu":
		for version := range sources.Ubuntu {
			versions = append(versions, version)
		}
	case "centos":
		for version := range sources.CentOS {
			versions = append(versions, version)
		}
	case "nixos":
		for version := range sources.NixOS {
			versions = append(versions, version)
		}
	default:
		http.Error(w, "Unsupported OS", http.StatusBadRequest)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"os":       os,
		"versions": versions,
	})
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

// Helper function to get OS display name
func getOSDisplayName(os string) string {
	switch os {
	case "ubuntu":
		return "Ubuntu"
	case "centos":
		return "CentOS"
	case "nixos":
		return "NixOS"
	default:
		return os
	}
}