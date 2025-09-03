package handlers

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"

	"ignite/dhcp"
	"ignite/handlers"
	"ignite/internal/validation"
)

// getQueryParam retrieves a specific query parameter from the HTTP request.
func getQueryParam(r *http.Request, param string) (string, error) {
	value := r.URL.Query().Get(param)
	if value == "" {
		return "", fmt.Errorf("missing %s parameter", param)
	}
	return value, nil
}

// getBoltDHCPServer retrieves a DHCP server from the database by its network identifier.
func (h *Handlers) getBoltDHCPServer(r *http.Request) (*dhcp.DHCPHandler, error) {
	network, err := getQueryParam(r, "network")
	if err != nil {
		return nil, fmt.Errorf("error getting network param: %w", err)
	}

	data, err := h.DB.GetKV(h.GetDBBucket(), []byte(network))
	if err != nil {
		return nil, fmt.Errorf("error getting DHCP handler from database: %w", err)
	}

	var dhcpHandler dhcp.DHCPHandler
	if err := json.Unmarshal(data, &dhcpHandler); err != nil {
		return nil, fmt.Errorf("unable to unmarshal DHCP handler: %w", err)
	}

	return &dhcpHandler, nil
}

// extractAndValidateIPData validates and extracts IP address data from form values.
func (h *Handlers) extractAndValidateIPData(r *http.Request) (net.IP, net.IP, net.IP, net.IP, net.IP, error) {
	fields := []string{"network", "subnet", "gateway", "dns", "startIP"}
	ips := make([]net.IP, len(fields))

	for i, field := range fields {
		value := r.Form.Get(field)
		if err := validation.ValidateRequired(field, value); err != nil {
			return nil, nil, nil, nil, nil, err
		}
		if err := validation.ValidateIP(value); err != nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("invalid %s: %w", field, err)
		}
		ips[i] = net.ParseIP(value)
	}

	return ips[0], ips[1], ips[2], ips[3], ips[4], nil
}

// newDHCPModal creates data for the DHCP modal
func (h *Handlers) newDHCPModal() map[string]any {
	return map[string]any{
		"Networks": h.getNetworkItems(),
	}
}

// getNetworkItems returns network interface information
func (h *Handlers) getNetworkItems() []map[string]interface{} {
	var networks []map[string]interface{}
	
	interfaces, err := net.Interfaces()
	if err != nil {
		h.Logger.Warn("Failed to get network interfaces", "error", err.Error())
		return networks
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue // Skip down or loopback interfaces
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil { // IPv4 only
					networks = append(networks, map[string]interface{}{
						"Name": iface.Name,
						"IP":   ipnet.IP.String(),
						"CIDR": ipnet.String(),
					})
				}
			}
		}
	}

	return networks
}

// newReserveModal creates data for the reserve lease modal
func (h *Handlers) newReserveModal(w http.ResponseWriter, r *http.Request) (map[string]any, error) {
	handler, err := h.getBoltDHCPServer(r)
	if err != nil {
		return nil, fmt.Errorf("error getting dhcp server for reserve modal: %w", err)
	}

	mac := r.URL.Query().Get("mac")
	decodedMac, err := url.QueryUnescape(mac)
	if err != nil {
		return nil, err
	}

	if lease, exists := handler.Leases[decodedMac]; exists {
		return map[string]any{
			"tftpip": handler.IP.String(),
			"mac":    lease.MAC,
			"ip":     lease.IP.String(),
			"static": lease.Reserved,
		}, nil
	}

	return nil, fmt.Errorf("lease not found for MAC: %s", mac)
}

// newBootModal creates data for the boot menu modal
func (h *Handlers) newBootModal(w http.ResponseWriter, r *http.Request) (map[string]any, error) {
	handler, err := h.getBoltDHCPServer(r)
	if err != nil {
		return nil, fmt.Errorf("error getting dhcp server for boot modal: %w", err)
	}

	mac := r.URL.Query().Get("mac")
	decodedMac, err := url.QueryUnescape(mac)
	if err != nil {
		return nil, err
	}

	if lease, exists := handler.Leases[decodedMac]; exists {
		return map[string]any{
			"tftpip":        handlers.CheckEmpty(handler.IP),
			"mac":           handlers.CheckEmpty(lease.MAC),
			"hostname":      handlers.CheckEmpty(lease.Menu.Hostname),
			"os":            handlers.CheckEmpty(lease.Menu.OS),
			"typeSelect":    handlers.CheckEmpty(lease.Menu.Template_Type),
			"template_name": handlers.CheckEmpty(lease.Menu.Template_Name),
			"ip":            handlers.CheckEmpty(lease.Menu.IP),
			"subnet":        handlers.CheckEmpty(lease.Menu.Subnet),
			"gateway":       handlers.CheckEmpty(lease.Menu.Gateway),
			"dns":           handlers.CheckEmpty(lease.Menu.DNS),
		}, nil
	}

	return nil, fmt.Errorf("lease not found for MAC: %s", mac)
}

// newIPMIModal creates data for the IPMI modal
func (h *Handlers) newIPMIModal(w http.ResponseWriter, r *http.Request) (map[string]any, error) {
	handler, err := h.getBoltDHCPServer(r)
	if err != nil {
		return nil, fmt.Errorf("error getting dhcp server for ipmi modal: %w", err)
	}

	mac := r.URL.Query().Get("mac")
	decodedMac, err := url.QueryUnescape(mac)
	if err != nil {
		return nil, err
	}

	if lease, exists := handler.Leases[decodedMac]; exists {
		return map[string]any{
			"tftpip":   handlers.CheckEmpty(handler.IP),
			"mac":      handlers.CheckEmpty(lease.MAC),
			"pxeboot":  lease.IPMI.Pxeboot,
			"reboot":   lease.IPMI.Reboot,
			"ipmi_ip":  handlers.CheckEmpty(lease.IPMI.IP),
			"username": handlers.CheckEmpty(lease.IPMI.Username),
		}, nil
	}

	return nil, fmt.Errorf("lease not found for MAC: %s", mac)
}

// newUploadModal creates data for the upload modal
func (h *Handlers) newUploadModal(w http.ResponseWriter, r *http.Request) map[string]any {
	return map[string]any{
		"upload_dir": h.GetTFTPDir(),
	}
}

// SubmitBootMenu handles boot menu configuration submissions
func (h *Handlers) SubmitBootMenu(w http.ResponseWriter, r *http.Request) {
	// This would need to be implemented based on the existing bootmenu handler
	// For now, return a placeholder
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte("Boot menu submission not yet implemented"))
}

// SubmitIPMI handles IPMI configuration submissions
func (h *Handlers) SubmitIPMI(w http.ResponseWriter, r *http.Request) {
	// This would need to be implemented based on the existing IPMI handler
	// For now, return a placeholder
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte("IPMI submission not yet implemented"))
}
