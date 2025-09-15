package handlers

import (
	"context"
	"fmt"
	"ignite/dhcp"
	"log"
	"net"
	"net/http"

	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/redfish"
)

// IPMIHandlers handles IPMI-related requests
type IPMIHandlers struct {
	container *Container
}

// NewIPMIHandlers creates a new IPMIHandlers instance
func NewIPMIHandlers(container *Container) *IPMIHandlers {
	return &IPMIHandlers{container: container}
}

// SubmitIPMI handles IPMI submission
func (h *IPMIHandlers) SubmitIPMI(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}

	tftpip := r.Form.Get("tftpip")
	mac := r.Form.Get("mac")
	ip := r.Form.Get("ip")
	username := r.Form.Get("username")
	password := r.Form.Get("password")

	if ip == "" || username == "" || password == "" {
		http.Error(w, "IP, username, and password are required", http.StatusBadRequest)
		return
	}

	bootConfigChecked := r.Form.Get("setBootOrder") == "on"
	rebootChecked := r.Form.Get("reboot") == "on"

	ctx := r.Context()

	// Update DHCP lease with IPMI configuration
	if err := h.updateDHCPLeaseWithIPMI(ctx, tftpip, mac, ip, username, bootConfigChecked, rebootChecked); err != nil {
		log.Printf("Failed to update DHCP lease: %v", err)
		// Continue with IPMI operations even if lease update fails
	}

	// Configure Redfish client for IPMI operations
	clientConfig := gofish.ClientConfig{
		Endpoint: fmt.Sprintf("https://%s/redfish/v1", ip),
		Username: username,
		Password: password,
		Insecure: true,
	}

	client, err := gofish.Connect(clientConfig)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to connect to Redfish service: %s", err.Error()), http.StatusInternalServerError)
		return
	}
	defer client.Logout()

	// Retrieve system information
	service := client.Service
	systems, err := service.Systems()
	if err != nil || len(systems) == 0 {
		http.Error(w, "No systems found or error retrieving systems", http.StatusNotFound)
		return
	}

	system := systems[0]
	var bootConfig = redfish.Boot{
		BootSourceOverrideTarget:  redfish.PxeBootSourceOverrideTarget,
		BootSourceOverrideEnabled: redfish.OnceBootSourceOverrideEnabled,
	}

	// Set PXE boot if checked
	if bootConfigChecked {
		if err := system.SetBoot(bootConfig); err != nil {
			http.Error(w, fmt.Sprintf("Failed to set PXE boot: %s", err.Error()), http.StatusInternalServerError)
			return
		}
	}

	// Reboot system if checked
	if rebootChecked {
		if err := system.Reset(redfish.ForceRestartResetType); err != nil {
			http.Error(w, fmt.Sprintf("Failed to reboot system: %s", err.Error()), http.StatusInternalServerError)
			return
		}
	}

	// Redirect to DHCP page after successful execution
	http.Redirect(w, r, "/dhcp", http.StatusSeeOther)
}

// updateDHCPLeaseWithIPMI updates the DHCP lease with IPMI configuration
func (h *IPMIHandlers) updateDHCPLeaseWithIPMI(ctx context.Context, tftpip, mac, ip, username string, pxeboot, reboot bool) error {
	// Find server by IP to get server ID
	networkIP := net.ParseIP(tftpip)
	if networkIP == nil {
		return fmt.Errorf("invalid network IP: %s", tftpip)
	}

	servers, err := h.container.ServerService.GetAllServers(ctx)
	if err != nil {
		return fmt.Errorf("failed to get servers: %w", err)
	}

	var serverID string
	for _, server := range servers {
		if server.IP.Equal(networkIP) {
			serverID = server.ID
			break
		}
	}

	if serverID == "" {
		return fmt.Errorf("server not found for IP: %s", tftpip)
	}

	// Get lease by MAC
	lease, err := h.container.LeaseService.GetLeaseByMAC(ctx, mac)
	if err != nil || lease == nil {
		return fmt.Errorf("lease not found for MAC %s: %w", mac, err)
	}

	// Update lease with IPMI configuration
	lease.IPMI = dhcp.IPMI{
		PXEBoot:  pxeboot,
		Reboot:   reboot,
		IP:       net.ParseIP(ip),
		Username: username,
		// Password is not stored for security reasons
	}

	// Save the updated lease
	if err := h.container.LeaseService.UpdateLease(ctx, lease); err != nil {
		return fmt.Errorf("failed to save lease with IPMI config: %w", err)
	}

	return nil
}
