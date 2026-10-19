package handlers

import (
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"ignite/dhcp"

	"github.com/gorilla/mux"
)

// configNamePattern is a single path segment: letters, digits, dot, underscore, hyphen.
var configNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,128}$`)

// ServeRenderedConfig handles GET /configs/{configType}/{name}.
//
// The installer downloads this file before it has the provision token, so
// it cannot present the token on the first request. The unauthenticated
// path is therefore the TCP source address: RemoteAddr must equal the
// lease IP for the MAC encoded in the filename. X-Forwarded-For is ignored.
// A client that already holds the token may also send it (Authorization
// Bearer or X-Ignite-Token). Everyone else, including an admin session,
// gets 404. The admin UI reads configs through the session-protected
// /prov/loadconfig routes. The file mode is 0600; this handler is the only
// HTTP path that returns the body.
func (h *BootMenuHandlers) ServeRenderedConfig(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.container == nil || h.container.Config == nil || h.container.LeaseService == nil {
		http.NotFound(w, r)
		return
	}
	vars := mux.Vars(r)
	configType := vars["configType"]
	name := vars["name"]
	if !configNamePattern.MatchString(configType) || !configNamePattern.MatchString(name) {
		http.NotFound(w, r)
		return
	}
	mac, err := dhcp.CanonicalMAC(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !configVisibleTo(r, h.container.LeaseService, mac) {
		http.NotFound(w, r)
		return
	}
	path, err := safeJoin(h.container.Config.Provision.Dir, filepath.Join("configs", configType, name))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	body, err := os.ReadFile(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// configVisibleTo allows the lease's own IP or a request that presents the
// provision token. It does not allow an admin session by itself.
func configVisibleTo(r *http.Request, leases dhcp.LeaseService, mac string) bool {
	lease, err := leases.GetLeaseByMAC(r.Context(), mac)
	if err != nil || lease == nil || lease.IP == nil {
		return false
	}
	if hostTokenMatches(r, leases, mac) {
		return true
	}
	ip := requestSourceIP(r)
	if ip == nil {
		return false
	}
	return ip.Equal(lease.IP)
}

// requestSourceIP is the TCP peer address. Proxy headers are not trusted.
func requestSourceIP(r *http.Request) net.IP {
	if r == nil {
		return nil
	}
	host := r.RemoteAddr
	if split, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		host = split
	}
	return net.ParseIP(strings.Trim(host, "[]"))
}
