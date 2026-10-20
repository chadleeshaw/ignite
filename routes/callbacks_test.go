package routes

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"ignite/dhcp"

	"github.com/stretchr/testify/require"
)

// callbackLeases is the small slice of LeaseService the host routes touch.
type callbackLeases struct {
	lease     *dhcp.Lease
	heartbeat int
	state     string
	source    string
}

func (s *callbackLeases) AssignLease(ctx context.Context, serverID string, mac string, requestedIP net.IP) (*dhcp.Lease, error) {
	return nil, nil
}
func (s *callbackLeases) ReleaseLease(ctx context.Context, mac string) error { return nil }
func (s *callbackLeases) ReserveLease(ctx context.Context, serverID string, mac string, ip net.IP) error {
	return nil
}
func (s *callbackLeases) UnreserveLease(ctx context.Context, mac string) error { return nil }
func (s *callbackLeases) GetLeaseByMAC(ctx context.Context, mac string) (*dhcp.Lease, error) {
	if s.lease != nil && s.lease.MAC == mac {
		return s.lease, nil
	}
	return nil, nil
}
func (s *callbackLeases) GetLeasesByServer(ctx context.Context, serverID string) ([]*dhcp.Lease, error) {
	return nil, nil
}
func (s *callbackLeases) CleanupExpiredLeases(ctx context.Context) error { return nil }
func (s *callbackLeases) UpdateLease(ctx context.Context, lease *dhcp.Lease) error {
	return nil
}
func (s *callbackLeases) UpdateLeaseState(ctx context.Context, mac string, newState string, source string) error {
	s.state = newState
	s.source = source
	return nil
}
func (s *callbackLeases) RecordHeartbeat(ctx context.Context, mac string) error {
	s.heartbeat++
	return nil
}
func (s *callbackLeases) GetLeaseStateHistory(ctx context.Context, mac string) ([]dhcp.StateTransition, error) {
	return nil, nil
}
func (s *callbackLeases) GetLeasesByState(ctx context.Context, state string) ([]*dhcp.Lease, error) {
	return nil, nil
}
func (s *callbackLeases) MarkOfflineLeases(ctx context.Context, offlineThreshold time.Duration) error {
	return nil
}

func TestHostCallbackRoutesBypassSessionLogin(t *testing.T) {
	const (
		mac   = "aa:bb:cc:dd:ee:04"
		token = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	)
	leases := &callbackLeases{lease: &dhcp.Lease{MAC: mac, ProvisionToken: token}}
	container := createTestContainer()
	container.LeaseService = leases
	router := SetupWithContainerAndStatic(container, createTestStaticHandler())

	for _, name := range []string{"RecordHeartbeat", "HostUpdateLeaseState", "DashboardSummary", "GetUIMode", "SetUIMode"} {
		require.NotNil(t, router.Get(name), "route %s is not registered", name)
	}

	noToken := httptest.NewRequest(http.MethodPost, "/dhcp/lease/"+mac+"/heartbeat", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, noToken)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.NotEqual(t, "/login", rec.Header().Get("Location"))

	withToken := httptest.NewRequest(http.MethodPost, "/dhcp/lease/"+mac+"/heartbeat", nil)
	withToken.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, withToken)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, leases.heartbeat)

	admin := httptest.NewRequest(http.MethodPost, "/dhcp/lease/state?mac="+mac, nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, admin)
	require.Equal(t, http.StatusFound, rec.Code)
	require.Equal(t, "/login", rec.Header().Get("Location"))
}

func TestPreferenceAndSummaryRoutesRequireSession(t *testing.T) {
	router := SetupWithContainerAndStatic(createTestContainer(), createTestStaticHandler())

	login := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(url.Values{
		"username": {"admin"},
		"password": {"admin"},
	}.Encode()))
	login.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginRec := httptest.NewRecorder()
	router.ServeHTTP(loginRec, login)
	require.Equal(t, http.StatusOK, loginRec.Code)
	var session *http.Cookie
	for _, c := range loginRec.Result().Cookies() {
		if c.Name == "ignite_session" {
			session = c
		}
	}
	require.NotNil(t, session)

	// /status is a bookmark for Overview. /status/content stays the fragment.
	statusReq := httptest.NewRequest(http.MethodGet, "/status", nil)
	statusReq.AddCookie(session)
	statusRec := httptest.NewRecorder()
	router.ServeHTTP(statusRec, statusReq)
	require.Equal(t, http.StatusFound, statusRec.Code)
	require.Equal(t, "/", statusRec.Header().Get("Location"))

	require.NotNil(t, router.Get("StatusContent"))

	for _, path := range []string{"/dashboard/summary", "/status/content"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusFound, rec.Code, path)
		require.Equal(t, "/login", rec.Header().Get("Location"), path)
	}

	// /api/ routes answer unauthenticated callers with JSON 401, not a login redirect.
	api := httptest.NewRequest(http.MethodGet, "/api/preferences/mode", nil)
	apiRec := httptest.NewRecorder()
	router.ServeHTTP(apiRec, api)
	require.Equal(t, http.StatusUnauthorized, apiRec.Code)
	require.Contains(t, apiRec.Body.String(), "Authentication required")
}
