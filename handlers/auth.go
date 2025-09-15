package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// AuthHandlers handles authentication requests
type AuthHandlers struct {
	container *Container
}

// NewAuthHandlers creates a new AuthHandlers instance
func NewAuthHandlers(container *Container) *AuthHandlers {
	return &AuthHandlers{container: container}
}

// LoginPage serves the login page
func (h *AuthHandlers) LoginPage(w http.ResponseWriter, r *http.Request) {
	templates := LoadTemplates()

	data := map[string]string{
		"Title": "Login - Ignite",
	}

	if err := templates["login"].Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
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

// ChangePasswordRequest represents a password change request
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// Default credentials - in production, these should be stored securely
var defaultUsername = "admin"
var defaultPassword = "admin"

// Login handles login requests
func (h *AuthHandlers) Login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(LoginResponse{
			Success: false,
			Message: "Invalid request format",
		})
		return
	}

	// Simple authentication check
	if req.Username == defaultUsername && req.Password == defaultPassword {
		// Create a simple session token (in production, use proper JWT or session management)
		token := generateSimpleToken(req.Username)

		// Set session cookie
		http.SetCookie(w, &http.Cookie{
			Name:     "ignite_session",
			Value:    token,
			Expires:  time.Now().Add(24 * time.Hour),
			HttpOnly: true,
			Path:     "/",
		})

		json.NewEncoder(w).Encode(LoginResponse{
			Success: true,
			Message: "Login successful",
			Token:   token,
		})
	} else {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(LoginResponse{
			Success: false,
			Message: "Invalid username or password",
		})
	}
}

// Logout handles logout requests
func (h *AuthHandlers) Logout(w http.ResponseWriter, r *http.Request) {
	// Clear the session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "ignite_session",
		Value:    "",
		Expires:  time.Now().Add(-time.Hour),
		HttpOnly: true,
		Path:     "/",
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Logged out successfully",
	})
}

// ChangePassword handles password change requests
func (h *AuthHandlers) ChangePassword(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Check if user is authenticated
	if !isAuthenticated(r) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "Not authenticated",
		})
		return
	}

	var req ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "Invalid request format",
		})
		return
	}

	// Verify current password
	if req.CurrentPassword != defaultPassword {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "Current password is incorrect",
		})
		return
	}

	// Update password (in production, hash the password)
	defaultPassword = req.NewPassword

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Password changed successfully",
	})
}

// generateSimpleToken creates a simple session token
func generateSimpleToken(username string) string {
	// This is a very basic token generation for demo purposes
	// In production, use proper JWT or secure session tokens
	return username + "_" + time.Now().Format("20060102150405")
}

// isAuthenticated checks if the request has a valid session
func isAuthenticated(r *http.Request) bool {
	cookie, err := r.Cookie("ignite_session")
	if err != nil {
		return false
	}

	// Basic token validation (in production, properly validate JWT or session)
	return cookie.Value != "" && len(cookie.Value) > 0
}

// AuthMiddleware is a middleware that checks authentication
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// List of public paths that don't require authentication
		publicPaths := []string{
			"/login",
			"/auth/login",
			"/auth/logout",
		}

		// Also allow static files
		if strings.HasPrefix(r.URL.Path, "/public/") {
			next.ServeHTTP(w, r)
			return
		}

		// Check if the current path is in the public paths
		for _, path := range publicPaths {
			if r.URL.Path == path {
				next.ServeHTTP(w, r)
				return
			}
		}

		// Check if user is authenticated
		if !isAuthenticated(r) {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		next.ServeHTTP(w, r)
	})
}
