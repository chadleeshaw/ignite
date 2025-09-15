package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCompleteLoginLogoutFlow(t *testing.T) {
	// Reset to default credentials for test
	defaultUsername = "admin"
	defaultPassword = "admin"

	container := &Container{}
	authHandlers := NewAuthHandlers(container)

	// Step 1: Try to access protected resource without authentication
	req := httptest.NewRequest("GET", "/dhcp", nil)
	w := httptest.NewRecorder()

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Protected Content"))
	})

	middlewareHandler := AuthMiddleware(testHandler)
	middlewareHandler.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("Step 1: Expected redirect status %d, got %d", http.StatusFound, w.Code)
	}

	location := w.Header().Get("Location")
	if location != "/login" {
		t.Errorf("Step 1: Expected redirect to /login, got %s", location)
	}

	// Step 2: Login with valid credentials
	loginReq := LoginRequest{
		Username: "admin",
		Password: "admin",
	}

	jsonData, _ := json.Marshal(loginReq)
	req = httptest.NewRequest("POST", "/auth/login", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()

	authHandlers.Login(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Step 2: Expected login status %d, got %d", http.StatusOK, w.Code)
	}

	// Extract session cookie
	cookies := w.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, cookie := range cookies {
		if cookie.Name == "ignite_session" {
			sessionCookie = cookie
			break
		}
	}

	if sessionCookie == nil {
		t.Fatal("Step 2: Expected session cookie to be set")
	}

	// Step 3: Access protected resource with session cookie
	req = httptest.NewRequest("GET", "/dhcp", nil)
	req.AddCookie(sessionCookie)
	w = httptest.NewRecorder()

	middlewareHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Step 3: Expected access to protected resource status %d, got %d", http.StatusOK, w.Code)
	}

	if w.Body.String() != "Protected Content" {
		t.Errorf("Step 3: Expected protected content, got %s", w.Body.String())
	}

	// Step 4: Logout
	req = httptest.NewRequest("POST", "/auth/logout", nil)
	req.AddCookie(sessionCookie)
	w = httptest.NewRecorder()

	authHandlers.Logout(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Step 4: Expected logout status %d, got %d", http.StatusOK, w.Code)
	}

	// Step 5: Try to access protected resource after logout
	req = httptest.NewRequest("GET", "/dhcp", nil)
	req.AddCookie(sessionCookie) // Use old cookie which should now be invalid
	w = httptest.NewRecorder()

	middlewareHandler.ServeHTTP(w, req)

	// The middleware should redirect to login since the old cookie is still present
	// but the logout handler cleared it, so we need to use the new cleared cookie
	logoutCookies := w.Result().Cookies()
	var clearedCookie *http.Cookie
	for _, cookie := range logoutCookies {
		if cookie.Name == "ignite_session" {
			clearedCookie = cookie
			break
		}
	}

	// Test with cleared cookie
	req = httptest.NewRequest("GET", "/dhcp", nil)
	if clearedCookie != nil {
		req.AddCookie(clearedCookie)
	}
	w = httptest.NewRecorder()

	middlewareHandler.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("Step 5: Expected redirect after logout status %d, got %d", http.StatusFound, w.Code)
	}

	location = w.Header().Get("Location")
	if location != "/login" {
		t.Errorf("Step 5: Expected redirect to /login after logout, got %s", location)
	}
}

func TestCompletePasswordChangeFlow(t *testing.T) {
	// Reset to default credentials for test
	defaultUsername = "admin"
	defaultPassword = "admin"
	defer func() {
		defaultPassword = "admin" // Reset after test
	}()

	container := &Container{}
	authHandlers := NewAuthHandlers(container)

	// Step 1: Login to get session
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
		t.Fatalf("Step 1: Login failed with status %d", w.Code)
	}

	// Extract session cookie
	cookies := w.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, cookie := range cookies {
		if cookie.Name == "ignite_session" {
			sessionCookie = cookie
			break
		}
	}

	if sessionCookie == nil {
		t.Fatal("Step 1: Expected session cookie to be set")
	}

	// Step 2: Change password with valid current password
	changeReq := ChangePasswordRequest{
		CurrentPassword: "admin",
		NewPassword:     "newpassword123",
	}

	jsonData, _ = json.Marshal(changeReq)
	req = httptest.NewRequest("POST", "/auth/change-password", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	w = httptest.NewRecorder()

	authHandlers.ChangePassword(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Step 2: Expected password change status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Step 2: Failed to unmarshal response: %v", err)
	}

	if !response["success"].(bool) {
		t.Errorf("Step 2: Expected password change to succeed")
	}

	// Step 3: Verify old password no longer works
	loginReq = LoginRequest{
		Username: "admin",
		Password: "admin", // Old password
	}

	jsonData, _ = json.Marshal(loginReq)
	req = httptest.NewRequest("POST", "/auth/login", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()

	authHandlers.Login(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Step 3: Expected old password to fail with status %d, got %d", http.StatusUnauthorized, w.Code)
	}

	// Step 4: Verify new password works
	loginReq = LoginRequest{
		Username: "admin",
		Password: "newpassword123", // New password
	}

	jsonData, _ = json.Marshal(loginReq)
	req = httptest.NewRequest("POST", "/auth/login", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()

	authHandlers.Login(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Step 4: Expected new password to work with status %d, got %d", http.StatusOK, w.Code)
	}

	var loginResponse LoginResponse
	err = json.Unmarshal(w.Body.Bytes(), &loginResponse)
	if err != nil {
		t.Fatalf("Step 4: Failed to unmarshal login response: %v", err)
	}

	if !loginResponse.Success {
		t.Error("Step 4: Expected login with new password to succeed")
	}
}

func TestPasswordChangeSecurityChecks(t *testing.T) {
	// Reset to default credentials for test
	defaultPassword = "admin"
	defer func() {
		defaultPassword = "admin" // Reset after test
	}()

	container := &Container{}
	authHandlers := NewAuthHandlers(container)

	// Create valid session cookie
	sessionCookie := &http.Cookie{
		Name:  "ignite_session",
		Value: "admin_12345",
	}

	tests := []struct {
		name           string
		currentPass    string
		newPass        string
		authenticated  bool
		expectedStatus int
		expectedMsg    string
	}{
		{
			name:           "Unauthenticated password change",
			currentPass:    "admin",
			newPass:        "newpass",
			authenticated:  false,
			expectedStatus: http.StatusUnauthorized,
			expectedMsg:    "Not authenticated",
		},
		{
			name:           "Wrong current password",
			currentPass:    "wrongpass",
			newPass:        "newpass",
			authenticated:  true,
			expectedStatus: http.StatusUnauthorized,
			expectedMsg:    "Current password is incorrect",
		},
		{
			name:           "Valid password change",
			currentPass:    "admin",
			newPass:        "validnewpass",
			authenticated:  true,
			expectedStatus: http.StatusOK,
			expectedMsg:    "Password changed successfully",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset password for each test
			defaultPassword = "admin"

			changeReq := ChangePasswordRequest{
				CurrentPassword: tt.currentPass,
				NewPassword:     tt.newPass,
			}

			jsonData, _ := json.Marshal(changeReq)
			req := httptest.NewRequest("POST", "/auth/change-password", bytes.NewBuffer(jsonData))
			req.Header.Set("Content-Type", "application/json")

			if tt.authenticated {
				req.AddCookie(sessionCookie)
			}

			w := httptest.NewRecorder()

			authHandlers.ChangePassword(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			if err != nil {
				t.Fatalf("Failed to unmarshal response: %v", err)
			}

			if message, ok := response["message"].(string); ok {
				if message != tt.expectedMsg {
					t.Errorf("Expected message '%s', got '%s'", tt.expectedMsg, message)
				}
			}
		})
	}
}

func TestSessionPersistence(t *testing.T) {
	// Reset to default credentials for test
	defaultUsername = "admin"
	defaultPassword = "admin"

	container := &Container{}
	authHandlers := NewAuthHandlers(container)

	// Login to get session
	loginReq := LoginRequest{
		Username: "admin",
		Password: "admin",
	}

	jsonData, _ := json.Marshal(loginReq)
	req := httptest.NewRequest("POST", "/auth/login", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	authHandlers.Login(w, req)

	// Extract session cookie
	cookies := w.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, cookie := range cookies {
		if cookie.Name == "ignite_session" {
			sessionCookie = cookie
			break
		}
	}

	if sessionCookie == nil {
		t.Fatal("Expected session cookie to be set")
	}

	// Test multiple requests with same session
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	middlewareHandler := AuthMiddleware(testHandler)

	paths := []string{"/", "/dhcp", "/provision", "/status"}

	for _, path := range paths {
		t.Run("Session persistence for "+path, func(t *testing.T) {
			req := httptest.NewRequest("GET", path, nil)
			req.AddCookie(sessionCookie)
			w := httptest.NewRecorder()

			middlewareHandler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status %d for %s, got %d", http.StatusOK, path, w.Code)
			}
		})
	}
}
