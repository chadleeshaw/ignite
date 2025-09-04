package dhcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/url"
	"time"

	"ignite/config"
	"ignite/db"

	d4 "github.com/krolaw/dhcp4"
	bolt "go.etcd.io/bbolt"
)

var Bucket = config.Defaults.DB.Bucket

// DHCPHandler represents the structure for a DHCP server, managing IP leases and network options.
type DHCPHandler struct {
	IP            net.IP           `json:"ip"`
	Options       d4.Options       `json:"options"`
	IPStart       net.IP           `json:"ipstart"`
	Started       bool             `json:"started"`
	LeaseRange    int              `json:"lease_range"`
	LeaseDuration time.Duration    `json:"lease_duration"`
	Leases        map[string]Lease `json:"leases"`
	ctx           context.Context
	cancel        context.CancelFunc
	listener      net.PacketConn
}

// Lease represents a single IP lease assigned to a MAC address.
type Lease struct {
	IP       net.IP    `json:"ip"`
	MAC      string    `json:"mac"`
	Expiry   time.Time `json:"expiry"`
	Reserved bool      `json:"reserved"`
	Menu     BootMenu  `json:"menu"`
	IPMI     IPMI      `json:"ipmi"`
}

// BootMenu contains information about the boot options for a client.
type BootMenu struct {
	Filename      string `json:"filename"`
	OS            string `json:"os"`
	Template_Type string `json:"typeSelect"`
	Template_Name string `json:"template_name"`
	Hostname      string `json:"hostname"`
	IP            net.IP `json:"ip"`
	Subnet        net.IP `json:"subnet"`
	Gateway       net.IP `json:"gateway"`
	DNS           net.IP `json:"dns"`
}

// IPMI holds IPMI-related configuration for a client.
type IPMI struct {
	Pxeboot  bool   `json:"pxeboot"`
	Reboot   bool   `json:"reboot"`
	IP       net.IP `json:"ip"`
	Username string `json:"username"`
}

// NewDHCPHandler initializes and returns a new DHCPHandler with the specified network parameters.
func NewDHCPHandler(serverIP, subnet, gateway, dns, start net.IP, leaseRange int) *DHCPHandler {
	h := &DHCPHandler{
		IP:            serverIP,
		LeaseDuration: 2 * time.Hour,
		IPStart:       start,
		Started:       false,
		LeaseRange:    leaseRange,
		Leases:        make(map[string]Lease, leaseRange),
		Options: d4.Options{
			d4.OptionTFTPServerName:   []byte(serverIP),
			d4.OptionSubnetMask:       []byte(subnet),
			d4.OptionRouter:           []byte(gateway),
			d4.OptionDomainNameServer: []byte(dns),
		},
	}

	hJSON, err := json.Marshal(h)
	if err != nil {
		log.Fatalf("Failed to marshal DHCPHandler: %v", err)
	}

	if err = db.KV.PutKV(Bucket, []byte(h.IP.String()), hJSON); err != nil {
		log.Fatalf("Failed to save DHCPHandler to database: %v", err)
	}
	return h
}

// Start begins the DHCP service, listening for incoming DHCP requests.
func (h *DHCPHandler) Start() error {
	if h == nil {
		return fmt.Errorf("DHCP handler is nil")
	}

	h.ctx, h.cancel = context.WithCancel(context.Background())

	if h.IP == nil {
		return fmt.Errorf("DHCPHandler IP is not set")
	}

	var err error
	addr := &net.UDPAddr{IP: h.IP, Port: 67}
	h.listener, err = net.ListenUDP("udp4", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %v", addr, err)
	}

	h.Started = true
	if err = h.UpdateDBState(); err != nil {
		return fmt.Errorf("failed to update database state: %v", err)
	}

	go func() {
		defer func() {
			h.Started = false
			if err := h.UpdateDBState(); err != nil {
				log.Printf("Failed to update database state on stop: %v", err)
			}
			if err := h.listener.Close(); err != nil {
				log.Printf("Failed to close listener: %v", err)
			}
		}()

		if err := d4.Serve(h.listener, h); err != nil {
			log.Printf("Error serving DHCP requests: %v", err)
		}
		<-h.ctx.Done()
	}()

	return nil
}

// Stop terminates the DHCP service, closing connections and updating the database.
func (h *DHCPHandler) Stop() error {
	if h.cancel == nil {
		h.Started = false
		h.UpdateDBState()
		return fmt.Errorf("server not running or improperly initialized")
	}

	h.cancel()

	if h.listener != nil {
		if err := h.listener.Close(); err != nil {
			return fmt.Errorf("failed to close listener: %v", err)
		}
	}

	select {
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout waiting for server to stop")
	case <-h.ctx.Done():
		h.Started = false
		if err := h.UpdateDBState(); err != nil {
			return fmt.Errorf("failed to update database state: %v", err)
		}
	}

	return nil
}

// GetAllLeases retrieves all current leases from the database.
func (h *DHCPHandler) GetAllLeases() (map[string]Lease, error) {
	key := []byte(h.IP.String())

	dhcpData, err := db.KV.GetKV(Bucket, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get DHCP handler from database: %v", err)
	}

	var storedHandler DHCPHandler
	if err = json.Unmarshal(dhcpData, &storedHandler); err != nil {
		return nil, fmt.Errorf("failed to unmarshal DHCP handler: %v", err)
	}

	return storedHandler.Leases, nil
}

// ServeDHCP handles incoming DHCP messages, responding appropriately based on the message type.
func (h *DHCPHandler) ServeDHCP(p d4.Packet, msgType d4.MessageType, options d4.Options) d4.Packet {
	switch msgType {
	case d4.Discover:
		bootType := "BIOS"
		if vendorOption, ok := options[d4.OptionVendorClassIdentifier]; ok {
			if string(vendorOption) == "iPXE" || string(vendorOption) == "gPXE" {
				bootType = "iPXE"
			}
		}

		filename := config.Defaults.DHCP.BiosFile
		if bootType == "iPXE" {
			filename = config.Defaults.DHCP.EFIFile
		}

		h.Options[d4.OptionBootFileName] = []byte(filename)

		if lease, ok := h.Leases[p.CHAddr().String()]; ok && lease.Reserved {
			return h.createOfferPacket(p, lease.IP, options)
		}

		if previousLeaseIP := h.findPreviousLease(p.CHAddr().String()); previousLeaseIP != nil {
			if freeIP := h.freeLease(); freeIP != nil && freeIP.Equal(previousLeaseIP) {
				return h.createOfferPacket(p, previousLeaseIP, options)
			}
		}

		if freeIP := h.freeLease(); freeIP != nil {
			return h.createOfferPacket(p, freeIP, options)
		}

		return nil

	case d4.Request:
		reqIP := h.getRequestedIP(options, p)
		macStr := p.CHAddr().String()

		if lease, exists := h.Leases[macStr]; exists {
			if lease.Reserved {
				if !reqIP.Equal(lease.IP) {
					return h.createNakPacket(p)
				}
				return h.createAckPacket(p, reqIP, options)
			}
			if !reqIP.Equal(lease.IP) {
				return h.createNakPacket(p)
			}
			return h.createAckPacket(p, reqIP, options)
		}

		if h.isRequestForAnotherServer(options) {
			return nil
		}

		// Check if the IP is within our range and not already leased
		if ipInRange := d4.IPRange(h.IPStart, reqIP); ipInRange >= 0 && ipInRange < h.LeaseRange {
			if _, exists := h.Leases[reqIP.String()]; !exists {
				h.assignNewLease(macStr, reqIP)
				if err := h.UpdateDBState(); err != nil {
					// Log the error but still ACK if we successfully assigned the lease
					log.Printf("Failed to update DB state for new lease: %v", err)
				}
				return h.createAckPacket(p, reqIP, options)
			}
		}
		return h.createNakPacket(p)

	case d4.Release, d4.Decline:
		h.removeLeaseByMAC(p.CHAddr().String())
	}

	return nil
}

// findPreviousLease searches for an existing lease by MAC address, returning IP if it's not expired.
func (h *DHCPHandler) findPreviousLease(mac string) net.IP {
	if lease, ok := h.Leases[mac]; ok {
		if time.Now().Before(lease.Expiry) {
			return lease.IP
		}
	}
	return nil
}

// createOfferPacket constructs a DHCP offer packet for a client.
func (h *DHCPHandler) createOfferPacket(p d4.Packet, freeIP net.IP, options d4.Options) d4.Packet {
	return d4.ReplyPacket(p, d4.Offer, h.IP, freeIP, h.LeaseDuration,
		h.Options.SelectOrderOrAll(options[d4.OptionParameterRequestList]))
}

// isRequestForAnotherServer checks if the DHCP request is meant for another server.
func (h *DHCPHandler) isRequestForAnotherServer(options d4.Options) bool {
	if server, ok := options[d4.OptionServerIdentifier]; ok {
		return !net.IP(server).Equal(h.IP)
	}
	return false
}

// getRequestedIP retrieves the IP address requested by the client from the DHCP options.
func (h *DHCPHandler) getRequestedIP(options d4.Options, p d4.Packet) net.IP {
	if reqIP := net.IP(options[d4.OptionRequestedIPAddress]); reqIP != nil {
		return reqIP
	}
	return net.IP(p.CIAddr())
}

// assignNewLease adds a new lease to the handler's lease map.
func (h *DHCPHandler) assignNewLease(mac string, ip net.IP) {
	newLease := Lease{
		IP:     ip,
		MAC:    mac,
		Expiry: time.Now().Add(h.LeaseDuration),
	}
	h.Leases[mac] = newLease
	if err := h.UpdateDBState(); err != nil {
		log.Printf("Failed to persist new lease: %v", err)
	}
}

// createAckPacket constructs an acknowledgment packet for a DHCP request.
func (h *DHCPHandler) createAckPacket(p d4.Packet, reqIP net.IP, options d4.Options) d4.Packet {
	return d4.ReplyPacket(p, d4.ACK, h.IP, reqIP, h.LeaseDuration,
		h.Options.SelectOrderOrAll(options[d4.OptionParameterRequestList]))
}

// createNakPacket constructs a negative acknowledgment packet for a DHCP request.
func (h *DHCPHandler) createNakPacket(p d4.Packet) d4.Packet {
	return d4.ReplyPacket(p, d4.NAK, h.IP, nil, 0, nil)
}

// removeLeaseByMAC removes a lease from the handler by MAC address.
func (h *DHCPHandler) removeLeaseByMAC(mac string) {
	if _, ok := h.Leases[mac]; ok {
		delete(h.Leases, mac)
		if err := h.UpdateDBState(); err != nil {
			log.Printf("Failed to delete lease in persistence: %v", err)
		}
	}
}

// freeLease finds an available IP address for leasing if one is available within the LeaseRange.
func (h *DHCPHandler) freeLease() net.IP {
	now := time.Now()

	if dbLeases, err := h.GetAllLeases(); err == nil {
		h.Leases = dbLeases
	}

	ipLeases := make(map[string]Lease)
	for _, lease := range h.Leases {
		ipLeases[lease.IP.String()] = lease
	}

	activeLeases := 0
	for _, lease := range h.Leases {
		if now.Before(lease.Expiry) {
			activeLeases++
		}
	}

	if activeLeases >= h.LeaseRange {
		return nil
	}

	for i := 0; i < h.LeaseRange; i++ {
		newIP := d4.IPAdd(h.IPStart, i)
		newIPStr := newIP.String()

		if lease, exists := ipLeases[newIPStr]; !exists || now.After(lease.Expiry) {
			return newIP
		}
	}

	return nil
}

// GetAllDHCPServers retrieves all DHCP server instances from the database.
func GetAllDHCPServers() ([]*DHCPHandler, error) {

	// Get or create the bucket for DHCP server data
	bucket, err := db.KV.GetOrCreateBucket(Bucket)
	if err != nil || bucket == nil {
		log.Printf("Failed to get or create bucket: %v", err)
		return nil, fmt.Errorf("failed to get or create bucket: %v", err)
	}

	// Use a read-only transaction to access the bucket
	var dhcpServers []*DHCPHandler
	err = db.KV.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(Bucket))
		if b == nil {
			log.Println("Bucket not found in transaction")
			return fmt.Errorf("bucket not found")
		}

		// Iterate over each item in the bucket
		return b.ForEach(func(k, v []byte) error {
			var dhcpServer DHCPHandler
			if err := json.Unmarshal(v, &dhcpServer); err != nil {
				log.Printf("Failed to unmarshal DHCP server data for key %s: %v", k, err)
				return nil // Skip this item but continue with others
			}
			dhcpServers = append(dhcpServers, &dhcpServer)
			return nil
		})
	})

	if err != nil {
		log.Printf("Error iterating BoltDB bucket: %v", err)
		return nil, fmt.Errorf("error iterating BoltDB bucket: %v", err)
	}

	return dhcpServers, nil
}

// GetDHCPServer fetches a specific DHCP server from the database by its key.
func GetDHCPServer(key string) (*DHCPHandler, error) {
	var dhcpServer DHCPHandler

	if bucket, err := db.KV.GetOrCreateBucket(Bucket); err != nil || bucket == nil {
		return nil, fmt.Errorf("failed to get or create bucket: %v", err)
	} else {
		err = db.KV.View(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte(Bucket))
			if b == nil {
				return fmt.Errorf("bucket not found")
			}

			data := b.Get([]byte(key))
			if data == nil {
				return fmt.Errorf("DHCP server with key %s not found", key)
			}

			if err := json.Unmarshal(data, &dhcpServer); err != nil {
				return fmt.Errorf("failed to unmarshal DHCP server data for key %s: %v", key, err)
			}

			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	return &dhcpServer, nil
}

// UpdateDBState updates the DHCP server's state in the database.
func (h *DHCPHandler) UpdateDBState() error {
	hBytes, err := json.Marshal(h)
	if err != nil {
		return fmt.Errorf("failed to marshal DHCPHandler: %v", err)
	}

	key := []byte(h.IP.String())
	if err := db.KV.PutKV(Bucket, key, hBytes); err != nil {
		return fmt.Errorf("failed to update state in database: %v", err)
	}
	return nil
}

// getLeaseByMAC looks up a lease by MAC address.
func (h *DHCPHandler) getLeaseByMAC(mac string) (Lease, error) {
	lease, exists := h.Leases[mac]
	if !exists {
		return Lease{}, fmt.Errorf("lease for MAC %s does not exist", mac)
	}
	return lease, nil
}

// GetLeaseReservation checks if a lease for a given MAC is reserved.
func (h *DHCPHandler) GetLeaseReservation(mac string) (bool, error) {
	lease, err := h.getLeaseByMAC(mac)
	if err != nil {
		return false, err
	}
	return lease.Reserved, nil
}

// SetLeaseReservation updates or creates a lease reservation for a specific MAC and IP.
func (h *DHCPHandler) SetLeaseReservation(mac string, ip string, reserved bool) error {
	macStr, err := url.QueryUnescape(mac)
	if err != nil {
		return fmt.Errorf("failed to unescape MAC address: %v", err)
	}

	ipAddr := net.ParseIP(ip)
	if ipAddr == nil {
		return fmt.Errorf("invalid IP address: %s", ip)
	}

	if !AreOnSameNetwork(ipAddr, h.IP, h.Options[d4.OptionSubnetMask]) {
		return fmt.Errorf("IP %s is not within the network %v", ipAddr, h.IP)
	}

	for _, existingLease := range h.Leases {
		if existingLease.IP.Equal(ipAddr) && existingLease.MAC != macStr {
			return fmt.Errorf("IP %s is already leased by another MAC", ip)
		}
	}

	lease, ok := h.Leases[macStr]
	if !ok {
		return fmt.Errorf("lease not found for MAC: %s", macStr)
	}

	lease.IP = ipAddr
	lease.Reserved = reserved
	lease.Expiry = time.Now().Add(h.LeaseDuration)

	h.Leases[macStr] = lease

	if err := h.UpdateDBState(); err != nil {
		return fmt.Errorf("failed to persist lease reservation change: %v", err)
	}

	return nil
}

// AreOnSameNetwork checks if two IP addresses are on the same network given a subnet mask.
func AreOnSameNetwork(ip1, ip2 net.IP, subnetMask net.IPMask) bool {
	networkAddr1 := ip1.Mask(subnetMask)
	networkAddr2 := ip2.Mask(subnetMask)
	return networkAddr1.Equal(networkAddr2)
}
