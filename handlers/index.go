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
	templates := LoadTemplates()

	data := map[string]string{"title": "Ignite"}
	if err := templates["index"].Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
