package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config represents the application configuration with immutable design
type Config struct {
	DB        DBConfig
	DHCP      DHCPConfig
	TFTP      TFTPConfig
	HTTP      HTTPConfig
	Provision ProvisionConfig
	OSImages  OSImageConfig
	// ServerIP overrides the IP address advertised to network-boot clients.
	// Empty means auto-detect; can also be set via the SERVER_IP environment
	// variable, which takes precedence.
	ServerIP string
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

type OSImageConfig struct {
	Sources map[string]OSDefinition `json:"sources"`
}

type OSDefinition struct {
	DisplayName string               `json:"display_name"`
	KernelFile  string               `json:"kernel_file"`
	InitrdFile  string               `json:"initrd_file"`
	Versions    map[string]OSVersion `json:"versions"`
}

type OSVersion struct {
	DisplayName      string   `json:"display_name"`
	BaseURL          string   `json:"base_url"`
	Architectures    []string `json:"architectures"`
	ExpectedChecksum string   `json:"expected_checksum,omitempty"` // Optional SHA256 hex digest; when set, each downloaded file for this version must match it
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
			OSImages: getDefaultOSImageConfig(),
			ServerIP: getEnv("SERVER_IP", ""),
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

	// Return a copy to maintain immutability. The OS image catalog holds
	// nested maps and slices, so it needs a deep copy.
	config := cb.config
	config.OSImages = deepCopyOSImageConfig(cb.config.OSImages)
	return &config, nil
}

// deepCopyOSImageConfig deep-copies the OS image catalog: sources, nested
// version maps, and architecture slices.
func deepCopyOSImageConfig(src OSImageConfig) OSImageConfig {
	dst := OSImageConfig{
		Sources: make(map[string]OSDefinition, len(src.Sources)),
	}
	for osName, def := range src.Sources {
		defCopy := OSDefinition{
			DisplayName: def.DisplayName,
			KernelFile:  def.KernelFile,
			InitrdFile:  def.InitrdFile,
			Versions:    make(map[string]OSVersion, len(def.Versions)),
		}
		for ver, v := range def.Versions {
			vCopy := v
			if v.Architectures != nil {
				vCopy.Architectures = append([]string(nil), v.Architectures...)
			}
			defCopy.Versions[ver] = vCopy
		}
		dst.Sources[osName] = defCopy
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

// getDefaultOSImageConfig returns the default OS image configuration
func getDefaultOSImageConfig() OSImageConfig {
	return OSImageConfig{
		Sources: map[string]OSDefinition{
			"ubuntu": {
				DisplayName: "Ubuntu",
				KernelFile:  "vmlinuz",
				InitrdFile:  "initrd",
				Versions: map[string]OSVersion{
					"20.04": {
						DisplayName:   "20.04 LTS",
						BaseURL:       "https://github.com/netbootxyz/ubuntu-squash/releases/download/20.04.6-c92baa25/",
						Architectures: []string{"x86_64"},
					},
					"22.04": {
						DisplayName:   "22.04 LTS",
						BaseURL:       "https://github.com/netbootxyz/ubuntu-squash/releases/download/22.04.5-b0159fca/",
						Architectures: []string{"x86_64"},
					},
					"24.04": {
						DisplayName:   "24.04 LTS",
						BaseURL:       "https://github.com/netbootxyz/ubuntu-squash/releases/download/24.04.3-8efa196d/",
						Architectures: []string{"x86_64"},
					},
				},
			},
			"centos": {
				DisplayName: "CentOS",
				KernelFile:  "vmlinuz",
				InitrdFile:  "initrd.img",
				Versions: map[string]OSVersion{
					"9": {
						DisplayName:   "9 Stream",
						BaseURL:       "http://mirror.stream.centos.org/9-stream/BaseOS/x86_64/os/images/pxeboot/",
						Architectures: []string{"x86_64"},
					},
					"10": {
						DisplayName:   "10 Stream",
						BaseURL:       "http://mirror.stream.centos.org/10-stream/BaseOS/x86_64/os/images/pxeboot/",
						Architectures: []string{"x86_64"},
					},
				},
			},
			"nixos": {
				DisplayName: "NixOS",
				KernelFile:  "bzImage-x86_64-linux",
				InitrdFile:  "initrd-x86_64-linux",
				Versions: map[string]OSVersion{
					"23.05": {
						DisplayName:   "23.05",
						BaseURL:       "https://github.com/nix-community/nixos-images/releases/download/nixos-23.05/",
						Architectures: []string{"x86_64"},
					},
					"23.11": {
						DisplayName:   "23.11",
						BaseURL:       "https://github.com/nix-community/nixos-images/releases/download/nixos-23.11/",
						Architectures: []string{"x86_64"},
					},
					"24.05": {
						DisplayName:   "24.05",
						BaseURL:       "https://github.com/nix-community/nixos-images/releases/download/nixos-24.05/",
						Architectures: []string{"x86_64"},
					},
				},
			},
			"debian": {
				DisplayName: "Debian",
				KernelFile:  "linux",
				InitrdFile:  "initrd.gz",
				Versions: map[string]OSVersion{
					"11": {
						DisplayName:   "11 (Bullseye)",
						BaseURL:       "http://deb.debian.org/debian/dists/bullseye/main/installer-amd64/current/images/netboot/debian-installer/amd64/",
						Architectures: []string{"x86_64"},
					},
					"12": {
						DisplayName:   "12 (Bookworm)",
						BaseURL:       "http://deb.debian.org/debian/dists/bookworm/main/installer-amd64/current/images/netboot/debian-installer/amd64/",
						Architectures: []string{"x86_64"},
					},
				},
			},
			"opensuse": {
				DisplayName: "openSUSE",
				KernelFile:  "linux",
				InitrdFile:  "initrd",
				Versions: map[string]OSVersion{
					"15.5": {
						DisplayName:   "Leap 15.5",
						BaseURL:       "http://download.opensuse.org/distribution/leap/15.5/repo/oss/boot/x86_64/loader/",
						Architectures: []string{"x86_64"},
					},
					"15.6": {
						DisplayName:   "Leap 15.6",
						BaseURL:       "http://download.opensuse.org/distribution/leap/15.6/repo/oss/boot/x86_64/loader/",
						Architectures: []string{"x86_64"},
					},
					"tumbleweed": {
						DisplayName:   "Tumbleweed (Rolling)",
						BaseURL:       "http://download.opensuse.org/tumbleweed/repo/oss/boot/x86_64/loader/",
						Architectures: []string{"x86_64"},
					},
				},
			},
			"fedora": {
				DisplayName: "Fedora",
				KernelFile:  "vmlinuz",
				InitrdFile:  "initrd.img",
				Versions: map[string]OSVersion{
					"39": {
						DisplayName:   "39",
						BaseURL:       "https://download.fedoraproject.org/pub/fedora/linux/releases/39/Everything/x86_64/os/images/pxeboot/",
						Architectures: []string{"x86_64"},
					},
					"40": {
						DisplayName:   "40",
						BaseURL:       "https://download.fedoraproject.org/pub/fedora/linux/releases/40/Everything/x86_64/os/images/pxeboot/",
						Architectures: []string{"x86_64"},
					},
					"41": {
						DisplayName:   "41",
						BaseURL:       "https://download.fedoraproject.org/pub/fedora/linux/releases/41/Everything/x86_64/os/images/pxeboot/",
						Architectures: []string{"x86_64"},
					},
				},
			},
		},
	}
}
