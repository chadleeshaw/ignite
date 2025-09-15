package handlers

import (
	"bytes"
	"ignite/config"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestProvisionHandlers_DeleteFile_Success(t *testing.T) {
	// Create temporary directories
	tempDir := t.TempDir()
	provisionDir := filepath.Join(tempDir, "provision")
	templatesDir := filepath.Join(provisionDir, "templates", "cloud-init")

	if err := os.MkdirAll(templatesDir, 0755); err != nil {
		t.Fatalf("Failed to create test directories: %v", err)
	}

	// Create a test file
	testFile := filepath.Join(templatesDir, "test.yml")
	testContent := "test: content"
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Setup handler
	cfg := &config.Config{
		Provision: config.ProvisionConfig{
			Dir: provisionDir,
		},
	}
	container := &Container{Config: cfg}
	handler := NewProvisionHandlers(container)

	// Create request
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	if err := writer.WriteField("path", testFile); err != nil {
		t.Fatalf("Failed to write form field: %v", err)
	}
	writer.Close()

	req := httptest.NewRequest("POST", "/provision/delete-file", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	// Execute request
	handler.DeleteFile(w, req)

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}

	// Check that file was deleted
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Error("Expected file to be deleted")
	}
}

func TestProvisionHandlers_DeleteFile_SecurityCheck(t *testing.T) {
	// Create temporary directories
	tempDir := t.TempDir()
	provisionDir := filepath.Join(tempDir, "provision")

	if err := os.MkdirAll(provisionDir, 0755); err != nil {
		t.Fatalf("Failed to create test directories: %v", err)
	}

	// Create a file outside the provision directory
	outsideFile := filepath.Join(tempDir, "outside.txt")
	if err := os.WriteFile(outsideFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Setup handler
	cfg := &config.Config{
		Provision: config.ProvisionConfig{
			Dir: provisionDir,
		},
	}
	container := &Container{Config: cfg}
	handler := NewProvisionHandlers(container)

	// Create request to delete file outside provision directory
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	if err := writer.WriteField("path", outsideFile); err != nil {
		t.Fatalf("Failed to write form field: %v", err)
	}
	writer.Close()

	req := httptest.NewRequest("POST", "/provision/delete-file", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	// Execute request
	handler.DeleteFile(w, req)

	// Check response - should be forbidden
	if w.Code != http.StatusForbidden {
		t.Errorf("Expected status code %d, got %d", http.StatusForbidden, w.Code)
	}

	// Check that file was NOT deleted
	if _, err := os.Stat(outsideFile); os.IsNotExist(err) {
		t.Error("File should not have been deleted due to security check")
	}
}

func TestProvisionHandlers_DeleteFile_NonExistentFile(t *testing.T) {
	// Create temporary directories
	tempDir := t.TempDir()
	provisionDir := filepath.Join(tempDir, "provision")

	if err := os.MkdirAll(provisionDir, 0755); err != nil {
		t.Fatalf("Failed to create test directories: %v", err)
	}

	// Setup handler
	cfg := &config.Config{
		Provision: config.ProvisionConfig{
			Dir: provisionDir,
		},
	}
	container := &Container{Config: cfg}
	handler := NewProvisionHandlers(container)

	// Create request to delete non-existent file
	nonExistentFile := filepath.Join(provisionDir, "nonexistent.txt")
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	if err := writer.WriteField("path", nonExistentFile); err != nil {
		t.Fatalf("Failed to write form field: %v", err)
	}
	writer.Close()

	req := httptest.NewRequest("POST", "/provision/delete-file", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	// Execute request
	handler.DeleteFile(w, req)

	// Check response - should be not found
	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status code %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestProvisionHandlers_isPathAllowed(t *testing.T) {
	cfg := &config.Config{}
	container := &Container{Config: cfg}
	handler := NewProvisionHandlers(container)

	// Create temporary directories for testing
	tempDir := t.TempDir()
	allowedDir := filepath.Join(tempDir, "allowed")
	if err := os.MkdirAll(allowedDir, 0755); err != nil {
		t.Fatalf("Failed to create test directories: %v", err)
	}

	tests := []struct {
		name       string
		filePath   string
		allowedDir string
		expected   bool
	}{
		{
			name:       "File within allowed directory",
			filePath:   filepath.Join(allowedDir, "test.txt"),
			allowedDir: allowedDir,
			expected:   true,
		},
		{
			name:       "File in subdirectory of allowed directory",
			filePath:   filepath.Join(allowedDir, "subdir", "test.txt"),
			allowedDir: allowedDir,
			expected:   true,
		},
		{
			name:       "File outside allowed directory",
			filePath:   filepath.Join(tempDir, "outside.txt"),
			allowedDir: allowedDir,
			expected:   false,
		},
		{
			name:       "Path traversal attempt",
			filePath:   filepath.Join(allowedDir, "..", "outside.txt"),
			allowedDir: allowedDir,
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.isPathAllowed(tt.filePath, tt.allowedDir)
			if result != tt.expected {
				t.Errorf("isPathAllowed(%s, %s) = %v, expected %v", tt.filePath, tt.allowedDir, result, tt.expected)
			}
		})
	}
}
