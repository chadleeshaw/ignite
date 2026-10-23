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
		require.Equal(t, "1 thing needs attention: HTTP is stopped", cozy)
		require.Equal(t, "1 issue · http stopped", console)
	})

	t.Run("stopped servers share one clause", func(t *testing.T) {
		cozy, console := describeStatus(&StatusPageData{
			HTTPServer: up,
			TFTPServer: up,
			DHCPServers: []DHCPServerStatus{
				{IP: "10.0.0.1", Status: "stopped"},
				{IP: "10.0.0.2", Status: "stopped"},
				{IP: "10.0.0.3", Status: "stopped"},
			},
		})
		require.Equal(t, "1 thing needs attention: DHCP 10.0.0.1, 10.0.0.2, 10.0.0.3 are stopped", cozy)
		require.Equal(t, "1 issue · dhcp 10.0.0.1, 10.0.0.2, 10.0.0.3 stopped", console)
	})

	t.Run("configured servers are not an issue", func(t *testing.T) {
		cozy, console := describeStatus(&StatusPageData{
			HTTPServer:  up,
			TFTPServer:  up,
			DHCPServers: []DHCPServerStatus{{IP: "10.0.0.1", Status: "configured"}},
		})
		require.Equal(t, "No DHCP server is running", cozy)
		require.Equal(t, "no dhcp server running", console)
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
