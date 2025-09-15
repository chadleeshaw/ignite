package handlers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPathSecurityValidator_ValidatePath(t *testing.T) {
	validator := NewPathSecurityValidator([]string{"/tmp"})

	tests := []struct {
		name      string
		path      string
		expectErr bool
	}{
		{"Valid path", "test.txt", false},
		{"Valid nested path", "dir/test.txt", false},
		{"Path traversal", "../etc/passwd", true},
		{"Path traversal nested", "dir/../../etc/passwd", true},
		{"Null byte", "test\x00.txt", true},
		{"Pipe character", "test|cmd.txt", true},
		{"Ampersand", "test&cmd.txt", true},
		{"Semicolon", "test;cmd.txt", true},
		{"Dollar sign", "test$var.txt", true},
		{"Backtick", "test`cmd`.txt", true},
		{"Backslash", "test\\cmd.txt", true},
		{"Less than", "test<input.txt", true},
		{"Greater than", "test>output.txt", true},
		{"Empty path", "", true},
		{"Very long path", string(make([]byte, 5000)), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidatePath(tt.path)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidatePath(%s) error = %v, expectErr %v", tt.path, err, tt.expectErr)
			}
		})
	}
}

func TestPathSecurityValidator_ValidateFileName(t *testing.T) {
	validator := NewPathSecurityValidator([]string{"/tmp"})

	tests := []struct {
		name      string
		filename  string
		expectErr bool
	}{
		{"Valid filename", "test.txt", false},
		{"Valid with numbers", "test123.txt", false},
		{"Valid with underscore", "test_file.txt", false},
		{"Valid with hyphen", "test-file.txt", false},
		{"Reserved name CON", "CON", true},
		{"Reserved name with extension", "CON.txt", true},
		{"Reserved name lowercase", "con", true},
		{"Path separator slash", "test/file.txt", true},
		{"Path separator backslash", "test\\file.txt", true},
		{"Colon", "test:file.txt", true},
		{"Asterisk", "test*.txt", true},
		{"Question mark", "test?.txt", true},
		{"Double quote", "test\".txt", true},
		{"Less than", "test<.txt", true},
		{"Greater than", "test>.txt", true},
		{"Pipe", "test|.txt", true},
		{"Null byte", "test\x00.txt", true},
		{"Hidden file", ".hidden", true},
		{"Empty filename", "", true},
		{"Very long filename", string(make([]byte, 300)), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateFileName(tt.filename)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateFileName(%s) error = %v, expectErr %v", tt.filename, err, tt.expectErr)
			}
		})
	}
}

func TestPathSecurityValidator_ValidateFileType(t *testing.T) {
	validator := NewPathSecurityValidator([]string{"/tmp"})

	allowedTypes := []string{".txt", ".log", ".cfg"}

	tests := []struct {
		name      string
		filename  string
		expectErr bool
	}{
		{"Allowed txt", "test.txt", false},
		{"Allowed log", "test.log", false},
		{"Allowed cfg", "test.cfg", false},
		{"Allowed uppercase", "test.TXT", false},
		{"Not allowed exe", "test.exe", true},
		{"Not allowed bin", "test.bin", true},
		{"No extension", "test", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateFileType(tt.filename, allowedTypes)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateFileType(%s) error = %v, expectErr %v", tt.filename, err, tt.expectErr)
			}
		})
	}
}

func TestPathSecurityValidator_SanitizePath(t *testing.T) {
	validator := NewPathSecurityValidator([]string{"/tmp"})

	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{"Clean path", "dir/file.txt", "dir/file.txt"},
		{"Path with dots", "dir/./file.txt", "dir/file.txt"},
		{"Path with traversal", "dir/../file.txt", "file.txt"},
		{"Multiple traversal", "dir/../../file.txt", "file.txt"},
		{"Leading slash", "/dir/file.txt", "dir/file.txt"},
		{"Trailing slash", "dir/file.txt/", "dir/file.txt"},
		{"Multiple slashes", "dir//file.txt", "dir/file.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.SanitizePath(tt.path)
			if result != tt.expected {
				t.Errorf("SanitizePath(%s) = %s, expected %s", tt.path, result, tt.expected)
			}
		})
	}
}

func TestTFTPSecurityValidator_ValidateTFTPUpload(t *testing.T) {
	tmpDir := t.TempDir()
	validator := NewTFTPSecurityValidator(tmpDir)

	tests := []struct {
		name      string
		filename  string
		size      int64
		expectErr bool
	}{
		{"Valid image file", "ubuntu.iso", 1024, false},
		{"Valid kernel", "vmlinuz", 1024, false},
		{"Valid config", "pxelinux.cfg", 1024, false},
		{"Invalid extension", "malware.exe", 1024, true},
		{"Hidden file", ".hidden.iso", 1024, true},
		{"Too large", "ubuntu.iso", 2 * 1024 * 1024 * 1024, true}, // 2GB
		{"Reserved name", "CON.iso", 1024, true},
		{"Path traversal", "../../../etc/passwd.iso", 1024, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateTFTPUpload(tt.filename, tt.size)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateTFTPUpload(%s, %d) error = %v, expectErr %v", tt.filename, tt.size, err, tt.expectErr)
			}
		})
	}
}

func TestTFTPSecurityValidator_GetSafePath(t *testing.T) {
	tmpDir := t.TempDir()
	validator := NewTFTPSecurityValidator(tmpDir)

	tests := []struct {
		name         string
		relativePath string
		expectErr    bool
	}{
		{"Valid relative path", "ubuntu.iso", false},
		{"Valid nested path", "images/ubuntu.iso", false},
		{"Path traversal attempt", "../../../etc/passwd", true},
		{"Mixed path traversal", "images/../../../etc/passwd", true},
		{"Hidden file", ".hidden", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			safePath, err := validator.GetSafePath(tmpDir, tt.relativePath)
			if (err != nil) != tt.expectErr {
				t.Errorf("GetSafePath(%s) error = %v, expectErr %v", tt.relativePath, err, tt.expectErr)
			}

			if err == nil {
				// Ensure the safe path is within the base directory
				relPath, err := filepath.Rel(tmpDir, safePath)
				if err != nil || filepath.IsAbs(relPath) || strings.HasPrefix(relPath, "..") {
					t.Errorf("GetSafePath(%s) returned unsafe path: %s", tt.relativePath, safePath)
				}
			}
		})
	}
}

func TestPathSecurityValidator_ValidatePathWithinBase(t *testing.T) {
	tmpDir := t.TempDir()
	validator := NewPathSecurityValidator([]string{tmpDir})

	// Create a test file within the base directory
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create a file outside the base directory
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "outside.txt")
	if err := os.WriteFile(outsideFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create outside file: %v", err)
	}

	tests := []struct {
		name      string
		path      string
		expectErr bool
	}{
		{"File within base", testFile, false},
		{"File outside base", outsideFile, true},
		{"Path traversal to outside", filepath.Join(tmpDir, "../outside.txt"), true},
		{"Non-existent file within base", filepath.Join(tmpDir, "nonexistent.txt"), false}, // Path validation, not existence
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidatePathWithinBase(tt.path)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidatePathWithinBase(%s) error = %v, expectErr %v", tt.path, err, tt.expectErr)
			}
		})
	}
}

func TestGetDefaultSecurityConfig(t *testing.T) {
	config := GetDefaultSecurityConfig()

	if config == nil {
		t.Fatal("GetDefaultSecurityConfig returned nil")
	}

	if config.MaxFileSize <= 0 {
		t.Error("MaxFileSize should be positive")
	}

	if config.MaxPathLength <= 0 {
		t.Error("MaxPathLength should be positive")
	}

	if len(config.AllowedTFTPTypes) == 0 {
		t.Error("AllowedTFTPTypes should not be empty")
	}

	if config.TFTPBasePath == "" {
		t.Error("TFTPBasePath should not be empty")
	}

	// Check that common TFTP file types are included
	expectedTypes := []string{".iso", ".img", ".bin", ".cfg"}
	for _, expectedType := range expectedTypes {
		found := false
		for _, allowedType := range config.AllowedTFTPTypes {
			if allowedType == expectedType {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected TFTP type %s not found in AllowedTFTPTypes", expectedType)
		}
	}
}
