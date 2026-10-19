package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"ignite/dhcp"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func hostLease(mac, token string) *dhcp.Lease {
	return &dhcp.Lease{
		ID:             "lease-1",
		MAC:            mac,
		ProvisionToken: token,
		State:          dhcp.StateImaging,
		Expiry:         time.Now().Add(time.Hour),
	}
}

func TestRecordHeartbeatRequiresProvisionToken(t *testing.T) {
	const (
		mac   = "aa:bb:cc:dd:ee:01"
		token = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	)
	leases := &MockLeaseService{}
	leases.On("GetLeaseByMAC", mock.Anything, mac).Return(hostLease(mac, token), nil)
	leases.On("RecordHeartbeat", mock.Anything, mac).Return(nil)

	h := &DHCPHandlers{leaseService: leases}

	t.Run("missing token is 401 json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/dhcp/lease/"+mac+"/heartbeat", nil)
		req = mux.SetURLVars(req, map[string]string{"mac": mac})
		rec := httptest.NewRecorder()
		h.RecordHeartbeat(rec, req)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
		require.Contains(t, rec.Body.String(), "Authentication required")
		require.NotContains(t, rec.Header().Get("Location"), "/login")
		leases.AssertNotCalled(t, "RecordHeartbeat", mock.Anything, mock.Anything)
	})

	t.Run("wrong token is 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/dhcp/lease/"+mac+"/heartbeat", nil)
		req = mux.SetURLVars(req, map[string]string{"mac": mac})
		req.Header.Set("Authorization", "Bearer not-the-token")
		rec := httptest.NewRecorder()
		h.RecordHeartbeat(rec, req)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("matching token records the heartbeat", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/dhcp/lease/"+mac+"/heartbeat", nil)
		req = mux.SetURLVars(req, map[string]string{"mac": strings.ToUpper(mac)})
		req.Header.Set("X-Ignite-Token", token)
		rec := httptest.NewRecorder()
		h.RecordHeartbeat(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Body.String(), "Heartbeat recorded")
		leases.AssertCalled(t, "RecordHeartbeat", mock.Anything, mac)
	})
}

func TestHostUpdateLeaseStateAcceptsFailed(t *testing.T) {
	const (
		mac   = "aa:bb:cc:dd:ee:02"
		token = "abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"
	)
	leases := &MockLeaseService{}
	leases.On("GetLeaseByMAC", mock.Anything, mac).Return(hostLease(mac, token), nil)
	leases.On("UpdateLeaseState", mock.Anything, mac, "failed", "cloud-init").Return(nil)

	h := &DHCPHandlers{leaseService: leases}
	body := strings.NewReader(`{"state":"failed","source":"cloud-init"}`)
	req := httptest.NewRequest(http.MethodPost, "/dhcp/lease/"+mac+"/state", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req = mux.SetURLVars(req, map[string]string{"mac": mac})
	rec := httptest.NewRecorder()

	h.HostUpdateLeaseState(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	leases.AssertCalled(t, "UpdateLeaseState", mock.Anything, mac, "failed", "cloud-init")
}

func TestHostUpdateLeaseStateRejectsUnknownState(t *testing.T) {
	const (
		mac   = "aa:bb:cc:dd:ee:03"
		token = "abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"
	)
	leases := &MockLeaseService{}
	leases.On("GetLeaseByMAC", mock.Anything, mac).Return(hostLease(mac, token), nil)
	h := &DHCPHandlers{leaseService: leases}

	form := url.Values{"state": {"on-fire"}}
	req := httptest.NewRequest(http.MethodPost, "/dhcp/lease/"+mac+"/state", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+token)
	req = mux.SetURLVars(req, map[string]string{"mac": mac})
	rec := httptest.NewRecorder()

	h.HostUpdateLeaseState(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	leases.AssertNotCalled(t, "UpdateLeaseState", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestAuthMiddlewareLetsHostCallbacksThrough(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	wrapped := AuthMiddleware(next)

	callback := httptest.NewRequest(http.MethodPost, "/dhcp/lease/aa:bb:cc:dd:ee:01/heartbeat", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, callback)
	require.Equal(t, http.StatusNoContent, rec.Code)

	admin := httptest.NewRequest(http.MethodPost, "/dhcp/lease/state?mac=aa:bb:cc:dd:ee:01", nil)
	rec = httptest.NewRecorder()
	wrapped.ServeHTTP(rec, admin)
	require.Equal(t, http.StatusFound, rec.Code)
	require.Equal(t, "/login", rec.Header().Get("Location"))
}
