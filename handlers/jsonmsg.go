package handlers

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// errInvalidJSON is returned when a JSON body cannot be decoded.
var errInvalidJSON = errors.New("invalid json")

const maxCallbackSourceLen = 64

// allowedCallbackSources is the only text stored from a state callback.
// The history UI renders this string, so anything outside the list is a
// stored-XSS candidate and is rejected.
var allowedCallbackSources = map[string]struct{}{
	"manual":     {},
	"provision":  {},
	"cloud-init": {},
	"kickstart":  {},
	"preseed":    {},
	"autoyast":   {},
}

var (
	errSourceTooLong    = errors.New("source is too long")
	errSourceNotAllowed = errors.New("source is not allowed")
)

// acceptCallbackSource trims source, fills fallback when it is empty, then
// enforces the length cap and the allowlist. fallback must itself be allowed.
func acceptCallbackSource(source, fallback string) (string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		source = fallback
	}
	if len(source) > maxCallbackSourceLen {
		return "", errSourceTooLong
	}
	if _, ok := allowedCallbackSources[source]; !ok {
		return "", errSourceNotAllowed
	}
	return source, nil
}

// writeJSONMessage writes the callback and preference envelope.
// Success and failure both use {"success": bool, "message": string}.
func writeJSONMessage(w http.ResponseWriter, status int, success bool, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": success,
		"message": message,
	})
}

// tokensEqual compares two secrets without leaking the length.
// Both sides are hashed first so ConstantTimeCompare always sees 32 bytes.
// An empty value never matches.
func tokensEqual(got, want string) bool {
	if got == "" || want == "" {
		return false
	}
	sumGot := sha256.Sum256([]byte(got))
	sumWant := sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(sumGot[:], sumWant[:]) == 1
}

// crossSiteRequest reports a browser POST that did not originate on this
// host. A missing Origin is allowed (non-browser clients). SameSite=Lax
// already blocks most cross-site cookie posts; this rejects the rest.
func crossSiteRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	if strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
		return true
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return false
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return true
	}
	return !strings.EqualFold(parsed.Host, r.Host)
}
