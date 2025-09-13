package routes

import (
	"embed"
	"net/http"
	"net/http/httptest"
	"testing"

	"ignite/handlers"
	"ignite/config"

	"github.com/stretchr/testify/assert"
)

var testFS embed.FS

// Helper function to create a test container
func createTestContainer() *handlers.Container {
	return &handlers.Container{
		Config: &config.Config{
			TFTP: config.TFTPConfig{
				Dir: "/tmp/test-tftp",
			},
			HTTP: config.HTTPConfig{
				Dir:  "/tmp/test-http",
				Port: "8080",
			},
		},
	}
}

// Helper function to create a test static handler
func createTestStaticHandler() *handlers.StaticHandlers {
	return handlers.NewStaticHandlers(testFS, "/tmp/test-http")
}

// Test SetupWithContainerAndStatic creates router with all routes
func TestSetupWithContainerAndStatic(t *testing.T) {
	container := createTestContainer()
	staticHandler := createTestStaticHandler()

	router := SetupWithContainerAndStatic(container, staticHandler)

	assert.NotNil(t, router)
}

// Test that all handlers are properly instantiated
func TestHandlersInstantiation(t *testing.T) {
	container := createTestContainer()
	staticHandler := createTestStaticHandler()

	// This should not panic
	router := SetupWithContainerAndStatic(container, staticHandler)
	assert.NotNil(t, router)

	// Test that we can handle a 404 request without panicking
	req := httptest.NewRequest("GET", "/nonexistent", nil)
	w := httptest.NewRecorder()

	assert.NotPanics(t, func() {
		router.ServeHTTP(w, req)
	})
	
	// Should return 404 for non-existent route
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// Test that router creation doesn't panic
func TestRouterCreation(t *testing.T) {
	container := createTestContainer()
	staticHandler := createTestStaticHandler()

	// Test that creating the router with all dependencies works
	assert.NotPanics(t, func() {
		router := SetupWithContainerAndStatic(container, staticHandler)
		assert.NotNil(t, router)
	})
}