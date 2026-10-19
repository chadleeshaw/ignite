package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestErrorPageIncludesUIMode(t *testing.T) {
	chdirRepoRoot(t)
	prev := preferenceStore
	t.Cleanup(func() { SetPreferenceStore(prev) })

	t.Run("no session uses cozy", func(t *testing.T) {
		SetPreferenceStore(&memoryPrefs{mode: ConsoleUIMode})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/missing", nil)
		req.Header.Set("Accept", "text/html")
		HandleError(rec, req, NewNotFoundError("secret-internal-detail", "The page you requested was not found."))

		require.Equal(t, http.StatusNotFound, rec.Code)
		body := rec.Body.String()
		require.Contains(t, body, `data-theme="cozy"`)
		require.Contains(t, body, "The page you requested was not found.")
		require.Contains(t, body, "Not Found")
		require.NotContains(t, body, "secret-internal-detail")
		require.NotContains(t, body, `data-theme="console"`)
		require.NotContains(t, body, `{{.Mode}}`)
	})

	t.Run("signed in uses the stored mode", func(t *testing.T) {
		SetPreferenceStore(&memoryPrefs{mode: ConsoleUIMode})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/broken", nil)
		req.Header.Set("Accept", "text/html")
		req.AddCookie(testSessionCookie(t))
		HandleError(rec, req, NewInternalError("database exploded", "Something went wrong."))

		require.Equal(t, http.StatusInternalServerError, rec.Code)
		body := rec.Body.String()
		require.Contains(t, body, `data-theme="console"`)
		require.Contains(t, body, "Something went wrong.")
		require.Contains(t, body, "Internal Server Error")
		require.NotContains(t, body, "database exploded")
	})

	t.Run("lookup failure uses cozy", func(t *testing.T) {
		SetPreferenceStore(errorPrefs{})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/broken", nil)
		req.Header.Set("Accept", "text/html")
		req.AddCookie(testSessionCookie(t))
		HandleError(rec, req, NewInternalError("lookup exploded", "Something went wrong."))

		require.Equal(t, http.StatusInternalServerError, rec.Code)
		require.Contains(t, rec.Body.String(), `data-theme="cozy"`)
	})

	t.Run("unknown stored mode uses cozy", func(t *testing.T) {
		SetPreferenceStore(fixedPrefs{mode: "neon"})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/missing", nil)
		req.Header.Set("Accept", "text/html")
		req.AddCookie(testSessionCookie(t))
		HandleError(rec, req, NewNotFoundError("missing", "The page you requested was not found."))

		require.Contains(t, rec.Body.String(), `data-theme="cozy"`)
	})
}

func TestLoginPageUsesCozyWithoutSession(t *testing.T) {
	chdirRepoRoot(t)
	prev := preferenceStore
	SetPreferenceStore(&memoryPrefs{mode: ConsoleUIMode})
	t.Cleanup(func() { SetPreferenceStore(prev) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	NewAuthHandlers(nil).LoginPage(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	require.Contains(t, body, `data-theme="cozy"`)
	require.NotContains(t, body, `data-theme="console"`)
	require.NotContains(t, body, `{{.Mode}}`)
}

type errorPrefs struct{}

func (errorPrefs) GetUIMode(context.Context, string) (string, error) {
	return ConsoleUIMode, errPreferenceLookup
}

func (errorPrefs) SetUIMode(context.Context, string, string) error {
	return errPreferenceLookup
}

type fixedPrefs struct {
	mode string
}

func (p fixedPrefs) GetUIMode(context.Context, string) (string, error) {
	return p.mode, nil
}

func (fixedPrefs) SetUIMode(context.Context, string, string) error {
	return nil
}

type preferenceLookupError struct{}

func (preferenceLookupError) Error() string { return "preference lookup failed" }

var errPreferenceLookup = preferenceLookupError{}
