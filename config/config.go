package config

import (
	"os"
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

// Defaults holds the default configuration values which can be overridden by environment variables
var Defaults = Config{
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

// getEnv returns the value of the environment variable key if it exists, otherwise it returns the fallback value
func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
