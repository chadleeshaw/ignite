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

	// Replacing a definition in the built config must not affect a later build.
	def := cfg.OSImages.Sources["ubuntu"]
	def.DisplayName = "mutated"
	def.KernelFile = "mutated-kernel"
	cfg.OSImages.Sources["ubuntu"] = def

	cfg2, err := b.Build()
	assert.NoError(t, err)
	assert.Equal(t, "Ubuntu", cfg2.OSImages.Sources["ubuntu"].DisplayName)
	assert.Equal(t, "vmlinuz", cfg2.OSImages.Sources["ubuntu"].KernelFile)
}

func TestDefaultOSImageBootFacts(t *testing.T) {
	cfg, err := NewConfigBuilder().Build()
	assert.NoError(t, err)

	want := map[string]OSDefinition{
		"ubuntu":   {DisplayName: "Ubuntu", KernelFile: "vmlinuz", InitrdFile: "initrd"},
		"debian":   {DisplayName: "Debian", KernelFile: "linux", InitrdFile: "initrd.gz"},
		"fedora":   {DisplayName: "Fedora", KernelFile: "vmlinuz", InitrdFile: "initrd.img"},
		"centos":   {DisplayName: "CentOS", KernelFile: "vmlinuz", InitrdFile: "initrd.img"},
		"nixos":    {DisplayName: "NixOS", KernelFile: "bzImage-x86_64-linux", InitrdFile: "initrd-x86_64-linux"},
		"opensuse": {DisplayName: "openSUSE", KernelFile: "linux", InitrdFile: "initrd"},
	}
	assert.Equal(t, want, cfg.OSImages.Sources)
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
