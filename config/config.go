package config

import (
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Config represents the application configuration with immutable design
type Config struct {
	DB        DBConfig
	DHCP      DHCPConfig
	TFTP      TFTPConfig
	HTTP      HTTPConfig
	Provision ProvisionConfig
	Syslinux  Syslinux
	OSImages  OSImageConfig
	// OSCatalog controls discovery of bootable OS releases.
	OSCatalog OSCatalogConfig
	// ServerIP overrides the IP address advertised to network-boot clients.
	// Empty means auto-detect; can also be set via the SERVER_IP environment
	// variable, which takes precedence.
	ServerIP string
	// BMC controls Redfish connections to baseboard management controllers.
	BMC BMCConfig
}

// BMCConfig controls how Ignite talks to a Redfish BMC.
// Certificate checks are off unless TLSVerify is set. Homelab BMCs are
// usually self-signed.
type BMCConfig struct {
	// TLSVerify is BMC_TLS_VERIFY. False, the default, skips certificate
	// verification. True checks the BMC certificate.
	TLSVerify bool
	// SystemID is the Redfish ComputerSystem Id used when a BMC exposes more
	// than one system. Empty means use the only system. A request can override
	// this with the system_id form field. Env: BMC_SYSTEM_ID.
	SystemID string
}

type DBConfig struct {
	DBPath string
	DBFile string
	Bucket string
}

type DHCPConfig struct {
	BiosFile string
	EFIFile  string
	// Enabled is DHCP_ENABLED. False means this process does not bind a
	// DHCP listener. Unset stays enabled. Only 0, false, no, and off disable it.
	Enabled bool
}

type TFTPConfig struct {
	Dir  string
	Port string
}

type HTTPConfig struct {
	Dir  string
	Port string
}

type ProvisionConfig struct {
	Dir string
}

// Syslinux is the download-cache location.
type Syslinux struct {
	// CacheDir is SYSLINUX_CACHE_DIR. Empty means a syslinux directory under DB_PATH.
	CacheDir string
}

type OSImageConfig struct {
	Sources map[string]OSDefinition `json:"sources"`
}

// OSCatalogConfig is how often the release catalog refreshes, whether it
// stays on the bundled list, and the optional GitHub token for NixOS.
type OSCatalogConfig struct {
	// Refresh is OS_CATALOG_REFRESH. Invalid or non-positive values become 24h.
	Refresh time.Duration
	// Offline is OS_CATALOG_OFFLINE. True never calls release sources.
	Offline bool
	// GitHubToken is GITHUB_TOKEN. It is sent only to the NixOS releases API.
	// It is never logged.
	GitHubToken string
}

// OSDefinition is the per-OS boot facts that do not change between releases.
// Versions, download URLs, and end-of-life dates live in osimage/catalog.
// Kernel arguments stay in the iPXE service because they include the server address.
type OSDefinition struct {
	DisplayName string `json:"display_name"`
	KernelFile  string `json:"kernel_file"`
	InitrdFile  string `json:"initrd_file"`
}

// SupportedNames returns each source's display name in case-insensitive order.
// The OS Images page prints this list, so the help text stays tied to the catalog.
func (c OSImageConfig) SupportedNames() []string {
	names := make([]string, 0, len(c.Sources))
	for _, def := range c.Sources {
		names = append(names, def.DisplayName)
	}
	sort.Slice(names, func(i, j int) bool {
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})
	return names
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
				Enabled:  dhcpEnabled(),
			},
			TFTP: TFTPConfig{
				Dir:  getEnv("TFTP_DIR", "./public/tftp"),
				Port: getEnv("TFTP_PORT", "69"),
			},
			HTTP: HTTPConfig{
				Dir:  getEnv("HTTP_DIR", "./public/http"),
				Port: getEnv("HTTP_PORT", "8080"),
			},
			Provision: ProvisionConfig{
				Dir: getEnv("PROV_DIR", "./public/provision"),
			},
			Syslinux: Syslinux{
				CacheDir: getEnv("SYSLINUX_CACHE_DIR", ""),
			},
			OSImages: getDefaultOSImageConfig(),
			OSCatalog: OSCatalogConfig{
				Refresh:     catalogRefreshInterval(),
				Offline:     catalogOffline(),
				GitHubToken: getEnv("GITHUB_TOKEN", ""),
			},
			ServerIP: getEnv("SERVER_IP", ""),
			BMC: BMCConfig{
				TLSVerify: bmcTLSVerify(),
				SystemID:  getEnv("BMC_SYSTEM_ID", ""),
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

	// Return a copy to maintain immutability. The OS map is copied so a caller
	// cannot replace a definition in the builder's catalog.
	config := cb.config
	config.OSImages = deepCopyOSImageConfig(cb.config.OSImages)
	return &config, nil
}

// deepCopyOSImageConfig copies the OS map. Definitions are plain strings.
func deepCopyOSImageConfig(src OSImageConfig) OSImageConfig {
	dst := OSImageConfig{
		Sources: make(map[string]OSDefinition, len(src.Sources)),
	}
	for osName, def := range src.Sources {
		dst.Sources[osName] = def
	}
	return dst
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
	if port, err := strconv.Atoi(cb.config.HTTP.Port); err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("HTTP port must be a number between 1 and 65535, got %q", cb.config.HTTP.Port)
	}
	if port, err := strconv.Atoi(cb.config.TFTP.Port); err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("TFTP port must be a number between 1 and 65535, got %q", cb.config.TFTP.Port)
	}
	if cb.config.TFTP.Dir == "" {
		return fmt.Errorf("TFTP directory cannot be empty")
	}
	if cb.config.HTTP.Dir == "" {
		return fmt.Errorf("HTTP directory cannot be empty")
	}
	if cb.config.Provision.Dir == "" {
		return fmt.Errorf("provision directory cannot be empty")
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

// dhcpEnabled reports whether this process should bind DHCP listeners.
// Unset, and any value other than 0, false, no, or off, leaves DHCP enabled
// so a typo cannot turn the listeners off.
func dhcpEnabled() bool {
	value, ok := os.LookupEnv("DHCP_ENABLED")
	if !ok {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

// bmcTLSVerify reports whether Redfish should check BMC certificates.
// Unset stays false, so a self-signed BMC connects. Parsing uses
// strconv.ParseBool. An invalid value logs a warning and stays false.
func bmcTLSVerify() bool {
	value, ok := os.LookupEnv("BMC_TLS_VERIFY")
	if !ok {
		return false
	}
	verify, err := strconv.ParseBool(value)
	if err != nil {
		log.Printf("BMC_TLS_VERIFY=%q is not a boolean; TLS verification stays disabled", value)
		return false
	}
	return verify
}

// BMCTLSModeLog is the single startup line for the effective Redfish TLS mode.
func BMCTLSModeLog(verify bool) string {
	if verify {
		return "BMC TLS verification: enabled"
	}
	return "BMC TLS verification: disabled (set BMC_TLS_VERIFY=true to enable)"
}

// catalogRefreshInterval reads OS_CATALOG_REFRESH. Empty, invalid, and
// non-positive values use 24h.
func catalogRefreshInterval() time.Duration {
	const fallback = 24 * time.Hour
	raw, ok := os.LookupEnv("OS_CATALOG_REFRESH")
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback
	}
	interval, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || interval <= 0 {
		log.Printf("OS_CATALOG_REFRESH=%q is not a positive duration; using 24h", raw)
		return fallback
	}
	return interval
}

// catalogOffline reads OS_CATALOG_OFFLINE with strconv.ParseBool.
// Unset stays online. An invalid value logs a warning and stays online.
func catalogOffline() bool {
	raw, ok := os.LookupEnv("OS_CATALOG_OFFLINE")
	if !ok || strings.TrimSpace(raw) == "" {
		return false
	}
	offline, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		log.Printf("OS_CATALOG_OFFLINE=%q is not a boolean; catalog refresh stays enabled", raw)
		return false
	}
	return offline
}

// getDefaultOSImageConfig returns per-OS boot file names. Release URLs live
// in the dynamic catalog; these names are the upstream files that catalog
// was generated from.
func getDefaultOSImageConfig() OSImageConfig {
	return OSImageConfig{
		Sources: map[string]OSDefinition{
			"ubuntu": {
				DisplayName: "Ubuntu",
				KernelFile:  "vmlinuz",
				InitrdFile:  "initrd",
			},
			"centos": {
				DisplayName: "CentOS",
				KernelFile:  "vmlinuz",
				InitrdFile:  "initrd.img",
			},
			"nixos": {
				DisplayName: "NixOS",
				KernelFile:  "bzImage-x86_64-linux",
				InitrdFile:  "initrd-x86_64-linux",
			},
			"debian": {
				DisplayName: "Debian",
				KernelFile:  "linux",
				InitrdFile:  "initrd.gz",
			},
			"opensuse": {
				DisplayName: "openSUSE",
				KernelFile:  "linux",
				InitrdFile:  "initrd",
			},
			"fedora": {
				DisplayName: "Fedora",
				KernelFile:  "vmlinuz",
				InitrdFile:  "initrd.img",
			},
		},
	}
}
