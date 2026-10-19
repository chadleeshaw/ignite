package handlers

import (
	"net/http"
	"strings"
)

// csrfFormField is the form field accepted in place of the X-CSRF-Token header.
const csrfFormField = "csrf_token"

// csrfHeader is the header htmx and fetch clients send.
const csrfHeader = "X-CSRF-Token"

func isStateChanging(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodDelete:
		return true
	default:
		return false
	}
}

// csrfAccepted reports whether a state-changing request presented the
// session's CSRF token. Safe methods always pass. The token may arrive as
// the X-CSRF-Token header or the csrf_token form field.
func csrfAccepted(r *http.Request) bool {
	if r == nil || !isStateChanging(r.Method) {
		return true
	}
	cookie, err := r.Cookie("ignite_session")
	if err != nil {
		return false
	}
	return tokensEqual(submittedCSRFToken(r), sessionCSRF(cookie.Value))
}

func submittedCSRFToken(r *http.Request) string {
	if tok := strings.TrimSpace(r.Header.Get(csrfHeader)); tok != "" {
		return tok
	}
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "application/x-www-form-urlencoded") ||
		strings.HasPrefix(ct, "multipart/form-data") {
		_ = r.ParseForm()
		return strings.TrimSpace(r.PostFormValue(csrfFormField))
	}
	return ""
}

func rejectCSRF(w http.ResponseWriter, r *http.Request) {
	HandleError(w, r, NewAppError(
		ErrorTypeForbidden,
		"csrf token mismatch",
		"CSRF token mismatch",
		http.StatusForbidden,
	))
}

// currentCSRFToken is the token for the request's session, or empty when
// the request is anonymous. Templates receive it as .CSRFToken.
func currentCSRFToken(r *http.Request) string {
	if r == nil || !isAuthenticated(r) {
		return ""
	}
	cookie, err := r.Cookie("ignite_session")
	if err != nil {
		return ""
	}
	return sessionCSRF(cookie.Value)
}
