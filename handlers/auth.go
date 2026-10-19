package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Admin credentials. Guarded by credentialMu so concurrent logins and password
// changes never race.
var (
	credentialMu    sync.RWMutex
	defaultUsername = "admin"
	defaultPassword = "admin"
)

// sessionEntry is one server-side login. The CSRF token is bound to the
// session and is not stored in a cookie, so a cross-site form cannot read it.
type sessionEntry struct {
	expiry time.Time
	csrf   string
}

// sessionStore holds the server-side session table: token -> entry.
// A session is valid only if its token is present here and not expired,
// so logout (delete) and expiry are enforced server-side.
var sessionStore = struct {
	sync.RWMutex
	tokens map[string]sessionEntry
}{tokens: make(map[string]sessionEntry)}

// sessionTTL is how long a login session stays valid.
const sessionTTL = 24 * time.Hour

func init() {
	credentialMu.RLock()
	user, pass := defaultUsername, defaultPassword
	credentialMu.RUnlock()
	if user == "admin" && pass == "admin" {
		log.Println("WARNING: Ignite is running with the default admin/admin credentials. " +
			"Change the password immediately via POST /auth/change-password.")
	}
}

// generateSessionToken creates a 32-byte cryptographically random session token.
func generateSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// createSession mints a new random token and a per-session CSRF token,
// stores them server-side, and returns the session token.
func createSession() (string, error) {
	token, err := generateSessionToken()
	if err != nil {
		return "", err
	}
	csrf, err := generateSessionToken()
	if err != nil {
		return "", err
	}
	sessionStore.Lock()
	sessionStore.tokens[token] = sessionEntry{
		expiry: time.Now().Add(sessionTTL),
		csrf:   csrf,
	}
	sessionStore.Unlock()
	return token, nil
}

// destroySession removes a token from the server-side store.
func destroySession(token string) {
	if token == "" {
		return
	}
	sessionStore.Lock()
	delete(sessionStore.tokens, token)
	sessionStore.Unlock()
}

// validSession reports whether token is a live server-side session.
func validSession(token string) bool {
	if token == "" {
		return false
	}
	sessionStore.RLock()
	entry, ok := sessionStore.tokens[token]
	sessionStore.RUnlock()
	if !ok {
		return false
	}
	if time.Now().After(entry.expiry) {
		destroySession(token)
		return false
	}
	return true
}

// sessionCSRF returns the CSRF token bound to a live session.
// An unknown or expired session returns an empty string.
func sessionCSRF(token string) string {
	if !validSession(token) {
		return ""
	}
	sessionStore.RLock()
	entry, ok := sessionStore.tokens[token]
	sessionStore.RUnlock()
	if !ok {
		return ""
	}
	return entry.csrf
}

// cookieSecure reports whether a cookie should set the Secure flag.
// COOKIE_SECURE is auto (the default), true, or false. auto, and any other
// value, sets Secure only when the request arrived on TLS or when
// X-Forwarded-Proto is https. The CSRF token is not a cookie.
func cookieSecure(r *http.Request) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("COOKIE_SECURE"))) {
	case "true":
		return true
	case "false":
		return false
	default:
		return requestIsHTTPS(r)
	}
}

// requestIsHTTPS reports a TLS connection or an https reverse-proxy hop.
func requestIsHTTPS(r *http.Request) bool {
	if r == nil {
		return false
	}
	if r.TLS != nil {
		return true
	}
	proto := r.Header.Get("X-Forwarded-Proto")
	if i := strings.Index(proto, ","); i >= 0 {
		proto = proto[:i]
	}
	return strings.EqualFold(strings.TrimSpace(proto), "https")
}

// sessionCookie builds the session cookie. Secure follows cookieSecure.
func sessionCookie(r *http.Request, token string, expired bool) *http.Cookie {
	c := &http.Cookie{
		Name:     "ignite_session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   cookieSecure(r),
		SameSite: http.SameSiteLaxMode,
	}
	if expired {
		c.Expires = time.Now().Add(-time.Hour)
		c.MaxAge = -1
	} else {
		c.Expires = time.Now().Add(sessionTTL)
	}
	return c
}

// AuthHandlers handles authentication-related HTTP requests
type AuthHandlers struct {
	container *Container
}

// NewAuthHandlers creates a new AuthHandlers instance
func NewAuthHandlers(container *Container) *AuthHandlers {
	return &AuthHandlers{container: container}
}

// LoginRequest represents a login request
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse represents a login response
type LoginResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Token   string `json:"token,omitempty"`
}

// Login handles user login
func (h *AuthHandlers) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest

	// Try to parse JSON first, then fall back to form data
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(LoginResponse{
				Success: false,
				Message: "Invalid request format",
			})
			return
		}
	} else {
		// Handle form data
		if err := r.ParseForm(); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(LoginResponse{
				Success: false,
				Message: "Invalid form data",
			})
			return
		}
		req.Username = r.FormValue("username")
		req.Password = r.FormValue("password")
	}

	credentialMu.RLock()
	validUser, validPass := defaultUsername, defaultPassword
	credentialMu.RUnlock()

	// Validate credentials
	if req.Username != validUser || req.Password != validPass {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(LoginResponse{
			Success: false,
			Message: "Invalid username or password",
		})
		return
	}

	// Generate a random server-side session token
	token, err := createSession()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(LoginResponse{
			Success: false,
			Message: "Failed to create session",
		})
		return
	}

	// Set session cookie with secure flags
	http.SetCookie(w, sessionCookie(r, token, false))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(LoginResponse{
		Success: true,
		Message: "Login successful",
		Token:   token,
	})
}

// Logout handles user logout by deleting the server-side session and expiring the cookie
func (h *AuthHandlers) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("ignite_session"); err == nil {
		destroySession(cookie.Value)
	}

	// Clear the session cookie
	http.SetCookie(w, sessionCookie(r, "", true))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(LoginResponse{
		Success: true,
		Message: "Logout successful",
	})
}

// ChangePasswordRequest represents a password change request
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// ChangePassword handles password changes
func (h *AuthHandlers) ChangePassword(w http.ResponseWriter, r *http.Request) {
	// Check if user is authenticated
	if !isAuthenticated(r) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "Not authenticated",
		})
		return
	}

	var req ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "Invalid request format",
		})
		return
	}

	if strings.TrimSpace(req.NewPassword) == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "New password cannot be empty",
		})
		return
	}

	credentialMu.Lock()
	defer credentialMu.Unlock()

	// Verify current password
	if req.CurrentPassword != defaultPassword {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "Current password is incorrect",
		})
		return
	}

	// Update password
	defaultPassword = req.NewPassword

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Password changed successfully",
	})
}

// LoginPage serves the login page
func (h *AuthHandlers) LoginPage(w http.ResponseWriter, r *http.Request) {
	// If already authenticated, redirect to dashboard
	if isAuthenticated(r) {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	renderCachedTemplate(w, r, "login", nil, "Unable to render the login page")
}

// isAuthenticated checks if the request carries a valid server-side session.
func isAuthenticated(r *http.Request) bool {
	cookie, err := r.Cookie("ignite_session")
	if err != nil {
		return false
	}
	return validSession(cookie.Value)
}

// isPublicAsset reports whether path is a static file the installer or the
// browser may fetch without a session. Provision configs are excluded even
// when they live under /public/provision/configs/.
func isPublicAsset(path string) bool {
	if !strings.HasPrefix(path, "/public/") {
		return false
	}
	if strings.Contains(path, "/provision/configs/") {
		return false
	}
	return true
}

// isRenderedConfig reports GET /configs/{type}/{name}. The handler, not the
// middleware, decides whether this client may see the file.
func isRenderedConfig(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	return len(parts) == 3 && parts[0] == "configs" && parts[1] != "" && parts[2] != ""
}

// isHostCallback reports whether the request is a per-lease provisioning
// callback (heartbeat or state), as opposed to the session-protected admin
// route POST /dhcp/lease/state.
func isHostCallback(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 4 || parts[0] != "dhcp" || parts[1] != "lease" || parts[2] == "" {
		return false
	}
	switch parts[3] {
	case "heartbeat", "state":
		return true
	default:
		return false
	}
}

// isAPIRequest reports whether the request expects a JSON 401 instead of a
// login redirect: anything under /api/ or explicitly asking for JSON.
func isAPIRequest(r *http.Request) bool {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		return true
	}
	return strings.Contains(r.Header.Get("Accept"), "application/json")
}

// AuthMiddleware checks authentication for protected routes.
// API/JSON clients get a 401 JSON response; browsers get a login redirect.
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Login has no session yet, so it cannot present a CSRF token.
		if r.URL.Path == "/login" || r.URL.Path == "/auth/login" {
			next.ServeHTTP(w, r)
			return
		}

		// Logout stays reachable without a session. A live session must
		// still present its CSRF token.
		if r.URL.Path == "/auth/logout" {
			if isAuthenticated(r) {
				if message := csrfFailure(r); message != "" {
					rejectCSRF(w, r, message)
					return
				}
			}
			next.ServeHTTP(w, r)
			return
		}

		// Skip auth for static files and PXE boot files (needed for network booting).
		// Rendered installer configs are not in that set: they carry the
		// per-lease token and are served only by GET /configs/{type}/{name}.
		if isPublicAsset(r.URL.Path) ||
			strings.HasPrefix(r.URL.Path, "/tftp/serve") {
			next.ServeHTTP(w, r)
			return
		}

		// The installer fetches its config before it has the token. The
		// handler allows the lease IP or a presented token, and 404s the rest.
		if isRenderedConfig(r) {
			next.ServeHTTP(w, r)
			return
		}

		// Host provisioning callbacks authenticate with a per-lease token
		// inside the handler. They must not be redirected to the login page.
		if isHostCallback(r) {
			next.ServeHTTP(w, r)
			return
		}

		// Check authentication
		if !isAuthenticated(r) {
			if isAPIRequest(r) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"success": false,
					"message": "Authentication required",
				})
				return
			}
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		if message := csrfFailure(r); message != "" {
			rejectCSRF(w, r, message)
			return
		}

		next.ServeHTTP(w, r)
	})
}
