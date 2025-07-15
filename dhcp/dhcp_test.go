package dhcp

import (
	"encoding/json"
	"ignite/config"
	"ignite/db"
	"log"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	d4 "github.com/krolaw/dhcp4"
)

// createTestHandler sets up a DHCP handler with predefined network configurations for testing.
func createTestHandler() *DHCPHandler {
	handler := NewDHCPHandler(
		net.IP{10, 0, 0, 10},
		net.IP{255, 255, 255, 0},
		net.IP{10, 0, 0, 1},
		net.IP{10, 0, 0, 5},
		net.IP{10, 0, 0, 50},
		50,
	)
	handler.Start()
	if err := db.KV.DeleteAllKV(Bucket); err != nil {
		log.Fatalf("Failed to clear leases from database: %v", err)
	}
	return handler
}

// removeDB removes the test database file after the tests are completed.
func removeDB(t *testing.T) {
	dbFile := filepath.Join(config.Defaults.DB.DBPath, config.Defaults.DB.DBFile)
	if err := os.Remove(dbFile); err != nil {
		t.Logf("Failed to remove database file: %v", err)
	}
}

// createTestPacket constructs a test DHCP packet with specified operation code, hardware address, and client IP.
func createTestPacket(opCode d4.OpCode, chAddr net.HardwareAddr, ciAddr net.IP) d4.Packet {
	packet := d4.NewPacket(opCode)
	packet.SetCHAddr(chAddr)
	if ciAddr != nil {
		packet.SetCIAddr(ciAddr)
	}
	return packet
}

// checkResponseMessageType verifies that the DHCP response packet contains the expected message type.
func checkResponseMessageType(t *testing.T, response d4.Packet, expectedType d4.MessageType) {
	t.Helper()
	options := response.ParseOptions()
	if messageTypeBytes, ok := options[d4.OptionDHCPMessageType]; ok && len(messageTypeBytes) > 0 {
		if d4.MessageType(messageTypeBytes[0]) != expectedType {
			t.Errorf("Expected %v, got %v", expectedType, d4.MessageType(messageTypeBytes[0]))
		}
	} else {
		t.Error("DHCP Message Type option is missing or empty")
	}
}

// TestServeDHCP_Discover tests the DHCP server's response to a DHCP Discover message for different boot types.
func TestServeDHCP_Discover(t *testing.T) {
	db.Init()
	handler := createTestHandler()
	defer removeDB(t)

	verifyBootFilename := func(t *testing.T, response d4.Packet, expectedFilename string) {
		options := response.ParseOptions()
		if bootFileName, ok := options[d4.OptionBootFileName]; ok {
			if string(bootFileName) != expectedFilename {
				t.Errorf("Expected boot file name %s, but got %s", expectedFilename, string(bootFileName))
			}
		} else {
			t.Errorf("DHCP options for filename do not exist: %v", options)
		}
	}

	t.Run("BIOS Boot", func(t *testing.T) {
		packet := createTestPacket(d4.BootRequest, net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}, nil)
		response := handler.ServeDHCP(packet, d4.Discover, d4.Options{})
		if response == nil {
			t.Fatal("No response for DHCP Discover with BIOS boot")
		}
		checkResponseMessageType(t, response, d4.Offer)
		verifyBootFilename(t, response, "boot-bios/pxelinux.0")
	})

	t.Run("iPXE Boot", func(t *testing.T) {
		packet := createTestPacket(d4.BootRequest, net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}, nil)
		options := d4.Options{
			d4.OptionVendorClassIdentifier: []byte("iPXE"),
		}
		response := handler.ServeDHCP(packet, d4.Discover, options)
		if response == nil {
			t.Fatal("No response for DHCP Discover with iPXE boot")
		}
		checkResponseMessageType(t, response, d4.Offer)
		verifyBootFilename(t, response, "boot-efi/syslinux.efi")
	})

	t.Run("No Free Leases", func(t *testing.T) {
		packet := createTestPacket(d4.BootRequest, net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}, nil)

		// Fill up all possible leases with unique MAC addresses
		for i := 0; i < handler.LeaseRange; i++ {
			leaseIP := net.IP{10, 0, 0, byte(50 + i)}
			mac := net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, byte(i)}
			handler.Leases[mac.String()] = Lease{
				IP:     leaseIP,
				MAC:    mac.String(),
				Expiry: time.Now().Add(2 * time.Hour),
			}
		}

		handler.UpdateDBState()

		freeIP := handler.freeLease()
		if freeIP != nil {
			t.Errorf("Expected no free lease to be available, but got %v", freeIP)
		}

		response := handler.ServeDHCP(packet, d4.Discover, d4.Options{})
		if response != nil {
			t.Errorf("Expected no response due to no free leases, but got one. Lease count: %d", len(handler.Leases))
		}
	})
}

// TestServeDHCP_Request tests the DHCP server's response to a DHCP Request message, ensuring lease assignment.
func TestServeDHCP_Request(t *testing.T) {
	db.Init()
	handler := createTestHandler()
	defer removeDB(t)

	mac := net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}
	ip := net.IP{10, 0, 0, 50}
	packet := createTestPacket(d4.OpCode(0), mac, ip)
	options := d4.Options{
		d4.OptionRequestedIPAddress: ip,
	}

	response := handler.ServeDHCP(packet, d4.Request, options)

	if response == nil {
		t.Fatal("No response for DHCP Request")
	}
	checkResponseMessageType(t, response, d4.ACK)

	macStr := mac.String()
	if _, ok := handler.Leases[macStr]; !ok {
		t.Error("Lease not added to in-memory leases map")
	} else if !handler.Leases[macStr].IP.Equal(ip) {
		t.Errorf("Mismatch in IP for MAC %s; expected %s, got %s", macStr, ip.String(), handler.Leases[macStr].IP.String())
	}

	v, err := db.KV.GetKV(Bucket, []byte(handler.IP.String()))
	if err != nil {
		t.Errorf("Failed to fetch DHCPHandler from database: %v", err)
	} else if v == nil {
		t.Error("DHCPHandler not found in database")
	} else {
		var storedHandler DHCPHandler
		if err := json.Unmarshal(v, &storedHandler); err != nil {
			t.Errorf("Failed to unmarshal stored DHCPHandler: %v", err)
		}
		if _, ok := storedHandler.Leases[macStr]; !ok {
			t.Error("Lease not found in stored DHCPHandler")
		} else if !storedHandler.Leases[macStr].IP.Equal(ip) {
			t.Errorf("Mismatch in IP for MAC %s in stored handler; expected %s, got %s", macStr, ip.String(), storedHandler.Leases[macStr].IP.String())
		}
	}
}

// TestServeDHCP_Release checks if a lease is correctly removed after a DHCP Release message.
func TestServeDHCP_Release(t *testing.T) {
	db.Init()
	handler := createTestHandler()
	defer removeDB(t)

	mac := net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}
	ip := net.IP{10, 0, 0, 66}
	packet := createTestPacket(d4.OpCode(0), mac, ip)
	options := d4.Options{
		d4.OptionRequestedIPAddress: ip,
	}

	response := handler.ServeDHCP(packet, d4.Request, options)
	if response == nil {
		t.Fatal("No response for DHCP Request")
	}
	checkResponseMessageType(t, response, d4.ACK)

	if lease, ok := handler.Leases[mac.String()]; !ok || lease.IP == nil || lease.MAC == "" {
		t.Error("Lease not correctly added to in-memory leases map after request")
	}

	handler.ServeDHCP(packet, d4.Release, d4.Options{})

	if _, exists := handler.Leases[mac.String()]; exists {
		t.Error("Lease not removed from in-memory leases map after release")
	}

	v, err := db.KV.GetKV(Bucket, []byte(handler.IP.String()))
	if err != nil {
		t.Errorf("Error checking lease in database after release: %v", err)
	}
	if err = json.Unmarshal(v, handler); err != nil {
		t.Error("Unable to unmarshal to handler")
	}
	if len(handler.Leases) != 0 {
		t.Errorf("Lease not removed from database after release; found: %v", handler.Leases)
	}
}

// TestFreeLease verifies if the DHCP server can correctly find and return an expired lease.
func TestFreeLease(t *testing.T) {
	db.Init()
	handler := createTestHandler()
	defer removeDB(t)

	mac1 := "00:11:22:33:44:55"
	mac2 := "00:11:22:33:44:56"

	ip1 := net.IP{10, 0, 0, 50}
	ip2 := net.IP{10, 0, 0, 51}

	handler.Leases[mac1] = Lease{IP: ip1, MAC: mac1, Expiry: time.Now().Add(-1 * time.Hour)}
	handler.Leases[mac2] = Lease{IP: ip2, MAC: mac2, Expiry: time.Now().Add(1 * time.Hour)}
	handler.UpdateDBState()

	freeIP := handler.freeLease()
	if freeIP == nil {
		t.Error("Expected to find a free lease, but none found")
	}

	for _, lease := range handler.Leases {
		if lease.IP.Equal(freeIP) {
			if now := time.Now(); now.Before(lease.Expiry) {
				t.Errorf("Expected the lease at IP %s to be free, but it exists and hasn't expired", freeIP.String())
				return
			}
		}
	}
}

// TestServeDHCP_ReservedIP tests DHCP behavior with reserved IP addresses, including requests for reserved and different IPs.
func TestServeDHCP_ReservedIP(t *testing.T) {
	db.Init()
	handler := createTestHandler()
	defer removeDB(t)

	mac := net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}
	reservedIP := net.IP{10, 0, 0, 100}
	anotherIP := net.IP{10, 0, 0, 101}
	macStr := mac.String()
	handler.Leases[macStr] = Lease{
		IP:       reservedIP,
		MAC:      macStr,
		Expiry:   time.Now().Add(1 * time.Hour),
		Reserved: true,
	}
	handler.UpdateDBState()

	t.Run("Request for Reserved IP", func(t *testing.T) {
		packet := createTestPacket(d4.OpCode(0), mac, reservedIP)
		options := d4.Options{
			d4.OptionRequestedIPAddress: reservedIP,
		}

		response := handler.ServeDHCP(packet, d4.Request, options)
		if response == nil {
			t.Fatal("No response for DHCP Request for reserved IP")
		}
		checkResponseMessageType(t, response, d4.ACK)

		// Check if the lease remains reserved
		reserved, err := handler.GetLeaseReservation(macStr)
		if err != nil {
			t.Fatalf("Failed to get lease reservation status: %v", err)
		}
		if !reserved {
			t.Error("Lease should remain reserved after ACK")
		}
	})

	t.Run("Request for Different IP When Reserved", func(t *testing.T) {
		packet := createTestPacket(d4.OpCode(0), mac, anotherIP)
		options := d4.Options{
			d4.OptionRequestedIPAddress: anotherIP,
		}

		response := handler.ServeDHCP(packet, d4.Request, options)
		if response == nil {
			t.Fatal("No response for DHCP Request for different IP when reserved")
		}
		checkResponseMessageType(t, response, d4.NAK)

		reserved, err := handler.GetLeaseReservation(macStr)
		if err != nil {
			t.Fatalf("Failed to get lease reservation status: %v", err)
		}
		if !reserved {
			t.Error("Lease should still be reserved after NAK for different IP")
		}
	})
}
