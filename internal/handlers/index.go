package handlers

import (
	"net/http"
)

// Index serves the main index page of the application.
func (h *Handlers) Index(w http.ResponseWriter, r *http.Request) {
	templates := LoadTemplates()

	data := map[string]string{"title": "Ignite"}
	if err := templates["index"].Execute(w, data); err != nil {
		// It would be better to use the structured error handling here,
		// but for this simple handler, http.Error is acceptable for now.
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
