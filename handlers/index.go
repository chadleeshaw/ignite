package handlers

import (
	"net/http"
)

// IndexHandlers handles index page requests
type IndexHandlers struct {
	container *Container
}

// NewIndexHandlers creates a new IndexHandlers instance
func NewIndexHandlers(container *Container) *IndexHandlers {
	return &IndexHandlers{container: container}
}

// Index serves the main index page of the application.
func (h *IndexHandlers) Index(w http.ResponseWriter, r *http.Request) {
	data := map[string]string{"title": "Ignite"}
	renderCachedTemplate(w, r, "index", data, "Unable to render the home page")
}
