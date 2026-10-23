package config

import (
	"bytes"
	"log"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadConfig_Defaults(t *testing.T) {
	// Test with no config file
	cfg, err := LoadDefault()
	assert.NoError(t, err)

	assert.Equal(t, "ignite.db", cfg.DB.DBFile)
	assert.Equal(t, "8080", cfg.HTTP.Port)
	assert.Equal(t, "./public/tftp", cfg.TFTP.Dir)
	assert.Equal(t, "./public/http", cfg.HTTP.Dir)
	assert.Equal(t, "dhcp", cfg.DB.Bucket)
}

func TestConfigBuilder(t *testing.T) {
	// Test building custom config
	cfg, err := NewConfigBuilder().
		WithDBPath("./testdata").
		WithDBFile("test.db").
		WithBucket("test").
		Build()

	assert.NoError(t, err)
	assert.Equal(t, "./testdata", cfg.DB.DBPath)
	assert.Equal(t, "test.db", cfg.DB.DBFile)
	assert.Equal(t, "test", cfg.DB.Bucket)
}

func TestConfigBuilder_InvalidHTTPPort(t *testing.T) {
	for _, port := range []string{"", "abc", "0", "65536", "-1", "80.5"} {
		b := NewConfigBuilder()
		b.config.HTTP.Port = port
		_, err := b.Build()
		assert.Error(t, err, "port %q should be rejected", port)
	}
}

func TestConfigBuilder_ValidHTTPPort(t *testing.T) {
	for _, port := range []string{"1", "80", "8080", "65535"} {
		b := NewConfigBuilder()
		b.config.HTTP.Port = port
		cfg, err := b.Build()
		assert.NoError(t, err, "port %q should be accepted", port)
		assert.Equal(t, port, cfg.HTTP.Port)
	}
}

func TestConfigBuilder_EmptyDirsRejected(t *testing.T) {
	b := NewConfigBuilder()
	b.config.TFTP.Dir = ""
	_, err := b.Build()
	assert.Error(t, err)

	b = NewConfigBuilder()
	b.config.HTTP.Dir = ""
	_, err = b.Build()
	assert.Error(t, err)

	b = NewConfigBuilder()
	b.config.Provision.Dir = ""
	_, err = b.Build()
	assert.Error(t, err)
}

func TestConfigBuilder_DeepCopyOSImages(t *testing.T) {
	b := NewConfigBuilder()
	cfg, err := b.Build()
	assert.NoError(t, err)

	// Mutating the built config's catalog must not affect the builder's
	// internal catalog (or a subsequently built config).
	cfg.OSImages.Sources["ubuntu"].Versions["22.04"] = OSVersion{DisplayName: "mutated"}
	archs := cfg.OSImages.Sources["ubuntu"].Versions["24.04"].Architectures
	archs[0] = "mutated-arch"

	cfg2, err := b.Build()
	assert.NoError(t, err)
	assert.Equal(t, "22.04 LTS", cfg2.OSImages.Sources["ubuntu"].Versions["22.04"].DisplayName)
	assert.Equal(t, []string{"x86_64"}, cfg2.OSImages.Sources["ubuntu"].Versions["24.04"].Architectures)
}

// catalogEntry is one version the bundled catalog must offer.
type catalogEntry struct {
	displayName string
	baseURL     string
	kernelFile  string
	initrdFile  string
}

func TestDefaultOSImageCatalog(t *testing.T) {
	cfg, err := NewConfigBuilder().Build()
	assert.NoError(t, err)

	want := map[string]map[string]catalogEntry{
		"ubuntu": {
			"22.04": {"22.04 LTS", "https://github.com/netbootxyz/ubuntu-squash/releases/download/22.04.5-3b4b797c/", "vmlinuz", "initrd"},
			"24.04": {"24.04 LTS", "https://github.com/netbootxyz/ubuntu-squash/releases/download/24.04.4-494663bf/", "vmlinuz", "initrd"},
			"26.04": {"26.04 LTS", "https://github.com/netbootxyz/ubuntu-squash/releases/download/26.04.1-c43af979/", "vmlinuz", "initrd"},
		},
		"debian": {
			"12": {"12 (Bookworm)", "http://deb.debian.org/debian/dists/bookworm/main/installer-amd64/current/images/netboot/debian-installer/amd64/", "linux", "initrd.gz"},
			"13": {"13 (Trixie)", "http://deb.debian.org/debian/dists/trixie/main/installer-amd64/current/images/netboot/debian-installer/amd64/", "linux", "initrd.gz"},
		},
		"fedora": {
			"43": {"43", "https://download.fedoraproject.org/pub/fedora/linux/releases/43/Everything/x86_64/os/images/pxeboot/", "vmlinuz", "initrd.img"},
			"44": {"44", "https://download.fedoraproject.org/pub/fedora/linux/releases/44/Everything/x86_64/os/images/pxeboot/", "vmlinuz", "initrd.img"},
		},
		"centos": {
			"9":  {"9 Stream", "http://mirror.stream.centos.org/9-stream/BaseOS/x86_64/os/images/pxeboot/", "vmlinuz", "initrd.img"},
			"10": {"10 Stream", "http://mirror.stream.centos.org/10-stream/BaseOS/x86_64/os/images/pxeboot/", "vmlinuz", "initrd.img"},
		},
		"nixos": {
			"26.05": {"26.05", "https://github.com/nix-community/nixos-images/releases/download/nixos-26.05/", "bzImage-x86_64-linux", "initrd-x86_64-linux"},
		},
		"opensuse": {
			"16.0":       {"Leap 16.0", "http://download.opensuse.org/distribution/leap/16.0/repo/oss/boot/x86_64/loader/", "linux", "initrd"},
			"tumbleweed": {"Tumbleweed (Rolling)", "http://download.opensuse.org/tumbleweed/repo/oss/boot/x86_64/loader/", "linux", "initrd"},
		},
	}

	assert.Len(t, cfg.OSImages.Sources, len(want))
	for osName, versions := range want {
		def, ok := cfg.OSImages.Sources[osName]
		if !assert.True(t, ok, "missing OS %s", osName) {
			continue
		}
		assert.Len(t, def.Versions, len(versions), osName)
		for version, entry := range versions {
			got, ok := def.Versions[version]
			if !assert.True(t, ok, "missing %s %s", osName, version) {
				continue
			}
			assert.Equal(t, entry.displayName, got.DisplayName, "%s %s", osName, version)
			assert.Equal(t, entry.baseURL, got.BaseURL, "%s %s", osName, version)
			assert.Equal(t, []string{"x86_64"}, got.Architectures, "%s %s", osName, version)
			// File names live on the OS, so every version of an OS shares them.
			assert.Equal(t, entry.kernelFile, def.KernelFile, "%s %s kernel", osName, version)
			assert.Equal(t, entry.initrdFile, def.InitrdFile, "%s %s initrd", osName, version)
		}
	}

	gone := []struct{ os, version string }{
		{"ubuntu", "20.04"},
		{"nixos", "23.05"},
		{"nixos", "23.11"},
		{"nixos", "24.05"},
		{"debian", "11"},
		{"opensuse", "15.5"},
		{"opensuse", "15.6"},
		{"fedora", "39"},
		{"fedora", "40"},
		{"fedora", "41"},
	}
	for _, item := range gone {
		def := cfg.OSImages.Sources[item.os]
		_, ok := def.Versions[item.version]
		assert.False(t, ok, "EOL release %s %s should be gone", item.os, item.version)
	}
}

func TestOSImageConfig_SupportedNames(t *testing.T) {
	cfg, err := NewConfigBuilder().Build()
	assert.NoError(t, err)
	assert.Equal(t, []string{
		"CentOS",
		"Debian",
		"Fedora",
		"NixOS",
		"openSUSE",
		"Ubuntu",
	}, cfg.OSImages.SupportedNames())
}

func TestOSVersion_ExpectedChecksum(t *testing.T) {
	v := OSVersion{
		DisplayName:      "22.04 LTS",
		BaseURL:          "https://example.com/",
		Architectures:    []string{"x86_64"},
		ExpectedChecksum: "abc123",
	}
	assert.Equal(t, "abc123", v.ExpectedChecksum)

	// Optional: zero value is empty
	var empty OSVersion
	assert.Empty(t, empty.ExpectedChecksum)
}

func TestLoadDefault_BMCEnv(t *testing.T) {
	unsetEnv(t, "BMC_TLS_VERIFY")
	t.Setenv("BMC_SYSTEM_ID", "")
	cfg, err := LoadDefault()
	assert.NoError(t, err)
	assert.False(t, cfg.BMC.TLSVerify)
	assert.Empty(t, cfg.BMC.SystemID)

	t.Setenv("BMC_TLS_VERIFY", "true")
	t.Setenv("BMC_SYSTEM_ID", "System.Embedded.1")
	cfg, err = LoadDefault()
	assert.NoError(t, err)
	assert.True(t, cfg.BMC.TLSVerify)
	assert.Equal(t, "System.Embedded.1", cfg.BMC.SystemID)

	t.Setenv("BMC_TLS_VERIFY", "1")
	cfg, err = LoadDefault()
	assert.NoError(t, err)
	assert.True(t, cfg.BMC.TLSVerify)

	t.Setenv("BMC_TLS_VERIFY", "false")
	cfg, err = LoadDefault()
	assert.NoError(t, err)
	assert.False(t, cfg.BMC.TLSVerify)
}

func TestBMCTLSVerifyInvalidFallsBack(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	t.Setenv("BMC_TLS_VERIFY", "maybe")
	cfg, err := LoadDefault()
	assert.NoError(t, err)
	assert.False(t, cfg.BMC.TLSVerify)
	assert.Contains(t, buf.String(), `BMC_TLS_VERIFY="maybe" is not a boolean`)
	assert.Equal(t, "BMC TLS verification: disabled (set BMC_TLS_VERIFY=true to enable)", BMCTLSModeLog(false))
	assert.Equal(t, "BMC TLS verification: enabled", BMCTLSModeLog(true))
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	prev, had := os.LookupEnv(key)
	assert.NoError(t, os.Unsetenv(key))
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(key, prev)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

func TestLoadDefault_ServerIPEnv(t *testing.T) {
	t.Setenv("SERVER_IP", "10.11.12.13")
	cfg, err := LoadDefault()
	assert.NoError(t, err)
	assert.Equal(t, "10.11.12.13", cfg.ServerIP)

	t.Setenv("SERVER_IP", "")
	cfg, err = LoadDefault()
	assert.NoError(t, err)
	assert.Empty(t, cfg.ServerIP)
}
