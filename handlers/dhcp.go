package handlers

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sort"
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

	// Check if this is an edit or create operation
	serverID := r.FormValue("server_id")
	isEdit := serverID != ""

	// Parse form data
	networkStr := r.FormValue("network")
	subnetStr := r.FormValue("subnet")
	gatewayStr := r.FormValue("gateway")
	dnsStr := r.FormValue("dns")
	startIPStr := r.FormValue("startIP")

	// Always use endIP approach now
	endIPStr := r.FormValue("endIP")
	endIP := net.ParseIP(endIPStr)
	if endIP == nil {
		http.Error(w, "Invalid end IP", http.StatusBadRequest)
		return
	}

	startIP := net.ParseIP(startIPStr)
	if startIP == nil {
		http.Error(w, "Invalid start IP", http.StatusBadRequest)
		return
	}

	// Calculate numLeases from start and end IP
	startInt := ipToInt(startIP)
	endInt := ipToInt(endIP)
	if endInt < startInt {
		http.Error(w, "End IP must be greater than start IP", http.StatusBadRequest)
		return
	}
	numLeases := int(endInt - startInt + 1)

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

	if isEdit {
		// Update existing server
		err := h.serverService.UpdateServer(ctx, serverID, config)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to update server: %v", err), http.StatusInternalServerError)
			return
		}

		// Redirect back to DHCP page to show the updated server list
		w.Header().Set("HX-Redirect", "/dhcp")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("DHCP server updated successfully"))
	} else {
		// Create new server
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

// AddManualLease handles POST /dhcp/add_manual_lease
func (h *DHCPHandlers) AddManualLease(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	networkStr := r.FormValue("network")
	macStr := r.FormValue("mac")
	ipStr := r.FormValue("ip")
	staticStr := r.FormValue("static")

	if networkStr == "" || macStr == "" || ipStr == "" {
		http.Error(w, "Network, MAC address, and IP address are required", http.StatusBadRequest)
		return
	}

	// Find the server by network IP
	servers, err := h.serverService.GetAllServers(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get servers: %v", err), http.StatusInternalServerError)
		return
	}

	var serverID string
	for _, server := range servers {
		if server.IP.String() == networkStr {
			serverID = server.ID
			break
		}
	}

	if serverID == "" {
		http.Error(w, "Server not found for network", http.StatusBadRequest)
		return
	}

	// Parse IP address
	ip := net.ParseIP(ipStr)
	if ip == nil {
		http.Error(w, "Invalid IP address", http.StatusBadRequest)
		return
	}

	// Check if it should be a static reservation
	isStatic := staticStr == "true"

	if isStatic {
		// Create a reserved lease
		err = h.leaseService.ReserveLease(ctx, serverID, macStr, ip)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to create reserved lease: %v", err), http.StatusInternalServerError)
			return
		}
	} else {
		// Create a regular lease
		_, err = h.leaseService.AssignLease(ctx, serverID, macStr, ip)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to create lease: %v", err), http.StatusInternalServerError)
			return
		}
	}

	// Redirect back to DHCP page to show updated lease list
	w.Header().Set("HX-Redirect", "/dhcp")
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte("Manual DHCP entry added successfully"))
}

// UpdateLeaseState handles POST /dhcp/lease/{mac}/state
func (h *DHCPHandlers) UpdateLeaseState(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	mac := r.URL.Query().Get("mac")
	newState := r.FormValue("state")
	source := r.FormValue("source")

	if mac == "" || newState == "" {
		http.Error(w, "MAC address and state are required", http.StatusBadRequest)
		return
	}

	if source == "" {
		source = "manual"
	}

	err := h.leaseService.UpdateLeaseState(ctx, mac, newState, source)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to update lease state: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "success", "message": "Lease state updated successfully"}`))
}

// RecordHeartbeat handles POST /dhcp/lease/{mac}/heartbeat
func (h *DHCPHandlers) RecordHeartbeat(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	mac := r.URL.Query().Get("mac")
	if mac == "" {
		http.Error(w, "MAC address is required", http.StatusBadRequest)
		return
	}

	err := h.leaseService.RecordHeartbeat(ctx, mac)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to record heartbeat: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "success", "message": "Heartbeat recorded"}`))
}

// GetLeaseStateHistory handles GET /dhcp/lease/{mac}/history
func (h *DHCPHandlers) GetLeaseStateHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	mac := r.URL.Query().Get("mac")
	if mac == "" {
		http.Error(w, "MAC address is required", http.StatusBadRequest)
		return
	}

	history, err := h.leaseService.GetLeaseStateHistory(ctx, mac)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get lease history: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	// Return proper JSON with history data
	response := map[string]interface{}{
		"mac":     mac,
		"history": history,
	}
	json.NewEncoder(w).Encode(response)
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
			MAC:              lease.MAC,
			IP:               lease.IP.String(),
			Static:           lease.Reserved,
			Menu:             lease.Menu,
			IPMI:             lease.IPMI,
			State:            lease.State,
			StateBadgeClass:  lease.GetStateBadgeClass(),
			StateDisplayName: lease.GetStateDisplayName(),
			LastSeen:         lease.LastSeen,
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
	MAC              string        `json:"mac"`
	IP               string        `json:"ip"`
	Static           bool          `json:"static"`
	Menu             dhcp.BootMenu `json:"menu"`
	IPMI             dhcp.IPMI     `json:"ipmi"`
	State            string        `json:"state"`
	StateBadgeClass  string        `json:"state_badge_class"`
	StateDisplayName string        `json:"state_display_name"`
	LastSeen         time.Time     `json:"last_seen"`
}

// renderTemplate is a placeholder for template rendering
func renderTemplate(w http.ResponseWriter, templateName string, data interface{}) {
	// Implementation would use your template engine
	// This is just a placeholder
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, "Template: %s with data: %+v", templateName, data)
}
