package handlers

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
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

func TestSubmitDHCPRestartDisabledUsesBackendMessage(t *testing.T) {
	servers := &MockServerService{}
	servers.On("UpdateServer", mock.Anything, "srv", mock.Anything).
		Return(fmt.Errorf("server configuration updated but failed to restart server: %w", dhcp.ErrDHCPDisabled))
	h := &DHCPHandlers{serverService: servers, leaseService: &MockLeaseService{}}

	form := url.Values{
		"server_id":  {"srv"},
		"network":    {"192.168.1.2"},
		"subnet":     {"255.255.255.0"},
		"gateway":    {"192.168.1.1"},
		"dns":        {"8.8.8.8"},
		"startIP":    {"192.168.1.100"},
		"endIP":      {"192.168.1.149"},
		"lease_time": {"2"},
	}
	req := httptest.NewRequest(http.MethodPost, "/dhcp/submit_dhcp", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	h.SubmitDHCPServer(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
	require.Contains(t, rec.Body.String(), "DHCP is disabled. Set DHCP_ENABLED=true before starting a server.")
	require.NotContains(t, rec.Body.String(), "<html")
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
	require.NotContains(t, body, "Server ID")

	views, _, err := (&DHCPHandlers{serverService: servers, leaseService: leases, config: cfg}).serverViews(req.Context(), []*dhcp.Server{server})
	require.NoError(t, err)
	require.Equal(t, "configured", views[0].Status)
}
