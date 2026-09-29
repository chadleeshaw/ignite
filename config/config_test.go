package config

import (
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
	archs := cfg.OSImages.Sources["ubuntu"].Versions["20.04"].Architectures
	archs[0] = "mutated-arch"

	cfg2, err := b.Build()
	assert.NoError(t, err)
	assert.Equal(t, "22.04 LTS", cfg2.OSImages.Sources["ubuntu"].Versions["22.04"].DisplayName)
	assert.Equal(t, []string{"x86_64"}, cfg2.OSImages.Sources["ubuntu"].Versions["20.04"].Architectures)
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
