package handlers

import (
	"net/http"
)

// IPXEHandlers handles iPXE-related HTTP requests
type IPXEHandlers struct {
	container *Container
}

// NewIPXEHandlers creates a new iPXE handlers instance
func NewIPXEHandlers(container *Container) *IPXEHandlers {
	return &IPXEHandlers{
		container: container,
	}
}

// GenerateConfig generates and serves iPXE configuration
func (h *IPXEHandlers) GenerateConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	config, err := h.container.IPXEService.GenerateConfig(ctx)
	if err != nil {
		http.Error(w, "Failed to generate iPXE config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(config))
}

// UpdateConfigFile generates and writes iPXE config to file
func (h *IPXEHandlers) UpdateConfigFile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := h.container.IPXEService.WriteConfigToFile(ctx); err != nil {
		http.Error(w, "Failed to update iPXE config file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("iPXE configuration updated successfully"))
}