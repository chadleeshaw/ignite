package handlers

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// tftpPort is the UDP port the TFTP server listens on (tftp/tftp.go binds
// port 69; there is no longer a config knob for it).
const tftpPort = 69

// serviceRegistry tracks in-process service running state. The status handlers
// read it instead of probing sockets: UDP dials cannot prove a TFTP/DHCP
// listener is actually running. The DHCP server handlers update the DHCP
// entries on start/stop/delete; the TFTP entry is updated via SetTFTPRunning
// by whatever component starts or stops the TFTP server.
var serviceRegistry = struct {
	sync.RWMutex
	tftpRunning bool
	tftpKnown   bool
	dhcpRunning map[string]bool // DHCP server ID -> running
}{dhcpRunning: make(map[string]bool)}

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

// tftpStatusString renders the TFTP state honestly: "unknown" until something
// reports it, never a guessed "running".
func tftpStatusString() string {
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

// setDHCPRunning records the running state of a DHCP server.
func setDHCPRunning(serverID string, running bool) {
	serviceRegistry.Lock()
	serviceRegistry.dhcpRunning[serverID] = running
	serviceRegistry.Unlock()
}

// forgetDHCPServer drops the tracked state for a deleted DHCP server.
func forgetDHCPServer(serverID string) {
	serviceRegistry.Lock()
	delete(serviceRegistry.dhcpRunning, serverID)
	serviceRegistry.Unlock()
}

// isDHCPRunning reports tracked running state for a DHCP server, falling back
// to the server model's Started field when nothing was tracked.
func isDHCPRunning(serverID string, modelStarted bool) bool {
	serviceRegistry.RLock()
	running, ok := serviceRegistry.dhcpRunning[serverID]
	serviceRegistry.RUnlock()
	if ok {
		return running
	}
	return modelStarted
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
	Name    string `json:"name"`
	Status  string `json:"status"`
	Details string `json:"details"`
	Port    int    `json:"port"`
}

// DHCPServerStatus represents DHCP server status for the status page
type DHCPServerStatus struct {
	ID       string `json:"id"`
	Network  string `json:"network"`
	Status   string `json:"status"`
	LeaseNum int    `json:"lease_num"`
}

// StatusPageData represents data for the status page
type StatusPageData struct {
	Title       string             `json:"title"`
	HTTPServer  ServiceStatus      `json:"http_server"`
	TFTPServer  ServiceStatus      `json:"tftp_server"`
	DHCPServers []DHCPServerStatus `json:"dhcp_servers"`
	LastUpdated string             `json:"last_updated"`
	AutoRefresh bool               `json:"auto_refresh"`
	RefreshSecs int                `json:"refresh_secs"`
}

// getStatusData builds the status page data from tracked runtime state.
func (h *StatusHandlers) getStatusData(r *http.Request) (*StatusPageData, error) {
	ctx := r.Context()

	// HTTP server status — a TCP dial to our own listener as a smoke check.
	// The configured port is a string; an unparseable value fails the check.
	httpPort, _ := strconv.Atoi(h.container.Config.HTTP.Port)
	httpStatus := checkHTTPServerStatus(httpPort)

	// TFTP server status — tracked runtime state, never a UDP guess.
	tftpStatus := ServiceStatus{
		Name:    "TFTP Server",
		Status:  tftpStatusString(),
		Details: fmt.Sprintf("Port %d", tftpPort),
		Port:    tftpPort,
	}

	// DHCP servers status — from the tracked running state, falling back to
	// the server model's Started field.
	servers, err := h.container.ServerService.GetAllServers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get DHCP servers: %w", err)
	}

	var dhcpServers []DHCPServerStatus
	for _, server := range servers {
		leases, _ := h.container.LeaseService.GetLeasesByServer(ctx, server.ID)

		status := "configured"
		if isDHCPRunning(server.ID, server.Started) {
			status = "running"
		}

		dhcpServers = append(dhcpServers, DHCPServerStatus{
			ID:       server.ID,
			Network:  server.IP.String(),
			Status:   status,
			LeaseNum: len(leases),
		})
	}

	return &StatusPageData{
		Title:       "System Status",
		HTTPServer:  httpStatus,
		TFTPServer:  tftpStatus,
		DHCPServers: dhcpServers,
		LastUpdated: time.Now().Format("2006-01-02 15:04:05"),
		AutoRefresh: true,
		RefreshSecs: 5,
	}, nil
}

// HandleStatusPage serves the status monitoring page
func (h *StatusHandlers) HandleStatusPage(w http.ResponseWriter, r *http.Request) {
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

	renderCachedTemplate(w, r, "status", data, "Unable to render the status page")
}

// HandleStatusContent serves status content for HTMX updates
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
		Name:    "HTTP Server",
		Status:  "stopped",
		Details: fmt.Sprintf("Port %d", port),
		Port:    port,
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
