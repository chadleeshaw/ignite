package dhcp

import (
	"net"
	"time"
)

// Server represents a DHCP server configuration and state
type Server struct {
	ID            string        `json:"id"`
	IP            net.IP        `json:"ip"`
	Options       DHCPOptions   `json:"options"`
	IPStart       net.IP        `json:"ip_start"`
	Started       bool          `json:"started"`
	LeaseRange    int           `json:"lease_range"`
	LeaseDuration time.Duration `json:"lease_duration"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

// DHCPOptions represents DHCP configuration options
type DHCPOptions struct {
	SubnetMask net.IP `json:"subnet_mask"`
	Gateway    net.IP `json:"gateway"`
	DNS        net.IP `json:"dns"`
	TFTPServer net.IP `json:"tftp_server"`
}

// Lease represents an IP lease assignment
type Lease struct {
	ID       string    `json:"id"`
	IP       net.IP    `json:"ip"`
	MAC      string    `json:"mac"`
	Expiry   time.Time `json:"expiry"`
	Reserved bool      `json:"reserved"`
	ServerID string    `json:"server_id"`
	Menu     BootMenu  `json:"menu"`
	IPMI     IPMI      `json:"ipmi"`
}

// BootMenu contains PXE boot configuration
type BootMenu struct {
	Filename     string `json:"filename"`
	OS           string `json:"os"`
	TemplateType string `json:"template_type"`
	TemplateName string `json:"template_name"`
	Hostname     string `json:"hostname"`
	IP           net.IP `json:"ip"`
	Subnet       net.IP `json:"subnet"`
	Gateway      net.IP `json:"gateway"`
	DNS          net.IP `json:"dns"`
}

// IPMI holds IPMI configuration for remote server management
type IPMI struct {
	PXEBoot  bool   `json:"pxe_boot"`
	Reboot   bool   `json:"reboot"`
	IP       net.IP `json:"ip"`
	Username string `json:"username"`
	// Password is not stored for security reasons
}

// IsExpired checks if the lease has expired
func (l *Lease) IsExpired() bool {
	return time.Now().After(l.Expiry)
}

// Extend extends the lease expiry time
func (l *Lease) Extend(duration time.Duration) {
	l.Expiry = time.Now().Add(duration)
}

// GetNetworkAddress returns the network address for the server
func (s *Server) GetNetworkAddress() net.IP {
	return s.IP.Mask(net.IPMask(s.Options.SubnetMask))
}

// IsInRange checks if an IP is within the server's lease range
func (s *Server) IsInRange(ip net.IP) bool {
	if s.IPStart == nil || len(s.IPStart) != len(ip) {
		return false
	}

	startInt := ipToInt(s.IPStart)
	ipInt := ipToInt(ip)
	endInt := startInt + uint32(s.LeaseRange)

	return ipInt >= startInt && ipInt < endInt
}

// Helper function to convert IP to uint32
func ipToInt(ip net.IP) uint32 {
	if len(ip) == 16 {
		ip = ip[12:16] // Convert IPv6 to IPv4 if needed
	}
	return uint32(ip[0])<<24 + uint32(ip[1])<<16 + uint32(ip[2])<<8 + uint32(ip[3])
}
