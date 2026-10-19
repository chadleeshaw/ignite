package handlers

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"ignite/dhcp"
)

// tftpPort is the UDP port the TFTP server listens on (tftp/tftp.go binds
// port 69; there is no longer a config knob for it).
const tftpPort = 69

// serviceRegistry tracks in-process TFTP running state. The status handlers
// read it instead of probing the socket: a UDP dial cannot prove the TFTP
// listener is actually running. DHCP listener state lives in dhcp.ServerState
// and is updated by the DHCP service when a listener starts, stops, or fails.
var serviceRegistry = struct {
	sync.RWMutex
	tftpRunning bool
	tftpKnown   bool
}{}

// SetTFTPRunning records whether the TFTP server is running. It is exported so
// the application layer that starts/stops the TFTP server can report state.
func SetTFTPRunning(running bool) {
	serviceRegistry.Lock()
	serviceRegistry.tftpRunning = running
	serviceRegistry.tftpKnown = true
	serviceRegistry.Unlock()
}

// isTFTPRunning reports the last known TFTP running state.
func isTFTPRunning() bool {
	serviceRegistry.RLock()
	defer serviceRegistry.RUnlock()
	return serviceRegistry.tftpKnown && serviceRegistry.tftpRunning
}

// TFTPStatus renders the TFTP state honestly: "unknown" until something
// reports it, never a guessed "running".
func TFTPStatus() string {
	serviceRegistry.RLock()
	defer serviceRegistry.RUnlock()
	if !serviceRegistry.tftpKnown {
		return "unknown"
	}
	if serviceRegistry.tftpRunning {
		return "running"
	}
	return "stopped"
}

// StatusHandlers handles status-related requests
type StatusHandlers struct {
	container *Container
}

// NewStatusHandlers creates a new StatusHandlers instance
func NewStatusHandlers(container *Container) *StatusHandlers {
	return &StatusHandlers{container: container}
}

// ServiceStatus represents the status of a service
type ServiceStatus struct {
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	Description string    `json:"description"`
	Details     string    `json:"details"`
	Port        int       `json:"port"`
	LastCheck   time.Time `json:"last_check"`
}

// DHCPServerStatus represents DHCP server status for the status page
type DHCPServerStatus struct {
	ID          string    `json:"id"`
	IP          string    `json:"ip"`
	Status      string    `json:"status"`
	Description string    `json:"description"`
	LeaseCount  int       `json:"lease_count"`
	LastCheck   time.Time `json:"last_check"`
}

// StatusPageData represents data for the status page
type StatusPageData struct {
	Title         string             `json:"title"`
	Mode          string             `json:"-"`
	CSRFToken     string             `json:"-"`
	HTTPServer    ServiceStatus      `json:"http_server"`
	TFTPServer    ServiceStatus      `json:"tftp_server"`
	DHCPServers   []DHCPServerStatus `json:"dhcp_servers"`
	LastUpdated   string             `json:"last_updated"`
	LastUpdatedAt string             `json:"last_updated_at"`
	OverallStatus string             `json:"overall_status"`
	AutoRefresh   bool               `json:"auto_refresh"`
	RefreshSecs   int                `json:"refresh_secs"`
}

// getStatusData builds the status page data from tracked runtime state.
func (h *StatusHandlers) getStatusData(r *http.Request) (*StatusPageData, error) {
	ctx := r.Context()
	now := time.Now()

	// HTTP server status — a TCP dial to our own listener as a smoke check.
	// The configured port is a string; an unparseable value fails the check.
	httpPort, _ := strconv.Atoi(h.container.Config.HTTP.Port)
	httpStatus := checkHTTPServerStatus(httpPort)

	// TFTP server status — tracked runtime state, never a UDP guess.
	tftpStatus := ServiceStatus{
		Name:        "TFTP Server",
		Status:      TFTPStatus(),
		Description: "Network boot file server for PXE clients",
		Details:     fmt.Sprintf("Port %d", tftpPort),
		Port:        tftpPort,
		LastCheck:   now,
	}

	// DHCP servers status — the listener record, never the persisted Started flag.
	servers, err := h.container.ServerService.GetAllServers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get DHCP servers: %w", err)
	}

	var dhcpServers []DHCPServerStatus
	var dhcpStates []dhcp.ServerState
	for _, server := range servers {
		leases, _ := h.container.LeaseService.GetLeasesByServer(ctx, server.ID)
		state := dhcp.CurrentServerState(server.ID)
		dhcpStates = append(dhcpStates, state)

		dhcpServers = append(dhcpServers, DHCPServerStatus{
			ID:          server.ID,
			IP:          server.IP.String(),
			Status:      string(state),
			Description: "Dynamic IP assignment for network boot clients",
			LeaseCount:  len(leases),
			LastCheck:   now,
		})
	}

	overallStatus := overallServiceStatus(httpStatus.Status, tftpStatus.Status, dhcpStates)

	return &StatusPageData{
		Title:         "System Status",
		HTTPServer:    httpStatus,
		TFTPServer:    tftpStatus,
		DHCPServers:   dhcpServers,
		LastUpdated:   now.Format("2006-01-02 15:04:05"),
		LastUpdatedAt: formatRFC3339(now),
		OverallStatus: overallStatus,
		AutoRefresh:   true,
		RefreshSecs:   5,
	}, nil
}

// overallServiceStatus rolls HTTP, TFTP, and DHCP listener state into one word.
// configured DHCP servers have not been started this boot, so they do not
// make the service partial. stopped means a listener failed or was stopped.
func overallServiceStatus(httpStatus, tftpStatus string, dhcpStates []dhcp.ServerState) string {
	if httpStatus != "running" {
		return "down"
	}
	if tftpStatus != "running" {
		return "partial"
	}
	for _, state := range dhcpStates {
		if state == dhcp.ServerStateStopped {
			return "partial"
		}
	}
	return "healthy"
}

// CurrentOverallStatus is the same overall service word /status/content
// renders: healthy, partial, or down. An error reading status yields "".
func (h *StatusHandlers) CurrentOverallStatus(r *http.Request) string {
	if h == nil || r == nil {
		return ""
	}
	data, err := h.getStatusData(r)
	if err != nil || data == nil {
		return ""
	}
	return data.OverallStatus
}

// HandleStatusContent serves status content for HTMX updates.
func (h *StatusHandlers) HandleStatusContent(w http.ResponseWriter, r *http.Request) {
	if !requireConfig(w, r, h.container) {
		return
	}

	data, err := h.getStatusData(r)
	if err != nil {
		HandleError(w, r, NewInternalError(
			fmt.Sprintf("failed to get status data: %v", err),
			"Unable to load system status",
		))
		return
	}

	renderCachedTemplate(w, r, "status-content", data, "Unable to render status content")
}

// checkHTTPServerStatus checks if the HTTP server is running by dialing its
// own TCP listener — a smoke check that the port accepts connections.
func checkHTTPServerStatus(port int) ServiceStatus {
	status := ServiceStatus{
		Name:        "HTTP Server",
		Status:      "stopped",
		Description: "Web interface and REST API",
		Details:     fmt.Sprintf("Port %d", port),
		Port:        port,
		LastCheck:   time.Now(),
	}

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 2*time.Second)
	if err != nil {
		status.Details = fmt.Sprintf("Port %d - not responding", port)
		return status
	}
	conn.Close()

	status.Status = "running"
	status.Details = fmt.Sprintf("Port %d - responding", port)
	return status
}
