package catalog

import "time"

// Origin says where a catalog snapshot came from.
type Origin string

const (
	OriginLive    Origin = "live"
	OriginCached  Origin = "cached"
	OriginBundled Origin = "bundled"
)

// Snapshot is the catalog as last refreshed, persisted in bbolt.
type Snapshot struct {
	Releases    []Release         `json:"releases"`
	Origin      Origin            `json:"source"`
	RefreshedAt time.Time         `json:"last_refreshed_at"`
	Errors      map[string]string `json:"errors,omitempty"`
}

func cloneSnapshot(src Snapshot) Snapshot {
	dst := src
	if src.Releases != nil {
		dst.Releases = append([]Release(nil), src.Releases...)
	}
	if src.Errors != nil {
		dst.Errors = make(map[string]string, len(src.Errors))
		for key, value := range src.Errors {
			dst.Errors[key] = value
		}
	}
	return dst
}
