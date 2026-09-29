package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
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

// sessionStore holds the server-side session table: token -> expiry.
// A session is valid only if its token is present here and not expired,
// so logout (delete) and expiry are enforced server-side.
var sessionStore = struct {
	sync.RWMutex
	tokens map[string]time.Time
}{tokens: make(map[string]time.Time)}

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

// createSession mints a new random token, stores it server-side, and returns it.
func createSession() (string, error) {
	token, err := generateSessionToken()
	if err != nil {
		return "", err
	}
	sessionStore.Lock()
	sessionStore.tokens[token] = time.Now().Add(sessionTTL)
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
	expiry, ok := sessionStore.tokens[token]
	sessionStore.RUnlock()
	if !ok {
		return false
	}
	if time.Now().After(expiry) {
		destroySession(token)
		return false
	}
	return true
}

// sessionCookie builds the session cookie with secure flags.
func sessionCookie(token string, expired bool) *http.Cookie {
	c := &http.Cookie{
		Name:     "ignite_session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
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
	http.SetCookie(w, sessionCookie(token, false))

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
	http.SetCookie(w, sessionCookie("", true))

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
		// Skip auth for login page and auth endpoints
		if r.URL.Path == "/login" ||
			r.URL.Path == "/auth/login" ||
			r.URL.Path == "/auth/logout" {
			next.ServeHTTP(w, r)
			return
		}

		// Skip auth for static files and PXE boot files (needed for network booting)
		if strings.HasPrefix(r.URL.Path, "/public/") ||
			strings.HasPrefix(r.URL.Path, "/tftp/serve") {
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

		next.ServeHTTP(w, r)
	})
}
