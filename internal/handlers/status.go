package handlers

import (
	"fmt"
	"net"
	"net/http"
	"time"
)

// HandleStatusPage renders the status overview page, showing whether DHCP and TFTP services are running.
func (h *Handlers) HandleStatusPage(w http.ResponseWriter, r *http.Request) {
	templates := LoadTemplates()

	data := struct {
		Title      string
		DHCPStatus bool
		TFTPStatus bool
	}{
		Title:      "Service Status Overview",
		DHCPStatus: isServerRunning("67"),
		TFTPStatus: isServerRunning("69"),
	}

	if err := templates["status"].Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// isServerRunning checks if a server is running on a given UDP port by attempting to establish a connection.
func isServerRunning(port string) bool {
	host := fmt.Sprintf("localhost:%s", port)
	conn, err := net.DialTimeout("udp4", host, time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()
	return true
}
