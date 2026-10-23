package catalog

import "net/http"

// DefaultSources is the release discovery used by the running server.
// githubToken is sent to the NixOS GitHub API when it is non-empty.
// An empty token still tries the unauthenticated API; a 403 keeps the
// previous NixOS releases.
func DefaultSources(client *http.Client, githubToken string) []ReleaseSource {
	return []ReleaseSource{
		Debian{Client: client},
		Fedora{Client: client},
		CentOS{Client: client},
		Ubuntu{Client: client},
		NixOS{Client: client, Token: githubToken},
		OpenSUSE{Client: client},
	}
}
