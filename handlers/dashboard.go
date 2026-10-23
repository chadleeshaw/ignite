package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ignite/dhcp"
)

// DashboardHandlers serves Overview fragments.
type DashboardHandlers struct {
	container *Container
}

// NewDashboardHandlers returns dashboard handlers.
func NewDashboardHandlers(container *Container) *DashboardHandlers {
	return &DashboardHandlers{container: container}
}

// Summary handles GET /dashboard/summary.
// Browsers and htmx receive an HTML fragment. Clients that ask for JSON
// (Accept: application/json, or ?format=json) receive the same fields as JSON.
func (h *DashboardHandlers) Summary(w http.ResponseWriter, r *http.Request) {
	summary, err := h.summary(r)
	if err != nil {
		HandleError(w, r, NewInternalError(
			fmt.Sprintf("failed to build dashboard summary: %v", err),
			"Unable to load the overview summary",
		))
		return
	}
	if wantsDashboardJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(summary)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	renderCachedTemplate(w, r, "dashboard-summary", summary, "Unable to render the overview summary")
}

func (h *DashboardHandlers) summary(r *http.Request) (dhcp.DashboardSummary, error) {
	empty := dhcp.DashboardSummary{Pools: []dhcp.PoolStat{}}
	if h == nil || h.container == nil || h.container.ServerService == nil || h.container.LeaseService == nil {
		return empty, fmt.Errorf("dashboard dependencies are not configured")
	}
	ctx := r.Context()
	servers, err := h.container.ServerService.GetAllServers(ctx)
	if err != nil {
		return empty, err
	}
	byServer := make(map[string][]*dhcp.Lease, len(servers))
	for _, server := range servers {
		if server == nil {
			continue
		}
		leases, err := h.container.LeaseService.GetLeasesByServer(ctx, server.ID)
		if err != nil {
			return empty, err
		}
		byServer[server.ID] = leases
	}
	summary := dhcp.BuildDashboardSummary(servers, byServer, time.Now())
	return summary, nil
}

// wantsDashboardJSON reports whether the summary should be JSON.
// htmx sends Accept: text/html, which stays on the HTML fragment.
func wantsDashboardJSON(r *http.Request) bool {
	if strings.EqualFold(r.URL.Query().Get("format"), "json") {
		return true
	}
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "application/json") && !strings.Contains(accept, "text/html") {
		return true
	}
	return false
}
