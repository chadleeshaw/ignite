package handlers

import (
	"context"
	"errors"
	"fmt"
	"ignite/dhcp"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/stmcginnis/gofish/redfish"
)

// IPMIHandlers handles IPMI-related requests
type IPMIHandlers struct {
	container *Container
	dial      bmcDialer
}

// NewIPMIHandlers creates a new IPMIHandlers instance.
// TLS verification follows config (on by default). Tests replace dial.
func NewIPMIHandlers(container *Container) *IPMIHandlers {
	return &IPMIHandlers{container: container, dial: dialGofish}
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
	systemID := strings.TrimSpace(r.Form.Get("system_id"))
	if systemID == "" {
		systemID = h.configuredSystemID()
	}

	ctx := r.Context()

	// Update DHCP lease with IPMI configuration
	if err := h.updateDHCPLeaseWithIPMI(ctx, tftpip, mac, ip, username, bootConfigChecked, rebootChecked); err != nil {
		log.Printf("Failed to update DHCP lease: %v", err)
		// Continue with IPMI operations even if lease update fails
	}

	bmcCtx, cancel := context.WithTimeout(ctx, bmcRequestTimeout)
	defer cancel()

	dial := h.dial
	if dial == nil {
		dial = dialGofish
	}
	session, err := dial(bmcCtx, fmt.Sprintf("https://%s/redfish/v1", ip), username, password, h.tlsInsecure())
	if err != nil {
		HandleError(w, r, NewInternalError(
			fmt.Sprintf("Failed to connect to Redfish service: %s", err.Error()),
			"Unable to connect to the Redfish service on the target system",
		))
		return
	}
	defer session.Logout()

	systems, err := session.Systems()
	if err != nil {
		HandleError(w, r, NewInternalError(
			fmt.Sprintf("Failed to list Redfish systems: %s", err.Error()),
			"Unable to list systems on the target",
		))
		return
	}
	system, err := selectBMCSystem(systems, systemID)
	if err != nil {
		h.writeBMCSelectError(w, r, err)
		return
	}

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

	// Power on a host that is off. ForceRestart only when it is already on.
	if rebootChecked {
		resetType, err := resetTypeForPower(system.PowerState())
		if err != nil {
			HandleError(w, r, NewAppError(
				ErrorTypeConflict,
				err.Error(),
				"The host power state is not On or Off, so it was not restarted",
				http.StatusConflict,
			))
			return
		}
		if err := system.Reset(resetType); err != nil {
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

func (h *IPMIHandlers) tlsInsecure() bool {
	if h == nil || h.container == nil || h.container.Config == nil {
		return false
	}
	return h.container.Config.BMC.TLSInsecure
}

func (h *IPMIHandlers) configuredSystemID() string {
	if h == nil || h.container == nil || h.container.Config == nil {
		return ""
	}
	return h.container.Config.BMC.SystemID
}

func (h *IPMIHandlers) writeBMCSelectError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, errNoBMCSystems):
		HandleError(w, r, NewNotFoundError(err.Error(), "No manageable systems were found on the target"))
	case errors.Is(err, errBMCSystemNotFound):
		HandleError(w, r, NewNotFoundError(err.Error(), "The requested Redfish system was not found"))
	case errors.Is(err, errAmbiguousBMCSystems):
		HandleError(w, r, NewAppError(
			ErrorTypeConflict,
			err.Error(),
			"This BMC has more than one system. Set system_id or BMC_SYSTEM_ID.",
			http.StatusConflict,
		))
	default:
		HandleError(w, r, NewInternalError(err.Error(), "Unable to choose a Redfish system"))
	}
}
