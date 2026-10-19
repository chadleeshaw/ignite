package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCSRFRejectsSessionMutationWithoutToken(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	wrapped := AuthMiddleware(next)
	cookie := testSessionCookie(t)

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(method, "/dhcp/start", nil)
		req.AddCookie(cookie)
		req.Header.Set("Accept", "application/json")
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)
		require.Equal(t, http.StatusForbidden, rec.Code, method)
		require.Contains(t, rec.Body.String(), "CSRF token mismatch")
	}
}

func TestCSRFAcceptsHeaderAndFormField(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	wrapped := AuthMiddleware(next)
	cookie := testSessionCookie(t)
	token := sessionCSRF(cookie.Value)
	require.NotEmpty(t, token)

	req := httptest.NewRequest(http.MethodPost, "/api/preferences/mode", nil)
	req.AddCookie(cookie)
	req.Header.Set(csrfHeader, token)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)

	form := url.Values{csrfFormField: {token}, "mode": {"cozy"}}
	req = httptest.NewRequest(http.MethodPut, "/dhcp/start", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)

	req = httptest.NewRequest(http.MethodPost, "/dhcp/start", nil)
	req.AddCookie(cookie)
	req.Header.Set(csrfHeader, "not-the-token")
	req.Header.Set("Accept", "application/json")
	rec = httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestCSRFExemptsLoginAndHostCallbacks(t *testing.T) {
	var seen []string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path)
		w.WriteHeader(http.StatusOK)
	})
	wrapped := AuthMiddleware(next)

	login := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, login)
	require.Equal(t, http.StatusOK, rec.Code)

	callback := httptest.NewRequest(http.MethodPost, "/dhcp/lease/aa:bb:cc:dd:ee:ff/heartbeat", nil)
	rec = httptest.NewRecorder()
	wrapped.ServeHTTP(rec, callback)
	require.Equal(t, http.StatusOK, rec.Code)

	state := httptest.NewRequest(http.MethodPost, "/dhcp/lease/aa:bb:cc:dd:ee:ff/state", nil)
	rec = httptest.NewRecorder()
	wrapped.ServeHTTP(rec, state)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, []string{"/auth/login", "/dhcp/lease/aa:bb:cc:dd:ee:ff/heartbeat", "/dhcp/lease/aa:bb:cc:dd:ee:ff/state"}, seen)
}

func TestCSRFLogoutRequiresTokenWhenSessionExists(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	wrapped := AuthMiddleware(next)
	cookie := testSessionCookie(t)

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.AddCookie(cookie)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	require.Equal(t, http.StatusForbidden, rec.Code)

	req = httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.AddCookie(cookie)
	req.Header.Set(csrfHeader, sessionCSRF(cookie.Value))
	rec = httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// No session: logout stays reachable so a stale browser can clear itself.
	req = httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	rec = httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestApplyUIModeSetsCSRFToken(t *testing.T) {
	cookie := testSessionCookie(t)
	req := httptest.NewRequest(http.MethodGet, "/dhcp", nil)
	req.AddCookie(cookie)

	page := struct {
		Title     string
		Mode      string
		CSRFToken string
	}{Title: "DHCP"}
	stamped := applyUIMode(req, page)
	got := stamped.(*struct {
		Title     string
		Mode      string
		CSRFToken string
	})
	require.Equal(t, sessionCSRF(cookie.Value), got.CSRFToken)

	mapped := map[string]string{"title": "Ignite"}
	applyUIMode(req, mapped)
	require.Equal(t, sessionCSRF(cookie.Value), mapped["CSRFToken"])

	anonymous := httptest.NewRequest(http.MethodGet, "/dhcp", nil)
	stamped = applyUIMode(anonymous, page)
	got = stamped.(*struct {
		Title     string
		Mode      string
		CSRFToken string
	})
	require.Empty(t, got.CSRFToken)
}

func TestSafeMethodsSkipCSRF(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	wrapped := AuthMiddleware(next)
	req := httptest.NewRequest(http.MethodGet, "/dhcp", nil)
	req.AddCookie(testSessionCookie(t))
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}
