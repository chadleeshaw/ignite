package catalog

import "net/http"

// DefaultSources is the release discovery used by the running server.
// Phase 1 covers Debian, Fedora, and CentOS Stream. Later sources are added
// beside them.
func DefaultSources(client *http.Client) []ReleaseSource {
	return []ReleaseSource{
		Debian{Client: client},
		Fedora{Client: client},
		CentOS{Client: client},
	}
}
