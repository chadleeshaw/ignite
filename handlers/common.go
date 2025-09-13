package handlers

import (
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"ignite/config"
)

// TFTPDir holds the directory path for TFTP server operations.
var TFTPDir string

// HTTPDir holds the directory path for HTTP server operations.
var HTTPDir string

func init() {
	cfg, err := config.LoadDefault()
	if err == nil {
		TFTPDir = cfg.TFTP.Dir
		HTTPDir = cfg.HTTP.Dir
	}
}

// LoadTemplates initializes and returns a map of templates for different pages.
func LoadTemplates() map[string]*template.Template {
	const baseTemplate = "templates/base.templ"

	return map[string]*template.Template{
		"index":              template.Must(template.ParseFiles(baseTemplate, "templates/pages/index.templ")),
		"dhcp":               template.Must(template.ParseFiles(baseTemplate, "templates/pages/dhcp.templ")),
		"tftp":               template.Must(template.ParseFiles(baseTemplate, "templates/pages/tftp.templ", "templates/modals/uploadmodal.templ")),
		"status":             template.Must(template.ParseFiles(baseTemplate, "templates/pages/status.templ")),
		"status-content":     template.Must(template.ParseFiles("templates/partials/status-content.templ")),
		"provision":          template.Must(template.ParseFiles(baseTemplate, "templates/pages/provision.templ")),
		"osimages":           template.Must(template.ParseFiles(baseTemplate, "templates/pages/osimages.templ")),
		"syslinux":           template.Must(template.ParseFiles(baseTemplate, "templates/pages/syslinux.templ")),
		"dhcpmodal":          template.Must(template.ParseFiles("templates/modals/dhcpmodal.templ")),
		"reservemodal":       template.Must(template.ParseFiles("templates/modals/reservemodal.templ")),
		"bootmodal":          template.Must(template.ParseFiles("templates/modals/bootmodal.templ")),
		"ipmimodal":          template.Must(template.ParseFiles("templates/modals/ipmimodal.templ")),
		"uploadmodal":        template.Must(template.ParseFiles("templates/modals/uploadmodal.templ")),
		"viewmodal":          template.Must(template.ParseFiles("templates/modals/viewmodal.templ")),
		"provision-new-file": template.Must(template.ParseFiles("templates/modals/provision-new-file.templ")),
		"manualleasemodal":   template.Must(template.ParseFiles("templates/modals/manualleasemodal.templ")),
	}
}

// GetQueryParam retrieves a specific query parameter from the HTTP request.
func GetQueryParam(r *http.Request, param string) (string, error) {
	value := r.URL.Query().Get(param)
	if value == "" {
		return "", fmt.Errorf("missing %s parameter", param)
	}
	return value, nil
}

// setNoCacheHeaders sets HTTP headers to prevent caching.
func SetNoCacheHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}

// ModalHandlers handles modal-related requests
type ModalHandlers struct {
	container *Container
}

// NewModalHandlers creates a new ModalHandlers instance
func NewModalHandlers(container *Container) *ModalHandlers {
	return &ModalHandlers{container: container}
}

// CloseModalHandler closes modal by returning an empty div
func (h *ModalHandlers) CloseModalHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte("<div id=\"modal-content\"></div>"))
}

// OpenModalHandler opens a modal given a template query parameter
func (h *ModalHandlers) OpenModalHandler(w http.ResponseWriter, r *http.Request) {
	template, err := GetQueryParam(r, "template")
	if err != nil {
		log.Printf("Error retrieving template parameter: %v", err)
		http.Error(w, "Invalid template parameter", http.StatusBadRequest)
		return
	}

	templates := LoadTemplates()
	if t, ok := templates[template]; !ok {
		log.Printf("Template %s not found", template)
		http.Error(w, fmt.Sprintf("Template %s not found", template), http.StatusNotFound)
		return
	} else {
		var data map[string]any
		var err error

		switch template {
		case "dhcpmodal":
			data, err = NewDHCPModal(w, r, h.container)
			if err != nil {
				log.Printf("Error creating DHCP modal data: %v", err)
				http.Error(w, "Failed to prepare DHCP data: "+err.Error(), http.StatusInternalServerError)
				return
			}
		case "reservemodal":
			data, err = NewReserveModal(w, r, h.container)
			if err != nil {
				log.Printf("Error creating reserve modal data: %v", err)
				http.Error(w, "Failed to prepare modal data: "+err.Error(), http.StatusInternalServerError)
				return
			}
		case "bootmodal":
			data, err = NewBootModal(w, r, h.container)
			if err != nil {
				log.Printf("Error creating boot modal data: %v", err)
				http.Error(w, "Failed to prepare boot data: "+err.Error(), http.StatusInternalServerError)
				return
			}
		case "ipmimodal":
			data, err = NewIPMIModal(w, r, h.container)
			if err != nil {
				log.Printf("Error creating ipmi modal data: %v", err)
				http.Error(w, "Failed to prepare ipmi data: "+err.Error(), http.StatusInternalServerError)
				return
			}
		case "upload":
			data = NewUploadModal(w, r)
		case "viewmodal":
			data, err = NewViewModal(w, r)
			if err != nil {
				log.Printf("Error creating view modal data: %v", err)
				http.Error(w, "Failed to prepare view data: "+err.Error(), http.StatusInternalServerError)
				return
			}
		case "provision-new-file":
			data = NewProvisionNewFileModal()
		case "manualleasemodal":
			data, err = NewManualLeaseModal(w, r, h.container)
			if err != nil {
				log.Printf("Error creating manual lease modal data: %v", err)
				http.Error(w, "Failed to prepare manual lease data: "+err.Error(), http.StatusInternalServerError)
				return
			}
		default:
			log.Printf("Unhandled template type: %s", template)
			http.Error(w, "Unhandled template type", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html")
		if err := t.Execute(w, data); err != nil {
			log.Printf("Error executing template %s: %v", template, err)
			http.Error(w, "Could not render template", http.StatusInternalServerError)
		}
	}
}

// NewDHCPModal creates data for DHCP modal
func NewDHCPModal(w http.ResponseWriter, r *http.Request, container *Container) (map[string]any, error) {
	networks := getLocalIPAddresses()

	// Initialize data with defaults for new server
	data := map[string]any{
		"title":      "DHCP Configuration",
		"Networks":   networks,
		"tftpip":     "",
		"startip":    "",
		"endip":      "",
		"gateway":    "",
		"dns":        "",
		"subnet":     "",
		"lease_time": "",
		"domain":     "",
		"bootfile":   "boot-bios/pxelinux.0", // Default boot file
		"IsEdit":     false,
	}

	// Check if we're editing an existing server
	serverID := r.URL.Query().Get("server_id")
	if serverID != "" && container != nil {
		ctx := r.Context()
		server, err := container.ServerService.GetServer(ctx, serverID)
		if err != nil {
			return nil, fmt.Errorf("failed to get server: %w", err)
		}

		// Populate with existing server data
		data["tftpip"] = server.IP.String()
		data["startip"] = server.IPStart.String()

		// Calculate end IP from start IP and lease range
		startInt := ipToInt(server.IPStart)
		endInt := startInt + uint32(server.LeaseRange) - 1
		endIP := net.IPv4(byte(endInt>>24), byte(endInt>>16), byte(endInt>>8), byte(endInt))
		data["endip"] = endIP.String()

		data["gateway"] = server.Options.Gateway.String()
		data["dns"] = server.Options.DNS.String()
		data["subnet"] = server.Options.SubnetMask.String()
		data["lease_time"] = fmt.Sprintf("%.0f", server.LeaseDuration.Hours())
		data["domain"] = ""                       // Not stored in current model
		data["bootfile"] = "boot-bios/pxelinux.0" // Default value
		data["IsEdit"] = true
		data["server_id"] = serverID
		data["title"] = "Edit DHCP Server"
	}

	return data, nil
}

// getLocalIPAddresses returns a list of local machine IP addresses
func getLocalIPAddresses() []string {
	var ips []string

	interfaces, err := net.Interfaces()
	if err != nil {
		log.Printf("Error getting network interfaces: %v", err)
		return ips
	}

	for _, iface := range interfaces {
		// Skip loopback and down interfaces
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			log.Printf("Error getting addresses for interface %s: %v", iface.Name, err)
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			// Only include IPv4 addresses that are not loopback
			if ip != nil && ip.To4() != nil && !ip.IsLoopback() {
				ips = append(ips, ip.String())
			}
		}
	}

	return ips
}

// NewReserveModal creates data for reservation modal
func NewReserveModal(w http.ResponseWriter, r *http.Request, container *Container) (map[string]any, error) {
	// Get query parameters
	network := r.URL.Query().Get("network")
	mac := r.URL.Query().Get("mac")

	if network == "" || mac == "" {
		return nil, fmt.Errorf("network and mac parameters are required")
	}

	ctx := r.Context()

	// Find server by network IP to get server ID
	networkIP := net.ParseIP(network)
	if networkIP == nil {
		return nil, fmt.Errorf("invalid network IP: %s", network)
	}

	servers, err := container.ServerService.GetAllServers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get servers: %w", err)
	}

	var serverID string
	for _, server := range servers {
		if server.IP.Equal(networkIP) {
			serverID = server.ID
			break
		}
	}

	if serverID == "" {
		return nil, fmt.Errorf("server not found for network IP: %s", network)
	}

	// Try to find existing lease by MAC
	lease, err := container.LeaseService.GetLeaseByMAC(ctx, mac)

	data := map[string]any{
		"title":    "Reserve Lease",
		"tftpip":   network,
		"mac":      mac,
		"serverid": serverID,
	}

	if err != nil || lease == nil {
		// No existing lease, show empty form for new reservation
		data["ip"] = ""
		data["static"] = false
	} else {
		// Existing lease found, populate with current data
		data["ip"] = lease.IP.String()
		data["static"] = lease.Reserved
	}

	return data, nil
}

// NewBootModal creates data for boot modal
func NewBootModal(w http.ResponseWriter, r *http.Request, container *Container) (map[string]any, error) {
	network := r.URL.Query().Get("network")
	mac := r.URL.Query().Get("mac")

	if network == "" || mac == "" {
		return nil, fmt.Errorf("network and mac parameters are required")
	}

	ctx := r.Context()

	// Get available OS images grouped by OS
	var osImages map[string][]map[string]interface{}
	if container.OSImageService != nil {
		allImages, err := container.OSImageService.GetAllOSImages(ctx)
		if err == nil {
			osImages = make(map[string][]map[string]interface{})
			for _, image := range allImages {
				if osImages[image.OS] == nil {
					osImages[image.OS] = []map[string]interface{}{}
				}
				osImages[image.OS] = append(osImages[image.OS], map[string]interface{}{
					"version": image.Version,
					"active":  image.Active,
				})
			}
		}
	}

	// Initialize data with basic required fields
	data := map[string]any{
		"title":         "Boot Menu",
		"tftpip":        network,
		"mac":           mac,
		"os":            "",
		"version":       "",
		"typeSelect":    "",
		"template_name": "",
		"hostname":      "",
		"ip":            "",
		"subnet":        "",
		"gateway":       "",
		"dns":           "",
		"osImages":      osImages,
	}

	// Try to load existing boot menu data from the lease
	if container != nil {
		lease, err := container.LeaseService.GetLeaseByMAC(ctx, mac)
		if err == nil && lease != nil {
			// Populate form with existing boot menu data
			if lease.Menu.OS != "" {
				data["os"] = lease.Menu.OS
			}
			if lease.Menu.Version != "" {
				data["version"] = lease.Menu.Version
			}
			if lease.Menu.TemplateType != "" {
				data["typeSelect"] = lease.Menu.TemplateType
			}
			if lease.Menu.TemplateName != "" {
				data["template_name"] = lease.Menu.TemplateName
			}
			if lease.Menu.Hostname != "" {
				data["hostname"] = lease.Menu.Hostname
			}
			if lease.Menu.IP != nil {
				data["ip"] = lease.Menu.IP.String()
			}
			if lease.Menu.Subnet != nil {
				data["subnet"] = lease.Menu.Subnet.String()
			}
			if lease.Menu.Gateway != nil {
				data["gateway"] = lease.Menu.Gateway.String()
			}
			if lease.Menu.DNS != nil {
				data["dns"] = lease.Menu.DNS.String()
			}
		}
	}

	return data, nil
}

// NewIPMIModal creates data for IPMI modal
func NewIPMIModal(w http.ResponseWriter, r *http.Request, container *Container) (map[string]any, error) {
	network := r.URL.Query().Get("network")
	mac := r.URL.Query().Get("mac")

	if network == "" || mac == "" {
		return nil, fmt.Errorf("network and mac parameters are required")
	}

	ctx := r.Context()

	// Initialize data with basic required fields
	data := map[string]any{
		"title":    "IPMI Configuration",
		"ip":       "",
		"username": "",
		"mac":      mac,
		"tftpip":   network,
		"pxeboot":  false,
		"reboot":   false,
	}

	// Try to load existing IPMI data from the lease
	if container != nil {
		lease, err := container.LeaseService.GetLeaseByMAC(ctx, mac)
		if err == nil && lease != nil {
			// Populate form with existing IPMI data
			if lease.IPMI.IP != nil {
				data["ip"] = lease.IPMI.IP.String()
			}
			if lease.IPMI.Username != "" {
				data["username"] = lease.IPMI.Username
			}
			data["pxeboot"] = lease.IPMI.PXEBoot
			data["reboot"] = lease.IPMI.Reboot
		}
	}

	return data, nil
}

// NewUploadModal creates data for upload modal
func NewUploadModal(w http.ResponseWriter, r *http.Request) map[string]any {
	return map[string]any{
		"title": "File Upload",
	}
}

// NewViewModal creates data for view modal
func NewViewModal(w http.ResponseWriter, r *http.Request) (map[string]any, error) {
	fileName := r.URL.Query().Get("file")
	if fileName == "" {
		return nil, fmt.Errorf("file parameter is required")
	}

	filePath := filepath.Join(TFTPDir, fileName)
	content, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file not found: %s", fileName)
		}
		return nil, fmt.Errorf("error reading file: %v", err)
	}

	return map[string]any{
		"FileName":    fileName,
		"FileContent": string(content),
	}, nil
}

// NewProvisionNewFileModal creates data for provision new file modal
func NewProvisionNewFileModal() map[string]any {
	return map[string]any{
		"title": "Create New File",
	}
}

// formatFileSize formats file size in bytes to human readable format
func formatFileSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// ipToInt converts IP to uint32 (helper function for IP calculations)
func ipToInt(ip net.IP) uint32 {
	if len(ip) == 16 {
		ip = ip[12:16] // Convert IPv6 to IPv4 if needed
	}
	return uint32(ip[0])<<24 + uint32(ip[1])<<16 + uint32(ip[2])<<8 + uint32(ip[3])
}

// NewManualLeaseModal creates data for manual lease modal
func NewManualLeaseModal(w http.ResponseWriter, r *http.Request, container *Container) (map[string]any, error) {
	networkStr := r.URL.Query().Get("network")

	data := map[string]any{
		"Network": networkStr,
	}

	return data, nil
}
