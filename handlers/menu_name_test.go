package handlers

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ignite/config"
	"ignite/dhcp"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestDHCPServersMenuNameHidesPaths(t *testing.T) {
	root := t.TempDir()
	cfgDir := filepath.Join(root, "pxelinux.cfg")
	require.NoError(t, os.MkdirAll(cfgDir, 0o755))
	pxe := "LABEL ubuntu\n  KERNEL ubuntu/24.04/vmlinuz\n"
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "01-00-11-22-33-44-66"), []byte(pxe), 0o644))

	stale := "/tmp/ignite-tftp.UPoTst/pxelinux.cfg/01-00-11-22-33-44-55"
	leases := []*dhcp.Lease{
		{
			ServerID: "srv",
			MAC:      "00:11:22:33:44:55",
			IP:       net.ParseIP("10.0.1.50"),
			State:    dhcp.StateAssigned,
			Menu:     dhcp.BootMenu{Filename: stale, OS: "ubuntu", Version: "24.04"},
		},
		{
			ServerID: "srv",
			MAC:      "00:11:22:33:44:66",
			IP:       net.ParseIP("10.0.1.100"),
			State:    dhcp.StateImaging,
			Menu:     dhcp.BootMenu{Filename: "/tmp/ignite-tftp.old/pxelinux.cfg/01-00-11-22-33-44-66"},
		},
	}
	servers := &MockServerService{}
	leaseSvc := &MockLeaseService{}
	servers.On("GetAllServers", mock.Anything).Return([]*dhcp.Server{{
		ID: "srv", IP: net.ParseIP("10.0.1.1"), LeaseRange: 50,
	}}, nil)
	leaseSvc.On("GetLeasesByServer", mock.Anything, "srv").Return(leases, nil)

	h := &DHCPHandlers{
		serverService: servers,
		leaseService:  leaseSvc,
		config:        &config.Config{TFTP: config.TFTPConfig{Dir: root}},
	}
	req := httptest.NewRequest(http.MethodGet, "/dhcp/servers?format=json", nil)
	rec := httptest.NewRecorder()
	h.GetDHCPServers(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	require.NotContains(t, body, "ignite-tftp")
	require.NotContains(t, body, "pxelinux.cfg")

	var parsed struct {
		Servers []DHCPServerView `json:"servers"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &parsed))
	require.Len(t, parsed.Servers, 1)
	byIP := map[string]LeaseView{}
	for _, lease := range parsed.Servers[0].Leases {
		byIP[lease.IP] = lease
	}
	require.Equal(t, "ubuntu 24.04", byIP["10.0.1.50"].MenuName)
	require.Empty(t, byIP["10.0.1.50"].Menu.Filename)
	require.Equal(t, "ubuntu 24.04", byIP["10.0.1.100"].MenuName)
	require.Equal(t, string(dhcp.StateImaging), byIP["10.0.1.100"].State)
	require.False(t, strings.Contains(byIP["10.0.1.100"].MenuName, "/"))
}
