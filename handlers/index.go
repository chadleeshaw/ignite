package handlers

import (
	"net/http"
)

// Index serves the main index page of the application.
func Index(w http.ResponseWriter, r *http.Request) {
	templates := LoadTemplates()

	data := map[string]string{"title": "Ignite"}
	if err := templates["index"].Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
