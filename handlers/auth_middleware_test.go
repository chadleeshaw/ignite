package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthMiddleware_PublicPaths(t *testing.T) {
	publicPaths := []string{
		"/login",
		"/auth/login",
		"/auth/logout",
		"/public/http/css/tailwind.css",
		"/public/http/img/logo.png",
		"/public/http/js/app.js",
	}

	// Create a simple handler that returns 200 OK
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Wrap with auth middleware
	middlewareHandler := AuthMiddleware(testHandler)

	for _, path := range publicPaths {
		t.Run("Public path: "+path, func(t *testing.T) {
			req := httptest.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()

			middlewareHandler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status code %d for public path %s, got %d", http.StatusOK, path, w.Code)
			}

			if w.Body.String() != "OK" {
				t.Errorf("Expected response body 'OK' for public path %s, got %s", path, w.Body.String())
			}
		})
	}
}

func TestAuthMiddleware_ProtectedPaths_Unauthenticated(t *testing.T) {
	protectedPaths := []string{
		"/",
		"/dhcp",
		"/provision",
		"/tftp",
		"/osimages",
		"/syslinux",
		"/status",
		"/dhcp/servers",
		"/provision/load-file",
	}

	// Create a simple handler that returns 200 OK
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Wrap with auth middleware
	middlewareHandler := AuthMiddleware(testHandler)

	for _, path := range protectedPaths {
		t.Run("Protected path unauthenticated: "+path, func(t *testing.T) {
			req := httptest.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()

			middlewareHandler.ServeHTTP(w, req)

			if w.Code != http.StatusFound {
				t.Errorf("Expected status code %d for protected path %s, got %d", http.StatusFound, path, w.Code)
			}

			location := w.Header().Get("Location")
			if location != "/login" {
				t.Errorf("Expected redirect to /login for protected path %s, got %s", path, location)
			}
		})
	}
}

func TestAuthMiddleware_ProtectedPaths_Authenticated(t *testing.T) {
	protectedPaths := []string{
		"/",
		"/dhcp",
		"/provision",
		"/tftp",
		"/osimages",
		"/syslinux",
		"/status",
	}

	// Create a simple handler that returns 200 OK
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Wrap with auth middleware
	middlewareHandler := AuthMiddleware(testHandler)

	for _, path := range protectedPaths {
		t.Run("Protected path authenticated: "+path, func(t *testing.T) {
			req := httptest.NewRequest("GET", path, nil)
			// Add valid session cookie
			req.AddCookie(&http.Cookie{
				Name:  "ignite_session",
				Value: "admin_12345",
			})
			w := httptest.NewRecorder()

			middlewareHandler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status code %d for authenticated protected path %s, got %d", http.StatusOK, path, w.Code)
			}

			if w.Body.String() != "OK" {
				t.Errorf("Expected response body 'OK' for authenticated protected path %s, got %s", path, w.Body.String())
			}
		})
	}
}

func TestAuthMiddleware_StaticFiles(t *testing.T) {
	staticPaths := []string{
		"/public/http/css/tailwind.css",
		"/public/http/css/provision.css",
		"/public/http/img/Ignite_small.png",
		"/public/http/js/app.js",
		"/public/provision/templates/kickstart/default.templ",
	}

	// Create a simple handler that returns 200 OK
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("static file content"))
	})

	// Wrap with auth middleware
	middlewareHandler := AuthMiddleware(testHandler)

	for _, path := range staticPaths {
		t.Run("Static file: "+path, func(t *testing.T) {
			req := httptest.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()

			middlewareHandler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status code %d for static file %s, got %d", http.StatusOK, path, w.Code)
			}

			if w.Body.String() != "static file content" {
				t.Errorf("Expected response body 'static file content' for static file %s, got %s", path, w.Body.String())
			}
		})
	}
}

func TestAuthMiddleware_EmptySessionCookie(t *testing.T) {
	// Create a simple handler that returns 200 OK
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Wrap with auth middleware
	middlewareHandler := AuthMiddleware(testHandler)

	req := httptest.NewRequest("GET", "/dhcp", nil)
	// Add empty session cookie
	req.AddCookie(&http.Cookie{
		Name:  "ignite_session",
		Value: "",
	})
	w := httptest.NewRecorder()

	middlewareHandler.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("Expected status code %d for empty session cookie, got %d", http.StatusFound, w.Code)
	}

	location := w.Header().Get("Location")
	if location != "/login" {
		t.Errorf("Expected redirect to /login for empty session cookie, got %s", location)
	}
}

func TestAuthMiddleware_POSTRequests(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		authenticated  bool
		expectedStatus int
	}{
		{
			name:           "POST to public auth endpoint",
			path:           "/auth/login",
			authenticated:  false,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "POST to protected endpoint unauthenticated",
			path:           "/dhcp/start",
			authenticated:  false,
			expectedStatus: http.StatusFound,
		},
		{
			name:           "POST to protected endpoint authenticated",
			path:           "/dhcp/start",
			authenticated:  true,
			expectedStatus: http.StatusOK,
		},
	}

	// Create a simple handler that returns 200 OK
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Wrap with auth middleware
	middlewareHandler := AuthMiddleware(testHandler)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", tt.path, nil)
			if tt.authenticated {
				req.AddCookie(&http.Cookie{
					Name:  "ignite_session",
					Value: "admin_12345",
				})
			}
			w := httptest.NewRecorder()

			middlewareHandler.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status code %d for %s, got %d", tt.expectedStatus, tt.name, w.Code)
			}

			if tt.expectedStatus == http.StatusFound {
				location := w.Header().Get("Location")
				if location != "/login" {
					t.Errorf("Expected redirect to /login for %s, got %s", tt.name, location)
				}
			}
		})
	}
}

func TestAuthMiddleware_ChainedMiddleware(t *testing.T) {
	// Create a test middleware that adds a header
	testMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Test-Middleware", "applied")
			next.ServeHTTP(w, r)
		})
	}

	// Create a simple handler that returns 200 OK
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Chain middlewares: testMiddleware -> AuthMiddleware -> testHandler
	chainedHandler := testMiddleware(AuthMiddleware(testHandler))

	t.Run("Chained middleware with authentication", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/dhcp", nil)
		req.AddCookie(&http.Cookie{
			Name:  "ignite_session",
			Value: "admin_12345",
		})
		w := httptest.NewRecorder()

		chainedHandler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
		}

		if w.Header().Get("X-Test-Middleware") != "applied" {
			t.Error("Expected test middleware to be applied")
		}

		if w.Body.String() != "OK" {
			t.Errorf("Expected response body 'OK', got %s", w.Body.String())
		}
	})

	t.Run("Chained middleware without authentication", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/dhcp", nil)
		w := httptest.NewRecorder()

		chainedHandler.ServeHTTP(w, req)

		if w.Code != http.StatusFound {
			t.Errorf("Expected status code %d, got %d", http.StatusFound, w.Code)
		}

		if w.Header().Get("X-Test-Middleware") != "applied" {
			t.Error("Expected test middleware to be applied even when redirecting")
		}

		location := w.Header().Get("Location")
		if location != "/login" {
			t.Errorf("Expected redirect to /login, got %s", location)
		}
	})
}
