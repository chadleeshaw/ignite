package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"ignite/config"
	"ignite/osimage/catalog"
	"net/http"
	"time"
)

const catalogRefreshRunning = "A catalog refresh is already running."

// catalogSourceFailureMessage is the 503 sentence for a refresh that reached
// no release source. The snapshot that remains decides the wording.
func catalogSourceFailureMessage(origin catalog.Origin) string {
	if origin == catalog.OriginBundled {
		return "Couldn't reach any release source. Using the built-in list."
	}
	return "Couldn't reach any release source. Showing the last saved catalog."
}

// GetCatalog serves the release catalog. End-of-life releases are omitted.
func (h *OSImageHandlers) GetCatalog(w http.ResponseWriter, r *http.Request) {
	listing, err := h.listingFromCurrent(r.Context())
	if err != nil {
		HandleError(w, r, NewInternalError(fmt.Sprintf("Failed to load catalog: %v", err), "Unable to load the OS catalog"))
		return
	}
	writeJSON(w, http.StatusOK, listing)
}

// RefreshCatalog reloads every release source. HTMX requests receive the
// catalog status fragment. Other clients receive the catalog JSON.
// A refresh already in flight is 409. Every source failing is 503.
// Both error bodies are the user-facing sentence, not an HTML page.
func (h *OSImageHandlers) RefreshCatalog(w http.ResponseWriter, r *http.Request) {
	if h.container == nil || h.container.Catalog == nil {
		writePlain(w, http.StatusServiceUnavailable, catalogSourceFailureMessage(""))
		return
	}
	snap, err := h.container.Catalog.Refresh(r.Context())
	if err != nil {
		switch {
		case errors.Is(err, catalog.ErrRefreshInProgress):
			writePlain(w, http.StatusConflict, catalogRefreshRunning)
		case errors.Is(err, catalog.ErrAllSourcesFailed):
			writePlain(w, http.StatusServiceUnavailable, catalogSourceFailureMessage(h.fallbackOrigin(r.Context())))
		default:
			HandleError(w, r, NewInternalError(fmt.Sprintf("Failed to refresh catalog: %v", err), "Unable to refresh the OS catalog"))
		}
		return
	}
	listing := h.listingFromSnapshot(snap, h.container.Catalog.Refreshing())
	if r.Header.Get("HX-Request") != "" {
		h.renderCatalogStatus(w, r, listing)
		return
	}
	writeJSON(w, http.StatusOK, listing)
}

// fallbackOrigin is the snapshot still being served after a failed refresh.
func (h *OSImageHandlers) fallbackOrigin(ctx context.Context) catalog.Origin {
	if h.container == nil || h.container.Catalog == nil {
		return ""
	}
	snap, err := h.container.Catalog.Current(ctx)
	if err != nil {
		return ""
	}
	return snap.Origin
}

func (h *OSImageHandlers) currentListing(ctx context.Context) catalog.Listing {
	listing, err := h.listingFromCurrent(ctx)
	if err != nil {
		return catalog.Listing{
			Source: string(catalog.OriginBundled),
			Errors: map[string]string{},
			OSes:   []catalog.OSListing{},
		}
	}
	return listing
}

func (h *OSImageHandlers) listingFromCurrent(ctx context.Context) (catalog.Listing, error) {
	if h.container == nil || h.container.Catalog == nil {
		return catalog.Listing{
			Source: string(catalog.OriginBundled),
			Errors: map[string]string{},
			OSes:   []catalog.OSListing{},
		}, nil
	}
	snap, err := h.container.Catalog.Current(ctx)
	if err != nil {
		return catalog.Listing{}, err
	}
	return h.listingFromSnapshot(snap, h.container.Catalog.Refreshing()), nil
}

func (h *OSImageHandlers) listingFromSnapshot(snap catalog.Snapshot, refreshing bool) catalog.Listing {
	var cfg *config.Config
	if h.container != nil {
		cfg = h.container.Config
	}
	return catalog.BuildListing(snap, time.Now(), osDisplayNames(cfg), refreshing, false)
}

func osDisplayNames(cfg *config.Config) map[string]string {
	names := map[string]string{}
	if cfg == nil {
		return names
	}
	for key, def := range cfg.OSImages.Sources {
		if def.DisplayName != "" {
			names[key] = def.DisplayName
		}
	}
	return names
}

func (h *OSImageHandlers) renderCatalogStatus(w http.ResponseWriter, r *http.Request, listing catalog.Listing) {
	data := struct {
		Catalog       catalog.Listing
		Mode          string
		CSRFToken     string
		OverallStatus string
	}{Catalog: listing}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	renderCachedTemplate(w, r, "catalog-status", data, "Unable to render the catalog status")
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func writePlain(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(message))
}
