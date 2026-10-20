package handlers

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
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

// dhcpStateWord is the raw listener word. ServerState and plain strings
// both arrive here from templates.
func dhcpStateWord(state any) string {
	if state == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(fmt.Sprint(state)))
}

// dhcpStatePill maps the three DHCP states to the label and pill class
// Overview, the DHCP page, and the status bar share.
func dhcpStatePill(state any) (label, class string) {
	switch dhcpStateWord(state) {
	case "running":
		return "Running", "ok"
	case "stopped":
		return "Stopped", "er"
	case "configured", "", "<nil>":
		return "Configured", "cf"
	default:
		word := strings.TrimSpace(fmt.Sprint(state))
		return word, "of"
	}
}

func dhcpStateLabel(state any) string {
	label, _ := dhcpStatePill(state)
	return label
}

func dhcpStateClass(state any) string {
	_, class := dhcpStatePill(state)
	return class
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
	Free        int       `json:"free"`
	Used        int       `json:"used"`
	Total       int       `json:"total"`
	LastCheck   time.Time `json:"last_check"`
}

// StatusPageData represents data for the status page.
// Mode is the server UI preference. Empty means cozy until the preference
// layer sets it. Page highlights Overview, where this content lives.
type StatusPageData struct {
	Title            string             `json:"title"`
	Mode             string             `json:"-"`
	Page             string             `json:"-"`
	CSRFToken        string             `json:"-"`
	HTTPServer       ServiceStatus      `json:"http_server"`
	TFTPServer       ServiceStatus      `json:"tftp_server"`
	DHCPServers      []DHCPServerStatus `json:"dhcp_servers"`
	LastUpdated      string             `json:"last_updated"`
	LastUpdatedAt    string             `json:"last_updated_at"`
	UpdatedAt        string             `json:"updated_at"`
	OverallStatus    string             `json:"overall_status"`
	AttentionCozy    string             `json:"-"`
	AttentionConsole string             `json:"-"`
	AutoRefresh      bool               `json:"auto_refresh"`
	RefreshSecs      int                `json:"refresh_secs"`
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
	for _, server := range servers {
		leases, _ := h.container.LeaseService.GetLeasesByServer(ctx, server.ID)
		state := dhcp.CurrentServerState(server.ID)
		used := dhcp.PoolOccupancy(leases, now)
		total := server.LeaseRange
		free := total - used
		if free < 0 {
			free = 0
		}

		dhcpServers = append(dhcpServers, DHCPServerStatus{
			ID:          server.ID,
			IP:          server.IP.String(),
			Status:      string(state),
			Description: "Dynamic IP assignment for network boot clients",
			LeaseCount:  len(leases),
			Free:        free,
			Used:        used,
			Total:       total,
			LastCheck:   now,
		})
	}
	sort.Slice(dhcpServers, func(i, j int) bool {
		return lessIP(dhcpServers[i].IP, dhcpServers[j].IP)
	})

	// Overall status and the subtitle share statusIssues. HTTP not running
	// is "down". A stopped DHCP listener or TFTP is "partial". A configured
	// DHCP server has not been started this boot and is not an issue.
	data := &StatusPageData{
		Title:         "System Status",
		Page:          "overview",
		HTTPServer:    httpStatus,
		TFTPServer:    tftpStatus,
		DHCPServers:   dhcpServers,
		LastUpdated:   now.UTC().Format("2006-01-02 15:04:05") + " UTC",
		LastUpdatedAt: formatRFC3339(now),
		UpdatedAt:     formatRFC3339(now),
		AutoRefresh:   true,
		RefreshSecs:   5,
	}
	annotateStatus(data)
	return data, nil
}

// lessIP orders dotted IPv4 addresses numerically. Unparseable values sort last.
func lessIP(a, b string) bool {
	ipa := net.ParseIP(a).To4()
	ipb := net.ParseIP(b).To4()
	switch {
	case ipa == nil && ipb == nil:
		return a < b
	case ipa == nil:
		return false
	case ipb == nil:
		return true
	default:
		return bytes.Compare(ipa, ipb) < 0
	}
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

// HandleStatusPage redirects a bookmark for /status to Overview.
// GET /status/content remains the fragment Overview polls.
func (h *StatusHandlers) HandleStatusPage(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/", http.StatusFound)
}

// HandleStatusContent serves status content for HTMX updates.
// There is no /status page; Overview polls this fragment.
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
