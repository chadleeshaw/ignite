package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
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

// convertServerToView converts a DHCP server model to its template view,
// loading the server's leases. It is the single server-to-view conversion
// used by every handler that lists servers.
func (h *DHCPHandlers) convertServerToView(ctx context.Context, server *dhcp.Server) (DHCPServerView, error) {
	leases, err := h.leaseService.GetLeasesByServer(ctx, server.ID)
	if err != nil {
		return DHCPServerView{}, fmt.Errorf("failed to get leases for server %s: %w", server.ID, err)
	}

	return DHCPServerView{
		ID:     server.ID,
		TFTPIP: server.IP.String(),
		Status: h.getServerStatusBadge(isDHCPRunning(server.ID, server.Started)),
		Leases: h.convertLeasesToViews(leases),
	}, nil
}

// serverViews builds sorted view models for a list of DHCP servers.
func (h *DHCPHandlers) serverViews(ctx context.Context, servers []*dhcp.Server) ([]DHCPServerView, error) {
	serverViews := make([]DHCPServerView, 0, len(servers))
	for _, server := range servers {
		view, err := h.convertServerToView(ctx, server)
		if err != nil {
			return nil, err
		}
		serverViews = append(serverViews, view)
	}

	// Sort servers by IP address for consistent ordering
	h.sortServerViewsByIP(serverViews)
	return serverViews, nil
}

// HandleDHCPPage serves the DHCP management page
func (h *DHCPHandlers) HandleDHCPPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	servers, err := h.serverService.GetAllServers(ctx)
	if err != nil {
		HandleError(w, r, NewInternalError(
			fmt.Sprintf("Failed to get DHCP servers: %v", err),
			"Unable to load DHCP servers. Please try again later.",
		))
		return
	}

	serverViews, err := h.serverViews(ctx, servers)
	if err != nil {
		HandleError(w, r, NewInternalError(
			fmt.Sprintf("Failed to load DHCP server details: %v", err),
			"Unable to load DHCP servers. Please try again later.",
		))
		return
	}

	data := struct {
		Title   string
		Servers []DHCPServerView
	}{
		Title:   "DHCP Management",
		Servers: serverViews,
	}

	renderCachedTemplate(w, r, "dhcp", data, "Unable to render the DHCP page")
}

// GetDHCPServers handles GET /dhcp/servers
func (h *DHCPHandlers) GetDHCPServers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	servers, err := h.serverService.GetAllServers(ctx)
	if err != nil {
		HandleError(w, r, NewInternalError(
			fmt.Sprintf("Failed to get servers: %v", err),
			"Unable to load DHCP servers. Please try again later.",
		))
		return
	}

	serverViews, err := h.serverViews(ctx, servers)
	if err != nil {
		HandleError(w, r, NewInternalError(
			fmt.Sprintf("Failed to load DHCP server details: %v", err),
			"Unable to load DHCP servers. Please try again later.",
		))
		return
	}

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
		HandleError(w, r, NewValidationError("Missing server_id", "Server ID is required"))
		return
	}

	if err := h.serverService.StartServer(ctx, serverID); err != nil {
		HandleError(w, r, NewInternalError(
			fmt.Sprintf("Failed to start server: %v", err),
			"Unable to start the DHCP server. Please try again later.",
		))
		return
	}
	setDHCPRunning(serverID, true)

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
		HandleError(w, r, NewValidationError("Missing server_id", "Server ID is required"))
		return
	}

	if err := h.serverService.StopServer(ctx, serverID); err != nil {
		HandleError(w, r, NewInternalError(
			fmt.Sprintf("Failed to stop server: %v", err),
			"Unable to stop the DHCP server. Please try again later.",
		))
		return
	}
	setDHCPRunning(serverID, false)

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
		HandleError(w, r, NewValidationError("Missing server_id", "Server ID is required"))
		return
	}

	if err := h.serverService.DeleteServer(ctx, serverID); err != nil {
		HandleError(w, r, NewInternalError(
			fmt.Sprintf("Failed to delete server: %v", err),
			"Unable to delete the DHCP server. Please try again later.",
		))
		return
	}
	forgetDHCPServer(serverID)

	// Redirect back to DHCP page to show the updated server list
	w.Header().Set("HX-Redirect", "/dhcp")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("DHCP server deleted successfully"))
}

// SubmitDHCPServer handles POST /dhcp/submit_dhcp
func (h *DHCPHandlers) SubmitDHCPServer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		HandleError(w, r, NewValidationError("Failed to parse form", "The submitted form could not be parsed"))
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
	endIPStr := r.FormValue("endIP")
	leaseTimeStr := r.FormValue("lease_time")

	// The subnet mask must be valid IPv4 and contiguous — never silently /24.
	maskBits, err := getMaskBits(subnetStr)
	if err != nil {
		HandleError(w, r, NewValidationError(
			fmt.Sprintf("Invalid subnet mask: %v", err),
			"The subnet mask is invalid. Use a valid IPv4 mask like 255.255.255.0.",
		))
		return
	}

	// Create DHCP configuration validator
	validator := NewDHCPConfigValidator()

	// Prepare configuration for validation
	validationConfig := map[string]string{
		"subnet": networkStr + "/" + maskBits, // Convert to CIDR
		"range":  startIPStr + "-" + endIPStr,
		"router": gatewayStr,
		"dns":    dnsStr,
	}

	// Validate configuration
	if validationErrors := validator.ValidateDHCPConfig(validationConfig); validationErrors.HasErrors() {
		SendValidationError(w, r, validationErrors)
		return
	}

	// Parse validated IPs
	startIP := net.ParseIP(startIPStr)
	endIP := net.ParseIP(endIPStr)
	network := net.ParseIP(networkStr)
	subnet := net.ParseIP(subnetStr)
	gateway := net.ParseIP(gatewayStr)

	// Calculate numLeases from start and end IP, guarding against underflow:
	// end must not precede start.
	startInt, err := ipToInt(startIP)
	if err != nil {
		HandleError(w, r, NewValidationError("Invalid start IP", "The start IP address is invalid"))
		return
	}
	endInt, err := ipToInt(endIP)
	if err != nil {
		HandleError(w, r, NewValidationError("Invalid end IP", "The end IP address is invalid"))
		return
	}
	if endInt < startInt {
		HandleError(w, r, NewValidationError(
			"End IP precedes start IP",
			"The end IP must be greater than or equal to the start IP",
		))
		return
	}
	numLeases := int(endInt - startInt + 1)

	// Honor the submitted lease time. The UI displays it in hours, so parse
	// hours; fall back to the 2h default when empty.
	leaseDuration := 2 * time.Hour
	if strings.TrimSpace(leaseTimeStr) != "" {
		hours, err := strconv.ParseFloat(strings.TrimSpace(leaseTimeStr), 64)
		if err != nil || hours <= 0 {
			HandleError(w, r, NewValidationError(
				fmt.Sprintf("Invalid lease time %q", leaseTimeStr),
				"Lease time must be a positive number of hours",
			))
			return
		}
		leaseDuration = time.Duration(hours * float64(time.Hour))
	}

	// Parse DNS (already validated above)
	dns := net.ParseIP(dnsStr)

	// Create server configuration
	config := dhcp.ServerConfig{
		IP:            network,
		SubnetMask:    subnet,
		Gateway:       gateway,
		DNS:           dns,
		StartIP:       startIP,
		LeaseRange:    numLeases,
		LeaseDuration: leaseDuration,
	}

	if isEdit {
		// Update existing server
		err := h.serverService.UpdateServer(ctx, serverID, config)
		if err != nil {
			HandleError(w, r, NewInternalError(
				fmt.Sprintf("Failed to update DHCP server %s: %v", serverID, err),
				"Unable to update DHCP server. Please try again later.",
			))
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
			HandleError(w, r, NewInternalError(
				fmt.Sprintf("Failed to create DHCP server: %v", err),
				"Unable to create DHCP server. Please check your configuration and try again.",
			))
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
		HandleError(w, r, NewValidationError("Missing parameters", "Server ID, MAC, and IP are required"))
		return
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		HandleError(w, r, NewValidationError("Invalid IP address", "The IP address is invalid"))
		return
	}

	if err := h.leaseService.ReserveLease(ctx, serverID, mac, ip); err != nil {
		HandleError(w, r, NewInternalError(
			fmt.Sprintf("Failed to reserve lease: %v", err),
			"Unable to reserve the lease. Please try again later.",
		))
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
		HandleError(w, r, NewValidationError("Missing MAC address", "MAC address is required"))
		return
	}

	if err := h.leaseService.UnreserveLease(ctx, mac); err != nil {
		HandleError(w, r, NewInternalError(
			fmt.Sprintf("Failed to unreserve lease: %v", err),
			"Unable to unreserve the lease. Please try again later.",
		))
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
		HandleError(w, r, NewValidationError("Missing MAC address", "MAC address is required"))
		return
	}

	if err := h.leaseService.ReleaseLease(ctx, mac); err != nil {
		HandleError(w, r, NewInternalError(
			fmt.Sprintf("Failed to delete lease: %v", err),
			"Unable to delete the lease. Please try again later.",
		))
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
		HandleError(w, r, NewValidationError("Missing parameters", "Network, MAC address, and IP address are required"))
		return
	}

	// Find the server by network IP via the shared lookup helper.
	server, err := findServerByIP(ctx, h.serverService, networkStr)
	if err != nil {
		HandleError(w, r, NewValidationError(
			fmt.Sprintf("Server lookup failed: %v", err),
			"No DHCP server found for that network",
		))
		return
	}
	serverID := server.ID

	// Parse IP address
	ip := net.ParseIP(ipStr)
	if ip == nil {
		HandleError(w, r, NewValidationError("Invalid IP address", "The IP address is invalid"))
		return
	}

	// Check if it should be a static reservation
	isStatic := staticStr == "true"

	if isStatic {
		// Create a reserved lease
		err = h.leaseService.ReserveLease(ctx, serverID, macStr, ip)
		if err != nil {
			HandleError(w, r, NewInternalError(
				fmt.Sprintf("Failed to create reserved lease: %v", err),
				"Unable to create the reserved lease. Please try again later.",
			))
			return
		}
	} else {
		// Create a regular lease
		_, err = h.leaseService.AssignLease(ctx, serverID, macStr, ip)
		if err != nil {
			HandleError(w, r, NewInternalError(
				fmt.Sprintf("Failed to create lease: %v", err),
				"Unable to create the lease. Please try again later.",
			))
			return
		}
	}

	// Redirect back to DHCP page to show updated lease list
	w.Header().Set("HX-Redirect", "/dhcp")
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte("Manual DHCP entry added successfully"))
}

// validLeaseStates is the allowlist of lease states accepted by UpdateLeaseState.
var validLeaseStates = map[string]bool{
	string(dhcp.StateAssigned):     true,
	string(dhcp.StatePXERequested): true,
	string(dhcp.StateBooting):      true,
	string(dhcp.StateImaging):      true,
	string(dhcp.StateImaged):       true,
	string(dhcp.StateConfiguring):  true,
	string(dhcp.StateComplete):     true,
	string(dhcp.StateFailed):       true,
	string(dhcp.StateOffline):      true,
}

// UpdateLeaseState handles POST /dhcp/lease/state
func (h *DHCPHandlers) UpdateLeaseState(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	mac := r.URL.Query().Get("mac")
	newState := r.FormValue("state")
	source := r.FormValue("source")

	if mac == "" || newState == "" {
		HandleError(w, r, NewValidationError("Missing parameters", "MAC address and state are required"))
		return
	}

	// Only real DHCP provisioning states are accepted.
	if !validLeaseStates[newState] {
		HandleError(w, r, NewValidationError(
			fmt.Sprintf("Unknown lease state %q", newState),
			"The requested lease state is not valid",
		))
		return
	}

	if source == "" {
		source = "manual"
	}

	err := h.leaseService.UpdateLeaseState(ctx, mac, newState, source)
	if err != nil {
		HandleError(w, r, NewInternalError(
			fmt.Sprintf("Failed to update lease state: %v", err),
			"Unable to update the lease state. Please try again later.",
		))
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
		HandleError(w, r, NewValidationError("Missing MAC address", "MAC address is required"))
		return
	}

	err := h.leaseService.RecordHeartbeat(ctx, mac)
	if err != nil {
		HandleError(w, r, NewInternalError(
			fmt.Sprintf("Failed to record heartbeat: %v", err),
			"Unable to record the heartbeat. Please try again later.",
		))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "success", "message": "Heartbeat recorded"}`))
}

// GetLeaseStateHistory handles GET /dhcp/lease/history
func (h *DHCPHandlers) GetLeaseStateHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	mac := r.URL.Query().Get("mac")
	if mac == "" {
		HandleError(w, r, NewValidationError("Missing MAC address", "MAC address is required"))
		return
	}

	history, err := h.leaseService.GetLeaseStateHistory(ctx, mac)
	if err != nil {
		HandleError(w, r, NewInternalError(
			fmt.Sprintf("Failed to get lease history: %v", err),
			"Unable to load lease history. Please try again later.",
		))
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

// sortServerViewsByIP sorts server views by IP address for consistent ordering.
// It uses the shared nil-guarded IP comparator.
func (h *DHCPHandlers) sortServerViewsByIP(serverViews []DHCPServerView) {
	sort.Slice(serverViews, func(i, j int) bool {
		return compareIPs(net.ParseIP(serverViews[i].TFTPIP), net.ParseIP(serverViews[j].TFTPIP)) < 0
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
			State:            string(lease.State),
			StateBadgeClass:  getStateBadgeClass(lease.State),
			StateDisplayName: lease.GetStateDisplayName(),
			LastSeen:         lease.LastSeen,
		})
	}

	// Sort leases by IP address for consistent ordering
	sort.Slice(views, func(i, j int) bool {
		return compareIPs(net.ParseIP(views[i].IP), net.ParseIP(views[j].IP)) < 0
	})

	return views
}

// getStateBadgeClass returns the CSS class for a lease state badge.
// Presentation lives in handlers, not in the dhcp domain model.
func getStateBadgeClass(state dhcp.LeaseState) string {
	switch state {
	case dhcp.StateAssigned:
		return "badge-info"
	case dhcp.StatePXERequested:
		return "badge-warning"
	case dhcp.StateBooting:
		return "badge-warning"
	case dhcp.StateImaging:
		return "badge-accent"
	case dhcp.StateImaged:
		return "badge-success"
	case dhcp.StateConfiguring:
		return "badge-accent"
	case dhcp.StateComplete:
		return "badge-success"
	case dhcp.StateFailed:
		return "badge-error"
	case dhcp.StateOffline:
		return "badge-ghost"
	default:
		return "badge-neutral"
	}
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

// getMaskBits converts a dotted-decimal subnet mask to its CIDR prefix length.
// Invalid IPv4 input and non-contiguous masks are rejected as errors instead
// of silently falling back to /24.
func getMaskBits(mask string) (string, error) {
	trimmed := strings.TrimSpace(mask)
	ip := net.ParseIP(trimmed)
	if ip == nil {
		return "", fmt.Errorf("not a valid IP address: %q", mask)
	}
	mask4 := ip.To4()
	if mask4 == nil {
		return "", fmt.Errorf("not an IPv4 mask: %q", mask)
	}
	ones, bits := net.IPv4Mask(mask4[0], mask4[1], mask4[2], mask4[3]).Size()
	if bits != 32 {
		return "", fmt.Errorf("not a contiguous subnet mask: %q", mask)
	}
	return strconv.Itoa(ones), nil
}
