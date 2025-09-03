package validation

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateMAC(t *testing.T) {
	tests := []struct {
		name    string
		mac     string
		wantErr bool
	}{
		{
			name:    "valid MAC with colons",
			mac:     "00:11:22:33:44:55",
			wantErr: false,
		},
		{
			name:    "valid MAC with hyphens",
			mac:     "00-11-22-33-44-55",
			wantErr: false,
		},
		{
			name:    "valid MAC with mixed case",
			mac:     "00:aB:cD:eF:12:34",
			wantErr: false,
		},
		{
			name:    "empty MAC",
			mac:     "",
			wantErr: true,
		},
		{
			name:    "invalid MAC format",
			mac:     "invalid-mac",
			wantErr: true,
		},
		{
			name:    "MAC too short",
			mac:     "00:11:22:33:44",
			wantErr: true,
		},
		{
			name:    "MAC too long",
			mac:     "00:11:22:33:44:55:66",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMAC(tt.mac)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateIP(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		wantErr bool
	}{
		{
			name:    "valid IPv4",
			ip:      "192.168.1.1",
			wantErr: false,
		},
		{
			name:    "valid IPv6",
			ip:      "2001:db8::1",
			wantErr: false,
		},
		{
			name:    "localhost IPv4",
			ip:      "127.0.0.1",
			wantErr: false,
		},
		{
			name:    "empty IP",
			ip:      "",
			wantErr: true,
		},
		{
			name:    "invalid IP format",
			ip:      "256.1.1.1",
			wantErr: true,
		},
		{
			name:    "not an IP",
			ip:      "not.an.ip",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIP(tt.ip)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateHostname(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		wantErr  bool
	}{
		{
			name:     "valid hostname",
			hostname: "example",
			wantErr:  false,
		},
		{
			name:     "valid hostname with numbers",
			hostname: "server1",
			wantErr:  false,
		},
		{
			name:     "valid hostname with hyphens",
			hostname: "my-server",
			wantErr:  false,
		},
		{
			name:     "empty hostname",
			hostname: "",
			wantErr:  true,
		},
		{
			name:     "hostname too long",
			hostname: "this-is-a-very-long-hostname-that-exceeds-the-maximum-allowed-length-of-sixty-three-characters",
			wantErr:  true,
		},
		{
			name:     "hostname with invalid characters",
			hostname: "server_1",
			wantErr:  true,
		},
		{
			name:     "hostname starting with hyphen",
			hostname: "-server",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHostname(tt.hostname)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateFilePath(t *testing.T) {
	tempDir := t.TempDir()
	
	tests := []struct {
		name     string
		basePath string
		userPath string
		wantErr  bool
	}{
		{
			name:     "valid relative path",
			basePath: tempDir,
			userPath: "file.txt",
			wantErr:  false,
		},
		{
			name:     "valid subdirectory path",
			basePath: tempDir,
			userPath: "subdir/file.txt",
			wantErr:  false,
		},
		{
			name:     "path traversal attempt",
			basePath: tempDir,
			userPath: "../../../etc/passwd",
			wantErr:  true,
		},
		{
			name:     "absolute path",
			basePath: tempDir,
			userPath: "/etc/passwd",
			wantErr:  true,
		},
		{
			name:     "empty path",
			basePath: tempDir,
			userPath: "",
			wantErr:  true,
		},
		{
			name:     "current directory reference",
			basePath: tempDir,
			userPath: "./file.txt",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidateFilePath(tt.basePath, tt.userPath)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Empty(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, result)
				// Ensure result is still within base directory
				assert.True(t, filepath.HasPrefix(result, tt.basePath))
			}
		})
	}
}

func TestValidateFilename(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		wantErr  bool
	}{
		{
			name:     "valid filename",
			filename: "document.txt",
			wantErr:  false,
		},
		{
			name:     "valid filename with numbers",
			filename: "file123.pdf",
			wantErr:  false,
		},
		{
			name:     "empty filename",
			filename: "",
			wantErr:  true,
		},
		{
			name:     "filename with path separator",
			filename: "dir/file.txt",
			wantErr:  true,
		},
		{
			name:     "filename with dangerous characters",
			filename: "file*.txt",
			wantErr:  true,
		},
		{
			name:     "filename with path traversal",
			filename: "../file.txt",
			wantErr:  true,
		},
		{
			name:     "very long filename",
			filename: string(make([]byte, 300)), // 300 characters
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFilename(tt.filename)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidatePort(t *testing.T) {
	tests := []struct {
		name    string
		port    string
		wantErr bool
	}{
		{
			name:    "valid port",
			port:    "8080",
			wantErr: false,
		},
		{
			name:    "port 80",
			port:    "80",
			wantErr: false,
		},
		{
			name:    "port 443",
			port:    "443",
			wantErr: false,
		},
		{
			name:    "empty port",
			port:    "",
			wantErr: true,
		},
		{
			name:    "invalid port format",
			port:    "abc",
			wantErr: true,
		},
		{
			name:    "port zero",
			port:    "0",
			wantErr: true,
		},
		{
			name:    "port too high",
			port:    "70000",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePort(tt.port)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateLeaseRange(t *testing.T) {
	tests := []struct {
		name       string
		leaseRange int
		wantErr    bool
	}{
		{
			name:       "valid lease range",
			leaseRange: 50,
			wantErr:    false,
		},
		{
			name:       "minimum lease range",
			leaseRange: 1,
			wantErr:    false,
		},
		{
			name:       "maximum lease range",
			leaseRange: 254,
			wantErr:    false,
		},
		{
			name:       "zero lease range",
			leaseRange: 0,
			wantErr:    true,
		},
		{
			name:       "negative lease range",
			leaseRange: -10,
			wantErr:    true,
		},
		{
			name:       "lease range too large",
			leaseRange: 300,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLeaseRange(tt.leaseRange)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateRequired(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		value   string
		wantErr bool
	}{
		{
			name:    "valid non-empty value",
			field:   "username",
			value:   "john",
			wantErr: false,
		},
		{
			name:    "empty string",
			field:   "username",
			value:   "",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			field:   "username",
			value:   "   ",
			wantErr: true,
		},
		{
			name:    "value with spaces",
			field:   "full_name",
			value:   "John Doe",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRequired(tt.field, tt.value)
			if tt.wantErr {
				assert.Error(t, err)
				// Check that error contains field name
				assert.Contains(t, err.Error(), tt.field)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
