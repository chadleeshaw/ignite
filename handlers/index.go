package handlers

import (
	"log"
	"net/http"
)

// OverviewPage is the home page. Mode is the server preference; an empty
// value renders as cozy. Status is nil when the status read fails, and the
// summary slot then shows only the figures we could collect.
type OverviewPage struct {
	Title           string
	Mode            string
	Page            string
	OverallStatus   string
	CozySubtitle    string
	ConsoleSubtitle string
	Status          *StatusPageData
	LeaseTotal      int
	ServerCount     int
	DHCPRunning     int
	HTTPStatus      string
	TFTPStatus      string
}

// IndexHandlers handles index page requests
type IndexHandlers struct {
	container *Container
}

// NewIndexHandlers creates a new IndexHandlers instance
func NewIndexHandlers(container *Container) *IndexHandlers {
	return &IndexHandlers{container: container}
}

// Index serves the overview. Service tiles use the same status data as
// /status. Lease and pool counts are filled by GET /dashboard/summary when
// that route exists; the slot keeps real service figures until then.
func (h *IndexHandlers) Index(w http.ResponseWriter, r *http.Request) {
	status, err := NewStatusHandlers(h.container).getStatusData(r)
	if err != nil {
		log.Printf("overview status: %v", err)
		status = nil
	}

	cozy, console := describeStatus(status)
	page := OverviewPage{
		Title:           "Overview",
		Page:            "overview",
		CozySubtitle:    cozy,
		ConsoleSubtitle: console,
		Status:          status,
		HTTPStatus:      "unknown",
		TFTPStatus:      "unknown",
	}
	if status != nil {
		page.OverallStatus = status.OverallStatus
		page.HTTPStatus = status.HTTPServer.Status
		page.TFTPStatus = status.TFTPServer.Status
		page.ServerCount = len(status.DHCPServers)
		for _, srv := range status.DHCPServers {
			page.LeaseTotal += srv.LeaseCount
			if srv.Status == "running" {
				page.DHCPRunning++
			}
		}
	}

	renderCachedTemplate(w, r, "index", page, "Unable to render the home page")
}
