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
