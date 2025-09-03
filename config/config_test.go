package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	// Test with default environment
	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.NotNil(t, cfg)

	// Validate default values
	assert.Equal(t, "./", cfg.DB.DBPath)
	assert.Equal(t, "ignite.db", cfg.DB.DBFile)
	assert.Equal(t, "dhcp", cfg.DB.Bucket)
	assert.Equal(t, "8080", cfg.HTTP.Port)
}

func TestLoadConfigWithEnvironment(t *testing.T) {
	// Save original environment
	originalPort := os.Getenv("HTTP_PORT")
	originalDBPath := os.Getenv("DB_PATH")
	
	defer func() {
		// Restore original environment
		if originalPort != "" {
			os.Setenv("HTTP_PORT", originalPort)
		} else {
			os.Unsetenv("HTTP_PORT")
		}
		if originalDBPath != "" {
			os.Setenv("DB_PATH", originalDBPath)
		} else {
			os.Unsetenv("DB_PATH")
		}
	}()

	// Set test environment variables
	os.Setenv("HTTP_PORT", "9090")
	os.Setenv("DB_PATH", "/tmp/test")

	cfg, err := LoadConfig()
	require.NoError(t, err)

	assert.Equal(t, "9090", cfg.HTTP.Port)
	assert.Equal(t, "/tmp/test", cfg.DB.DBPath)
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				DB: DBConfig{
					DBPath: "./",
					DBFile: "test.db",
					Bucket: "test",
				},
				DHCP: DHCPConfig{
					BiosFile: "bios.0",
					EFIFile:  "efi.0",
				},
				TFTP: TFTPConfig{
					Dir: "./tftp",
				},
				HTTP: HTTPConfig{
					Dir:  "./http",
					Port: "8080",
				},
				Provision: ProvisionConfig{
					Dir: "./provision",
				},
			},
			wantErr: false,
		},
		{
			name: "missing DB path",
			config: &Config{
				DB: DBConfig{
					DBPath: "",
					DBFile: "test.db",
					Bucket: "test",
				},
				HTTP: HTTPConfig{
					Port: "8080",
				},
				DHCP: DHCPConfig{
					BiosFile: "bios.0",
					EFIFile:  "efi.0",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid port",
			config: &Config{
				DB: DBConfig{
					DBPath: "./",
					DBFile: "test.db",
					Bucket: "test",
				},
				HTTP: HTTPConfig{
					Port: "invalid",
				},
				DHCP: DHCPConfig{
					BiosFile: "bios.0",
					EFIFile:  "efi.0",
				},
			},
			wantErr: true,
		},
		{
			name: "missing BIOS file",
			config: &Config{
				DB: DBConfig{
					DBPath: "./",
					DBFile: "test.db",
					Bucket: "test",
				},
				HTTP: HTTPConfig{
					Port: "8080",
				},
				DHCP: DHCPConfig{
					BiosFile: "",
					EFIFile:  "efi.0",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetDatabasePath(t *testing.T) {
	cfg := &Config{
		DB: DBConfig{
			DBPath: "/tmp/test",
			DBFile: "ignite.db",
		},
	}

	expected := filepath.Join("/tmp/test", "ignite.db")
	assert.Equal(t, expected, cfg.GetDatabasePath())
}

func TestGetPortInt(t *testing.T) {
	tests := []struct {
		name     string
		portStr  string
		expected int
		wantErr  bool
	}{
		{
			name:     "valid port",
			portStr:  "8080",
			expected: 8080,
			wantErr:  false,
		},
		{
			name:     "port 80",
			portStr:  "80",
			expected: 80,
			wantErr:  false,
		},
		{
			name:    "invalid port",
			portStr: "invalid",
			wantErr: true,
		},
		{
			name:    "empty port",
			portStr: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				HTTP: HTTPConfig{
					Port: tt.portStr,
				},
			}

			port, err := cfg.GetPortInt()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, port)
			}
		})
	}
}

func TestValidateDirectories(t *testing.T) {
	tempDir := t.TempDir()
	
	cfg := &Config{
		DB: DBConfig{
			DBPath: tempDir,
		},
		TFTP: TFTPConfig{
			Dir: filepath.Join(tempDir, "tftp"),
		},
		HTTP: HTTPConfig{
			Dir: filepath.Join(tempDir, "http"),
		},
		Provision: ProvisionConfig{
			Dir: filepath.Join(tempDir, "provision"),
		},
	}

	err := cfg.validateDirectories()
	assert.NoError(t, err)

	// Check that directories were created
	assert.DirExists(t, cfg.TFTP.Dir)
	assert.DirExists(t, cfg.HTTP.Dir)
	assert.DirExists(t, cfg.Provision.Dir)
}

func TestGetEnv(t *testing.T) {
	// Test with existing environment variable
	os.Setenv("TEST_VAR", "test_value")
	defer os.Unsetenv("TEST_VAR")

	result := getEnv("TEST_VAR", "default")
	assert.Equal(t, "test_value", result)

	// Test with non-existing environment variable
	result = getEnv("NON_EXISTING_VAR", "default")
	assert.Equal(t, "default", result)

	// Test with empty environment variable
	os.Setenv("EMPTY_VAR", "")
	defer os.Unsetenv("EMPTY_VAR")
	
	result = getEnv("EMPTY_VAR", "default")
	assert.Equal(t, "", result) // Empty string should be returned, not default
}
