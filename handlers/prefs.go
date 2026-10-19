package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"ignite/db"
)

const (
	// DefaultUIMode is the density used until a user stores a preference.
	DefaultUIMode = "cozy"
	// ConsoleUIMode is the compact terminal density.
	ConsoleUIMode = "console"
	uiModeCookie  = "ignite_mode"
)

// PreferenceStore persists the per-user UI mode.
type PreferenceStore interface {
	GetUIMode(ctx context.Context, username string) (string, error)
	SetUIMode(ctx context.Context, username string, mode string) error
}

// ValidUIMode reports whether mode is one of the two supported densities.
func ValidUIMode(mode string) bool {
	return mode == DefaultUIMode || mode == ConsoleUIMode
}

// BoltPreferenceStore saves UI mode in a bbolt bucket, one key per username.
type BoltPreferenceStore struct {
	database db.Database
	bucket   string
}

// NewBoltPreferenceStore returns a store rooted at bucket.
// The bucket is created immediately so the first read is "no preference"
// rather than a missing-bucket error.
func NewBoltPreferenceStore(database db.Database, bucket string) *BoltPreferenceStore {
	if database != nil && bucket != "" {
		_ = database.GetOrCreateBucket(context.Background(), bucket)
	}
	return &BoltPreferenceStore{database: database, bucket: bucket}
}

// GetUIMode returns the stored mode, or DefaultUIMode when the user has
// never chosen one or the stored value is not a known mode.
func (s *BoltPreferenceStore) GetUIMode(ctx context.Context, username string) (string, error) {
	if s == nil || s.database == nil || username == "" {
		return DefaultUIMode, nil
	}
	raw, err := s.database.GetKV(ctx, s.bucket, []byte(username))
	if err != nil {
		return "", err
	}
	mode := string(raw)
	if !ValidUIMode(mode) {
		return DefaultUIMode, nil
	}
	return mode, nil
}

// SetUIMode stores mode for username. The caller validates mode.
func (s *BoltPreferenceStore) SetUIMode(ctx context.Context, username, mode string) error {
	if s == nil || s.database == nil {
		return errPrefsUnavailable
	}
	return s.database.PutKV(ctx, s.bucket, []byte(username), []byte(mode))
}

// PreferenceHandlers serves the UI-mode endpoints.
type PreferenceHandlers struct {
	store PreferenceStore
}

// NewPreferenceHandlers returns handlers backed by store.
func NewPreferenceHandlers(store PreferenceStore) *PreferenceHandlers {
	return &PreferenceHandlers{store: store}
}

type uiModeResponse struct {
	Mode string `json:"mode"`
}

type uiModeRequest struct {
	Mode string `json:"mode"`
}

// GetMode handles GET /api/preferences/mode.
func (h *PreferenceHandlers) GetMode(w http.ResponseWriter, r *http.Request) {
	mode := DefaultUIMode
	if h != nil && h.store != nil {
		stored, err := h.store.GetUIMode(r.Context(), currentUsername())
		if err != nil {
			HandleError(w, r, NewInternalError(
				"failed to read UI mode: "+err.Error(),
				"Unable to load the interface preference",
			))
			return
		}
		if ValidUIMode(stored) {
			mode = stored
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(uiModeResponse{Mode: mode})
}

// SetMode handles POST /api/preferences/mode.
// The body is either JSON {"mode":"cozy"|"console"} or a form field mode.
// A recognized value is stored and answered with 204. Anything else is 400.
func (h *PreferenceHandlers) SetMode(w http.ResponseWriter, r *http.Request) {
	mode, err := readUIMode(r)
	if err != nil || !ValidUIMode(mode) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"message": "mode must be cozy or console",
		})
		return
	}
	if h == nil || h.store == nil {
		HandleError(w, r, NewInternalError("UI preference store is not configured", "Unable to save the interface preference"))
		return
	}
	if err := h.store.SetUIMode(r.Context(), currentUsername(), mode); err != nil {
		HandleError(w, r, NewInternalError(
			"failed to save UI mode: "+err.Error(),
			"Unable to save the interface preference",
		))
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     uiModeCookie,
		Value:    mode,
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
		MaxAge:   60 * 60 * 24 * 365,
	})
	w.WriteHeader(http.StatusNoContent)
}

func readUIMode(r *http.Request) (string, error) {
	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "application/json") {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			return "", err
		}
		var req uiModeRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return "", err
		}
		return strings.TrimSpace(req.Mode), nil
	}
	if err := r.ParseForm(); err != nil {
		return "", err
	}
	return strings.TrimSpace(r.FormValue("mode")), nil
}

// errPrefsUnavailable is returned when a preference write has nowhere to go.
var errPrefsUnavailable = errString("preference store is not configured")

type errString string

func (e errString) Error() string { return string(e) }
