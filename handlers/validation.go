package handlers

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
)

// IPValidator provides IP address validation functionality
type IPValidator struct{}

// NewIPValidator creates a new IP validator
func NewIPValidator() *IPValidator {
	return &IPValidator{}
}

// ValidateIPAddress validates an IP address string
func (v *IPValidator) ValidateIPAddress(ip string) error {
	if ip == "" {
		return fmt.Errorf("IP address cannot be empty")
	}

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return fmt.Errorf("invalid IP address format: %s", ip)
	}

	// Check if it's IPv4
	if parsedIP.To4() == nil {
		return fmt.Errorf("only IPv4 addresses are supported: %s", ip)
	}

	return nil
}

// ValidateIPRange validates an IP range (start-end format)
func (v *IPValidator) ValidateIPRange(rangeStr string) error {
	if rangeStr == "" {
		return fmt.Errorf("IP range cannot be empty")
	}

	parts := strings.Split(rangeStr, "-")
	if len(parts) != 2 {
		return fmt.Errorf("IP range must be in format 'start-end': %s", rangeStr)
	}

	startIP := strings.TrimSpace(parts[0])
	endIP := strings.TrimSpace(parts[1])

	// Validate both IPs
	if err := v.ValidateIPAddress(startIP); err != nil {
		return fmt.Errorf("invalid start IP in range: %v", err)
	}

	if err := v.ValidateIPAddress(endIP); err != nil {
		return fmt.Errorf("invalid end IP in range: %v", err)
	}

	// Check if start IP is less than or equal to end IP
	start := net.ParseIP(startIP).To4()
	end := net.ParseIP(endIP).To4()

	if compareIPs(start, end) > 0 {
		return fmt.Errorf("start IP (%s) must be less than or equal to end IP (%s)", startIP, endIP)
	}

	return nil
}

// ValidateSubnet validates a subnet in CIDR notation
func (v *IPValidator) ValidateSubnet(subnet string) error {
	if subnet == "" {
		return fmt.Errorf("subnet cannot be empty")
	}

	_, network, err := net.ParseCIDR(subnet)
	if err != nil {
		return fmt.Errorf("invalid subnet format: %s", subnet)
	}

	// Check if it's IPv4
	if network.IP.To4() == nil {
		return fmt.Errorf("only IPv4 subnets are supported: %s", subnet)
	}

	return nil
}

// ValidateIPInSubnet checks if an IP address is within a subnet
func (v *IPValidator) ValidateIPInSubnet(ip, subnet string) error {
	if err := v.ValidateIPAddress(ip); err != nil {
		return err
	}

	if err := v.ValidateSubnet(subnet); err != nil {
		return err
	}

	_, network, _ := net.ParseCIDR(subnet)
	ipAddr := net.ParseIP(ip)

	if !network.Contains(ipAddr) {
		return fmt.Errorf("IP address %s is not within subnet %s", ip, subnet)
	}

	return nil
}

// ValidateRangeInSubnet checks if an IP range is within a subnet
func (v *IPValidator) ValidateRangeInSubnet(rangeStr, subnet string) error {
	if err := v.ValidateIPRange(rangeStr); err != nil {
		return err
	}

	if err := v.ValidateSubnet(subnet); err != nil {
		return err
	}

	parts := strings.Split(rangeStr, "-")
	startIP := strings.TrimSpace(parts[0])
	endIP := strings.TrimSpace(parts[1])

	if err := v.ValidateIPInSubnet(startIP, subnet); err != nil {
		return fmt.Errorf("start IP not in subnet: %v", err)
	}

	if err := v.ValidateIPInSubnet(endIP, subnet); err != nil {
		return fmt.Errorf("end IP not in subnet: %v", err)
	}

	return nil
}

// ValidateMACAddress validates a MAC address
func (v *IPValidator) ValidateMACAddress(mac string) error {
	if mac == "" {
		return fmt.Errorf("MAC address cannot be empty")
	}

	// Support both colon and hyphen separated formats, but not mixed
	colonRegex := regexp.MustCompile(`^([0-9A-Fa-f]{2}:){5}([0-9A-Fa-f]{2})$`)
	hyphenRegex := regexp.MustCompile(`^([0-9A-Fa-f]{2}-){5}([0-9A-Fa-f]{2})$`)

	if !colonRegex.MatchString(mac) && !hyphenRegex.MatchString(mac) {
		return fmt.Errorf("invalid MAC address format: %s (expected format: XX:XX:XX:XX:XX:XX or XX-XX-XX-XX-XX-XX)", mac)
	}

	return nil
}

// ValidatePort validates a port number
func (v *IPValidator) ValidatePort(port string) error {
	if port == "" {
		return fmt.Errorf("port cannot be empty")
	}

	portNum, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("invalid port number: %s", port)
	}

	if portNum < 1 || portNum > 65535 {
		return fmt.Errorf("port number must be between 1 and 65535: %d", portNum)
	}

	return nil
}

// ValidateHostname validates a hostname
func (v *IPValidator) ValidateHostname(hostname string) error {
	if hostname == "" {
		return fmt.Errorf("hostname cannot be empty")
	}

	if len(hostname) > 253 {
		return fmt.Errorf("hostname too long (max 253 characters): %s", hostname)
	}

	// Hostname regex (simplified)
	hostnameRegex := regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?)*$`)
	if !hostnameRegex.MatchString(hostname) {
		return fmt.Errorf("invalid hostname format: %s", hostname)
	}

	return nil
}

// DHCPConfigValidator validates DHCP configuration
type DHCPConfigValidator struct {
	ipValidator *IPValidator
}

// NewDHCPConfigValidator creates a new DHCP config validator
func NewDHCPConfigValidator() *DHCPConfigValidator {
	return &DHCPConfigValidator{
		ipValidator: NewIPValidator(),
	}
}

// ValidateDHCPConfig validates a complete DHCP configuration
func (v *DHCPConfigValidator) ValidateDHCPConfig(config map[string]string) ValidationErrors {
	errors := make(ValidationErrors)

	// Validate subnet
	if subnet, ok := config["subnet"]; ok {
		if err := v.ipValidator.ValidateSubnet(subnet); err != nil {
			errors.Add("subnet", err.Error())
		}
	} else {
		errors.Add("subnet", "subnet is required")
	}

	// Validate IP range
	if rangeStr, ok := config["range"]; ok {
		if err := v.ipValidator.ValidateIPRange(rangeStr); err != nil {
			errors.Add("range", err.Error())
		} else if subnet, ok := config["subnet"]; ok {
			// Validate range is within subnet
			if err := v.ipValidator.ValidateRangeInSubnet(rangeStr, subnet); err != nil {
				errors.Add("range", err.Error())
			}
		}
	} else {
		errors.Add("range", "IP range is required")
	}

	// Validate router (gateway) if provided
	if router, ok := config["router"]; ok && router != "" {
		if err := v.ipValidator.ValidateIPAddress(router); err != nil {
			errors.Add("router", err.Error())
		} else if subnet, ok := config["subnet"]; ok {
			// Validate router is within subnet
			if err := v.ipValidator.ValidateIPInSubnet(router, subnet); err != nil {
				errors.Add("router", err.Error())
			}
		}
	}

	// Validate DNS servers if provided
	if dns, ok := config["dns"]; ok && dns != "" {
		dnsServers := strings.Split(dns, ",")
		for i, server := range dnsServers {
			server = strings.TrimSpace(server)
			if err := v.ipValidator.ValidateIPAddress(server); err != nil {
				errors.Add("dns", fmt.Sprintf("DNS server %d: %v", i+1, err))
			}
		}
	}

	// Validate lease time if provided
	if leaseTime, ok := config["lease_time"]; ok && leaseTime != "" {
		if _, err := strconv.Atoi(leaseTime); err != nil {
			errors.Add("lease_time", "lease time must be a valid number (seconds)")
		}
	}

	return errors
}

// ValidateReservation validates a DHCP reservation
func (v *DHCPConfigValidator) ValidateReservation(ip, mac string) ValidationErrors {
	errors := make(ValidationErrors)

	if err := v.ipValidator.ValidateIPAddress(ip); err != nil {
		errors.Add("ip", err.Error())
	}

	if err := v.ipValidator.ValidateMACAddress(mac); err != nil {
		errors.Add("mac", err.Error())
	}

	return errors
}

// Helper function to compare IPv4 addresses
func compareIPs(ip1, ip2 net.IP) int {
	ip1 = ip1.To4()
	ip2 = ip2.To4()

	for i := 0; i < 4; i++ {
		if ip1[i] < ip2[i] {
			return -1
		} else if ip1[i] > ip2[i] {
			return 1
		}
	}
	return 0
}
