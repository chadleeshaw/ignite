package handlers

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"

	"ignite/config"
	"ignite/dhcp"
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

// templateCache holds the parsed templates, built once and shared by all
// requests so template parsing (and any parse failure) never happens on the
// request path.
var (
	templatesOnce sync.Once
	templates     map[string]*template.Template
)

// LoadTemplates returns the parsed template set, parsing once and caching the
// result for all subsequent calls.
func LoadTemplates() map[string]*template.Template {
	templatesOnce.Do(func() {
		templates = parseTemplates()
	})
	return templates
}

// parseTemplates parses every page/modal template exactly once.
// A template that fails to parse is logged and left out of the map —
// callers treat a missing entry as "template not available" instead of
// panicking on the first request that needs it.
func parseTemplates() map[string]*template.Template {
	const baseTemplate = "templates/base.templ"

	files := map[string][]string{
		"index":              {baseTemplate, "templates/pages/index.templ"},
		"login":              {"templates/base-login.templ", "templates/pages/login.templ"},
		"dhcp":               {baseTemplate, "templates/pages/dhcp.templ"},
		"tftp":               {baseTemplate, "templates/pages/tftp.templ", "templates/modals/uploadmodal.templ"},
		"status":             {baseTemplate, "templates/pages/status.templ"},
		"status-content":     {"templates/partials/status-content.templ"},
		"provision":          {baseTemplate, "templates/pages/provision.templ"},
		"osimages":           {baseTemplate, "templates/pages/osimages.templ"},
		"syslinux":           {baseTemplate, "templates/pages/syslinux.templ"},
		"dhcpmodal":          {"templates/modals/dhcpmodal.templ"},
		"reservemodal":       {"templates/modals/reservemodal.templ"},
		"bootmodal":          {"templates/modals/bootmodal.templ"},
		"ipmimodal":          {"templates/modals/ipmimodal.templ"},
		"uploadmodal":        {"templates/modals/uploadmodal.templ"},
		"viewmodal":          {"templates/modals/viewmodal.templ"},
		"provision-new-file": {"templates/modals/provision-new-file.templ"},
		"manualleasemodal":   {"templates/modals/manualleasemodal.templ"},
	}

	parsed := make(map[string]*template.Template, len(files))
	for name, paths := range files {
		tmpl, err := template.ParseFiles(paths...)
		if err != nil {
			log.Printf("Error parsing template %s: %v", name, err)
			continue
		}
		parsed[name] = tmpl
	}
	return parsed
}

// renderCachedTemplate executes a cached template by name. A missing entry
// (parse failure) or an execution error produces a generic error response
// instead of a nil-pointer panic or an internal-error leak.
func renderCachedTemplate(w http.ResponseWriter, r *http.Request, name string, data any, userMessage string) {
	tmpl, ok := LoadTemplates()[name]
	if !ok || tmpl == nil {
		HandleError(w, r, NewInternalError(
			fmt.Sprintf("template %q not available", name),
			userMessage,
		))
		return
	}
	if err := tmpl.Execute(w, data); err != nil {
		HandleError(w, r, NewInternalError(
			fmt.Sprintf("template %q error: %v", name, err),
			userMessage,
		))
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
		HandleError(w, r, NewValidationError("Invalid template parameter", "The template parameter is invalid"))
		return
	}

	templates := LoadTemplates()
	if t, ok := templates[template]; !ok {
		HandleError(w, r, NewNotFoundError(
			fmt.Sprintf("Template %s not found", template),
			"The requested dialog does not exist",
		))
		return
	} else {
		var data map[string]any
		var err error

		switch template {
		case "dhcpmodal":
			data, err = NewDHCPModal(w, r, h.container)
			if err != nil {
				HandleError(w, r, NewInternalError("Failed to prepare DHCP data: "+err.Error(), "Unable to prepare the DHCP form"))
				return
			}
		case "reservemodal":
			data, err = NewReserveModal(w, r, h.container)
			if err != nil {
				HandleError(w, r, NewInternalError("Failed to prepare modal data: "+err.Error(), "Unable to prepare the reservation form"))
				return
			}
		case "bootmodal":
			data, err = NewBootModal(w, r, h.container)
			if err != nil {
				HandleError(w, r, NewInternalError("Failed to prepare boot data: "+err.Error(), "Unable to prepare the boot menu form"))
				return
			}
		case "ipmimodal":
			data, err = NewIPMIModal(w, r, h.container)
			if err != nil {
				HandleError(w, r, NewInternalError("Failed to prepare ipmi data: "+err.Error(), "Unable to prepare the IPMI form"))
				return
			}
		case "upload":
			data = NewUploadModal(w, r)
		case "viewmodal":
			data, err = NewViewModal(w, r)
			if err != nil {
				HandleError(w, r, NewInternalError("Failed to prepare view data: "+err.Error(), "Unable to prepare the file view"))
				return
			}
		case "provision-new-file":
			data = NewProvisionNewFileModal()
		case "manualleasemodal":
			data, err = NewManualLeaseModal(w, r, h.container)
			if err != nil {
				HandleError(w, r, NewInternalError("Failed to prepare manual lease data: "+err.Error(), "Unable to prepare the manual lease form"))
				return
			}
		default:
			HandleError(w, r, NewInternalError("Unhandled template type: "+template, "Unable to open the requested dialog"))
			return
		}

		w.Header().Set("Content-Type", "text/html")
		if err := t.Execute(w, data); err != nil {
			HandleError(w, r, NewInternalError(
				fmt.Sprintf("Error executing template %s: %v", template, err),
				"Unable to render the dialog",
			))
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
		startInt, err := ipToInt(server.IPStart)
		if err != nil {
			return nil, fmt.Errorf("invalid server start IP: %w", err)
		}
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
		"title":          "Boot Menu",
		"tftpip":         network,
		"mac":            mac,
		"os":             "",
		"version":        "",
		"typeSelect":     "",
		"template_name":  "",
		"hostname":       "",
		"ip":             "",
		"subnet":         "",
		"gateway":        "",
		"dns":            "",
		"kernel_options": "",
		"osImages":       osImages,
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
			if lease.Menu.KernelOptions != "" {
				data["kernel_options"] = lease.Menu.KernelOptions
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

	filePath, err := safeJoin(TFTPDir, fileName)
	if err != nil {
		return nil, fmt.Errorf("invalid file name: %v", err)
	}
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

// ipToInt converts an IPv4 address to uint32. Non-IPv4 input returns an
// error instead of panicking or silently truncating.
func ipToInt(ip net.IP) (uint32, error) {
	ip4 := ip.To4()
	if ip4 == nil {
		return 0, fmt.Errorf("not an IPv4 address: %s", ip.String())
	}
	return uint32(ip4[0])<<24 | uint32(ip4[1])<<16 | uint32(ip4[2])<<8 | uint32(ip4[3]), nil
}

// findServerByIP looks up a DHCP server by its interface IP. It is shared by
// the DHCP, boot menu, and IPMI handlers so the lookup logic lives in one place.
func findServerByIP(ctx context.Context, serverService dhcp.ServerService, ipStr string) (*dhcp.Server, error) {
	if serverService == nil {
		return nil, fmt.Errorf("DHCP server service not initialized")
	}
	ip := net.ParseIP(strings.TrimSpace(ipStr))
	if ip == nil {
		return nil, fmt.Errorf("invalid IP address: %s", ipStr)
	}
	servers, err := serverService.GetAllServers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list DHCP servers: %w", err)
	}
	for _, server := range servers {
		if server != nil && server.IP.Equal(ip) {
			return server, nil
		}
	}
	return nil, fmt.Errorf("no DHCP server found for IP: %s", ipStr)
}

// requireConfig writes a generic 500 and reports false when the container's
// config is nil, so handlers never nil-dereference container.Config.
func requireConfig(w http.ResponseWriter, r *http.Request, container *Container) bool {
	if container == nil || container.Config == nil {
		HandleError(w, r, NewInternalError("server configuration not initialized", "Server is not initialized. Please try again later."))
		return false
	}
	return true
}

// NewManualLeaseModal creates data for manual lease modal
func NewManualLeaseModal(w http.ResponseWriter, r *http.Request, container *Container) (map[string]any, error) {
	networkStr := r.URL.Query().Get("network")

	data := map[string]any{
		"Network": networkStr,
	}

	return data, nil
}
