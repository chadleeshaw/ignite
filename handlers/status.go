package handlers

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// StatusHandlers handles status-related requests
type StatusHandlers struct {
	container *Container
}

// ServiceStatus represents the status of a single service
type ServiceStatus struct {
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	Description string    `json:"description"`
	LastCheck   time.Time `json:"last_check"`
	Address     string    `json:"address,omitempty"`
	Port        string    `json:"port,omitempty"`
	Details     string    `json:"details,omitempty"`
}

// DHCPServerStatus represents the status of a DHCP server
type DHCPServerStatus struct {
	ID          string    `json:"id"`
	IP          string    `json:"ip"`
	Status      string    `json:"status"`
	Description string    `json:"description"`
	LastCheck   time.Time `json:"last_check"`
	LeaseCount  int       `json:"lease_count"`
}

// SystemStatus represents the overall system status
type SystemStatus struct {
	Title         string             `json:"title"`
	LastUpdated   time.Time          `json:"last_updated"`
	HTTPServer    ServiceStatus      `json:"http_server"`
	APIServer     ServiceStatus      `json:"api_server"`
	TFTPServer    ServiceStatus      `json:"tftp_server"`
	DHCPServers   []DHCPServerStatus `json:"dhcp_servers"`
	OverallStatus string             `json:"overall_status"`
}

// NewStatusHandlers creates a new StatusHandlers instance
func NewStatusHandlers(container *Container) *StatusHandlers {
	return &StatusHandlers{container: container}
}

// HandleStatusPage serves the status page
func (h *StatusHandlers) HandleStatusPage(w http.ResponseWriter, r *http.Request) {
	status := h.getSystemStatus()

	templates := LoadTemplates()
	if err := templates["status"].Execute(w, status); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// HandleStatusContent returns just the status content for HTMX updates
func (h *StatusHandlers) HandleStatusContent(w http.ResponseWriter, r *http.Request) {
	status := h.getSystemStatus()

	templates := LoadTemplates()
	if err := templates["status-content"].Execute(w, status); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// getSystemStatus collects real-time status information for all services
func (h *StatusHandlers) getSystemStatus() *SystemStatus {
	now := time.Now()

	status := &SystemStatus{
		Title:       "Service Status Overview",
		LastUpdated: now,
	}

	// Check HTTP/API Server status (combined since they run on same server)
	status.HTTPServer = h.checkHTTPServerStatus()
	status.APIServer = status.HTTPServer // Same server, same status

	// Check TFTP Server status
	status.TFTPServer = h.checkTFTPServerStatus()

	// Check DHCP Server statuses
	status.DHCPServers = h.checkDHCPServersStatus()

	// Determine overall status
	status.OverallStatus = h.calculateOverallStatus(status)

	return status
}

// checkHTTPServerStatus checks if the HTTP/API server is running
func (h *StatusHandlers) checkHTTPServerStatus() ServiceStatus {
	now := time.Now()
	port := h.container.Config.HTTP.Port

	status := ServiceStatus{
		Name:      "HTTP/API Server",
		LastCheck: now,
		Address:   "localhost",
		Port:      port,
	}

	// Try to connect to the HTTP server
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%s", port), 2*time.Second)
	if err != nil {
		status.Status = "stopped"
		status.Description = "HTTP/API Server is not responding"
		status.Details = fmt.Sprintf("Error: %v", err)
	} else {
		conn.Close()
		status.Status = "running"
		status.Description = "HTTP/API Server is running"
		status.Details = fmt.Sprintf("Listening on port %s", port)
	}

	return status
}

// checkTFTPServerStatus checks if the TFTP server is running
func (h *StatusHandlers) checkTFTPServerStatus() ServiceStatus {
	now := time.Now()

	status := ServiceStatus{
		Name:      "TFTP Server",
		LastCheck: now,
		Address:   "localhost",
		Port:      "69",
	}

	// Try to connect to the TFTP server on UDP port 69
	conn, err := net.DialTimeout("udp", "localhost:69", 2*time.Second)
	if err != nil {
		status.Status = "stopped"
		status.Description = "TFTP Server is not responding"
		status.Details = fmt.Sprintf("Error: %v", err)
	} else {
		conn.Close()
		status.Status = "running"
		status.Description = "TFTP Server is running"
		status.Details = fmt.Sprintf("Serving from %s", h.container.Config.TFTP.Dir)
	}

	return status
}

// checkDHCPServersStatus checks the status of all configured DHCP servers
func (h *StatusHandlers) checkDHCPServersStatus() []DHCPServerStatus {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	servers, err := h.container.ServerService.GetAllServers(ctx)
	if err != nil {
		// Return empty array if we can't get servers
		return []DHCPServerStatus{}
	}

	var dhcpStatuses []DHCPServerStatus
	now := time.Now()

	for _, server := range servers {
		status := DHCPServerStatus{
			ID:        server.ID,
			IP:        server.IP.String(),
			LastCheck: now,
		}

		// Get lease count for this server
		leases, err := h.container.LeaseService.GetLeasesByServer(ctx, server.ID)
		if err == nil {
			status.LeaseCount = len(leases)
		}

		// Check if server is marked as started in database
		if server.Started {
			// Try to verify the server is actually running by checking UDP port
			conn, err := net.DialTimeout("udp", fmt.Sprintf("%s:67", server.IP.String()), 2*time.Second)
			if err != nil {
				status.Status = "configured"
				status.Description = "Server configured but not responding"
			} else {
				conn.Close()
				status.Status = "running"
				status.Description = "DHCP Server is running"
			}
		} else {
			status.Status = "stopped"
			status.Description = "DHCP Server is stopped"
		}

		dhcpStatuses = append(dhcpStatuses, status)
	}

	return dhcpStatuses
}

// calculateOverallStatus determines the overall system status
func (h *StatusHandlers) calculateOverallStatus(status *SystemStatus) string {
	runningCount := 0
	totalServices := 2 // HTTP and TFTP

	if status.HTTPServer.Status == "running" {
		runningCount++
	}
	if status.TFTPServer.Status == "running" {
		runningCount++
	}

	// Count running DHCP servers
	runningDHCP := 0
	for _, dhcp := range status.DHCPServers {
		if dhcp.Status == "running" {
			runningDHCP++
		}
	}

	if len(status.DHCPServers) > 0 {
		totalServices += len(status.DHCPServers)
		runningCount += runningDHCP
	}

	if runningCount == totalServices {
		return "healthy"
	} else if runningCount > 0 {
		return "partial"
	} else {
		return "down"
	}
}
