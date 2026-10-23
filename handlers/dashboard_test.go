package handlers

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"ignite/dhcp"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestDashboardSummaryHTMLAndJSON(t *testing.T) {
	chdirRepoRoot(t)

	now := time.Now()
	server := &dhcp.Server{
		ID:         "srv-1",
		IP:         net.ParseIP("192.168.1.10"),
		LeaseRange: 10,
	}
	leases := []*dhcp.Lease{
		{ServerID: "srv-1", Reserved: true, Expiry: now.Add(-time.Hour), IP: net.ParseIP("192.168.1.20")},
		{ServerID: "srv-1", Expiry: now.Add(time.Hour), IP: net.ParseIP("192.168.1.21"), State: dhcp.StateFailed},
		{ServerID: "srv-1", Expiry: now.Add(-time.Hour), IP: net.ParseIP("192.168.1.22")},
	}

	servers := &MockServerService{}
	servers.On("GetAllServers", mock.Anything).Return([]*dhcp.Server{server}, nil)
	leaseSvc := &MockLeaseService{}
	leaseSvc.On("GetLeasesByServer", mock.Anything, "srv-1").Return(leases, nil)

	h := NewDashboardHandlers(&Container{ServerService: servers, LeaseService: leaseSvc})

	t.Run("html fragment", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/dashboard/summary", nil)
		req.Header.Set("Accept", "text/html")
		rec := httptest.NewRecorder()
		h.Summary(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Header().Get("Content-Type"), "text/html")
		body := rec.Body.String()
		require.Contains(t, body, `class="stat-value">1`)
		require.Contains(t, body, "In flight")
		require.Contains(t, body, "Failed")
		require.Contains(t, body, "192.168.1.10")
		require.Contains(t, body, "stat")
		require.NotContains(t, body, "#ff")
	})

	t.Run("json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/dashboard/summary?format=json", nil)
		rec := httptest.NewRecorder()
		h.Summary(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		var summary dhcp.DashboardSummary
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &summary))
		require.Equal(t, 1, summary.Active)
		require.Equal(t, 1, summary.Expired)
		require.Equal(t, 1, summary.Reserved)
		require.Equal(t, 0, summary.InFlight)
		require.Equal(t, 1, summary.Failed)
		require.Len(t, summary.Pools, 1)
		require.Equal(t, "192.168.1.10", summary.Pools[0].Server)
		require.Equal(t, 2, summary.Pools[0].Used)
		require.Equal(t, 8, summary.Pools[0].Free)
		require.Equal(t, 10, summary.Pools[0].Total)
		require.Equal(t, 0, summary.Pools[0].InFlight)
		require.Equal(t, 1, summary.Pools[0].Failed)
	})
}

func chdirRepoRoot(t *testing.T) {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test source file")
	}
	root := filepath.Join(filepath.Dir(thisFile), "..")
	prev, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(root))
	t.Cleanup(func() { _ = os.Chdir(prev) })
}

func TestDashboardSummaryJSONAcceptHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/dashboard/summary", nil)
	req.Header.Set("Accept", "application/json")
	require.True(t, wantsDashboardJSON(req))

	html := httptest.NewRequest(http.MethodGet, "/dashboard/summary", nil)
	html.Header.Set("Accept", "text/html, application/json")
	require.False(t, wantsDashboardJSON(html))

	htmx := httptest.NewRequest(http.MethodGet, "/dashboard/summary", nil)
	htmx.Header.Set("Accept", "text/html")
	htmx.Header.Set("HX-Request", "true")
	require.False(t, wantsDashboardJSON(htmx))
	require.False(t, strings.Contains(htmx.Header.Get("Accept"), "unused"))
}
