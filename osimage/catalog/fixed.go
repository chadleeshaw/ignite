package catalog

import (
	"context"
	"time"
)

// Fixed is a Catalog with a static snapshot and no network calls.
type Fixed struct {
	Snap       Snapshot
	RefreshErr error
	InFlight   bool
}

// Offerable returns Snap with end-of-life releases removed.
func (f *Fixed) Offerable(ctx context.Context) (Snapshot, error) {
	if f == nil {
		return Snapshot{Origin: OriginBundled, Errors: map[string]string{}, Releases: []Release{}}, nil
	}
	snap := cloneSnapshot(f.Snap)
	now := time.Now()
	kept := make([]Release, 0, len(snap.Releases))
	for _, rel := range snap.Releases {
		if !rel.EOL(now) {
			kept = append(kept, rel)
		}
	}
	snap.Releases = kept
	return snap, nil
}

// Current returns Snap, including EOL releases.
func (f *Fixed) Current(ctx context.Context) (Snapshot, error) {
	if f == nil {
		return Snapshot{}, nil
	}
	return cloneSnapshot(f.Snap), nil
}

// Lookup finds one release, including one past its end-of-life date.
func (f *Fixed) Lookup(ctx context.Context, osName, version, arch string) (Release, bool) {
	if f == nil {
		return Release{}, false
	}
	for _, rel := range f.Snap.Releases {
		if rel.OS != osName || rel.Version != version {
			continue
		}
		if arch != "" && rel.Arch != "" && rel.Arch != arch {
			continue
		}
		return rel, true
	}
	return Release{}, false
}

// Refresh returns RefreshErr, or Snap when RefreshErr is nil.
func (f *Fixed) Refresh(ctx context.Context) (Snapshot, error) {
	if f == nil {
		return Snapshot{}, ErrAllSourcesFailed
	}
	if f.RefreshErr != nil {
		return Snapshot{}, f.RefreshErr
	}
	return cloneSnapshot(f.Snap), nil
}

// Refreshing reports InFlight.
func (f *Fixed) Refreshing() bool {
	return f != nil && f.InFlight
}

// Start does nothing. The snapshot does not change.
func (f *Fixed) Start(ctx context.Context) {}

var (
	_ Catalog = (*Service)(nil)
	_ Catalog = (*Fixed)(nil)
)
