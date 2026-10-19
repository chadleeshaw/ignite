package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"ignite/config"
	"ignite/db"

	"github.com/stretchr/testify/require"
)

type failPrefBucket struct{ db.Database }

func (failPrefBucket) GetOrCreateBucket(context.Context, string) error {
	return errors.New("disk full")
}

func TestNewBoltPreferenceStoreBucketFailure(t *testing.T) {
	store, err := NewBoltPreferenceStore(failPrefBucket{}, "prefs")
	require.Nil(t, store)
	require.ErrorContains(t, err, "preferences bucket")
	require.ErrorContains(t, err, "disk full")
}

func TestPreferenceModeEndpoints(t *testing.T) {
	cfg, err := config.NewConfigBuilder().
		WithDBPath(t.TempDir()).
		WithDBFile("prefs.db").
		WithBucket("test").
		Build()
	require.NoError(t, err)
	database, err := db.NewBoltDB(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })

	store, err := NewBoltPreferenceStore(database, "test_prefs")
	require.NoError(t, err)
	h := NewPreferenceHandlers(store)

	t.Run("default is cozy", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/preferences/mode", nil)
		rec := httptest.NewRecorder()
		h.GetMode(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		var body uiModeResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		require.Equal(t, "cozy", body.Mode)
	})

	t.Run("rejects unknown mode", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/preferences/mode", strings.NewReader(`{"mode":"compact"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.SetMode(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("stores console from json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/preferences/mode", strings.NewReader(`{"mode":"console"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.SetMode(rec, req)
		require.Equal(t, http.StatusNoContent, rec.Code)
		require.Empty(t, rec.Body.String())
		cookie := rec.Result().Cookies()
		require.NotEmpty(t, cookie)
		require.Equal(t, "ignite_mode", cookie[0].Name)
		require.Equal(t, "console", cookie[0].Value)

		got, err := store.GetUIMode(req.Context(), currentUsername())
		require.NoError(t, err)
		require.Equal(t, "console", got)
	})

	t.Run("form cozy round trip", func(t *testing.T) {
		form := url.Values{"mode": {"cozy"}}
		req := httptest.NewRequest(http.MethodPost, "/api/preferences/mode", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		rec := httptest.NewRecorder()
		h.SetMode(rec, req)
		require.Equal(t, http.StatusNoContent, rec.Code)

		get := httptest.NewRequest(http.MethodGet, "/api/preferences/mode", nil)
		rec = httptest.NewRecorder()
		h.GetMode(rec, get)
		var body uiModeResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		require.Equal(t, "cozy", body.Mode)
	})
}

func TestPageRenderKeepsBodyClass(t *testing.T) {
	chdirRepoRoot(t)
	prev := preferenceStore
	SetPreferenceStore(&memoryPrefs{mode: ConsoleUIMode})
	t.Cleanup(func() { SetPreferenceStore(prev) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	renderCachedTemplate(rec, req, "index", map[string]string{"Title": "Ignite"}, "Unable to render the home page")

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	require.Contains(t, body, `<body class="bg-base-100">`)
	require.Contains(t, body, `data-theme="ignite"`)
	require.NotContains(t, body, `data-theme="console"`)
	require.NotContains(t, body, `data-theme="cozy"`)
	require.NotContains(t, body, `bg-base-100 console`)
	require.NotContains(t, body, `bg-base-100 cozy`)
	require.NotContains(t, body, `{{.Mode}}`)
}

func TestApplyUIModeUsesStoredPreference(t *testing.T) {
	prev := preferenceStore
	t.Cleanup(func() { SetPreferenceStore(prev) })
	SetPreferenceStore(&memoryPrefs{mode: ConsoleUIMode})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(testSessionCookie(t))
	page := struct {
		Title string
		Mode  string
	}{Title: "DHCP"}
	stamped := applyUIMode(req, page)
	got, ok := stamped.(*struct {
		Title string
		Mode  string
	})
	require.True(t, ok)
	require.Equal(t, "DHCP", got.Title)
	require.Equal(t, "console", got.Mode)

	mapped := map[string]string{"title": "Ignite"}
	applyUIMode(req, mapped)
	require.Equal(t, "console", mapped["Mode"])

	anonymous := httptest.NewRequest(http.MethodGet, "/", nil)
	stamped = applyUIMode(anonymous, page)
	got, ok = stamped.(*struct {
		Title string
		Mode  string
	})
	require.True(t, ok)
	require.Equal(t, "cozy", got.Mode)
}

func TestApplyUIModeOverwritesHardcodedMode(t *testing.T) {
	prev := preferenceStore
	t.Cleanup(func() { SetPreferenceStore(prev) })
	SetPreferenceStore(&memoryPrefs{mode: ConsoleUIMode})

	req := httptest.NewRequest(http.MethodGet, "/dhcp", nil)
	req.AddCookie(testSessionCookie(t))
	page := struct {
		Title string
		Mode  string
	}{Title: "DHCP", Mode: DefaultUIMode}
	stamped := applyUIMode(req, page)
	got, ok := stamped.(*struct {
		Title string
		Mode  string
	})
	require.True(t, ok)
	require.Equal(t, ConsoleUIMode, got.Mode)

	anonymous := httptest.NewRequest(http.MethodGet, "/dhcp", nil)
	forced := struct {
		Title string
		Mode  string
	}{Title: "DHCP", Mode: ConsoleUIMode}
	stamped = applyUIMode(anonymous, forced)
	got, ok = stamped.(*struct {
		Title string
		Mode  string
	})
	require.True(t, ok)
	require.Equal(t, DefaultUIMode, got.Mode)
}

func TestSetModeContentNegotiation(t *testing.T) {
	store := &memoryPrefs{mode: DefaultUIMode}
	h := NewPreferenceHandlers(store)

	t.Run("json stays 204", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/preferences/mode", strings.NewReader(`{"mode":"console"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Host = "example.com"
		rec := httptest.NewRecorder()
		h.SetMode(rec, req)
		require.Equal(t, http.StatusNoContent, rec.Code)
		require.Equal(t, ConsoleUIMode, store.mode)
	})

	t.Run("accept json stays 204", func(t *testing.T) {
		form := url.Values{"mode": {"cozy"}}
		req := httptest.NewRequest(http.MethodPost, "/api/preferences/mode", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		rec := httptest.NewRecorder()
		h.SetMode(rec, req)
		require.Equal(t, http.StatusNoContent, rec.Code)
		require.Equal(t, DefaultUIMode, store.mode)
	})

	t.Run("htmx stays 204", func(t *testing.T) {
		form := url.Values{"mode": {"console"}}
		req := httptest.NewRequest(http.MethodPost, "/api/preferences/mode", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("HX-Request", "true")
		rec := httptest.NewRecorder()
		h.SetMode(rec, req)
		require.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("html form redirects to same-origin referer", func(t *testing.T) {
		form := url.Values{"mode": {"cozy"}}
		req := httptest.NewRequest(http.MethodPost, "/api/preferences/mode", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Host = "example.com"
		req.Header.Set("Referer", "http://example.com/dhcp?tab=leases")
		rec := httptest.NewRecorder()
		h.SetMode(rec, req)
		require.Equal(t, http.StatusSeeOther, rec.Code)
		require.Equal(t, "/dhcp?tab=leases", rec.Header().Get("Location"))
		require.Equal(t, DefaultUIMode, store.mode)
	})

	t.Run("foreign referer redirects home", func(t *testing.T) {
		form := url.Values{"mode": {"console"}}
		req := httptest.NewRequest(http.MethodPost, "/api/preferences/mode", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Host = "example.com"
		req.Header.Set("Referer", "http://evil.example/phish")
		rec := httptest.NewRecorder()
		h.SetMode(rec, req)
		require.Equal(t, http.StatusSeeOther, rec.Code)
		require.Equal(t, "/", rec.Header().Get("Location"))
	})

	t.Run("invalid mode is 400", func(t *testing.T) {
		form := url.Values{"mode": {"neon"}}
		req := httptest.NewRequest(http.MethodPost, "/api/preferences/mode", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Host = "example.com"
		req.Header.Set("Referer", "http://example.com/dhcp")
		rec := httptest.NewRecorder()
		h.SetMode(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
		require.Empty(t, rec.Header().Get("Location"))
		require.Contains(t, rec.Body.String(), "mode must be cozy or console")
		require.Equal(t, ConsoleUIMode, store.mode)
	})
}

type memoryPrefs struct {
	mode string
}

func (m *memoryPrefs) GetUIMode(ctx context.Context, username string) (string, error) {
	if !ValidUIMode(m.mode) {
		return DefaultUIMode, nil
	}
	return m.mode, nil
}

func (m *memoryPrefs) SetUIMode(ctx context.Context, username, mode string) error {
	m.mode = mode
	return nil
}
