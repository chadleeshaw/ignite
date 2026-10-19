package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
// rather than a missing-bucket error. A creation failure is returned.
func NewBoltPreferenceStore(database db.Database, bucket string) (*BoltPreferenceStore, error) {
	if database == nil || bucket == "" {
		return nil, fmt.Errorf("preference store requires a database and bucket")
	}
	if err := database.GetOrCreateBucket(context.Background(), bucket); err != nil {
		return nil, fmt.Errorf("failed to create preferences bucket: %w", err)
	}
	return &BoltPreferenceStore{database: database, bucket: bucket}, nil
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
// A recognized value is stored. JSON, Accept: application/json, and
// HTMX requests are answered with 204. A plain HTML form is answered with
// 303 back to a same-origin relative path, or / when the Referer is not
// one. An invalid mode is 400 and is not redirected.
func (h *PreferenceHandlers) SetMode(w http.ResponseWriter, r *http.Request) {
	if crossSiteRequest(r) {
		writeJSONMessage(w, http.StatusForbidden, false, "Cross-site requests are not allowed")
		return
	}
	mode, err := readUIMode(r)
	if errors.Is(err, errInvalidJSON) {
		writeJSONMessage(w, http.StatusBadRequest, false, "The request body is not valid JSON")
		return
	}
	if err != nil || !ValidUIMode(mode) {
		writeJSONMessage(w, http.StatusBadRequest, false, "mode must be cozy or console")
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
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   60 * 60 * 24 * 365,
	})
	if modeWantsNoContent(r) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, modeRedirectTarget(r), http.StatusSeeOther)
}

// modeWantsNoContent reports a programmatic save: JSON body, a client that
// asked for JSON, or an HTMX request. A plain HTML form is the other case.
func modeWantsNoContent(r *http.Request) bool {
	if r == nil {
		return false
	}
	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		return true
	}
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		return true
	}
	return r.Header.Get("HX-Request") != ""
}

// modeRedirectTarget is a same-origin relative path taken from Referer.
// Anything else, including an off-site URL, becomes /.
func modeRedirectTarget(r *http.Request) string {
	if r == nil {
		return "/"
	}
	ref := strings.TrimSpace(r.Referer())
	if ref == "" {
		return "/"
	}
	parsed, err := url.Parse(ref)
	if err != nil {
		return "/"
	}
	if parsed.IsAbs() {
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return "/"
		}
		if !strings.EqualFold(parsed.Host, r.Host) {
			return "/"
		}
	} else if parsed.Host != "" {
		return "/"
	}
	path := parsed.Path
	if path == "" || !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return "/"
	}
	if strings.ContainsAny(path, "\\\r\n") {
		return "/"
	}
	if parsed.RawQuery != "" {
		return path + "?" + parsed.RawQuery
	}
	return path
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
			return "", errInvalidJSON
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
