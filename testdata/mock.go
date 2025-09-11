package testdata

import (
	"context"
	"net"
	"time"

	"ignite/dhcp"
)

// MockDataService provides methods to populate the database with test data
type MockDataService struct {
	serverService dhcp.ServerService
	leaseService  dhcp.LeaseService
}

// NewMockDataService creates a new mock data service
func NewMockDataService(serverService dhcp.ServerService, leaseService dhcp.LeaseService) *MockDataService {
	return &MockDataService{
		serverService: serverService,
		leaseService:  leaseService,
	}
}

// PopulateMockData creates sample DHCP servers and leases for UI testing
func (m *MockDataService) PopulateMockData(ctx context.Context) error {
	// Create first test DHCP server
	server1Config := dhcp.ServerConfig{
		IP:            net.ParseIP("192.168.1.2"),
		SubnetMask:    net.ParseIP("255.255.255.0"),
		Gateway:       net.ParseIP("192.168.1.1"),
		DNS:           net.ParseIP("8.8.8.8"),
		StartIP:       net.ParseIP("192.168.1.100"),
		LeaseRange:    50,
		LeaseDuration: 2 * time.Hour,
	}

	server1, err := m.serverService.CreateServer(ctx, server1Config)
	if err != nil {
		return err
	}

	// Create second test DHCP server
	server2Config := dhcp.ServerConfig{
		IP:            net.ParseIP("10.0.1.2"),
		SubnetMask:    net.ParseIP("255.255.255.0"),
		Gateway:       net.ParseIP("10.0.1.1"),
		DNS:           net.ParseIP("1.1.1.1"),
		StartIP:       net.ParseIP("10.0.1.100"),
		LeaseRange:    25,
		LeaseDuration: 4 * time.Hour,
	}

	server2, err := m.serverService.CreateServer(ctx, server2Config)
	if err != nil {
		return err
	}

	// Create mock leases for server1
	leases1 := []struct {
		mac      string
		ip       string
		reserved bool
		menu     dhcp.BootMenu
		ipmi     dhcp.IPMI
	}{
		{
			mac:      "00:11:22:33:44:55",
			ip:       "192.168.1.100",
			reserved: false,
			menu: dhcp.BootMenu{
				Filename:     "pxelinux.0",
				OS:           "Ubuntu 20.04",
				TemplateType: "preseed",
				TemplateName: "ubuntu-desktop",
				Hostname:     "workstation-01",
				IP:           net.ParseIP("192.168.1.100"),
				Subnet:       net.ParseIP("255.255.255.0"),
				Gateway:      net.ParseIP("192.168.1.1"),
				DNS:          net.ParseIP("8.8.8.8"),
			},
			ipmi: dhcp.IPMI{
				PXEBoot:  true,
				Reboot:   false,
				IP:       net.ParseIP("192.168.1.200"),
				Username: "admin",
			},
		},
		{
			mac:      "AA:BB:CC:DD:EE:FF",
			ip:       "192.168.1.101",
			reserved: true,
			menu: dhcp.BootMenu{
				Filename:     "ipxe.efi",
				OS:           "Windows 11",
				TemplateType: "unattend",
				TemplateName: "windows-enterprise",
				Hostname:     "server-01",
				IP:           net.ParseIP("192.168.1.101"),
				Subnet:       net.ParseIP("255.255.255.0"),
				Gateway:      net.ParseIP("192.168.1.1"),
				DNS:          net.ParseIP("8.8.8.8"),
			},
			ipmi: dhcp.IPMI{
				PXEBoot:  false,
				Reboot:   true,
				IP:       net.ParseIP("192.168.1.201"),
				Username: "root",
			},
		},
		{
			mac:      "00:11:22:33:44:66",
			ip:       "192.168.1.102",
			reserved: true,
			menu: dhcp.BootMenu{
				Filename:     "grub.efi",
				OS:           "CentOS 8",
				TemplateType: "kickstart",
				TemplateName: "centos-minimal",
				Hostname:     "db-server",
				IP:           net.ParseIP("192.168.1.102"),
				Subnet:       net.ParseIP("255.255.255.0"),
				Gateway:      net.ParseIP("192.168.1.1"),
				DNS:          net.ParseIP("1.1.1.1"),
			},
			ipmi: dhcp.IPMI{
				PXEBoot:  true,
				Reboot:   true,
				IP:       net.ParseIP("192.168.1.202"),
				Username: "ipmi-admin",
			},
		},
	}

	for _, leaseData := range leases1 {
		if err := m.leaseService.ReserveLease(ctx, server1.ID, leaseData.mac, net.ParseIP(leaseData.ip)); err != nil {
			return err
		}
	}

	// Create mock leases for server2
	leases2 := []struct {
		mac      string
		ip       string
		reserved bool
	}{
		{
			mac:      "11:22:33:44:55:66",
			ip:       "10.0.1.100",
			reserved: false,
		},
		{
			mac:      "77:88:99:AA:BB:CC",
			ip:       "10.0.1.101",
			reserved: true,
		},
	}

	for _, leaseData := range leases2 {
		if err := m.leaseService.ReserveLease(ctx, server2.ID, leaseData.mac, net.ParseIP(leaseData.ip)); err != nil {
			return err
		}
	}

	return nil
}

// ClearAllData removes all DHCP servers and leases from the database
func (m *MockDataService) ClearAllData(ctx context.Context) error {
	// Get all servers
	servers, err := m.serverService.GetAllServers(ctx)
	if err != nil {
		return err
	}

	// Delete all servers (this should cascade delete leases)
	for _, server := range servers {
		if err := m.serverService.DeleteServer(ctx, server.ID); err != nil {
			return err
		}
	}

	return nil
}
