package handlers

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"ignite/config"
	"ignite/dhcp"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestEveryDHCPViewReportsTheSameListenerState(t *testing.T) {
	chdirRepoRoot(t)

	const (
		runningID    = "state-running"
		stoppedID    = "state-stopped"
		configuredID = "state-configured"
	)
	dhcp.NoteServerState(runningID, dhcp.ServerStateRunning)
	dhcp.NoteServerState(stoppedID, dhcp.ServerStateStopped)
	t.Cleanup(func() {
		dhcp.ClearServerState(runningID)
		dhcp.ClearServerState(stoppedID)
		dhcp.ClearServerState(configuredID)
		SetTFTPRunning(false)
	})

	servers := []*dhcp.Server{
		{ID: runningID, IP: net.ParseIP("10.0.0.1"), LeaseRange: 10, Started: false},
		{ID: stoppedID, IP: net.ParseIP("10.0.0.2"), LeaseRange: 10, Started: true},
		{ID: configuredID, IP: net.ParseIP("10.0.0.3"), LeaseRange: 10, Started: true},
	}
	serverSvc := &MockServerService{}
	leaseSvc := &MockLeaseService{}
	serverSvc.On("GetAllServers", mock.Anything).Return(servers, nil)
	for _, id := range []string{runningID, stoppedID, configuredID} {
		leaseSvc.On("GetLeasesByServer", mock.Anything, id).Return([]*dhcp.Lease{}, nil)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	go func() { _ = http.Serve(ln, http.NewServeMux()) }()
	t.Cleanup(func() { _ = ln.Close() })
	SetTFTPRunning(true)

	cfg := &config.Config{HTTP: config.HTTPConfig{Port: strconv.Itoa(port)}}
	container := &Container{ServerService: serverSvc, LeaseService: leaseSvc, Config: cfg}
	dhcpHandlers := &DHCPHandlers{serverService: serverSvc, leaseService: leaseSvc, config: cfg}

	want := map[string]string{
		runningID:    "running",
		stoppedID:    "stopped",
		configuredID: "configured",
	}

	t.Run("dhcp servers json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/dhcp/servers?format=json", nil)
		rec := httptest.NewRecorder()
		dhcpHandlers.GetDHCPServers(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		var body struct {
			Servers []DHCPServerView `json:"servers"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		require.Len(t, body.Servers, 3)
		for _, server := range body.Servers {
			require.Equal(t, want[server.ID], server.Status)
		}
	})

	t.Run("dhcp servers html", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/dhcp/servers", nil)
		req.Header.Set("Accept", "text/html")
		rec := httptest.NewRecorder()
		dhcpHandlers.GetDHCPServers(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		body := rec.Body.String()
		require.Contains(t, body, `data-status="running"`)
		require.Contains(t, body, `data-status="stopped"`)
		require.Contains(t, body, `data-status="configured"`)
		require.Contains(t, body, ">Running<")
		require.Contains(t, body, ">Stopped<")
		require.Contains(t, body, ">Configured<")
		require.NotContains(t, body, "badge-success\">stopped")
	})

	t.Run("dhcp page", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/dhcp", nil)
		rec := httptest.NewRecorder()
		dhcpHandlers.HandleDHCPPage(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		body := rec.Body.String()
		require.Contains(t, body, `data-status="running"`)
		require.Contains(t, body, `data-status="stopped"`)
		require.Contains(t, body, `data-status="configured"`)
		require.Contains(t, body, ">Running<")
		require.Contains(t, body, ">Stopped<")
		require.Contains(t, body, ">Configured<")
	})

	t.Run("status content", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/status/content", nil)
		rec := httptest.NewRecorder()
		NewStatusHandlers(container).HandleStatusContent(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		body := rec.Body.String()
		require.Contains(t, body, `data-status="running"`)
		require.Contains(t, body, `data-status="stopped"`)
		require.Contains(t, body, `data-status="configured"`)
		require.Contains(t, body, ">Running<")
		require.Contains(t, body, ">Stopped<")
		require.Contains(t, body, ">Configured<")
		require.Contains(t, body, "Partial service")
		require.Contains(t, body, `data-attention-cozy="1 thing needs attention: DHCP 10.0.0.2 is stopped"`)
	})

	t.Run("dashboard summary", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/dashboard/summary?format=json", nil)
		rec := httptest.NewRecorder()
		NewDashboardHandlers(container).Summary(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		var summary dhcp.DashboardSummary
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &summary))
		got := map[string]dhcp.ServerState{}
		for _, pool := range summary.Pools {
			got[pool.Server] = pool.Status
		}
		require.Equal(t, dhcp.ServerStateRunning, got["10.0.0.1"])
		require.Equal(t, dhcp.ServerStateStopped, got["10.0.0.2"])
		require.Equal(t, dhcp.ServerStateConfigured, got["10.0.0.3"])

		htmlReq := httptest.NewRequest(http.MethodGet, "/dashboard/summary", nil)
		htmlReq.Header.Set("Accept", "text/html")
		htmlRec := httptest.NewRecorder()
		NewDashboardHandlers(container).Summary(htmlRec, htmlReq)
		require.Equal(t, http.StatusOK, htmlRec.Code)
		htmlBody := htmlRec.Body.String()
		for _, title := range []string{"Active leases", "Reserved", "Expired", "In flight", "Failed", "Free addresses"} {
			require.Contains(t, htmlBody, title)
		}
		require.Contains(t, htmlBody, "0 used / 30 total")
	})
}

func TestOverallHealthIgnoresConfiguredDHCP(t *testing.T) {
	require.Equal(t, "healthy", overallServiceStatus("running", "running", []dhcp.ServerState{
		dhcp.ServerStateRunning,
		dhcp.ServerStateConfigured,
	}))
	require.Equal(t, "partial", overallServiceStatus("running", "running", []dhcp.ServerState{
		dhcp.ServerStateRunning,
		dhcp.ServerStateStopped,
	}))
	require.Equal(t, "partial", overallServiceStatus("running", "stopped", nil))
	require.Equal(t, "down", overallServiceStatus("stopped", "running", []dhcp.ServerState{dhcp.ServerStateRunning}))
}

func TestDashboardSummaryIncludesConfiguredWhenUnset(t *testing.T) {
	now := time.Now()
	stat := dhcp.PoolStatForServer(&dhcp.Server{ID: "unset-pool", IP: net.ParseIP("10.9.9.9"), LeaseRange: 1}, nil, now)
	require.Equal(t, dhcp.ServerStateConfigured, stat.Status)
}
