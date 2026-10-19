package handlers

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ignite/dhcp"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestInFlightAndFailedOnOverviewAndDHCP(t *testing.T) {
	chdirRepoRoot(t)
	now := time.Now()
	server := &dhcp.Server{
		ID:         "srv-1",
		IP:         net.ParseIP("10.1.0.1"),
		LeaseRange: 8,
	}
	leases := []*dhcp.Lease{
		{ServerID: "srv-1", MAC: "aa:bb:cc:dd:ee:01", IP: net.ParseIP("10.1.0.10"), Reserved: true, Expiry: now.Add(-time.Hour), State: dhcp.StateImaging},
		{ServerID: "srv-1", MAC: "aa:bb:cc:dd:ee:02", IP: net.ParseIP("10.1.0.11"), Expiry: now.Add(-time.Hour), State: dhcp.StateFailed},
		{ServerID: "srv-1", MAC: "aa:bb:cc:dd:ee:03", IP: net.ParseIP("10.1.0.12"), Expiry: now.Add(time.Hour), State: dhcp.StatePXERequested},
	}

	servers := &MockServerService{}
	servers.On("GetAllServers", mock.Anything).Return([]*dhcp.Server{server}, nil)
	leaseSvc := &MockLeaseService{}
	leaseSvc.On("GetLeasesByServer", mock.Anything, "srv-1").Return(leases, nil)
	container := &Container{ServerService: servers, LeaseService: leaseSvc}

	t.Run("dashboard json and html", func(t *testing.T) {
		h := NewDashboardHandlers(container)
		req := httptest.NewRequest(http.MethodGet, "/dashboard/summary?format=json", nil)
		rec := httptest.NewRecorder()
		h.Summary(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		body := rec.Body.String()
		require.Contains(t, body, `"in_flight":`)
		require.Contains(t, body, `"failed":`)
		var summary dhcp.DashboardSummary
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &summary))
		require.Equal(t, 1, summary.Active)
		require.Equal(t, 1, summary.Expired)
		require.Equal(t, 1, summary.Reserved)
		require.Equal(t, 2, summary.InFlight)
		require.Equal(t, 1, summary.Failed)
		require.Equal(t, 2, summary.Pools[0].InFlight)
		require.Equal(t, 1, summary.Pools[0].Failed)

		htmlReq := httptest.NewRequest(http.MethodGet, "/dashboard/summary", nil)
		htmlReq.Header.Set("Accept", "text/html")
		htmlRec := httptest.NewRecorder()
		h.Summary(htmlRec, htmlReq)
		require.Equal(t, http.StatusOK, htmlRec.Code)
		page := htmlRec.Body.String()
		require.Contains(t, page, "In flight")
		require.Contains(t, page, "Failed")
		require.Contains(t, page, `class="stat-value">2`)
		require.Contains(t, page, "2 in flight")
		require.Contains(t, page, "1 failed")
	})

	t.Run("dhcp servers json and html", func(t *testing.T) {
		h := NewDHCPHandlers(container)
		req := httptest.NewRequest(http.MethodGet, "/dhcp/servers?format=json", nil)
		rec := httptest.NewRecorder()
		h.GetDHCPServers(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		body := rec.Body.String()
		require.Contains(t, body, `"in_flight":`)
		require.Contains(t, body, `"failed":`)
		var payload struct {
			Servers []DHCPServerView `json:"servers"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
		require.Len(t, payload.Servers, 1)
		require.Equal(t, 2, payload.Servers[0].InFlight)
		require.Equal(t, 1, payload.Servers[0].Failed)
		require.Equal(t, 2, payload.Servers[0].Used)

		htmlReq := httptest.NewRequest(http.MethodGet, "/dhcp/servers", nil)
		htmlReq.Header.Set("Accept", "text/html")
		htmlRec := httptest.NewRecorder()
		h.GetDHCPServers(htmlRec, htmlReq)
		require.Equal(t, http.StatusOK, htmlRec.Code)
		page := htmlRec.Body.String()
		require.Contains(t, page, "2 in flight")
		require.Contains(t, page, "1 failed")
	})

	t.Run("dhcp page strip", func(t *testing.T) {
		h := NewDHCPHandlers(container)
		req := httptest.NewRequest(http.MethodGet, "/dhcp", nil)
		rec := httptest.NewRecorder()
		h.HandleDHCPPage(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		page := rec.Body.String()
		require.Contains(t, page, "In flight")
		require.Contains(t, page, "Failed")
		require.Contains(t, page, `class="stat-value">2`)
		require.Contains(t, page, `class="stat-value">1`)
	})
}
