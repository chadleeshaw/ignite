package handlers

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ignite/config"
	"ignite/dhcp"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestInstallerFieldsRejectInjection(t *testing.T) {
	base := map[string]string{
		"mac":      "AA:BB:CC:DD:EE:01",
		"hostname": "cb-host",
		"tftpip":   "10.0.0.1",
		"ip":       "10.0.0.2",
		"subnet":   "255.255.255.0",
		"gateway":  "10.0.0.1",
		"dns":      "1.1.1.1",
	}
	ok := copyForm(base)
	require.NoError(t, validateInstallerFields(ok))
	require.Equal(t, "aa:bb:cc:dd:ee:01", ok["mac"])

	quoted := copyForm(base)
	quoted["hostname"] = `cb'host`
	require.Error(t, validateInstallerFields(quoted))

	marked := copyForm(base)
	marked["hostname"] = "cb<host"
	require.Error(t, validateInstallerFields(marked))

	badIP := copyForm(base)
	badIP["tftpip"] = `10.0.0.1';id`
	require.Error(t, validateInstallerFields(badIP))

	xmlIP := copyForm(base)
	xmlIP["tftpip"] = "<script>"
	require.Error(t, validateInstallerFields(xmlIP))
}

func TestKernelOptionsRejectControlCharacters(t *testing.T) {
	base := map[string]string{
		"mac":      "aa:bb:cc:dd:ee:01",
		"hostname": "cb-host",
		"tftpip":   "10.0.0.1",
		"ip":       "10.0.0.2",
		"subnet":   "255.255.255.0",
		"gateway":  "10.0.0.1",
		"dns":      "1.1.1.1",
	}
	for _, bad := range []string{"quiet\nrd.break", "quiet\rconsole=ttyS0", "quiet\tconsole=ttyS0", "quiet\x1b[31m"} {
		form := copyForm(base)
		form["kernel_options"] = bad
		err := validateInstallerFields(form)
		require.Error(t, err, bad)
		require.Contains(t, err.Error(), "kernel_options")
	}
	ok := copyForm(base)
	ok["kernel_options"] = "quiet console=ttyS0,115200n8"
	require.NoError(t, validateInstallerFields(ok))
	require.Equal(t, "quiet console=ttyS0,115200n8", ok["kernel_options"])

	empty := copyForm(base)
	require.NoError(t, validateInstallerFields(empty))
}

func TestShellAndXMLEscaping(t *testing.T) {
	require.Equal(t, `'it'\''s'`, shellQuote("it's"))
	require.Contains(t, xmlEscape("a<b"), "&lt;")
}

func TestMissingProvisionTokenFailsRender(t *testing.T) {
	dir := t.TempDir()
	tmplPath := filepath.Join(dir, "tok.templ")
	require.NoError(t, os.WriteFile(tmplPath, []byte("Bearer {{.provision_token}}"), 0o644))
	out := filepath.Join(dir, "out.cfg")

	err := (&BootMenuHandlers{}).writeTemplateToDisk(tmplPath, out, map[string]string{})
	require.Error(t, err)
	_, statErr := os.Stat(out)
	require.True(t, os.IsNotExist(statErr))
	body, _ := os.ReadFile(out)
	require.NotContains(t, string(body), "<no value>")

	_, err = (&BootMenuHandlers{}).provisionToken(context.Background(), "aa:bb:cc:dd:ee:01")
	require.Error(t, err)
}

func TestInstallerConfigIsOwnerReadOnly(t *testing.T) {
	dir := t.TempDir()
	tmplPath := filepath.Join(dir, "plain.templ")
	require.NoError(t, os.WriteFile(tmplPath, []byte("host {{.hostname}}"), 0o644))
	out := filepath.Join(dir, "out.cfg")
	err := (&BootMenuHandlers{}).writeInstallerConfig(tmplPath, out, map[string]string{"hostname": "cb-host"})
	require.NoError(t, err)
	info, err := os.Stat(out)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestRenderedConfigRequiresLeaseIPOrToken(t *testing.T) {
	const (
		token = "secret-token-value"
		name  = "aa-bb-cc-dd-ee-01"
		mac   = "aa:bb:cc:dd:ee:01"
	)
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "configs", "kickstart")
	require.NoError(t, os.MkdirAll(cfgDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, name), []byte("TOKEN "+token), 0o600))

	leases := &MockLeaseService{}
	leases.On("GetLeaseByMAC", mock.Anything, mac).Return(&dhcp.Lease{
		MAC:            mac,
		IP:             net.ParseIP("192.0.2.10"),
		ProvisionToken: token,
	}, nil)
	h := NewBootMenuHandlers(&Container{
		LeaseService: leases,
		Config:       &config.Config{Provision: config.ProvisionConfig{Dir: dir}},
	})

	stranger := configRequest(name, "192.0.2.1:1234")
	stranger.Header.Set("X-Forwarded-For", "192.0.2.10")
	stranger.AddCookie(testSessionCookie(t))
	rec := httptest.NewRecorder()
	h.ServeRenderedConfig(rec, stranger)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.NotContains(t, rec.Body.String(), token)

	owner := configRequest(name, "192.0.2.10:9")
	rec = httptest.NewRecorder()
	h.ServeRenderedConfig(rec, owner)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), token)

	bearer := configRequest(name, "198.51.100.8:9")
	bearer.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	h.ServeRenderedConfig(rec, bearer)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestMethodNotAllowedHidesAdminShell(t *testing.T) {
	chdirRepoRoot(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/dhcp", nil)
	req.Header.Set("Accept", "text/html")
	MethodNotAllowed(rec, req)
	require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	body := rec.Body.String()
	require.Contains(t, body, `data-theme="cozy"`)
	require.NotContains(t, body, `href="/dhcp"`)
}

func TestHostStateJSONEnvelope(t *testing.T) {
	const (
		mac   = "aa:bb:cc:dd:ee:09"
		token = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	)
	leases := &MockLeaseService{}
	leases.On("GetLeaseByMAC", mock.Anything, mac).Return(hostLease(mac, token), nil)
	leases.On("RecordHeartbeat", mock.Anything, mac).Return(nil)
	h := &DHCPHandlers{leaseService: leases}

	req := httptest.NewRequest(http.MethodPost, "/dhcp/lease/"+mac+"/heartbeat", nil)
	req = mux.SetURLVars(req, map[string]string{"mac": mac})
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.RecordHeartbeat(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"success":true`)
	require.NotContains(t, rec.Body.String(), `"status"`)

	bad := httptest.NewRequest(http.MethodPost, "/dhcp/lease/"+mac+"/state", strings.NewReader("{"))
	bad = mux.SetURLVars(bad, map[string]string{"mac": mac})
	bad.Header.Set("Content-Type", "application/json")
	bad.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	h.HostUpdateLeaseState(rec, bad)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "not valid JSON")

	empty := &MockLeaseService{}
	empty.On("GetLeaseByMAC", mock.Anything, mac).Return(hostLease(mac, ""), nil)
	h = &DHCPHandlers{leaseService: empty}
	req = httptest.NewRequest(http.MethodPost, "/dhcp/lease/"+mac+"/heartbeat", nil)
	req = mux.SetURLVars(req, map[string]string{"mac": mac})
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	h.RecordHeartbeat(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	empty.AssertNotCalled(t, "RecordHeartbeat", mock.Anything, mock.Anything)
}

func TestSourceAllowlistRejectsMarkup(t *testing.T) {
	const (
		mac   = "aa:bb:cc:dd:ee:0a"
		token = "abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"
	)
	markup := "<img src=x onerror=alert(1)>"

	hostLeases := &MockLeaseService{}
	hostLeases.On("GetLeaseByMAC", mock.Anything, mac).Return(hostLease(mac, token), nil)
	host := &DHCPHandlers{leaseService: hostLeases}
	body := `{"state":"imaging","source":"` + markup + `"}`
	req := httptest.NewRequest(http.MethodPost, "/dhcp/lease/"+mac+"/state", strings.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"mac": mac})
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	host.HostUpdateLeaseState(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "source is not allowed")
	hostLeases.AssertNotCalled(t, "UpdateLeaseState", mock.Anything, mock.Anything, mock.Anything, mock.Anything)

	longBody := `{"state":"imaging","source":"` + strings.Repeat("a", 65) + `"}`
	req = httptest.NewRequest(http.MethodPost, "/dhcp/lease/"+mac+"/state", strings.NewReader(longBody))
	req = mux.SetURLVars(req, map[string]string{"mac": mac})
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	host.HostUpdateLeaseState(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "source is too long")

	adminLeases := &MockLeaseService{}
	admin := &DHCPHandlers{leaseService: adminLeases}
	form := url.Values{"state": {"imaging"}, "source": {markup}}
	req = httptest.NewRequest(http.MethodPost, "/dhcp/lease/state?mac="+mac, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	admin.UpdateLeaseState(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "source is not allowed")
	adminLeases.AssertNotCalled(t, "UpdateLeaseState", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestAddManualLeaseRejectsMalformedMAC(t *testing.T) {
	servers := &MockServerService{}
	leases := &MockLeaseService{}
	servers.On("GetAllServers", mock.Anything).Return([]*dhcp.Server{{
		ID: "srv",
		IP: net.ParseIP("10.0.0.1"),
	}}, nil)
	h := &DHCPHandlers{serverService: servers, leaseService: leases}

	form := url.Values{
		"network": {"10.0.0.1"},
		"mac":     {"not-a-mac"},
		"ip":      {"10.0.0.10"},
	}
	req := httptest.NewRequest(http.MethodPost, "/dhcp/add_manual_lease", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	h.AddManualLease(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "The MAC address is invalid")
	leases.AssertNotCalled(t, "AssignLease", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	leases.AssertNotCalled(t, "ReserveLease", mock.Anything, mock.Anything, mock.Anything, mock.Anything)

	leases.On("AssignLease", mock.Anything, "srv", "aa:bb:cc:dd:ee:01", mock.Anything).Return(&dhcp.Lease{}, nil)
	form.Set("mac", "AA-BB-CC-DD-EE-01")
	req = httptest.NewRequest(http.MethodPost, "/dhcp/add_manual_lease", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	h.AddManualLease(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)
	leases.AssertCalled(t, "AssignLease", mock.Anything, "srv", "aa:bb:cc:dd:ee:01", mock.Anything)
}

func TestDHCPPoolFreeUsesOccupancy(t *testing.T) {
	now := time.Now()
	server := &dhcp.Server{
		ID:         "srv",
		IP:         net.ParseIP("10.1.0.1"),
		LeaseRange: 10,
	}
	leases := []*dhcp.Lease{
		{IP: net.ParseIP("10.1.0.10"), Expiry: now.Add(time.Hour)},
		{IP: net.ParseIP("10.1.0.11"), Expiry: now.Add(-time.Hour)},
		{IP: net.ParseIP("10.1.0.12"), Reserved: true, Expiry: now.Add(-time.Hour)},
	}
	mockLeases := &MockLeaseService{}
	mockLeases.On("GetLeasesByServer", mock.Anything, "srv").Return(leases, nil)
	h := &DHCPHandlers{leaseService: mockLeases}
	view, err := h.convertServerToView(context.Background(), server)
	require.NoError(t, err)
	require.Equal(t, 2, view.Used)
	require.Equal(t, 8, view.Free)
	require.Equal(t, 10, view.LeaseRange)
}

func TestTokensEqualRejectsEmptyAndDifferentLengths(t *testing.T) {
	require.False(t, tokensEqual("", "abc"))
	require.False(t, tokensEqual("abc", ""))
	require.False(t, tokensEqual("ab", "abc"))
	require.True(t, tokensEqual("abc", "abc"))
}

func TestSetModeRejectsCrossSite(t *testing.T) {
	store := &memoryPrefs{mode: DefaultUIMode}
	h := NewPreferenceHandlers(store)
	req := httptest.NewRequest(http.MethodPost, "/api/preferences/mode", strings.NewReader(`{"mode":"console"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Host = "example.com"
	req.Header.Set("Origin", "http://evil.example")
	rec := httptest.NewRecorder()
	h.SetMode(rec, req)
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Equal(t, DefaultUIMode, store.mode)

	req = httptest.NewRequest(http.MethodPost, "/api/preferences/mode", strings.NewReader("{"))
	req.Header.Set("Content-Type", "application/json")
	req.Host = "example.com"
	req.Header.Set("Origin", "http://example.com")
	rec = httptest.NewRecorder()
	h.SetMode(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "not valid JSON")

	missing := httptest.NewRequest(http.MethodPost, "/api/preferences/mode", strings.NewReader(`{"mode":"console"}`))
	missing.Header.Set("Content-Type", "application/json")
	missing.Host = "example.com"
	rec = httptest.NewRecorder()
	h.SetMode(rec, missing)
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "Cross-site requests are not allowed")
	require.Equal(t, DefaultUIMode, store.mode)
}

func TestRenderedConfigBypassesAuthButPublicConfigsDoNot(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	h := AuthMiddleware(next)

	req := httptest.NewRequest(http.MethodGet, "/configs/kickstart/aa-bb-cc-dd-ee-01", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)

	req = httptest.NewRequest(http.MethodGet, "/public/provision/configs/kickstart/aa", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusFound, rec.Code)

	req = httptest.NewRequest(http.MethodGet, "/public/provision/templates/kickstart/default.templ", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)
}

func copyForm(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func configRequest(name, remote string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/configs/kickstart/"+name, nil)
	req.RemoteAddr = remote
	return mux.SetURLVars(req, map[string]string{"configType": "kickstart", "name": name})
}
