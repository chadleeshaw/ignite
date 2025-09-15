package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PathSecurityValidator provides path validation and security checks
type PathSecurityValidator struct {
	allowedBasePaths []string
	maxPathLength    int
	maxFileSize      int64
}

// NewPathSecurityValidator creates a new path security validator
func NewPathSecurityValidator(allowedBasePaths []string) *PathSecurityValidator {
	return &PathSecurityValidator{
		allowedBasePaths: allowedBasePaths,
		maxPathLength:    4096,               // 4KB max path length
		maxFileSize:      1024 * 1024 * 1024, // 1GB max file size
	}
}

// SetMaxPathLength sets the maximum allowed path length
func (v *PathSecurityValidator) SetMaxPathLength(length int) *PathSecurityValidator {
	v.maxPathLength = length
	return v
}

// SetMaxFileSize sets the maximum allowed file size
func (v *PathSecurityValidator) SetMaxFileSize(size int64) *PathSecurityValidator {
	v.maxFileSize = size
	return v
}

// ValidatePath validates a file path for security issues
func (v *PathSecurityValidator) ValidatePath(path string) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}

	// Check path length
	if len(path) > v.maxPathLength {
		return fmt.Errorf("path too long (max %d characters): %d", v.maxPathLength, len(path))
	}

	// Clean the path to resolve any .. or . components
	cleanPath := filepath.Clean(path)

	// Check for path traversal attempts
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("path traversal detected: %s", path)
	}

	// Check for null bytes (can be used to bypass filters)
	if strings.Contains(path, "\x00") {
		return fmt.Errorf("null byte detected in path: %s", path)
	}

	// Check for suspicious characters
	suspiciousChars := []string{"|", "&", ";", "$", "`", "\\", "<", ">"}
	for _, char := range suspiciousChars {
		if strings.Contains(path, char) {
			return fmt.Errorf("suspicious character '%s' detected in path: %s", char, path)
		}
	}

	return nil
}

// ValidatePathWithinBase ensures the path is within one of the allowed base paths
func (v *PathSecurityValidator) ValidatePathWithinBase(path string) error {
	if err := v.ValidatePath(path); err != nil {
		return err
	}

	// Convert to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path: %v", err)
	}

	// Check if path is within any allowed base path
	for _, basePath := range v.allowedBasePaths {
		absBasePath, err := filepath.Abs(basePath)
		if err != nil {
			continue // Skip invalid base paths
		}

		// Check if the file path is within the base path
		relPath, err := filepath.Rel(absBasePath, absPath)
		if err == nil && !strings.HasPrefix(relPath, "..") {
			return nil // Path is within this base path
		}
	}

	return fmt.Errorf("path '%s' is not within any allowed base directory", path)
}

// ValidateFileName validates a filename for security issues
func (v *PathSecurityValidator) ValidateFileName(filename string) error {
	if filename == "" {
		return fmt.Errorf("filename cannot be empty")
	}

	// Check for reserved names (Windows)
	reservedNames := []string{
		"CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9",
	}

	upperFilename := strings.ToUpper(filename)
	for _, reserved := range reservedNames {
		if upperFilename == reserved || strings.HasPrefix(upperFilename, reserved+".") {
			return fmt.Errorf("filename uses reserved name: %s", filename)
		}
	}

	// Check for dangerous characters
	dangerousChars := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|", "\x00"}
	for _, char := range dangerousChars {
		if strings.Contains(filename, char) {
			return fmt.Errorf("filename contains dangerous character '%s': %s", char, filename)
		}
	}

	// Check for files starting with dot (hidden files)
	if strings.HasPrefix(filename, ".") {
		return fmt.Errorf("hidden files not allowed: %s", filename)
	}

	// Check filename length
	if len(filename) > 255 {
		return fmt.Errorf("filename too long (max 255 characters): %s", filename)
	}

	return nil
}

// ValidateFileSize validates file size
func (v *PathSecurityValidator) ValidateFileSize(filePath string) error {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("failed to get file info: %v", err)
	}

	if fileInfo.Size() > v.maxFileSize {
		return fmt.Errorf("file too large (max %d bytes): %d", v.maxFileSize, fileInfo.Size())
	}

	return nil
}

// ValidateFileType validates file type based on extension or known filenames
func (v *PathSecurityValidator) ValidateFileType(filename string, allowedExtensions []string) error {
	if len(allowedExtensions) == 0 {
		return nil // No restrictions
	}

	ext := strings.ToLower(filepath.Ext(filename))
	baseFilename := strings.ToLower(filepath.Base(filename))

	// Known extensionless files that should be allowed
	knownFiles := []string{"vmlinuz", "initrd", "kernel", "boot"}
	for _, known := range knownFiles {
		if baseFilename == known || strings.Contains(baseFilename, known) {
			return nil
		}
	}

	if ext == "" {
		return fmt.Errorf("file must have an extension or be a known system file")
	}

	for _, allowed := range allowedExtensions {
		if strings.ToLower(allowed) == ext {
			return nil
		}
	}

	return fmt.Errorf("file type '%s' not allowed (allowed: %v)", ext, allowedExtensions)
}

// SanitizePath sanitizes a path by removing dangerous components
func (v *PathSecurityValidator) SanitizePath(path string) string {
	// Clean the path
	cleanPath := filepath.Clean(path)

	// Remove any remaining .. components
	parts := strings.Split(cleanPath, string(filepath.Separator))
	var safeParts []string

	for _, part := range parts {
		if part != ".." && part != "." && part != "" {
			safeParts = append(safeParts, part)
		}
	}

	return strings.Join(safeParts, string(filepath.Separator))
}

// TFTPSecurityValidator provides TFTP-specific security validation
type TFTPSecurityValidator struct {
	pathValidator *PathSecurityValidator
	allowedTypes  []string
}

// NewTFTPSecurityValidator creates a new TFTP security validator
func NewTFTPSecurityValidator(tftpBasePath string) *TFTPSecurityValidator {
	return &TFTPSecurityValidator{
		pathValidator: NewPathSecurityValidator([]string{tftpBasePath}),
		allowedTypes: []string{
			".img", ".iso", ".bin", ".pxe", ".efi", ".cfg", ".conf",
			".txt", ".sh", ".tar", ".gz", ".zip", ".deb", ".rpm",
			".vmlinuz", ".initrd", ".kernel", ".boot",
		},
	}
}

// ValidateTFTPPath validates a TFTP file path
func (v *TFTPSecurityValidator) ValidateTFTPPath(path string) error {
	// Validate basic path security
	if err := v.pathValidator.ValidatePathWithinBase(path); err != nil {
		return err
	}

	// Extract filename
	filename := filepath.Base(path)
	if err := v.pathValidator.ValidateFileName(filename); err != nil {
		return err
	}

	// Validate file type
	if err := v.pathValidator.ValidateFileType(filename, v.allowedTypes); err != nil {
		return err
	}

	return nil
}

// ValidateTFTPUpload validates a file upload for TFTP
func (v *TFTPSecurityValidator) ValidateTFTPUpload(filename string, size int64) error {
	// Validate filename
	if err := v.pathValidator.ValidateFileName(filename); err != nil {
		return err
	}

	// Validate file type
	if err := v.pathValidator.ValidateFileType(filename, v.allowedTypes); err != nil {
		return err
	}

	// Check file size
	if size > v.pathValidator.maxFileSize {
		return fmt.Errorf("file too large (max %d bytes): %d", v.pathValidator.maxFileSize, size)
	}

	return nil
}

// GetSafePath returns a safe path within the TFTP directory
func (v *TFTPSecurityValidator) GetSafePath(basePath, relativePath string) (string, error) {
	// First validate the relative path
	if err := v.pathValidator.ValidatePath(relativePath); err != nil {
		return "", err
	}

	// Validate filename
	filename := filepath.Base(relativePath)
	if err := v.pathValidator.ValidateFileName(filename); err != nil {
		return "", err
	}

	// Sanitize the relative path
	safePath := v.pathValidator.SanitizePath(relativePath)

	// Combine with base path
	fullPath := filepath.Join(basePath, safePath)

	// Validate the final path
	if err := v.pathValidator.ValidatePathWithinBase(fullPath); err != nil {
		return "", err
	}

	return fullPath, nil
}

// SecurityConfig holds security configuration
type SecurityConfig struct {
	MaxFileSize      int64    `json:"max_file_size"`
	MaxPathLength    int      `json:"max_path_length"`
	AllowedTFTPTypes []string `json:"allowed_tftp_types"`
	TFTPBasePath     string   `json:"tftp_base_path"`
}

// GetDefaultSecurityConfig returns default security configuration
func GetDefaultSecurityConfig() *SecurityConfig {
	return &SecurityConfig{
		MaxFileSize:   1024 * 1024 * 1024, // 1GB
		MaxPathLength: 4096,               // 4KB
		AllowedTFTPTypes: []string{
			".img", ".iso", ".bin", ".pxe", ".efi", ".cfg", ".conf",
			".txt", ".sh", ".tar", ".gz", ".zip", ".deb", ".rpm",
			".vmlinuz", ".initrd", ".kernel", ".boot",
		},
		TFTPBasePath: "/var/lib/ignite/tftp",
	}
}
