// dhcp/protocol_handler.go - DHCP protocol handler
package dhcp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
	d4 "github.com/krolaw/dhcp4"
)

// ErrHandlerNotRunning is returned by Stop when the handler was never started
// (or has already been stopped).
var ErrHandlerNotRunning = errors.New("DHCP handler is not running")

// ProtocolBootFiles carries the boot-file settings a ProtocolHandler needs.
// The values are loaded once at construction time (see WithBootFiles) so the
// packet path never pays for config.LoadDefault() per packet.
type ProtocolBootFiles struct {
	BiosFile string
	EFIFile  string
}

// ProtocolHandlerOption configures a ProtocolHandler.
type ProtocolHandlerOption func(*ProtocolHandler)

// WithAllocationMutex sets the mutex used to serialize IP allocation.
// Handlers that share a mutex (via the DHCPServerService/DHCPLeaseService)
// close the check-then-act race between scan, select, and save.
func WithAllocationMutex(mu *sync.Mutex) ProtocolHandlerOption {
	return func(h *ProtocolHandler) {
		if mu != nil {
			h.allocMu = mu
		}
	}
}

// WithBootFiles sets the boot-file settings for the handler.
func WithBootFiles(files ProtocolBootFiles) ProtocolHandlerOption {
	return func(h *ProtocolHandler) {
		h.bootFiles = files
	}
}

// ProtocolHandler implements the DHCP protocol handling
type ProtocolHandler struct {
	server    *Server
	leaseRepo LeaseRepository

	// allocMu serializes IP allocation (scan -> select -> save) across the
	// packet path and the service path. It is shared with
	// DHCPLeaseService so both allocate from the same critical section.
	allocMu *sync.Mutex
	// bootFiles holds the boot-file settings loaded once at construction.
	bootFiles ProtocolBootFiles

	// mu guards the lifecycle fields below.
	mu       sync.Mutex
	running  bool
	ctx      context.Context
	cancel   context.CancelFunc
	conn     net.PacketConn
	serveErr error
}

// NewProtocolHandler creates a new DHCP protocol handler.
// The (server, leaseRepo) call shape is preserved for backward
// compatibility; functional options are optional.
func NewProtocolHandler(server *Server, leaseRepo LeaseRepository, opts ...ProtocolHandlerOption) *ProtocolHandler {
	h := &ProtocolHandler{
		server:    server,
		leaseRepo: leaseRepo,
		allocMu:   &sharedAllocMu,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Start binds the DHCP listener and begins serving. It refuses a second
// start without an intervening Stop and never overwrites the original
// listener: the bind happens before any lifecycle state is published, so a
// failed Start leaves the handler exactly as it was.
func (h *ProtocolHandler) Start() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.running {
		return fmt.Errorf("DHCP handler is already running")
	}

	// Bind first; only publish state after the bind succeeds.
	ctx, cancel := context.WithCancel(context.Background())
	conn, err := net.ListenPacket("udp4", ":67")
	if err != nil {
		cancel()
		return fmt.Errorf("failed to listen on UDP port 67: %w", err)
	}

	h.ctx = ctx
	h.cancel = cancel
	h.conn = conn
	h.running = true
	h.serveErr = nil
	go h.serveLoop()

	return nil
}

// Stop shuts the handler down. Stopping a handler that was never started
// (or is already stopped) is a clean ErrHandlerNotRunning, not a hang.
func (h *ProtocolHandler) Stop() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.running {
		return ErrHandlerNotRunning
	}

	h.running = false
	if h.conn != nil {
		// Closing the socket unblocks the serve loop's ReadFrom.
		_ = h.conn.Close()
	}
	if h.cancel != nil {
		h.cancel()
	}
	return nil
}

// ServeError returns the last error from the serve loop, if any.
func (h *ProtocolHandler) ServeError() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.serveErr
}

// serveLoop runs the DHCP serve loop until the socket closes.
func (h *ProtocolHandler) serveLoop() {
	err := d4.Serve(h.conn, h)

	h.mu.Lock()
	h.running = false
	if err != nil {
		h.serveErr = err
	}
	h.mu.Unlock()

	if err != nil {
		log.Printf("DHCP server stopped with error: %v", err)
	}
}

// ServeDHCP handles DHCP protocol messages
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

// newLease builds a fully-initialized lease record for a fresh allocation:
// unique ID, assigned state, state-history entry, and timestamps.
func (h *ProtocolHandler) newLease(mac string, ip net.IP) *Lease {
	now := time.Now()
	return &Lease{
		ID:             uuid.New().String(),
		MAC:            mac,
		IP:             ip,
		ServerID:       h.server.ID,
		Expiry:         now.Add(h.server.LeaseDuration),
		Reserved:       false,
		State:          StateAssigned,
		StateUpdatedAt: now,
		LastSeen:       now,
		StateHistory: []StateTransition{
			{FromState: "", ToState: StateAssigned, Timestamp: now, Source: "dhcp"},
		},
	}
}

// handleDiscover processes DHCP Discover messages
func (h *ProtocolHandler) handleDiscover(p d4.Packet, options d4.Options) d4.Packet {
	// Serialize with the allocation path so the scan below can't interleave
	// with a concurrent request's save.
	h.allocMu.Lock()
	defer h.allocMu.Unlock()

	ctx := context.Background()
	mac := p.CHAddr().String()

	// Re-offer the current IP when the MAC already has a live lease — or a
	// static reservation, which never expires for offer purposes — on this
	// server. No row is persisted for an offer; the lease is created on
	// request, where conflicts are resolved under the same lock.
	if lease, err := h.leaseRepo.GetByMAC(ctx, mac); err == nil && lease != nil {
		if lease.ServerID == h.server.ID && (lease.Reserved || !lease.IsExpired()) {
			return h.createOfferPacket(p, lease.IP, options)
		}
	}

	// Find available IP
	ip := h.findAvailableIP(ctx, mac)
	if ip == nil {
		log.Printf("No available IP for MAC %s", mac)
		return nil
	}

	return h.createOfferPacket(p, ip, options)
}

// handleRequest processes DHCP Request messages
func (h *ProtocolHandler) handleRequest(p d4.Packet, options d4.Options) d4.Packet {
	// Serialize scan -> select -> save against the service allocation path.
	h.allocMu.Lock()
	defer h.allocMu.Unlock()

	ctx := context.Background()
	mac := p.CHAddr().String()

	requestedIP := net.IP(options[d4.OptionRequestedIPAddress])
	if requestedIP == nil {
		// Some clients put it in CIAddr
		requestedIP = p.CIAddr()
	}

	// Silently ignore requests addressed to a different DHCP server.
	if h.isRequestForAnotherServer(options) {
		return nil
	}

	// validRequest is true when the requested address is a usable IPv4
	// address inside this server's range and not taken by another client.
	validRequest := requestedIP.To4() != nil &&
		h.server.IsInRange(requestedIP) &&
		h.isIPAvailable(ctx, requestedIP, mac)

	// Check for existing lease
	lease, err := h.leaseRepo.GetByMAC(ctx, mac)
	if err == nil && lease != nil {
		// An existing lease may only be extended when it belongs to this
		// server; otherwise NAK so the client restarts cleanly.
		if lease.ServerID != h.server.ID {
			log.Printf("Lease for MAC %s belongs to another server; sending NAK", mac)
			return h.createNakPacket(p)
		}
		// A static reservation pins the MAC to its IP: NAK anything else.
		if lease.Reserved && !lease.IP.Equal(requestedIP) {
			log.Printf("MAC %s has reserved IP %s; NAKing request for %s", mac, lease.IP, requestedIP)
			return h.createNakPacket(p)
		}
		// Existing lease - extend if requested IP matches or no specific IP requested
		if requestedIP == nil || lease.IP.Equal(requestedIP) {
			lease.Extend(h.server.LeaseDuration)
			lease.LastSeen = time.Now()
			// Never ACK an unpersisted lease.
			if err := h.leaseRepo.Save(ctx, lease); err != nil {
				log.Printf("Failed to save extended lease for MAC %s: %v", mac, err)
				return h.createNakPacket(p)
			}
			return h.createAckPacket(p, lease.IP)
		}
		// The client wants a different IP: only replace the existing lease
		// when the requested IP is actually valid — never delete a good
		// lease just to NAK.
		if !validRequest {
			log.Printf("Requested IP %s not available for MAC %s", requestedIP, mac)
			return h.createNakPacket(p)
		}
		if derr := h.leaseRepo.DeleteByMAC(ctx, mac); derr != nil {
			log.Printf("Failed to delete stale lease for MAC %s: %v", mac, derr)
		}
		lease = h.newLease(mac, requestedIP.To4())
		// Never ACK an unpersisted lease.
		if err := h.leaseRepo.Save(ctx, lease); err != nil {
			log.Printf("Failed to save lease for MAC %s: %v", mac, err)
			return h.createNakPacket(p)
		}
		return h.createAckPacket(p, requestedIP.To4())
	}

	// New lease request - use the requested IP when it is valid.
	if validRequest {
		lease := h.newLease(mac, requestedIP.To4())
		// Never ACK an unpersisted lease.
		if err := h.leaseRepo.Save(ctx, lease); err != nil {
			log.Printf("Failed to save lease for MAC %s: %v", mac, err)
			return h.createNakPacket(p)
		}
		return h.createAckPacket(p, requestedIP.To4())
	}

	// Find available IP for new lease
	ip := h.findAvailableIP(ctx, mac)
	if ip == nil {
		log.Printf("No available IP for MAC %s", mac)
		return h.createNakPacket(p)
	}

	// Create new lease
	lease = h.newLease(mac, ip)

	// Never ACK an unpersisted lease.
	if err := h.leaseRepo.Save(ctx, lease); err != nil {
		log.Printf("Failed to save lease for MAC %s: %v", mac, err)
		return h.createNakPacket(p)
	}

	return h.createAckPacket(p, ip)
}

// handleRelease processes DHCP Release/Decline messages
func (h *ProtocolHandler) handleRelease(p d4.Packet, options d4.Options) {
	ctx := context.Background()
	mac := p.CHAddr().String()

	if err := h.leaseRepo.DeleteByMAC(ctx, mac); err != nil {
		log.Printf("Failed to release lease for MAC %s: %v", mac, err)
	}
}

// getBootFilename determines the boot filename based on client type.
// The boot-file settings were loaded once at handler construction, so this
// never touches the config system on the packet path.
func (h *ProtocolHandler) getBootFilename(options d4.Options) string {
	bootType := "BIOS"
	if vendorOption, ok := options[d4.OptionVendorClassIdentifier]; ok {
		if string(vendorOption) == "iPXE" || string(vendorOption) == "gPXE" {
			bootType = "iPXE"
		}
	}

	if bootType == "iPXE" {
		return h.bootFiles.EFIFile
	}
	return h.bootFiles.BiosFile
}

// ipv4Option normalizes an address to its 4-byte IPv4 form for DHCP wire
// options. DHCP options carry 4-byte IPv4 addresses; encoding a 16-byte
// IPv4-in-IPv6 form would put garbage on the wire.
func ipv4Option(ip net.IP) []byte {
	if ip4 := ip.To4(); ip4 != nil {
		out := make([]byte, 4)
		copy(out, ip4)
		return out
	}
	return nil
}

// buildDHCPOptions creates DHCP options for responses
func (h *ProtocolHandler) buildDHCPOptions(filename string) d4.Options {
	return d4.Options{
		d4.OptionTFTPServerName:   ipv4Option(h.server.IP),
		d4.OptionSubnetMask:       ipv4Option(h.server.Options.SubnetMask),
		d4.OptionRouter:           ipv4Option(h.server.Options.Gateway),
		d4.OptionDomainNameServer: ipv4Option(h.server.Options.DNS),
		d4.OptionBootFileName:     []byte(filename),
	}
}

// createOfferPacket creates a DHCP Offer packet
func (h *ProtocolHandler) createOfferPacket(p d4.Packet, ip net.IP, options d4.Options) d4.Packet {
	return d4.ReplyPacket(p, d4.Offer, h.server.IP.To4(), ip.To4(), h.server.LeaseDuration,
		options.SelectOrderOrAll(options[d4.OptionParameterRequestList]))
}

// createAckPacket creates a DHCP ACK packet
func (h *ProtocolHandler) createAckPacket(p d4.Packet, ip net.IP) d4.Packet {
	filename := h.getBootFilename(p.ParseOptions())
	options := h.buildDHCPOptions(filename)

	return d4.ReplyPacket(p, d4.ACK, h.server.IP.To4(), ip.To4(), h.server.LeaseDuration,
		options.SelectOrderOrAll(options[d4.OptionParameterRequestList]))
}

// createNakPacket creates a DHCP NAK packet
func (h *ProtocolHandler) createNakPacket(p d4.Packet) d4.Packet {
	return d4.ReplyPacket(p, d4.NAK, h.server.IP.To4(), nil, 0, nil)
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
		candidate, err := incrementIP(h.server.IPStart, i)
		if err != nil {
			return nil
		}
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
