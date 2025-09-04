package handlers

import (
	"fmt"
	"net"
	"net/http"

	"ignite/dhcp"
	"ignite/internal/errors"
	"ignite/internal/validation"

	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/redfish"
)

// SubmitIPMI handles the configuration of IPMI settings for a system, including PXE boot setup and potential reboot.
func (h *Handlers) SubmitIPMI(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("parse_form", err))
		return
	}

	tftpip := r.Form.Get("tftpip")
	mac := r.Form.Get("mac")
	ip := r.Form.Get("ip")
	username := r.Form.Get("username")
	password := r.Form.Get("password")

	for k, v := range map[string]string{"ip": ip, "username": username, "password": password} {
		if err := validation.ValidateRequired(k, v); err != nil {
			errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("validate_required", err))
			return
		}
	}

	bootConfigChecked := r.Form.Get("setBootOrder") == "on"
	rebootChecked := r.Form.Get("reboot") == "on"

	// Retrieve DHCP handler
	dhcpHandler, err := h.App.GetDhcpServer(tftpip)
	if err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewDHCPError("get_dhcp_server", err))
		return
	}

	// Update lease in DHCP handler
	if lease, ok := dhcpHandler.Leases[mac]; ok {
		lease.IPMI = dhcp.IPMI{
			Pxeboot:  bootConfigChecked,
			Reboot:   rebootChecked,
			IP:       net.ParseIP(ip),
			Username: username,
		}
		dhcpHandler.Leases[mac] = lease
		if err := dhcpHandler.UpdateDBState(); err != nil {
			errors.HandleHTTPError(w, h.Logger, errors.NewDatabaseError("update_lease", err))
			return
		}
	} else {
		err := fmt.Errorf("lease not found for MAC: %s", mac)
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("lease_not_found", err))
		return
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
		err = fmt.Errorf("failed to connect to redfish: %w", err)
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("redfish_connect", err))
		return
	}
	defer client.Logout()

	// Retrieve system information
	service := client.Service
	systems, err := service.Systems()
	if err != nil || len(systems) == 0 {
		err = fmt.Errorf("failed to retrieve systems: %w", err)
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("redfish_get_systems", err))
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
			err = fmt.Errorf("failed to set boot config: %w", err)
			errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("redfish_set_boot", err))
			return
		}
	}

	// Reboot system if checked
	if rebootChecked {
		if err := system.Reset(redfish.ForceRestartResetType); err != nil {
			err = fmt.Errorf("failed to reset system: %w", err)
			errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("redfish_reset", err))
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Actions processed for IP: %s", ip)
}
