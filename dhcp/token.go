package dhcp

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"strings"
)

// NewProvisionToken returns a fresh per-lease callback secret (32 random
// bytes, hex-encoded). Callers store it on the lease and hand it to the
// host being provisioned. It is not an admin session token.
func NewProvisionToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate provision token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// CanonicalMAC parses mac and returns the lowercase colon-separated form
// used as the lease key (the same form net.HardwareAddr.String produces
// for a 48-bit address).
func CanonicalMAC(mac string) (string, error) {
	hw, err := net.ParseMAC(strings.TrimSpace(mac))
	if err != nil {
		return "", fmt.Errorf("invalid MAC address %q", mac)
	}
	if len(hw) != 6 {
		return "", fmt.Errorf("MAC address %q is not 48 bits", mac)
	}
	return hw.String(), nil
}

// ensureProvisionToken sets a token when the lease does not have one.
func ensureProvisionToken(lease *Lease) error {
	if lease == nil || lease.ProvisionToken != "" {
		return nil
	}
	token, err := NewProvisionToken()
	if err != nil {
		return err
	}
	lease.ProvisionToken = token
	return nil
}
