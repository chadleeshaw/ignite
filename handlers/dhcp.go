package handlers

import (
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"time"

	"ignite/config"
	"ignite/dhcp"
)

// DHCPHandlers contains DHCP-related HTTP handlers
type DHCPHandlers struct {
	serverService dhcp.ServerService
	leaseService  dhcp.LeaseService
	config        *config.Config
}

// NewDHCPHandlers creates a new DHCP handlers instance
func NewDHCPHandlers(container *Container) *DHCPHandlers {
	return &DHCPHandlers{
		serverService: container.ServerService,
		leaseService:  container.LeaseService,
		config:        container.Config,
	}
}

// HandleDHCPPage serves the DHCP management page
func (h *DHCPHandlers) HandleDHCPPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	servers, err := h.serverService.GetAllServers(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get servers: %v", err), http.StatusInternalServerError)
		return
	}

	// Convert to view models for template rendering
	serverViews := make([]DHCPServerView, 0, len(servers))
	for _, server := range servers {
		leases, err := h.leaseService.GetLeasesByServer(ctx, server.ID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get leases: %v", err), http.StatusInternalServerError)
			return
		}

		serverView := DHCPServerView{
			ID:     server.ID,
			TFTPIP: server.IP.String(),
			Status: h.getServerStatusBadge(server.Started),
			Leases: h.convertLeasesToViews(leases),
		}
		serverViews = append(serverViews, serverView)
	}

	// Sort servers by IP address for consistent ordering
	h.sortServerViewsByIP(serverViews)

	data := struct {
		Title   string
		Servers []DHCPServerView
	}{
		Title:   "DHCP Management",
		Servers: serverViews,
	}

	templates := LoadTemplates()
	if err := templates["dhcp"].Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// GetDHCPServers handles GET /dhcp/servers
func (h *DHCPHandlers) GetDHCPServers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	servers, err := h.serverService.GetAllServers(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get servers: %v", err), http.StatusInternalServerError)
		return
	}

	// Convert to view models for template rendering
	serverViews := make([]DHCPServerView, 0, len(servers))
	for _, server := range servers {
		leases, err := h.leaseService.GetLeasesByServer(ctx, server.ID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get leases: %v", err), http.StatusInternalServerError)
			return
		}

		serverView := DHCPServerView{
			ID:     server.ID,
			TFTPIP: server.IP.String(),
			Status: h.getServerStatusBadge(server.Started),
			Leases: h.convertLeasesToViews(leases),
		}
		serverViews = append(serverViews, serverView)
	}

	// Sort servers by IP address for consistent ordering
	h.sortServerViewsByIP(serverViews)

	data := struct {
		Title   string
		Servers []DHCPServerView
	}{
		Title:   "DHCP Servers",
		Servers: serverViews,
	}

	renderTemplate(w, "dhcp.templ", data)
}

// StartDHCPServer handles POST /dhcp/start
func (h *DHCPHandlers) StartDHCPServer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	serverID := r.URL.Query().Get("server_id")

	if serverID == "" {
		http.Error(w, "Server ID is required", http.StatusBadRequest)
		return
	}

	if err := h.serverService.StartServer(ctx, serverID); err != nil {
		http.Error(w, fmt.Sprintf("Failed to start server: %v", err), http.StatusInternalServerError)
		return
	}

	// Redirect back to DHCP page to show the updated server list
	w.Header().Set("HX-Redirect", "/dhcp")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("DHCP server started successfully"))
}

// StopDHCPServer handles POST /dhcp/stop
func (h *DHCPHandlers) StopDHCPServer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	serverID := r.URL.Query().Get("server_id")

	if serverID == "" {
		http.Error(w, "Server ID is required", http.StatusBadRequest)
		return
	}

	if err := h.serverService.StopServer(ctx, serverID); err != nil {
		http.Error(w, fmt.Sprintf("Failed to stop server: %v", err), http.StatusInternalServerError)
		return
	}

	// Redirect back to DHCP page to show the updated server list
	w.Header().Set("HX-Redirect", "/dhcp")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("DHCP server stopped successfully"))
}

// DeleteDHCPServer handles POST /dhcp/delete
func (h *DHCPHandlers) DeleteDHCPServer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	serverID := r.URL.Query().Get("server_id")

	if serverID == "" {
		http.Error(w, "Server ID is required", http.StatusBadRequest)
		return
	}

	if err := h.serverService.DeleteServer(ctx, serverID); err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete server: %v", err), http.StatusInternalServerError)
		return
	}

	// Redirect back to DHCP page to show the updated server list
	w.Header().Set("HX-Redirect", "/dhcp")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("DHCP server deleted successfully"))
}

// SubmitDHCPServer handles POST /dhcp/submit_dhcp
func (h *DHCPHandlers) SubmitDHCPServer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	// Parse form data
	networkStr := r.FormValue("network")
	subnetStr := r.FormValue("subnet")
	gatewayStr := r.FormValue("gateway")
	dnsStr := r.FormValue("dns")
	startIPStr := r.FormValue("startIP")
	numLeasesStr := r.FormValue("numLeases")

	// Validate and parse inputs
	network := net.ParseIP(networkStr)
	if network == nil {
		http.Error(w, "Invalid network IP", http.StatusBadRequest)
		return
	}

	subnet := net.ParseIP(subnetStr)
	if subnet == nil {
		http.Error(w, "Invalid subnet mask", http.StatusBadRequest)
		return
	}

	gateway := net.ParseIP(gatewayStr)
	if gateway == nil {
		http.Error(w, "Invalid gateway IP", http.StatusBadRequest)
		return
	}

	dns := net.ParseIP(dnsStr)
	if dns == nil {
		http.Error(w, "Invalid DNS IP", http.StatusBadRequest)
		return
	}

	startIP := net.ParseIP(startIPStr)
	if startIP == nil {
		http.Error(w, "Invalid start IP", http.StatusBadRequest)
		return
	}

	numLeases, err := strconv.Atoi(numLeasesStr)
	if err != nil || numLeases <= 0 {
		http.Error(w, "Invalid number of leases", http.StatusBadRequest)
		return
	}

	// Create server configuration
	config := dhcp.ServerConfig{
		IP:            network,
		SubnetMask:    subnet,
		Gateway:       gateway,
		DNS:           dns,
		StartIP:       startIP,
		LeaseRange:    numLeases,
		LeaseDuration: 2 * time.Hour, // Default lease duration
	}

	// Create server
	server, err := h.serverService.CreateServer(ctx, config)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create server: %v", err), http.StatusInternalServerError)
		return
	}

	// Redirect back to DHCP page to show the updated server list
	w.Header().Set("HX-Redirect", "/dhcp")
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(fmt.Sprintf("DHCP server created with ID: %s", server.ID)))
}

// ReserveLease handles POST /dhcp/submit_reserve
func (h *DHCPHandlers) ReserveLease(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	serverID := r.URL.Query().Get("server_id")
	mac := r.URL.Query().Get("mac")
	ipStr := r.URL.Query().Get("ip")

	if serverID == "" || mac == "" || ipStr == "" {
		http.Error(w, "Server ID, MAC, and IP are required", http.StatusBadRequest)
		return
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		http.Error(w, "Invalid IP address", http.StatusBadRequest)
		return
	}

	if err := h.leaseService.ReserveLease(ctx, serverID, mac, ip); err != nil {
		http.Error(w, fmt.Sprintf("Failed to reserve lease: %v", err), http.StatusInternalServerError)
		return
	}

	// Redirect back to DHCP page to show updated lease status
	w.Header().Set("HX-Redirect", "/dhcp")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Lease reserved successfully"))
}

// UnreserveLease handles POST /dhcp/remove_reserve
func (h *DHCPHandlers) UnreserveLease(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	mac := r.URL.Query().Get("mac")
	if mac == "" {
		http.Error(w, "MAC address is required", http.StatusBadRequest)
		return
	}

	if err := h.leaseService.UnreserveLease(ctx, mac); err != nil {
		http.Error(w, fmt.Sprintf("Failed to unreserve lease: %v", err), http.StatusInternalServerError)
		return
	}

	// Redirect back to DHCP page to show updated lease status
	w.Header().Set("HX-Redirect", "/dhcp")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Lease unreserved successfully"))
}

// DeleteLease handles POST /dhcp/delete_lease
func (h *DHCPHandlers) DeleteLease(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	mac := r.URL.Query().Get("mac")
	if mac == "" {
		http.Error(w, "MAC address is required", http.StatusBadRequest)
		return
	}

	if err := h.leaseService.ReleaseLease(ctx, mac); err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete lease: %v", err), http.StatusInternalServerError)
		return
	}

	// Redirect back to DHCP page to show updated lease list
	w.Header().Set("HX-Redirect", "/dhcp")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Lease deleted successfully"))
}

// Helper methods
func (h *DHCPHandlers) getServerStatusBadge(started bool) string {
	if started {
		return "badge-success"
	}
	return "badge-error"
}

// sortServerViewsByIP sorts server views by IP address for consistent ordering
func (h *DHCPHandlers) sortServerViewsByIP(serverViews []DHCPServerView) {
	sort.Slice(serverViews, func(i, j int) bool {
		ipA := net.ParseIP(serverViews[i].TFTPIP)
		ipB := net.ParseIP(serverViews[j].TFTPIP)

		// Convert IPs to 4-byte representation for comparison
		if ipA.To4() != nil {
			ipA = ipA.To4()
		}
		if ipB.To4() != nil {
			ipB = ipB.To4()
		}

		// Compare byte by byte
		for k := 0; k < len(ipA) && k < len(ipB); k++ {
			if ipA[k] != ipB[k] {
				return ipA[k] < ipB[k]
			}
		}

		// If all compared bytes are equal, shorter IP comes first
		return len(ipA) < len(ipB)
	})
}

func (h *DHCPHandlers) convertLeasesToViews(leases []*dhcp.Lease) []LeaseView {
	views := make([]LeaseView, 0, len(leases))
	for _, lease := range leases {
		views = append(views, LeaseView{
			MAC:    lease.MAC,
			IP:     lease.IP.String(),
			Static: lease.Reserved,
			Menu:   lease.Menu,
			IPMI:   lease.IPMI,
		})
	}

	// Sort leases by IP address for consistent ordering
	sort.Slice(views, func(i, j int) bool {
		ipA := net.ParseIP(views[i].IP)
		ipB := net.ParseIP(views[j].IP)

		// Convert IPs to 4-byte representation for comparison
		if ipA.To4() != nil {
			ipA = ipA.To4()
		}
		if ipB.To4() != nil {
			ipB = ipB.To4()
		}

		// Compare byte by byte
		for k := 0; k < len(ipA) && k < len(ipB); k++ {
			if ipA[k] != ipB[k] {
				return ipA[k] < ipB[k]
			}
		}

		// If all compared bytes are equal, shorter IP comes first
		return len(ipA) < len(ipB)
	})

	return views
}

// View models for templates
type DHCPServerView struct {
	ID     string      `json:"id"`
	TFTPIP string      `json:"tftpip"`
	Status string      `json:"status"`
	Leases []LeaseView `json:"leases"`
}

type LeaseView struct {
	MAC    string        `json:"mac"`
	IP     string        `json:"ip"`
	Static bool          `json:"static"`
	Menu   dhcp.BootMenu `json:"menu"`
	IPMI   dhcp.IPMI     `json:"ipmi"`
}

// renderTemplate is a placeholder for template rendering
func renderTemplate(w http.ResponseWriter, templateName string, data interface{}) {
	// Implementation would use your template engine
	// This is just a placeholder
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, "Template: %s with data: %+v", templateName, data)
}
