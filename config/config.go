package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"ignite/internal/errors"
	"ignite/internal/validation"
)

type Config struct {
	DB        DBConfig
	DHCP      DHCPConfig
	TFTP      TFTPConfig
	HTTP      HTTPConfig
	Provision ProvisionConfig
}

type DBConfig struct {
	DBPath string // Path to store db file
	DBFile string // Name of database file
	Bucket string // Database name
}

type DHCPConfig struct {
	BiosFile string // Path to bios file
	EFIFile  string // Path to efi file
}

type TFTPConfig struct {
	Dir string // Directory to serve files from
}

type HTTPConfig struct {
	Dir  string // Directory to serve http
	Port string // Port to listen on
}

type ProvisionConfig struct {
	Dir string // Directory for provision scripts
}

// LoadConfig loads and validates configuration from environment variables
func LoadConfig() (*Config, error) {
	cfg := &Config{
		DB: DBConfig{
			DBPath: getEnv("DB_PATH", "./"),
			DBFile: getEnv("DB_FILE", "ignite.db"),
			Bucket: getEnv("DB_BUCKET", "dhcp"),
		},
		DHCP: DHCPConfig{
			BiosFile: getEnv("BIOS_FILE", "boot-bios/pxelinux.0"),
			EFIFile:  getEnv("EFI_FILE", "boot-efi/syslinux.efi"),
		},
		TFTP: TFTPConfig{
			Dir: getEnv("TFTP_DIR", "./public/tftp"),
		},
		HTTP: HTTPConfig{
			Dir:  getEnv("HTTP_DIR", "./public/http"),
			Port: getEnv("HTTP_PORT", "8080"),
		},
		Provision: ProvisionConfig{
			Dir: getEnv("PROV_DIR", "./public/provision"),
		},
	}

	if err := cfg.Validate(); err != nil {
		return nil, errors.NewConfigurationError("validate_config", err)
	}

	return cfg, nil
}

// Defaults holds the default configuration values for backward compatibility
var Defaults Config

// Validate validates the configuration values
func (c *Config) Validate() error {
	// Validate database configuration
	if err := validation.ValidateRequired("db_path", c.DB.DBPath); err != nil {
		return err
	}
	if err := validation.ValidateRequired("db_file", c.DB.DBFile); err != nil {
		return err
	}
	if err := validation.ValidateRequired("db_bucket", c.DB.Bucket); err != nil {
		return err
	}

	// Validate port
	if err := validation.ValidatePort(c.HTTP.Port); err != nil {
		return err
	}

	// Validate directories exist or can be created
	if err := c.validateDirectories(); err != nil {
		return err
	}

	// Validate file paths
	if err := validation.ValidateRequired("bios_file", c.DHCP.BiosFile); err != nil {
		return err
	}
	if err := validation.ValidateRequired("efi_file", c.DHCP.EFIFile); err != nil {
		return err
	}

	return nil
}

// validateDirectories ensures all required directories exist or can be created
func (c *Config) validateDirectories() error {
	dirs := []struct {
		path string
		name string
	}{
		{c.DB.DBPath, "database directory"},
		{c.TFTP.Dir, "TFTP directory"},
		{c.HTTP.Dir, "HTTP directory"},
		{c.Provision.Dir, "provision directory"},
	}

	for _, dir := range dirs {
		if _, err := os.Stat(dir.path); os.IsNotExist(err) {
			if err := os.MkdirAll(dir.path, 0755); err != nil {
				return fmt.Errorf("failed to create %s (%s): %w", dir.name, dir.path, err)
			}
		}
	}

	return nil
}

// GetDatabasePath returns the full path to the database file
func (c *Config) GetDatabasePath() string {
	return filepath.Join(c.DB.DBPath, c.DB.DBFile)
}

// GetPortInt returns the HTTP port as an integer
func (c *Config) GetPortInt() (int, error) {
	port, err := strconv.Atoi(c.HTTP.Port)
	if err != nil {
		return 0, fmt.Errorf("invalid port number: %w", err)
	}
	return port, nil
}

// getEnv returns the value of the environment variable key if it exists, otherwise it returns the fallback value
func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

// init initializes the Defaults configuration for backward compatibility
func init() {
	cfg, err := LoadConfig()
	if err != nil {
		// If configuration loading fails, use basic defaults
		Defaults = Config{
			DB: DBConfig{
				DBPath: "./",
				DBFile: "ignite.db",
				Bucket: "dhcp",
			},
			DHCP: DHCPConfig{
				BiosFile: "boot-bios/pxelinux.0",
				EFIFile:  "boot-efi/syslinux.efi",
			},
			TFTP: TFTPConfig{
				Dir: "./public/tftp",
			},
			HTTP: HTTPConfig{
				Dir:  "./public/http",
				Port: "8080",
			},
			Provision: ProvisionConfig{
				Dir: "./public/provision",
			},
		}
	} else {
		Defaults = *cfg
	}
}
