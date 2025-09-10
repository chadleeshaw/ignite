package config

import (
	"fmt"
	"os"
)

// Config represents the application configuration with immutable design
type Config struct {
	DB        DBConfig
	DHCP      DHCPConfig
	TFTP      TFTPConfig
	HTTP      HTTPConfig
	Provision ProvisionConfig
}

type DBConfig struct {
	DBPath string
	DBFile string
	Bucket string
}

type DHCPConfig struct {
	BiosFile string
	EFIFile  string
}

type TFTPConfig struct {
	Dir string
}

type HTTPConfig struct {
	Dir  string
	Port string
}

type ProvisionConfig struct {
	Dir string
}

// ConfigBuilder provides a builder pattern for configuration
type ConfigBuilder struct {
	config Config
}

// NewConfigBuilder creates a new configuration builder with defaults
func NewConfigBuilder() *ConfigBuilder {
	return &ConfigBuilder{
		config: Config{
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
		},
	}
}

// WithDBPath sets the database path
func (cb *ConfigBuilder) WithDBPath(path string) *ConfigBuilder {
	cb.config.DB.DBPath = path
	return cb
}

// WithDBFile sets the database file name
func (cb *ConfigBuilder) WithDBFile(file string) *ConfigBuilder {
	cb.config.DB.DBFile = file
	return cb
}

// WithBucket sets the database bucket name
func (cb *ConfigBuilder) WithBucket(bucket string) *ConfigBuilder {
	cb.config.DB.Bucket = bucket
	return cb
}

// Build creates the final configuration
func (cb *ConfigBuilder) Build() (*Config, error) {
	if err := cb.validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Return a copy to maintain immutability
	config := cb.config
	return &config, nil
}

// validate ensures the configuration is valid
func (cb *ConfigBuilder) validate() error {
	if cb.config.DB.DBPath == "" {
		return fmt.Errorf("database path cannot be empty")
	}
	if cb.config.DB.DBFile == "" {
		return fmt.Errorf("database file cannot be empty")
	}
	if cb.config.DB.Bucket == "" {
		return fmt.Errorf("database bucket cannot be empty")
	}
	return nil
}

// LoadDefault creates a configuration with default values
func LoadDefault() (*Config, error) {
	return NewConfigBuilder().Build()
}

// getEnv returns the value of the environment variable key if it exists, otherwise it returns the fallback value
func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
