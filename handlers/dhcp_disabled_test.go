package handlers

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"ignite/config"
	"ignite/dhcp"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestStartDHCPServerDisabledIsConflict(t *testing.T) {
	servers := &MockServerService{}
	servers.On("StartServer", mock.Anything, "srv").Return(dhcp.ErrDHCPDisabled)
	h := &DHCPHandlers{serverService: servers, leaseService: &MockLeaseService{}}

	req := httptest.NewRequest(http.MethodPost, "/dhcp/start?server_id=srv", nil)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	h.StartDHCPServer(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
	require.Contains(t, rec.Body.String(), "DHCP is disabled")
	require.Contains(t, rec.Body.String(), "DHCP_ENABLED=true")
	servers.AssertExpectations(t)
}

func TestDHCPDisabledStatusStaysConfiguredAndHealthy(t *testing.T) {
	chdirRepoRoot(t)
	const id = "disabled-stopped"
	dhcp.SetListenersEnabled(false)
	dhcp.NoteServerState(id, dhcp.ServerStateStopped)
	t.Cleanup(func() {
		dhcp.SetListenersEnabled(true)
		dhcp.ClearServerState(id)
		SetTFTPRunning(false)
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	go func() { _ = http.Serve(ln, http.NewServeMux()) }()
	t.Cleanup(func() { _ = ln.Close() })
	SetTFTPRunning(true)

	server := &dhcp.Server{ID: id, IP: net.ParseIP("10.8.0.1"), LeaseRange: 4, Started: true}
	servers := &MockServerService{}
	leases := &MockLeaseService{}
	servers.On("GetAllServers", mock.Anything).Return([]*dhcp.Server{server}, nil)
	leases.On("GetLeasesByServer", mock.Anything, id).Return([]*dhcp.Lease{}, nil)
	cfg := &config.Config{
		HTTP: config.HTTPConfig{Port: strconv.Itoa(port)},
		TFTP: config.TFTPConfig{Port: "6969"},
		DHCP: config.DHCPConfig{Enabled: false},
	}
	h := NewStatusHandlers(&Container{ServerService: servers, LeaseService: leases, Config: cfg})

	req := httptest.NewRequest(http.MethodGet, "/status/content", nil)
	rec := httptest.NewRecorder()
	h.HandleStatusContent(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	require.Contains(t, body, `data-status="configured"`)
	require.Contains(t, body, ">Configured<")
	require.Contains(t, body, "All services up")
	require.NotContains(t, body, ">Stopped<")
	require.Contains(t, body, "port 6969")

	views, _, err := (&DHCPHandlers{serverService: servers, leaseService: leases, config: cfg}).serverViews(req.Context(), []*dhcp.Server{server})
	require.NoError(t, err)
	require.Equal(t, "configured", views[0].Status)
}
