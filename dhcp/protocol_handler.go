// dhcp/protocol_handler.go - DHCP protocol handler
package dhcp

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"ignite/config"

	d4 "github.com/krolaw/dhcp4"
)

// ProtocolHandler handles DHCP protocol packets for a specific server
type ProtocolHandler struct {
	server    *Server
	leaseRepo LeaseRepository
	listener  net.PacketConn
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewProtocolHandler creates a new DHCP protocol handler
func NewProtocolHandler(server *Server, leaseRepo LeaseRepository) *ProtocolHandler {
	return &ProtocolHandler{
		server:    server,
		leaseRepo: leaseRepo,
	}
}

// Start starts the DHCP protocol handler
func (h *ProtocolHandler) Start() error {
	if h.server.IP == nil {
		return fmt.Errorf("server IP is not set")
	}

	h.ctx, h.cancel = context.WithCancel(context.Background())

	var err error
	addr := &net.UDPAddr{IP: h.server.IP, Port: 67}
	h.listener, err = net.ListenUDP("udp4", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	go func() {
		defer func() {
			if err := h.listener.Close(); err != nil {
				log.Printf("Failed to close listener: %v", err)
			}
		}()

		if err := d4.Serve(h.listener, h); err != nil {
			log.Printf("Error serving DHCP requests: %v", err)
		}
	}()

	return nil
}

// Stop stops the DHCP protocol handler
func (h *ProtocolHandler) Stop() error {
	if h.cancel != nil {
		h.cancel()
	}

	if h.listener != nil {
		if err := h.listener.Close(); err != nil {
			return fmt.Errorf("failed to close listener: %w", err)
		}
	}

	// Wait for context to be done or timeout
	select {
	case <-h.ctx.Done():
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout waiting for server to stop")
	}
}

// ServeDHCP implements the DHCP packet handler interface
func (h *ProtocolHandler) ServeDHCP(p d4.Packet, msgType d4.MessageType, options d4.Options) d4.Packet {
	switch msgType {
	case d4.Discover:
		return h.handleDiscover(p, options)
	case d4.Request:
		return h.handleRequest(p, options)
	case d4.Release, d4.Decline:
		h.handleRelease(p, options)
		return nil
	default:
		return nil
	}
}

// handleDiscover processes DHCP Discover messages
func (h *ProtocolHandler) handleDiscover(p d4.Packet, options d4.Options) d4.Packet {
	ctx := context.Background()
	mac := p.CHAddr().String()

	// Determine boot type and filename
	filename := h.getBootFilename(options)
	dhcpOptions := h.buildDHCPOptions(filename)

	// Check for existing reserved lease
	lease, err := h.leaseRepo.GetByMAC(ctx, mac)
	if err == nil && lease != nil && lease.Reserved && lease.ServerID == h.server.ID {
		return h.createOfferPacket(p, lease.IP, dhcpOptions)
	}

	// Find available IP
	availableIP := h.findAvailableIP(ctx, mac)
	if availableIP == nil {
		log.Printf("No available IP for MAC %s", mac)
		return nil
	}

	return h.createOfferPacket(p, availableIP, dhcpOptions)
}

// handleRequest processes DHCP Request messages
func (h *ProtocolHandler) handleRequest(p d4.Packet, options d4.Options) d4.Packet {
	ctx := context.Background()
	mac := p.CHAddr().String()
	requestedIP := h.getRequestedIP(options, p)

	if requestedIP == nil {
		return h.createNakPacket(p)
	}

	// Check if request is for another server
	if h.isRequestForAnotherServer(options) {
		return nil
	}

	// Check existing lease
	lease, err := h.leaseRepo.GetByMAC(ctx, mac)
	if err == nil && lease != nil && lease.ServerID == h.server.ID {
		if lease.Reserved && !requestedIP.Equal(lease.IP) {
			return h.createNakPacket(p)
		}

		if requestedIP.Equal(lease.IP) {
			// Update lease expiry
			lease.Extend(h.server.LeaseDuration)
			if err := h.leaseRepo.Save(ctx, lease); err != nil {
				log.Printf("Failed to update lease: %v", err)
			}
			return h.createAckPacket(p, requestedIP)
		}
	}

	// Check if IP is available and in range
	if !h.server.IsInRange(requestedIP) || !h.isIPAvailable(ctx, requestedIP, mac) {
		return h.createNakPacket(p)
	}

	// Create new lease
	newLease := &Lease{
		IP:       requestedIP,
		MAC:      mac,
		Expiry:   time.Now().Add(h.server.LeaseDuration),
		Reserved: false,
		ServerID: h.server.ID,
	}

	if err := h.leaseRepo.Save(ctx, newLease); err != nil {
		log.Printf("Failed to save new lease: %v", err)
		return h.createNakPacket(p)
	}

	return h.createAckPacket(p, requestedIP)
}

// handleRelease processes DHCP Release/Decline messages
func (h *ProtocolHandler) handleRelease(p d4.Packet, options d4.Options) {
	ctx := context.Background()
	mac := p.CHAddr().String()

	if err := h.leaseRepo.DeleteByMAC(ctx, mac); err != nil {
		log.Printf("Failed to release lease for MAC %s: %v", mac, err)
	}
}

// getBootFilename determines the boot filename based on client type
func (h *ProtocolHandler) getBootFilename(options d4.Options) string {
	cfg, _ := config.LoadDefault() // Should be injected in real implementation

	bootType := "BIOS"
	if vendorOption, ok := options[d4.OptionVendorClassIdentifier]; ok {
		if string(vendorOption) == "iPXE" || string(vendorOption) == "gPXE" {
			bootType = "iPXE"
		}
	}

	if bootType == "iPXE" {
		return cfg.DHCP.EFIFile
	}
	return cfg.DHCP.BiosFile
}

// buildDHCPOptions creates DHCP options for responses
func (h *ProtocolHandler) buildDHCPOptions(filename string) d4.Options {
	return d4.Options{
		d4.OptionTFTPServerName:   []byte(h.server.IP),
		d4.OptionSubnetMask:       []byte(h.server.Options.SubnetMask),
		d4.OptionRouter:           []byte(h.server.Options.Gateway),
		d4.OptionDomainNameServer: []byte(h.server.Options.DNS),
		d4.OptionBootFileName:     []byte(filename),
	}
}

// createOfferPacket creates a DHCP Offer packet
func (h *ProtocolHandler) createOfferPacket(p d4.Packet, ip net.IP, options d4.Options) d4.Packet {
	return d4.ReplyPacket(p, d4.Offer, h.server.IP, ip, h.server.LeaseDuration,
		options.SelectOrderOrAll(options[d4.OptionParameterRequestList]))
}

// createAckPacket creates a DHCP ACK packet
func (h *ProtocolHandler) createAckPacket(p d4.Packet, ip net.IP) d4.Packet {
	filename := h.getBootFilename(p.ParseOptions())
	options := h.buildDHCPOptions(filename)

	return d4.ReplyPacket(p, d4.ACK, h.server.IP, ip, h.server.LeaseDuration,
		options.SelectOrderOrAll(options[d4.OptionParameterRequestList]))
}

// createNakPacket creates a DHCP NAK packet
func (h *ProtocolHandler) createNakPacket(p d4.Packet) d4.Packet {
	return d4.ReplyPacket(p, d4.NAK, h.server.IP, nil, 0, nil)
}

// getRequestedIP extracts the requested IP from DHCP options or packet
func (h *ProtocolHandler) getRequestedIP(options d4.Options, p d4.Packet) net.IP {
	if reqIP := net.IP(options[d4.OptionRequestedIPAddress]); reqIP != nil {
		return reqIP
	}
	return net.IP(p.CIAddr())
}

// isRequestForAnotherServer checks if the request is meant for another DHCP server
func (h *ProtocolHandler) isRequestForAnotherServer(options d4.Options) bool {
	if server, ok := options[d4.OptionServerIdentifier]; ok {
		return !net.IP(server).Equal(h.server.IP)
	}
	return false
}

// findAvailableIP finds an available IP for assignment
func (h *ProtocolHandler) findAvailableIP(ctx context.Context, excludeMAC string) net.IP {
	for i := 0; i < h.server.LeaseRange; i++ {
		candidate := incrementIP(h.server.IPStart, i)
		if h.isIPAvailable(ctx, candidate, excludeMAC) {
			return candidate
		}
	}
	return nil
}

// isIPAvailable checks if an IP address is available for assignment
func (h *ProtocolHandler) isIPAvailable(ctx context.Context, ip net.IP, excludeMAC string) bool {
	leases, err := h.leaseRepo.GetByServerID(ctx, h.server.ID)
	if err != nil {
		return false
	}

	for _, lease := range leases {
		if lease.IP.Equal(ip) && lease.MAC != excludeMAC && !lease.IsExpired() {
			return false
		}
	}
	return true
}
