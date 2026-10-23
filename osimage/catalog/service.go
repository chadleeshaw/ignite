package catalog

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"
)

// ErrRefreshInProgress is returned when a refresh is already running.
var ErrRefreshInProgress = errors.New("catalog refresh already in progress")

// ErrAllSourcesFailed is returned when every release source failed.
// The previous snapshot is left in place.
var ErrAllSourcesFailed = errors.New("every catalog source failed")

// Catalog serves the offerable releases and refreshes them on a schedule.
type Catalog interface {
	// Offerable returns non-EOL releases for the download picker.
	Offerable(ctx context.Context) (Snapshot, error)
	// Current is the snapshot being served, including releases past EOL.
	Current(ctx context.Context) (Snapshot, error)
	// Lookup finds one release, EOL or not, so an existing image can be re-resolved.
	Lookup(ctx context.Context, osName, version, arch string) (Release, bool)
	// Refresh queries every source now and stores the merged result.
	Refresh(ctx context.Context) (Snapshot, error)
	// Refreshing reports whether a refresh is in flight.
	Refreshing() bool
	// Start refreshes once in the background, then on each interval.
	// OS_CATALOG_OFFLINE skips the loop and leaves the bundled list in place.
	Start(ctx context.Context)
}

// Options configures a Service. A nil client is not used: every source
// receives the client it was built with.
type Options struct {
	Sources   []ReleaseSource
	Lifecycle LifecycleSource
	Store     Store
	Offline   bool
	Interval  time.Duration
	Now       func() time.Time
}

// Service merges release sources, applies end-of-life dates, and falls
// back from a live refresh to the stored snapshot to the bundled list.
type Service struct {
	sources   []ReleaseSource
	lifecycle LifecycleSource
	store     Store
	offline   bool
	interval  time.Duration
	now       func() time.Time

	mu      sync.RWMutex
	current Snapshot
	running bool
}

// New loads the stored snapshot, or the bundled list when the store is
// empty or the process is offline.
func New(opt Options) (*Service, error) {
	bundled, err := Bundled()
	if err != nil {
		return nil, err
	}
	svc := &Service{
		sources:   opt.Sources,
		lifecycle: opt.Lifecycle,
		store:     opt.Store,
		offline:   opt.Offline,
		interval:  opt.Interval,
		now:       opt.Now,
		current:   bundled,
	}
	if svc.interval <= 0 {
		svc.interval = 24 * time.Hour
	}
	if svc.offline || svc.store == nil {
		return svc, nil
	}
	snap, err := svc.store.Load(context.Background())
	if err != nil {
		if !IsNotFound(err) {
			log.Printf("OS catalog: stored snapshot unreadable, using the bundled list: %v", err)
		}
		return svc, nil
	}
	if len(snap.Releases) == 0 {
		return svc, nil
	}
	snap.Origin = OriginCached
	svc.current = snap
	return svc, nil
}

// Offerable returns the snapshot with end-of-life releases removed.
func (s *Service) Offerable(ctx context.Context) (Snapshot, error) {
	snap := s.snapshot()
	kept := make([]Release, 0, len(snap.Releases))
	now := s.clock()
	for _, rel := range snap.Releases {
		if !rel.EOL(now) {
			kept = append(kept, rel)
		}
	}
	snap.Releases = kept
	return snap, nil
}

// Current returns the snapshot being served, including EOL releases.
func (s *Service) Current(ctx context.Context) (Snapshot, error) {
	return s.snapshot(), nil
}

// Lookup finds a release by OS, version, and architecture. An empty arch
// matches the first release for that OS and version. EOL releases match.
func (s *Service) Lookup(ctx context.Context, osName, version, arch string) (Release, bool) {
	for _, rel := range s.snapshot().Releases {
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

// Refreshing reports whether Refresh is in progress.
func (s *Service) Refreshing() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// Refresh queries every source. A source that fails keeps that OS's releases
// from the previous snapshot and records Errors[os]. An end-of-life lookup
// that fails keeps the new releases with an empty EOL date. Every source
// failing returns ErrAllSourcesFailed and does not replace the snapshot.
// Offline mode returns the bundled snapshot without calling sources.
func (s *Service) Refresh(ctx context.Context) (Snapshot, error) {
	if s.offline {
		return s.snapshot(), nil
	}
	if !s.begin() {
		return Snapshot{}, ErrRefreshInProgress
	}
	defer s.end()

	prev := s.snapshot()
	results := s.querySources(ctx)
	if len(s.sources) > 0 && failedCount(results) == len(s.sources) {
		return Snapshot{}, ErrAllSourcesFailed
	}

	next := map[string][]Release{}
	for _, rel := range prev.Releases {
		next[rel.OS] = append(next[rel.OS], rel)
	}
	errs := map[string]string{}
	for _, result := range results {
		if result.err != nil {
			errs[result.os] = result.err.Error()
			continue
		}
		releases := append([]Release(nil), result.releases...)
		if s.lifecycle != nil {
			dated, err := s.applyEOL(ctx, result.os, releases)
			if err != nil {
				for i := range releases {
					releases[i].EOLDate = time.Time{}
				}
				errs[result.os] = "eol lookup: " + err.Error()
			} else {
				releases = dated
			}
		}
		next[result.os] = releases
	}

	snap := Snapshot{
		Releases:    flatten(next),
		Origin:      OriginLive,
		RefreshedAt: s.clock(),
		Errors:      errs,
	}
	if s.store != nil {
		if err := s.store.Save(ctx, snap); err != nil {
			log.Printf("OS catalog: save snapshot: %v", err)
		}
	}
	s.mu.Lock()
	s.current = snap
	s.mu.Unlock()
	return cloneSnapshot(snap), nil
}

// Start refreshes in the background, then on each interval, until ctx ends.
func (s *Service) Start(ctx context.Context) {
	if s == nil || s.offline {
		if s != nil && s.offline {
			log.Print("OS catalog: offline, using the bundled list")
		}
		return
	}
	log.Printf("OS catalog: refresh every %s", s.interval)
	go s.loop(ctx)
}

func (s *Service) loop(ctx context.Context) {
	s.refreshQuiet(ctx)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.refreshQuiet(ctx)
		}
	}
}

func (s *Service) refreshQuiet(ctx context.Context) {
	snap, err := s.Refresh(ctx)
	if err != nil {
		if !errors.Is(err, ErrRefreshInProgress) {
			log.Printf("OS catalog refresh: %v", err)
		}
		return
	}
	log.Printf("OS catalog refresh: source=%s releases=%d errors=%d", snap.Origin, len(snap.Releases), len(snap.Errors))
}

type sourceResult struct {
	os       string
	releases []Release
	err      error
}

func (s *Service) querySources(ctx context.Context) []sourceResult {
	out := make([]sourceResult, len(s.sources))
	var wg sync.WaitGroup
	for i, src := range s.sources {
		wg.Add(1)
		go func(i int, src ReleaseSource) {
			defer wg.Done()
			sctx, cancel := withTimeout(ctx)
			defer cancel()
			releases, err := src.Releases(sctx)
			if err == nil && len(releases) == 0 {
				err = fmt.Errorf("%s: no releases found", src.OS())
			}
			out[i] = sourceResult{os: src.OS(), releases: releases, err: err}
		}(i, src)
	}
	wg.Wait()
	return out
}

func (s *Service) applyEOL(ctx context.Context, osName string, releases []Release) ([]Release, error) {
	ectx, cancel := withTimeout(ctx)
	defer cancel()
	dates, err := s.lifecycle.EOLDates(ectx, eolProduct(osName))
	if err != nil {
		return releases, err
	}
	for i := range releases {
		if day, ok := lookupCycle(releases[i].Version, dates); ok {
			releases[i].EOLDate = day
		}
	}
	return releases, nil
}

func (s *Service) begin() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return false
	}
	s.running = true
	return true
}

func (s *Service) end() {
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
}

func (s *Service) snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneSnapshot(s.current)
}

func (s *Service) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func failedCount(results []sourceResult) int {
	n := 0
	for _, result := range results {
		if result.err != nil {
			n++
		}
	}
	return n
}

func flatten(byOS map[string][]Release) []Release {
	var out []Release
	seen := map[string]bool{}
	for _, osName := range osOrder {
		out = append(out, dedupe(byOS[osName])...)
		seen[osName] = true
	}
	var extras []string
	for osName := range byOS {
		if !seen[osName] {
			extras = append(extras, osName)
		}
	}
	// extras stay in map iteration order only after a sort for stability
	for i := 1; i < len(extras); i++ {
		for j := i; j > 0 && extras[j] < extras[j-1]; j-- {
			extras[j], extras[j-1] = extras[j-1], extras[j]
		}
	}
	for _, osName := range extras {
		out = append(out, dedupe(byOS[osName])...)
	}
	return out
}

func dedupe(releases []Release) []Release {
	seen := map[string]bool{}
	out := make([]Release, 0, len(releases))
	for _, rel := range releases {
		key := rel.OS + "\x00" + rel.Version + "\x00" + rel.Arch
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, rel)
	}
	return out
}
