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
		"tftp":               template.Must(template.ParseFiles(baseTemplate, "templates/pages/tftp.templ")),
		"status":             template.Must(template.ParseFiles(baseTemplate, "templates/pages/status.templ")),
		"status-content":     template.Must(template.ParseFiles("templates/partials/status-content.templ")),
		"provision":          template.Must(template.ParseFiles(baseTemplate, "templates/pages/provision.templ")),
		"dhcpmodal":          template.Must(template.ParseFiles("templates/modals/dhcpmodal.templ")),
		"reservemodal":       template.Must(template.ParseFiles("templates/modals/reservemodal.templ")),
		"bootmodal":          template.Must(template.ParseFiles("templates/modals/bootmodal.templ")),
		"ipmimodal":          template.Must(template.ParseFiles("templates/modals/ipmimodal.templ")),
		"uploadmodal":        template.Must(template.ParseFiles("templates/modals/uploadmodal.templ")),
		"viewmodal":          template.Must(template.ParseFiles("templates/modals/viewmodal.templ")),
		"provision-new-file": template.Must(template.ParseFiles("templates/modals/provision-new-file.templ")),
		"provtempmodal":      template.Must(template.ParseFiles("templates/modals/provtempmodal.templ")),
		"provconfigmodal":    template.Must(template.ParseFiles("templates/modals/provconfigmodal.templ")),
		"provsaveasmodal":    template.Must(template.ParseFiles("templates/modals/provsaveasmodal.templ")),
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
			data = NewDHCPModal()
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
		case "provtempmodal":
		case "provconfigmodal":
		case "provsaveasmodal":
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
func NewDHCPModal() map[string]any {
	networks := getLocalIPAddresses()
	return map[string]any{
		"title":    "DHCP Configuration",
		"Networks": networks,
	}
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

	// Initialize data with basic required fields
	data := map[string]any{
		"title":         "Boot Menu",
		"tftpip":        network,
		"mac":           mac,
		"os":            "",
		"typeSelect":    "",
		"template_name": "",
		"hostname":      "",
		"ip":            "",
		"subnet":        "",
		"gateway":       "",
		"dns":           "",
	}

	// Try to load existing boot menu data from the lease
	if container != nil {
		lease, err := container.LeaseService.GetLeaseByMAC(ctx, mac)
		if err == nil && lease != nil {
			// Populate form with existing boot menu data
			if lease.Menu.OS != "" {
				data["os"] = lease.Menu.OS
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
