package handlers

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"

	"ignite/dhcp"
	"html/template"
	"ignite/handlers"
	"ignite/internal/validation"
	"os"
	"path/filepath"
	"strings"

	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/redfish"
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

// SubmitBootMenu handles the submission of PXE boot menu configurations.
func (h *Handlers) SubmitBootMenu(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}

	formData := map[string]string{
		"tftpip":        r.Form.Get("tftpip"),
		"mac":           r.Form.Get("mac"),
		"os":            r.Form.Get("os"),
		"typeSelect":    r.Form.Get("typeSelect"),
		"template_name": r.Form.Get("template_name"),
		"hostname":      r.Form.Get("hostname"),
		"ip":            r.Form.Get("ip"),
		"subnet":        r.Form.Get("subnet"),
		"gateway":       r.Form.Get("gateway"),
		"dns":           r.Form.Get("dns"),
	}

	for key, value := range formData {
		if value == "" {
			http.Error(w, fmt.Sprintf("Missing required field: %s", key), http.StatusBadRequest)
			return
		}
	}

	buildpxe := fmt.Sprintf("pxelinux.cfg/01-%s", strings.ReplaceAll(formData["mac"], ":", "-"))
	pxefile := filepath.Join(h.GetTFTPDir(), buildpxe)
	pxetempl := fmt.Sprintf("%s/templates/bootmenu/default.templ", h.GetProvisionDir())

	bootMenu := dhcp.BootMenu{
		Filename:      pxefile,
		OS:            formData["os"],
		Template_Type: formData["typeSelect"],
		Template_Name: formData["template_name"],
		Hostname:      formData["hostname"],
		IP:            net.ParseIP(formData["ip"]),
		Subnet:        net.ParseIP(formData["subnet"]),
		Gateway:       net.ParseIP(formData["gateway"]),
		DNS:           net.ParseIP(formData["dns"]),
	}

	buildconfig := fmt.Sprintf("configs/%s/%s", formData["typeSelect"], strings.ReplaceAll(formData["mac"], ":", "-"))
	configFile := filepath.Join(h.GetProvisionDir(), buildconfig)
	templBuild := fmt.Sprintf("templates/%s/%s", formData["typeSelect"], formData["template_name"])
	configTempl := filepath.Join(h.GetProvisionDir(), templBuild)

	pxedata := generateBootData(formData, configFile)

	if err := writeTemplateToDisk(pxetempl, pxefile, pxedata); err != nil {
		http.Error(w, fmt.Sprintf("Error writing PXE menu: %v", err), http.StatusInternalServerError)
		return
	}

	if err := writeTemplateToDisk(configTempl, configFile, formData); err != nil {
		http.Error(w, fmt.Sprintf("Error writing config file: %v", err), http.StatusInternalServerError)
		return
	}

	if err := h.updateDHCPLease(formData["tftpip"], formData["mac"], bootMenu); err != nil {
		fmt.Printf("Failed to update DHCP lease: %v\n", err)
	}

	http.Redirect(w, r, "/dhcp", http.StatusSeeOther)
}

// BootMenuData holds the data used for generating a PXE boot menu configuration.
type BootMenuData struct {
	Name    string
	Kernel  string
	Initrd  string
	Options string
}

// generateBootData constructs the boot configuration data based on provided OS and network details.
func generateBootData(formData map[string]string, configFile string) BootMenuData {
	options := getBootOptions(formData["os"], formData["dns"], formData["tftpip"], configFile)

	return BootMenuData{
		Name:    osToName(formData["os"]),
		Kernel:  osToKernel(formData["os"]),
		Initrd:  osToInitrd(formData["os"]),
		Options: options,
	}
}

// getBootOptions returns the appropriate boot options string based on the operating system.
func getBootOptions(os, dns, tftpip, configFile string) string {
	switch os {
	case "Ubuntu", "NixOS":
		return fmt.Sprintf(`url=http://%s/%s autoinstall ds=nocloud-net;s=http://%s/ nameserver=%s`,
			tftpip, configFile, tftpip, dns)
	case "Redhat":
		return fmt.Sprintf(`ks=http://%s/%s nameserver=%s`,
			tftpip, configFile, dns)
	}
	return ""
}

// osToName maps the OS name to a standardized name used in file paths.
func osToName(os string) string {
	switch os {
	case "Ubuntu":
		return "ubuntu"
	case "NixOS":
		return "nixos"
	case "Redhat":
		return "redhat"
	}
	return ""
}

// osToKernel constructs the kernel file path for the given OS.
func osToKernel(os string) string {
	return fmt.Sprintf("%s/vmlinuz", osToName(os))
}

// osToInitrd constructs the initrd file path for the given OS.
func osToInitrd(os string) string {
	return fmt.Sprintf("%s/initrd.img", osToName(os))
}

// writeTemplateToDisk writes the template to disk.
func writeTemplateToDisk(templpath string, fpath string, data interface{}) error {
	templates := map[string]*template.Template{
		"templ": template.Must(template.ParseFiles(templpath)),
	}

	if err := os.MkdirAll(filepath.Dir(fpath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	file, err := os.Create(fpath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	if err := templates["templ"].Execute(file, data); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	return nil
}

// updateDHCPLease updates the DHCP lease with new boot menu data.
func (h *Handlers) updateDHCPLease(tftpip, mac string, menu dhcp.BootMenu) error {
	data, err := h.DB.GetKV(h.GetDBBucket(), []byte(tftpip))
	if err != nil {
		return fmt.Errorf("error getting DHCP handler from database: %w", err)
	}

	var dhcpHandler dhcp.DHCPHandler
	if err := json.Unmarshal(data, &dhcpHandler); err != nil {
		return fmt.Errorf("unable to unmarshal DHCP handler: %w", err)
	}

	if lease, exists := dhcpHandler.Leases[mac]; exists {
		lease.Menu = menu
		dhcpHandler.Leases[mac] = lease
	} else {
		return fmt.Errorf("lease not found for MAC %s", mac)
	}

	updatedData, err := json.Marshal(dhcpHandler)
	if err != nil {
		return fmt.Errorf("unable to marshal updated DHCP handler: %w", err)
	}

	return h.DB.PutKV(h.GetDBBucket(), []byte(tftpip), updatedData)
}

// SubmitIPMI handles the configuration of IPMI settings for a system, including PXE boot setup and potential reboot.
func (h *Handlers) SubmitIPMI(w http.ResponseWriter, r *http.Request) {
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

	// Retrieve DHCP handler from the database
	data, err := h.DB.GetKV(h.GetDBBucket(), []byte(tftpip))
	if err != nil {
		h.Logger.Error("Unable to retrieve DHCP server", "error", err)
		http.Error(w, "Unable to retrieve DHCP server", http.StatusInternalServerError)
		return
	}

	var dhcpHandler dhcp.DHCPHandler
	if err := json.Unmarshal(data, &dhcpHandler); err != nil {
		h.Logger.Error("Unable to unmarshal DHCP handler", "error", err)
		http.Error(w, "Unable to process DHCP data", http.StatusInternalServerError)
		return
	}

	// Update lease in DHCP handler
	if lease, exists := dhcpHandler.Leases[mac]; exists {
		lease.IPMI = dhcp.IPMI{
			Pxeboot:  bootConfigChecked,
			Reboot:   rebootChecked,
			IP:       net.ParseIP(ip),
			Username: username,
		}
		dhcpHandler.Leases[mac] = lease
	} else {
		http.Error(w, fmt.Sprintf("Lease not found for MAC: %s", mac), http.StatusNotFound)
		return
	}

	updatedData, err := json.Marshal(dhcpHandler)
	if err != nil {
		h.Logger.Error("Failed to marshal updated DHCP handler", "error", err)
		http.Error(w, "Failed to update DHCP lease", http.StatusInternalServerError)
		return
	}

	if err := h.DB.PutKV(h.GetDBBucket(), []byte(tftpip), updatedData); err != nil {
		h.Logger.Error("Failed to update DB state", "error", err)
		http.Error(w, "Failed to update DHCP lease", http.StatusInternalServerError)
		return
	}

	// Configure Redfish client for IPMI operations
	clientConfig := gofish.ClientConfig{
		Endpoint: fmt.Sprintf("https://%s", ip),
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
