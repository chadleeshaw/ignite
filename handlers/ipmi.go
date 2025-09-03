package handlers

import (
	"fmt"
	"ignite/dhcp"
	"net"
	"net/http"

	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/redfish"
)

// SubmitIPMI handles the configuration of IPMI settings for a system, including PXE boot setup and potential reboot.
func SubmitIPMI(w http.ResponseWriter, r *http.Request) {
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

	// Retrieve DHCP handler
	dhcpHandler, err := dhcp.GetDHCPServer(tftpip)
	if err != nil {
		fmt.Printf("Unable to get DHCP server: %v\n", err)
		http.Error(w, "Unable to retrieve DHCP server", http.StatusInternalServerError)
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
			fmt.Printf("Failed to update DB state: %v\n", err)
			http.Error(w, "Failed to update DHCP lease", http.StatusInternalServerError)
			return
		}
	} else {
		http.Error(w, fmt.Sprintf("Lease not found for MAC: %s", mac), http.StatusNotFound)
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

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Actions processed for IP: %s", ip)
}
