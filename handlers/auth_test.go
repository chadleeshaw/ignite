package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAuthHandlers_LoginPage(t *testing.T) {
	// Skip this test when running from handlers directory
	// because templates are not accessible from this path
	t.Skip("Template rendering test requires running from project root")
}

// resetTestCredentials restores the default admin/admin credentials under lock.
func resetTestCredentials() {
	credentialMu.Lock()
	defaultUsername = "admin"
	defaultPassword = "admin"
	credentialMu.Unlock()
}

// testSessionCookie creates a real server-side session and returns a cookie
// carrying it. The session is destroyed when the test finishes.
func testSessionCookie(t *testing.T) *http.Cookie {
	t.Helper()
	token, err := createSession()
	if err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}
	t.Cleanup(func() { destroySession(token) })
	return &http.Cookie{Name: "ignite_session", Value: token}
}

func TestAuthHandlers_Login_Success(t *testing.T) {
	// Reset to default credentials for test
	resetTestCredentials()

	container := &Container{}
	authHandlers := NewAuthHandlers(container)

	loginReq := LoginRequest{
		Username: "admin",
		Password: "admin",
	}

	jsonData, _ := json.Marshal(loginReq)
	req := httptest.NewRequest("POST", "/auth/login", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	authHandlers.Login(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}

	var response LoginResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if !response.Success {
		t.Error("Expected login to succeed")
	}

	if response.Token == "" {
		t.Error("Expected token to be set")
	}

	// Check if session cookie is set
	cookies := w.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, cookie := range cookies {
		if cookie.Name == "ignite_session" {
			sessionCookie = cookie
			break
		}
	}

	if sessionCookie == nil {
		t.Error("Expected session cookie to be set")
		return
	}

	if sessionCookie.Value == "" {
		t.Error("Expected session cookie to have a value")
	}

	if !sessionCookie.HttpOnly {
		t.Error("Expected session cookie to be HttpOnly")
	}
}

func TestAuthHandlers_Login_InvalidCredentials(t *testing.T) {
	// Reset to default credentials for test
	resetTestCredentials()

	container := &Container{}
	authHandlers := NewAuthHandlers(container)

	loginReq := LoginRequest{
		Username: "wrong",
		Password: "credentials",
	}

	jsonData, _ := json.Marshal(loginReq)
	req := httptest.NewRequest("POST", "/auth/login", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	authHandlers.Login(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status code %d, got %d", http.StatusUnauthorized, w.Code)
	}

	var response LoginResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.Success {
		t.Error("Expected login to fail")
	}

	if response.Message == "" {
		t.Error("Expected error message to be set")
	}
}

func TestAuthHandlers_Login_InvalidJSON(t *testing.T) {
	container := &Container{}
	authHandlers := NewAuthHandlers(container)

	req := httptest.NewRequest("POST", "/auth/login", strings.NewReader("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	authHandlers.Login(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, w.Code)
	}

	var response LoginResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.Success {
		t.Error("Expected login to fail")
	}
}

func TestAuthHandlers_Logout(t *testing.T) {
	container := &Container{}
	authHandlers := NewAuthHandlers(container)

	// Log in to get a real session
	token, err := createSession()
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	req := httptest.NewRequest("POST", "/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: "ignite_session", Value: token})
	w := httptest.NewRecorder()

	authHandlers.Logout(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}

	// The server-side session must be destroyed, not just the cookie expired
	if validSession(token) {
		t.Error("Expected server-side session to be destroyed on logout")
	}

	// Check if session cookie is cleared
	cookies := w.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, cookie := range cookies {
		if cookie.Name == "ignite_session" {
			sessionCookie = cookie
			break
		}
	}

	if sessionCookie == nil {
		t.Error("Expected session cookie to be set for clearing")
		return
	}

	if sessionCookie.Value != "" {
		t.Error("Expected session cookie value to be empty")
	}

	// Check if cookie expires in the past
	if sessionCookie.Expires.After(time.Now()) {
		t.Error("Expected session cookie to expire in the past")
	}
}

func TestAuthHandlers_ChangePassword_Success(t *testing.T) {
	// Reset to default credentials for test
	resetTestCredentials()

	container := &Container{}
	authHandlers := NewAuthHandlers(container)

	// Create a request with valid session cookie
	req := httptest.NewRequest("POST", "/auth/change-password", nil)
	req.AddCookie(testSessionCookie(t))

	changeReq := ChangePasswordRequest{
		CurrentPassword: "admin",
		NewPassword:     "newpassword123",
	}

	jsonData, _ := json.Marshal(changeReq)
	req = httptest.NewRequest("POST", "/auth/change-password", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(testSessionCookie(t))

	w := httptest.NewRecorder()

	authHandlers.ChangePassword(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if !response["success"].(bool) {
		t.Error("Expected password change to succeed")
	}

	// Verify password was actually changed
	credentialMu.RLock()
	changed := defaultPassword == "newpassword123"
	credentialMu.RUnlock()
	if !changed {
		t.Error("Expected password to be updated")
	}

	// Reset for other tests
	resetTestCredentials()
}

func TestAuthHandlers_ChangePassword_NotAuthenticated(t *testing.T) {
	container := &Container{}
	authHandlers := NewAuthHandlers(container)

	changeReq := ChangePasswordRequest{
		CurrentPassword: "admin",
		NewPassword:     "newpassword123",
	}

	jsonData, _ := json.Marshal(changeReq)
	req := httptest.NewRequest("POST", "/auth/change-password", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()

	authHandlers.ChangePassword(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status code %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestAuthHandlers_ChangePassword_WrongCurrentPassword(t *testing.T) {
	// Reset to default credentials for test
	resetTestCredentials()

	container := &Container{}
	authHandlers := NewAuthHandlers(container)

	changeReq := ChangePasswordRequest{
		CurrentPassword: "wrongpassword",
		NewPassword:     "newpassword123",
	}

	jsonData, _ := json.Marshal(changeReq)
	req := httptest.NewRequest("POST", "/auth/change-password", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(testSessionCookie(t))

	w := httptest.NewRecorder()

	authHandlers.ChangePassword(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status code %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestIsAuthenticated(t *testing.T) {
	tests := []struct {
		name           string
		cookie         *http.Cookie
		expectedResult bool
	}{
		{
			name:           "Valid session cookie",
			cookie:         nil, // filled in below with a real session
			expectedResult: true,
		},
		{
			name: "Empty session cookie",
			cookie: &http.Cookie{
				Name:  "ignite_session",
				Value: "",
			},
			expectedResult: false,
		},
		{
			name:           "No session cookie",
			cookie:         nil,
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if tt.name == "Valid session cookie" {
				req.AddCookie(testSessionCookie(t))
			} else if tt.cookie != nil {
				req.AddCookie(tt.cookie)
			}

			result := isAuthenticated(req)
			if result != tt.expectedResult {
				t.Errorf("Expected %v, got %v", tt.expectedResult, result)
			}
		})
	}
}

func TestGenerateSessionToken(t *testing.T) {
	token, err := generateSessionToken()
	if err != nil {
		t.Fatalf("generateSessionToken returned error: %v", err)
	}

	if token == "" {
		t.Error("Expected token to be generated")
	}

	// 32 random bytes hex-encoded = 64 chars
	if len(token) != 64 {
		t.Errorf("Expected 64-char hex token, got %d chars", len(token))
	}

	// Tokens must be unique and unpredictable
	token2, err := generateSessionToken()
	if err != nil {
		t.Fatalf("generateSessionToken returned error: %v", err)
	}
	if token == token2 {
		t.Error("Expected tokens to be unique")
	}

	// A minted session must validate server-side
	token3, err := createSession()
	if err != nil {
		t.Fatalf("createSession returned error: %v", err)
	}
	defer destroySession(token3)
	if !validSession(token3) {
		t.Error("Expected freshly created session to be valid")
	}
	if validSession(token) {
		t.Error("Expected unregistered token to be invalid")
	}

	// Destroyed sessions must stop validating
	destroySession(token3)
	if validSession(token3) {
		t.Error("Expected destroyed session to be invalid")
	}
}
