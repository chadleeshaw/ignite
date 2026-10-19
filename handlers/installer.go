package handlers

import (
	"encoding/xml"
	"fmt"
	"net"
	"strings"

	"ignite/dhcp"
)

// validateInstallerFields checks the values that are interpolated into
// installer configs. IP fields must be IPv4 addresses and the hostname must
// be DNS labels, so a quote or a "<" cannot reach a shell or XML template.
// On success the map holds the canonical IP strings and the canonical MAC.
func validateInstallerFields(form map[string]string) error {
	if form == nil {
		return fmt.Errorf("installer fields are missing")
	}
	canon, err := dhcp.CanonicalMAC(form["mac"])
	if err != nil {
		return fmt.Errorf("invalid MAC address: %w", err)
	}
	form["mac"] = canon
	if err := validateHostname(form["hostname"]); err != nil {
		return err
	}
	for _, field := range []string{"tftpip", "ip", "subnet", "gateway", "dns"} {
		normalized, err := normalizeIPv4(form[field])
		if err != nil {
			return fmt.Errorf("%s must be an IPv4 address", field)
		}
		form[field] = normalized
	}
	return nil
}

func normalizeIPv4(value string) (string, error) {
	ip := net.ParseIP(strings.TrimSpace(value))
	if ip == nil || ip.To4() == nil {
		return "", fmt.Errorf("invalid IPv4 address")
	}
	return ip.To4().String(), nil
}

// validateHostname accepts a single DNS label or a dotted name (RFC 1123).
func validateHostname(hostname string) error {
	hostname = strings.TrimSpace(hostname)
	if hostname == "" || len(hostname) > 253 {
		return fmt.Errorf("hostname is invalid")
	}
	hostname = strings.TrimSuffix(hostname, ".")
	if hostname == "" {
		return fmt.Errorf("hostname is invalid")
	}
	for _, label := range strings.Split(hostname, ".") {
		if len(label) == 0 || len(label) > 63 {
			return fmt.Errorf("hostname is invalid")
		}
		for i, c := range label {
			switch {
			case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
			case c == '-' && i > 0 && i < len(label)-1:
			default:
				return fmt.Errorf("hostname is invalid")
			}
		}
	}
	return nil
}

// shellQuote wraps s in POSIX single quotes so it is one shell word.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// xmlEscape escapes s for an XML text node.
func xmlEscape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
