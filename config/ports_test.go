package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadDefaultPortsAndDHCP(t *testing.T) {
	prev, had := os.LookupEnv("DHCP_ENABLED")
	require.NoError(t, os.Unsetenv("DHCP_ENABLED"))
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("DHCP_ENABLED", prev)
		} else {
			_ = os.Unsetenv("DHCP_ENABLED")
		}
	})
	t.Setenv("HTTP_PORT", "8080")
	t.Setenv("TFTP_PORT", "69")

	cfg, err := LoadDefault()
	require.NoError(t, err)
	require.Equal(t, "8080", cfg.HTTP.Port)
	require.Equal(t, "69", cfg.TFTP.Port)
	require.True(t, cfg.DHCP.Enabled)
}

func TestDHCPEnabledValues(t *testing.T) {
	for _, value := range []string{"false", "0", "no", "off", "FALSE"} {
		t.Run("disabled "+value, func(t *testing.T) {
			t.Setenv("DHCP_ENABLED", value)
			cfg, err := LoadDefault()
			require.NoError(t, err)
			require.False(t, cfg.DHCP.Enabled)
		})
	}
	for _, value := range []string{"true", "1", "yes", "on", "TRUE", "maybe"} {
		t.Run("enabled "+value, func(t *testing.T) {
			t.Setenv("DHCP_ENABLED", value)
			cfg, err := LoadDefault()
			require.NoError(t, err)
			require.True(t, cfg.DHCP.Enabled)
		})
	}
}

func TestTFTPPortOverrideAndValidation(t *testing.T) {
	t.Setenv("TFTP_PORT", "6969")
	t.Setenv("HTTP_PORT", "18080")
	cfg, err := LoadDefault()
	require.NoError(t, err)
	require.Equal(t, "6969", cfg.TFTP.Port)
	require.Equal(t, "18080", cfg.HTTP.Port)

	for _, port := range []string{"", "abc", "0", "65536", "-1"} {
		b := NewConfigBuilder()
		b.config.TFTP.Port = port
		_, err := b.Build()
		require.Error(t, err, "port %q", port)
	}
}
