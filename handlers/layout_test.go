package handlers

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDescribeStatusGrammar(t *testing.T) {
	up := ServiceStatus{Status: "running"}

	t.Run("one thing needs attention", func(t *testing.T) {
		cozy, console := describeStatus(&StatusPageData{
			HTTPServer:  ServiceStatus{Status: "stopped"},
			TFTPServer:  up,
			DHCPServers: []DHCPServerStatus{{IP: "10.0.0.1", Status: "running"}},
		})
		require.Equal(t, "1 thing needs attention: HTTP stopped", cozy)
		require.Equal(t, "HTTP stopped", console)
	})

	t.Run("three stopped servers are three things", func(t *testing.T) {
		cozy, console := describeStatus(&StatusPageData{
			HTTPServer: up,
			TFTPServer: up,
			DHCPServers: []DHCPServerStatus{
				{IP: "10.0.0.1", Status: "stopped"},
				{IP: "10.0.0.2", Status: "stopped"},
				{IP: "10.0.0.3", Status: "stopped"},
			},
		})
		require.Equal(t, "3 things need attention: DHCP 10.0.0.1 · DHCP 10.0.0.2 · DHCP 10.0.0.3", cozy)
		require.Equal(t, "DHCP 10.0.0.1 · DHCP 10.0.0.2 · DHCP 10.0.0.3", console)
	})

	t.Run("all running", func(t *testing.T) {
		cozy, console := describeStatus(&StatusPageData{
			HTTPServer:  up,
			TFTPServer:  up,
			DHCPServers: []DHCPServerStatus{{IP: "10.0.0.1", Status: "running"}},
		})
		require.Equal(t, "All services are up", cozy)
		require.Equal(t, "services up", console)
	})

	t.Run("missing status", func(t *testing.T) {
		cozy, console := describeStatus(nil)
		require.Equal(t, "Service status is unavailable", cozy)
		require.Equal(t, "status unavailable", console)
	})
}
