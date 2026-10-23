package catalog

import (
	"context"
	"time"
)

// ReleaseSource discovers the current releases of one distribution.
// Implementations make network calls and must honor ctx cancellation.
type ReleaseSource interface {
	OS() string
	Releases(ctx context.Context) ([]Release, error)
}

// LifecycleSource returns end-of-life dates keyed by version cycle,
// for example "24.04" -> 2029-05-31. endoflife.date is the default.
type LifecycleSource interface {
	EOLDates(ctx context.Context, product string) (map[string]time.Time, error)
}

// Store persists the last good snapshot.
type Store interface {
	Load(ctx context.Context) (Snapshot, error)
	Save(ctx context.Context, snap Snapshot) error
}
