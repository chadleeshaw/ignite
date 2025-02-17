//go:build manual
// +build manual

package main

import (
	"ignite/config"
	"ignite/db"
	"ignite/dhcp"
	"net"
	"testing"
)

var Bucket = config.Defaults.DB.Bucket

// createTestHandler sets up a DHCP handler with predefined network configurations for testing.
func createTestHandler() *dhcp.DHCPHandler {
	handler := dhcp.NewDHCPHandler(
		net.IP{192, 168, 1, 2},
		net.IP{255, 255, 255, 0},
		net.IP{192, 168, 1, 1},
		net.IP{192, 168, 1, 5},
		net.IP{192, 168, 1, 100},
		50,
	)
	handler.Start()
	return handler
}

func TestManual_MockUI_Data(t *testing.T) {
	db.Init()
	h := createTestHandler()

	// manual testing
	h.Leases["00:11:22:33:44:55"] = dhcp.Lease{MAC: "00:11:22:33:44:55", IP: net.ParseIP("192.168.1.100")}
	h.Leases["AA:BB:CC:DD:EE:FF"] = dhcp.Lease{MAC: "AA:BB:CC:DD:EE:FF", IP: net.ParseIP("192.168.1.101")}
	h.Leases["00:11:22:33:44:66"] = dhcp.Lease{MAC: "00:11:22:33:44:66", IP: net.ParseIP("192.168.1.210")}
	h.Leases["00:11:22:33:44:66"] = dhcp.Lease{MAC: "00:11:22:33:44:66", IP: net.ParseIP("192.168.1.211"), Reserved: true}

	h.UpdateDBState()
}

// Test manually:
// go test -tags=manual

// Delete db manually
// rm -f ignite.db
