package handlers

import (
	"fmt"
	"strings"
)

// describeStatus turns tracked service state into the Overview subtitle.
// Each DHCP server that is not running is its own item, so three stopped
// servers are three things, not one clause. One item uses "needs"; more
// than one uses "need".
func describeStatus(s *StatusPageData) (cozy, console string) {
	if s == nil {
		return "Service status is unavailable", "status unavailable"
	}
	var parts []string
	if s.HTTPServer.Status != "running" {
		parts = append(parts, "HTTP "+s.HTTPServer.Status)
	}
	if s.TFTPServer.Status != "running" {
		parts = append(parts, "TFTP "+s.TFTPServer.Status)
	}
	for _, srv := range s.DHCPServers {
		if srv.Status != "running" {
			parts = append(parts, "DHCP "+srv.IP)
		}
	}
	if len(parts) == 0 {
		return "All services are up", "services up"
	}
	noun := "things"
	verb := "need"
	if len(parts) == 1 {
		noun = "thing"
		verb = "needs"
	}
	joined := strings.Join(parts, " · ")
	return fmt.Sprintf("%d %s %s attention: %s", len(parts), noun, verb, joined), joined
}
