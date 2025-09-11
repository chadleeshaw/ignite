package tftp

import (
	"net"
	"os"
	"testing"
	"time"
)

func TestNewServer(t *testing.T) {
	serveDir := "./testdata"
	server := NewServer(serveDir)
	if server == nil {
		t.Error("Expected server to be created, got nil")
	}
	// Test that server can be created without errors
	if err := os.MkdirAll(serveDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}
	defer os.RemoveAll(serveDir)
}

func TestServerStart(t *testing.T) {
	// Skip this test if not running as root (TFTP requires port 69)
	if os.Getuid() != 0 {
		t.Skip("Skipping TFTP server test - requires root privileges for port 69")
	}

	serveDir := "./testdata"
	if err := os.MkdirAll(serveDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}
	defer os.RemoveAll(serveDir)

	server := NewServer(serveDir)

	err := server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Test basic connectivity
	conn, err := net.DialTimeout("udp", "127.0.0.1:69", time.Second)
	if err != nil {
		t.Errorf("Failed to connect to server: %v", err)
	} else {
		conn.Close()
	}
}

func TestTFTPFileOperations(t *testing.T) {
	serveDir := "./testdata"
	if err := os.MkdirAll(serveDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}
	defer os.RemoveAll(serveDir)

	server := NewServer(serveDir)
	if server == nil {
		t.Error("Expected server to be created, got nil")
	}

	// Test file creation in serve directory
	testFile := serveDir + "/testfile.txt"
	err := os.WriteFile(testFile, []byte("test content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Verify file exists and can be read
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Errorf("Failed to read test file: %v", err)
	}
	if string(data) != "test content" {
		t.Errorf("Expected 'test content', but got '%s'", string(data))
	}
}

func TestTFTPServerCreation(t *testing.T) {
	serveDir := "./testdata"
	if err := os.MkdirAll(serveDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}
	defer os.RemoveAll(serveDir)

	server := NewServer(serveDir)
	if server == nil {
		t.Error("Expected server to be created, got nil")
	}

	// Test that server can be stopped without starting
	server.Stop() // Should not panic
}

// Removed mock types as we're now testing public interface only
