package validation

import (
	"net"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	macAddressRegex = regexp.MustCompile(`^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$`)
	hostnameRegex   = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?$`)
)

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}

// NewValidationError creates a new validation error
func NewValidationError(field, message string) *ValidationError {
	return &ValidationError{
		Field:   field,
		Message: message,
	}
}

// ValidateMAC validates MAC address format
func ValidateMAC(mac string) error {
	if mac == "" {
		return NewValidationError("mac", "MAC address is required")
	}
	if !macAddressRegex.MatchString(mac) {
		return NewValidationError("mac", "invalid MAC address format")
	}
	return nil
}

// ValidateIP validates IP address format and ensures it's not nil
func ValidateIP(ip string) error {
	if ip == "" {
		return NewValidationError("ip", "IP address is required")
	}
	if net.ParseIP(ip) == nil {
		return NewValidationError("ip", "invalid IP address format")
	}
	return nil
}

// ValidateIPNet validates network CIDR format
func ValidateIPNet(cidr string) error {
	if cidr == "" {
		return NewValidationError("cidr", "CIDR is required")
	}
	_, _, err := net.ParseCIDR(cidr)
	if err != nil {
		return NewValidationError("cidr", "invalid CIDR format")
	}
	return nil
}

// ValidateHostname validates hostname format according to RFC standards
func ValidateHostname(hostname string) error {
	if hostname == "" {
		return NewValidationError("hostname", "hostname is required")
	}
	if len(hostname) > 63 {
		return NewValidationError("hostname", "hostname too long (max 63 characters)")
	}
	if !hostnameRegex.MatchString(hostname) {
		return NewValidationError("hostname", "invalid hostname format")
	}
	return nil
}

// ValidateFilePath validates file paths and prevents path traversal attacks
func ValidateFilePath(basePath, userPath string) (string, error) {
	if userPath == "" {
		return "", NewValidationError("path", "file path is required")
	}
	
	// Clean the user input to resolve any relative path components
	cleanPath := filepath.Clean(userPath)
	
	// Check for path traversal attempts
	if strings.Contains(cleanPath, "..") {
		return "", NewValidationError("path", "path traversal detected")
	}
	
	// Check for absolute paths that might escape the base directory
	if filepath.IsAbs(cleanPath) {
		return "", NewValidationError("path", "absolute paths not allowed")
	}
	
	// Join with base path
	fullPath := filepath.Join(basePath, cleanPath)
	
	// Ensure the result is still within the base path
	if !strings.HasPrefix(fullPath, basePath) {
		return "", NewValidationError("path", "path outside allowed directory")
	}
	
	return fullPath, nil
}

// ValidateFilename validates filename for safety
func ValidateFilename(filename string) error {
	if filename == "" {
		return NewValidationError("filename", "filename is required")
	}
	
	// Check for dangerous characters
	dangerous := []string{"/", "\\", "..", ":", "*", "?", "\"", "<", ">", "|"}
	for _, char := range dangerous {
		if strings.Contains(filename, char) {
			return NewValidationError("filename", "filename contains invalid characters")
		}
	}
	
	// Check length
	if len(filename) > 255 {
		return NewValidationError("filename", "filename too long (max 255 characters)")
	}
	
	return nil
}

// ValidatePort validates port number
func ValidatePort(port string) error {
	if port == "" {
		return NewValidationError("port", "port is required")
	}
	
	// Try to parse as number and validate range
	portNum, err := strconv.Atoi(port)
	if err != nil {
		return NewValidationError("port", "invalid port number")
	}
	
	if portNum <= 0 || portNum > 65535 {
		return NewValidationError("port", "port number out of range (1-65535)")
	}
	
	return nil
}

// ValidateLeaseRange validates DHCP lease range
func ValidateLeaseRange(leaseRange int) error {
	if leaseRange <= 0 {
		return NewValidationError("lease_range", "lease range must be positive")
	}
	if leaseRange > 254 {
		return NewValidationError("lease_range", "lease range too large (max 254)")
	}
	return nil
}

// ValidateRequired checks if a string field is not empty
func ValidateRequired(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return NewValidationError(field, field+" is required")
	}
	return nil
}

// ValidateMaxLength validates maximum string length
func ValidateMaxLength(field, value string, maxLen int) error {
	if len(value) > maxLen {
		return NewValidationError(field, field+" too long")
	}
	return nil
}

// ValidateMinLength validates minimum string length
func ValidateMinLength(field, value string, minLen int) error {
	if len(value) < minLen {
		return NewValidationError(field, field+" too short")
	}
	return nil
}

// ValidateFileExtension validates allowed file extensions
func ValidateFileExtension(filename string, allowedExts []string) error {
	ext := strings.ToLower(filepath.Ext(filename))
	for _, allowed := range allowedExts {
		if ext == strings.ToLower(allowed) {
			return nil
		}
	}
	return NewValidationError("filename", "file extension not allowed")
}
