package dhcp

import (
	"fmt"
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
	ID             string            `json:"id"`
	IP             net.IP            `json:"ip"`
	MAC            string            `json:"mac"`
	Expiry         time.Time         `json:"expiry"`
	Reserved       bool              `json:"reserved"`
	ServerID       string            `json:"server_id"`
	Menu           BootMenu          `json:"menu"`
	IPMI           IPMI              `json:"ipmi"`
	State          LeaseState        `json:"state"`
	StateUpdatedAt time.Time         `json:"state_updated_at"`
	LastSeen       time.Time         `json:"last_seen"`
	StateHistory   []StateTransition `json:"state_history"`
}

// StateTransition represents a state change event
type StateTransition struct {
	FromState LeaseState `json:"from_state"`
	ToState   LeaseState `json:"to_state"`
	Timestamp time.Time  `json:"timestamp"`
	Source    string     `json:"source"` // "dhcp", "pxe", "imaging", "manual", "heartbeat"
}

// LeaseState is the provisioning state of a lease.
type LeaseState string

// LeaseState constants
const (
	StateAssigned     LeaseState = "assigned"      // DHCP lease created, waiting for PXE request
	StatePXERequested LeaseState = "pxe_requested" // Machine requested PXE boot configuration
	StateBooting      LeaseState = "booting"       // PXE config delivered, machine is booting
	StateImaging      LeaseState = "imaging"       // OS installation/imaging in progress
	StateImaged       LeaseState = "imaged"        // OS imaging completed successfully
	StateConfiguring  LeaseState = "configuring"   // Post-install configuration running
	StateComplete     LeaseState = "complete"      // Machine fully provisioned and operational
	StateFailed       LeaseState = "failed"        // Error occurred in any stage
	StateOffline      LeaseState = "offline"       // Machine hasn't checked in recently
)

// NOTE: GetStateBadgeClass (CSS classes for state display) was removed from
// this package — presentation belongs in handlers, not in the dhcp domain
// model. handlers/dhcp.go must carry its own copy (integration item).

// GetStateDisplayName returns a human-readable state name
func (l *Lease) GetStateDisplayName() string {
	switch l.State {
	case StateAssigned:
		return "Assigned"
	case StatePXERequested:
		return "PXE Requested"
	case StateBooting:
		return "Booting"
	case StateImaging:
		return "Imaging"
	case StateImaged:
		return "Imaged"
	case StateConfiguring:
		return "Configuring"
	case StateComplete:
		return "Complete"
	case StateFailed:
		return "Failed"
	case StateOffline:
		return "Offline"
	default:
		return "Unknown"
	}
}

// UpdateState transitions the lease to a new state and records the transition
func (l *Lease) UpdateState(newState LeaseState, source string) {
	now := time.Now()
	if l.State != newState {
		transition := StateTransition{
			FromState: l.State,
			ToState:   newState,
			Timestamp: now,
			Source:    source,
		}

		l.StateHistory = append(l.StateHistory, transition)
		l.State = newState
		l.StateUpdatedAt = now
	}
	l.LastSeen = now
}

// IsActive returns true if the lease is in an active state.
// A fully provisioned machine (StateComplete) is not "active" for the
// purposes of heartbeat/offline tracking.
func (l *Lease) IsActive() bool {
	return l.State != StateOffline && l.State != StateFailed && l.State != StateComplete
}

// BootMenu contains PXE boot configuration
type BootMenu struct {
	Filename      string `json:"filename"`
	OS            string `json:"os"`
	Version       string `json:"version"`
	TemplateType  string `json:"template_type"`
	TemplateName  string `json:"template_name"`
	Hostname      string `json:"hostname"`
	IP            net.IP `json:"ip"`
	Subnet        net.IP `json:"subnet"`
	Gateway       net.IP `json:"gateway"`
	DNS           net.IP `json:"dns"`
	KernelOptions string `json:"kernel_options"`
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
	ip4 := s.IP.To4()
	mask := net.IPMask(s.Options.SubnetMask.To4())
	if ip4 == nil || mask == nil {
		return nil
	}
	return ip4.Mask(mask)
}

// IsInRange checks if an IP is within the server's lease range
func (s *Server) IsInRange(ip net.IP) bool {
	start := s.IPStart.To4()
	ip4 := ip.To4()
	if start == nil || ip4 == nil {
		return false
	}

	startInt, err := ipToInt(start)
	if err != nil {
		return false
	}
	ipInt, err := ipToInt(ip4)
	if err != nil {
		return false
	}
	if s.LeaseRange <= 0 {
		return false
	}

	// Use 64-bit math so a large LeaseRange can't wrap past 255.255.255.255.
	end := uint64(startInt) + uint64(s.LeaseRange)
	return uint64(ipInt) >= uint64(startInt) && uint64(ipInt) < end
}

// ipToInt converts an IPv4 address to a uint32.
// It returns an error instead of panicking on unexpected input.
func ipToInt(ip net.IP) (uint32, error) {
	ip4 := ip.To4()
	if ip4 == nil {
		return 0, fmt.Errorf("not an IPv4 address")
	}
	return uint32(ip4[0])<<24 | uint32(ip4[1])<<16 | uint32(ip4[2])<<8 | uint32(ip4[3]), nil
}
