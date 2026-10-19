package handlers

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCookieSecure(t *testing.T) {
	tests := []struct {
		name    string
		env     string
		tls     bool
		forward string
		want    bool
	}{
		{name: "auto plain http", want: false},
		{name: "auto unset is plain http", env: "", want: false},
		{name: "auto tls", tls: true, want: true},
		{name: "auto forwarded https", forward: "https", want: true},
		{name: "auto forwarded HTTPS", forward: "HTTPS", want: true},
		{name: "auto forwarded https list", forward: "https, http", want: true},
		{name: "auto forwarded http", forward: "http", want: false},
		{name: "forced true on plain http", env: "true", want: true},
		{name: "forced TRUE on plain http", env: "TRUE", want: true},
		{name: "forced false on tls", env: "false", tls: true, forward: "https", want: false},
		{name: "unknown value follows auto", env: "yes", want: false},
		{name: "unknown value still honors tls", env: "yes", tls: true, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("COOKIE_SECURE", tt.env)
			req := httptest.NewRequest(http.MethodGet, "http://10.1.1.8:8080/dhcp", nil)
			if tt.tls {
				req.TLS = &tls.ConnectionState{}
			}
			if tt.forward != "" {
				req.Header.Set("X-Forwarded-Proto", tt.forward)
			}
			require.Equal(t, tt.want, cookieSecure(req))

			session := sessionCookie(req, "tok", false)
			mode := modeCookie(req, "cozy")
			require.Equal(t, "ignite_session", session.Name)
			require.Equal(t, "ignite_mode", mode.Name)
			require.Equal(t, tt.want, session.Secure)
			require.Equal(t, tt.want, mode.Secure)
			require.True(t, session.HttpOnly)
			require.True(t, mode.HttpOnly)
		})
	}
}
