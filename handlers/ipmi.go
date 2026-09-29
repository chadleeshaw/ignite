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
		HandleError(w, r, NewValidationError("Failed to parse form data", "The submitted form could not be parsed"))
		return
	}

	tftpip := r.Form.Get("tftpip")
	mac := r.Form.Get("mac")
	ip := r.Form.Get("ip")
	username := r.Form.Get("username")
	password := r.Form.Get("password")

	if ip == "" || username == "" || password == "" {
		HandleError(w, r, NewValidationError("IP, username, and password are required", "IP, username, and password are required"))
		return
	}

	// The BMC IP ends up in the Redfish endpoint URL — validate it up front.
	if parsedIP := net.ParseIP(ip); parsedIP == nil {
		HandleError(w, r, NewValidationError(
			fmt.Sprintf("Invalid BMC IP address: %q", ip),
			"The BMC IP address is invalid",
		))
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
		HandleError(w, r, NewInternalError(
			fmt.Sprintf("Failed to connect to Redfish service: %s", err.Error()),
			"Unable to connect to the Redfish service on the target system",
		))
		return
	}
	defer client.Logout()

	// Retrieve system information
	service := client.Service
	systems, err := service.Systems()
	if err != nil || len(systems) == 0 {
		HandleError(w, r, NewNotFoundError("No systems found or error retrieving systems", "No manageable systems were found on the target"))
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
			HandleError(w, r, NewInternalError(
				fmt.Sprintf("Failed to set PXE boot: %s", err.Error()),
				"Unable to set the PXE boot order on the target system",
			))
			return
		}
	}

	// Reboot system if checked
	if rebootChecked {
		if err := system.Reset(redfish.ForceRestartResetType); err != nil {
			HandleError(w, r, NewInternalError(
				fmt.Sprintf("Failed to reboot system: %s", err.Error()),
				"Unable to reboot the target system",
			))
			return
		}
	}

	// Redirect to DHCP page after successful execution
	http.Redirect(w, r, "/dhcp", http.StatusSeeOther)
}

// updateDHCPLeaseWithIPMI updates the DHCP lease with IPMI configuration
func (h *IPMIHandlers) updateDHCPLeaseWithIPMI(ctx context.Context, tftpip, mac, ip, username string, pxeboot, reboot bool) error {
	// Find server by IP via the shared lookup helper (validates the TFTP IP too).
	if _, err := findServerByIP(ctx, h.container.ServerService, tftpip); err != nil {
		return err
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
