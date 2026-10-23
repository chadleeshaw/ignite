package catalog

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"ignite/config"
	"ignite/db"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReleaseEOLUsesTheCalendarDay(t *testing.T) {
	day := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	rel := Release{EOLDate: day}
	assert.False(t, rel.EOL(time.Date(2026, 10, 22, 23, 0, 0, 0, time.UTC)))
	assert.True(t, rel.EOL(time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)))
	assert.False(t, Release{}.EOL(time.Now()))
	assert.Equal(t, "", FormatDay(time.Time{}))
	assert.Equal(t, "2026-10-22", FormatDay(day))
	assert.Equal(t, "", FormatRFC3339(time.Time{}))
}

func TestReleaseJSONDateRoundTrip(t *testing.T) {
	rel := Release{
		OS: "debian", Version: "12", DisplayName: "12 (Bookworm)", Arch: "x86_64",
		KernelURL: "http://example/linux", InitrdURL: "http://example/initrd.gz",
		EOLDate: time.Date(2028, 6, 30, 15, 0, 0, 0, time.FixedZone("X", 3600)),
	}
	raw, err := rel.MarshalJSON()
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"eol_date":"2028-06-30"`)
	var got Release
	require.NoError(t, got.UnmarshalJSON(raw))
	assert.Equal(t, "2028-06-30", FormatDay(got.EOLDate))
	assert.True(t, got.EOLDate.Equal(dateOnly(rel.EOLDate)))

	var unknown Release
	require.NoError(t, unknown.UnmarshalJSON([]byte(`{"os":"debian","version":"12","eol_date":""}`)))
	assert.True(t, unknown.EOLDate.IsZero())
}

func TestBundledMatchesPhase0Catalog(t *testing.T) {
	snap, err := Bundled()
	require.NoError(t, err)
	assert.Equal(t, OriginBundled, snap.Origin)
	assert.True(t, snap.RefreshedAt.IsZero())

	want := map[string]map[string][2]string{
		"ubuntu": {
			"22.04": {
				"https://github.com/netbootxyz/ubuntu-squash/releases/download/22.04.5-3b4b797c/vmlinuz",
				"https://github.com/netbootxyz/ubuntu-squash/releases/download/22.04.5-3b4b797c/initrd",
			},
			"24.04": {
				"https://github.com/netbootxyz/ubuntu-squash/releases/download/24.04.4-494663bf/vmlinuz",
				"https://github.com/netbootxyz/ubuntu-squash/releases/download/24.04.4-494663bf/initrd",
			},
			"26.04": {
				"https://github.com/netbootxyz/ubuntu-squash/releases/download/26.04.1-c43af979/vmlinuz",
				"https://github.com/netbootxyz/ubuntu-squash/releases/download/26.04.1-c43af979/initrd",
			},
		},
		"debian": {
			"12": {
				"http://deb.debian.org/debian/dists/bookworm/main/installer-amd64/current/images/netboot/debian-installer/amd64/linux",
				"http://deb.debian.org/debian/dists/bookworm/main/installer-amd64/current/images/netboot/debian-installer/amd64/initrd.gz",
			},
			"13": {
				"http://deb.debian.org/debian/dists/trixie/main/installer-amd64/current/images/netboot/debian-installer/amd64/linux",
				"http://deb.debian.org/debian/dists/trixie/main/installer-amd64/current/images/netboot/debian-installer/amd64/initrd.gz",
			},
		},
		"fedora": {
			"43": {
				"https://download.fedoraproject.org/pub/fedora/linux/releases/43/Everything/x86_64/os/images/pxeboot/vmlinuz",
				"https://download.fedoraproject.org/pub/fedora/linux/releases/43/Everything/x86_64/os/images/pxeboot/initrd.img",
			},
			"44": {
				"https://download.fedoraproject.org/pub/fedora/linux/releases/44/Everything/x86_64/os/images/pxeboot/vmlinuz",
				"https://download.fedoraproject.org/pub/fedora/linux/releases/44/Everything/x86_64/os/images/pxeboot/initrd.img",
			},
		},
		"centos": {
			"9": {
				"http://mirror.stream.centos.org/9-stream/BaseOS/x86_64/os/images/pxeboot/vmlinuz",
				"http://mirror.stream.centos.org/9-stream/BaseOS/x86_64/os/images/pxeboot/initrd.img",
			},
			"10": {
				"http://mirror.stream.centos.org/10-stream/BaseOS/x86_64/os/images/pxeboot/vmlinuz",
				"http://mirror.stream.centos.org/10-stream/BaseOS/x86_64/os/images/pxeboot/initrd.img",
			},
		},
		"nixos": {
			"26.05": {
				"https://github.com/nix-community/nixos-images/releases/download/nixos-26.05/bzImage-x86_64-linux",
				"https://github.com/nix-community/nixos-images/releases/download/nixos-26.05/initrd-x86_64-linux",
			},
		},
		"opensuse": {
			"16.0": {
				"http://download.opensuse.org/distribution/leap/16.0/repo/oss/boot/x86_64/loader/linux",
				"http://download.opensuse.org/distribution/leap/16.0/repo/oss/boot/x86_64/loader/initrd",
			},
			"tumbleweed": {
				"http://download.opensuse.org/tumbleweed/repo/oss/boot/x86_64/loader/linux",
				"http://download.opensuse.org/tumbleweed/repo/oss/boot/x86_64/loader/initrd",
			},
		},
	}
	got := map[string]map[string]Release{}
	for _, rel := range snap.Releases {
		if got[rel.OS] == nil {
			got[rel.OS] = map[string]Release{}
		}
		got[rel.OS][rel.Version] = rel
		assert.Equal(t, "x86_64", rel.Arch)
		assert.NotContains(t, rel.KernelURL, "/x86_64/vmlinuz")
	}
	require.Len(t, got, len(want))
	for osName, versions := range want {
		require.Len(t, got[osName], len(versions), osName)
		for version, urls := range versions {
			rel, ok := got[osName][version]
			require.True(t, ok, "%s %s", osName, version)
			assert.Equal(t, urls[0], rel.KernelURL, "%s %s kernel", osName, version)
			assert.Equal(t, urls[1], rel.InitrdURL, "%s %s initrd", osName, version)
		}
	}
}

func TestBoltStoreRoundTripLeavesOtherBuckets(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	require.NoError(t, database.PutKV(ctx, "osimages", []byte("img-1"), []byte(`{"id":"img-1"}`)))

	store, err := NewBoltStore(database)
	require.NoError(t, err)
	_, err = store.Load(ctx)
	require.ErrorIs(t, err, db.ErrNotFound)

	snap := Snapshot{
		Origin:      OriginLive,
		RefreshedAt: time.Date(2026, 10, 23, 1, 2, 3, 0, time.UTC),
		Errors:      map[string]string{"fedora": "timeout"},
		Releases: []Release{{
			OS: "debian", Version: "13", DisplayName: "13 (Trixie)", Arch: "x86_64",
			KernelURL: "http://example/linux", InitrdURL: "http://example/initrd.gz",
			EOLDate: time.Date(2030, 6, 30, 0, 0, 0, 0, time.UTC),
		}},
	}
	require.NoError(t, store.Save(ctx, snap))
	loaded, err := store.Load(ctx)
	require.NoError(t, err)
	require.Len(t, loaded.Releases, 1)
	assert.Equal(t, "13", loaded.Releases[0].Version)
	assert.Equal(t, "2030-06-30", FormatDay(loaded.Releases[0].EOLDate))
	assert.Equal(t, "timeout", loaded.Errors["fedora"])
	assert.Equal(t, OriginLive, loaded.Origin)

	raw, err := database.GetKV(ctx, "osimages", []byte("img-1"))
	require.NoError(t, err)
	assert.Equal(t, `{"id":"img-1"}`, string(raw))
}

func TestRefreshKeepsFailedOSAndRecordsError(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	store := &memStore{}
	debian := &fakeSource{os: "debian", releases: []Release{{
		OS: "debian", Version: "13", DisplayName: "13 (Trixie)", Arch: "x86_64",
		KernelURL: "http://deb/linux", InitrdURL: "http://deb/initrd.gz",
	}}}
	fedora := &fakeSource{os: "fedora", err: errors.New("releases.json timed out")}
	svc, err := New(Options{
		Sources: []ReleaseSource{debian, fedora},
		Lifecycle: fakeEOL{dates: map[string]map[string]time.Time{
			"debian": {"13": time.Date(2030, 6, 30, 0, 0, 0, 0, time.UTC)},
		}},
		Store: store,
		Now:   func() time.Time { return now },
	})
	require.NoError(t, err)

	snap, err := svc.Refresh(context.Background())
	require.NoError(t, err)
	assert.Equal(t, OriginLive, snap.Origin)
	assert.Equal(t, "releases.json timed out", snap.Errors["fedora"])
	assert.NotContains(t, snap.Errors, "debian")

	var sawDebian13, sawFedora43 bool
	for _, rel := range snap.Releases {
		if rel.OS == "debian" && rel.Version == "13" {
			sawDebian13 = true
			assert.Equal(t, "http://deb/linux", rel.KernelURL)
			assert.Equal(t, "2030-06-30", FormatDay(rel.EOLDate))
		}
		if rel.OS == "fedora" && rel.Version == "43" {
			sawFedora43 = true
			assert.Contains(t, rel.KernelURL, "/43/")
		}
		if rel.OS == "debian" && rel.Version == "12" {
			t.Fatal("debian 12 should have been replaced")
		}
	}
	assert.True(t, sawDebian13)
	assert.True(t, sawFedora43)
	assert.Equal(t, 1, store.saves)

	offer, err := svc.Offerable(context.Background())
	require.NoError(t, err)
	for _, rel := range offer.Releases {
		assert.False(t, rel.EOL(now), rel.OS+" "+rel.Version)
	}
	rel, ok := svc.Lookup(context.Background(), "fedora", "43", "x86_64")
	assert.True(t, ok)
	assert.Equal(t, "43", rel.Version)
}

func TestRefreshEOLFailureKeepsReleaseVisible(t *testing.T) {
	debian := &fakeSource{os: "debian", releases: []Release{{
		OS: "debian", Version: "13", Arch: "x86_64", DisplayName: "13 (Trixie)",
	}}}
	svc, err := New(Options{
		Sources:   []ReleaseSource{debian},
		Lifecycle: fakeEOL{err: map[string]error{"debian": errors.New("endoflife down")}},
		Now:       func() time.Time { return time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC) },
	})
	require.NoError(t, err)
	snap, err := svc.Refresh(context.Background())
	require.NoError(t, err)
	assert.Contains(t, snap.Errors["debian"], "endoflife down")
	var found bool
	for _, rel := range snap.Releases {
		if rel.OS == "debian" && rel.Version == "13" {
			found = true
			assert.True(t, rel.EOLDate.IsZero())
			assert.False(t, rel.EOL(time.Now()))
		}
	}
	assert.True(t, found)
	offer, err := svc.Offerable(context.Background())
	require.NoError(t, err)
	_, ok := findRelease(offer.Releases, "debian", "13")
	assert.True(t, ok, "unknown EOL date must not hide the release")
}

func TestRefreshAllSourcesFailedKeepsSnapshot(t *testing.T) {
	svc, err := New(Options{
		Sources: []ReleaseSource{
			&fakeSource{os: "debian", err: errors.New("down")},
			&fakeSource{os: "fedora", err: errors.New("down")},
		},
	})
	require.NoError(t, err)
	before, err := svc.Current(context.Background())
	require.NoError(t, err)
	_, err = svc.Refresh(context.Background())
	require.ErrorIs(t, err, ErrAllSourcesFailed)
	after, err := svc.Current(context.Background())
	require.NoError(t, err)
	assert.Equal(t, OriginBundled, after.Origin)
	assert.Equal(t, len(before.Releases), len(after.Releases))
}

func TestRefreshInProgress(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	src := &blockingSource{os: "debian", started: started, release: release, releases: []Release{{
		OS: "debian", Version: "13", Arch: "x86_64",
	}}}
	svc, err := New(Options{Sources: []ReleaseSource{src}, Lifecycle: fakeEOL{}})
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() {
		_, err := svc.Refresh(context.Background())
		done <- err
	}()
	<-started
	assert.True(t, svc.Refreshing())
	_, err = svc.Refresh(context.Background())
	require.ErrorIs(t, err, ErrRefreshInProgress)
	close(release)
	require.NoError(t, <-done)
	assert.False(t, svc.Refreshing())
}

func TestSourceTimeoutKeepsPreviousOS(t *testing.T) {
	previous := SourceTimeout
	SourceTimeout = 30 * time.Millisecond
	t.Cleanup(func() { SourceTimeout = previous })

	svc, err := New(Options{Sources: []ReleaseSource{&slowSource{os: "debian"}}})
	require.NoError(t, err)
	snap, err := svc.Refresh(context.Background())
	require.ErrorIs(t, err, ErrAllSourcesFailed)
	assert.Empty(t, snap.Releases)
	current, err := svc.Current(context.Background())
	require.NoError(t, err)
	_, ok := findRelease(current.Releases, "debian", "13")
	assert.True(t, ok)
}

func TestOfflinePinsBundledList(t *testing.T) {
	called := false
	src := &fakeSource{os: "debian", releases: []Release{{OS: "debian", Version: "99", Arch: "x86_64"}}}
	src.onCall = func() { called = true }
	store := &memStore{snap: Snapshot{Origin: OriginLive, Releases: []Release{{OS: "debian", Version: "1", Arch: "x86_64"}}}}
	svc, err := New(Options{Sources: []ReleaseSource{src}, Store: store, Offline: true})
	require.NoError(t, err)
	snap, err := svc.Current(context.Background())
	require.NoError(t, err)
	assert.Equal(t, OriginBundled, snap.Origin)
	_, ok := findRelease(snap.Releases, "debian", "13")
	assert.True(t, ok)
	_, ok = findRelease(snap.Releases, "debian", "1")
	assert.False(t, ok)
	refreshed, err := svc.Refresh(context.Background())
	require.NoError(t, err)
	assert.Equal(t, OriginBundled, refreshed.Origin)
	assert.False(t, called)
}

func TestLookupFindsEOLRelease(t *testing.T) {
	now := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
	svc, err := New(Options{Now: func() time.Time { return now }})
	require.NoError(t, err)
	past := now.AddDate(0, 0, -1)
	svc.mu.Lock()
	svc.current.Releases = append(svc.current.Releases, Release{
		OS: "fedora", Version: "42", Arch: "x86_64", DisplayName: "42", EOLDate: past,
	})
	svc.mu.Unlock()

	rel, ok := svc.Lookup(context.Background(), "fedora", "42", "x86_64")
	require.True(t, ok)
	assert.True(t, rel.EOL(now))
	offer, err := svc.Offerable(context.Background())
	require.NoError(t, err)
	_, ok = findRelease(offer.Releases, "fedora", "42")
	assert.False(t, ok)
}

func TestListingSortsNewestFirstAndMarksLatest(t *testing.T) {
	now := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
	snap := Snapshot{
		Origin: OriginLive,
		Errors: map[string]string{"nixos": "HTTP 403"},
		Releases: []Release{
			{OS: "ubuntu", Version: "22.04", DisplayName: "22.04 LTS", Arch: "x86_64", EOLDate: time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC)},
			{OS: "ubuntu", Version: "26.04", DisplayName: "26.04 LTS", Arch: "x86_64", EOLDate: time.Date(2031, 5, 29, 0, 0, 0, 0, time.UTC)},
			{OS: "ubuntu", Version: "20.04", DisplayName: "20.04 LTS", Arch: "x86_64", EOLDate: time.Date(2025, 5, 31, 0, 0, 0, 0, time.UTC)},
			{OS: "ubuntu", Version: "24.04", DisplayName: "24.04 LTS", Arch: "x86_64", EOLDate: time.Date(2029, 5, 31, 0, 0, 0, 0, time.UTC)},
			{OS: "fedora", Version: "43", DisplayName: "43", Arch: "x86_64"},
			{OS: "fedora", Version: "44", DisplayName: "44", Arch: "aarch64"},
		},
	}
	listing := BuildListing(snap, now, nil, true, false)
	assert.Equal(t, "live", listing.Source)
	assert.True(t, listing.Refreshing)
	assert.Equal(t, "HTTP 403", listing.Errors["nixos"])
	require.NotEmpty(t, listing.OSes)
	assert.Equal(t, "ubuntu", listing.OSes[0].OS)
	var values []string
	var latest string
	for _, version := range listing.OSes[0].Versions {
		values = append(values, version.Value)
		assert.False(t, version.EOL)
		assert.NotEmpty(t, version.EOLDate)
		if version.Latest {
			latest = version.Value
		}
	}
	assert.Equal(t, []string{"26.04", "24.04", "22.04"}, values)
	assert.Equal(t, "26.04", latest)

	withEOL := Versions("ubuntu", snap.Releases, now, true)
	require.Len(t, withEOL, 4)
	assert.Equal(t, "20.04", withEOL[3].Value)
	assert.True(t, withEOL[3].EOL)
	assert.Equal(t, "2025-05-31", withEOL[3].EOLDate)
	assert.False(t, withEOL[3].Latest)
	assert.True(t, withEOL[0].Latest)

	fedora := Versions("fedora", snap.Releases, now, false)
	require.Len(t, fedora, 2)
	assert.True(t, fedora[0].Latest)
	assert.True(t, fedora[1].Latest)
	assert.NotEqual(t, fedora[0].Arch, fedora[1].Arch)
}

func TestCompareVersions(t *testing.T) {
	assert.Equal(t, -1, compareVersions("9", "10"))
	assert.Equal(t, 1, compareVersions("10", "9"))
	assert.Equal(t, 0, compareVersions("22.04", "22.04"))
	assert.Equal(t, -1, compareVersions("20.04", "22.04"))
	assert.Equal(t, -1, compareVersions("6.03", "6.10"))
	assert.Equal(t, -1, compareVersions("6.03", "6.03.1"))
	assert.Equal(t, 1, compareVersions("tumbleweed", "16.0"))
}

func TestParseEOLDatesSkipsNonDates(t *testing.T) {
	dates, err := parseEOLDates([]byte(`[
		{"cycle":"24.04","eol":"2029-05-31","lts":true},
		{"cycle":"26.10","eol":false},
		{"cycle":"rolling","eol":true},
		{"cycle":"16.0","eol":null}
	]`))
	require.NoError(t, err)
	assert.Equal(t, "2029-05-31", FormatDay(dates["24.04"]))
	_, ok := dates["26.10"]
	assert.False(t, ok)
	day, ok := lookupCycle("24.04.4", dates)
	assert.True(t, ok)
	assert.Equal(t, "2029-05-31", FormatDay(day))
	_, ok = lookupCycle("13.7", map[string]time.Time{"13": time.Date(2030, 6, 30, 0, 0, 0, 0, time.UTC)})
	assert.True(t, ok)
}

type fakeSource struct {
	os       string
	releases []Release
	err      error
	onCall   func()
	calls    int
}

func (f *fakeSource) OS() string { return f.os }

func (f *fakeSource) Releases(ctx context.Context) ([]Release, error) {
	f.calls++
	if f.onCall != nil {
		f.onCall()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.releases, nil
}

type fakeEOL struct {
	dates map[string]map[string]time.Time
	err   map[string]error
}

func (f fakeEOL) EOLDates(ctx context.Context, product string) (map[string]time.Time, error) {
	if err := f.err[product]; err != nil {
		return nil, err
	}
	if f.dates == nil {
		return map[string]time.Time{}, nil
	}
	dates := f.dates[product]
	if dates == nil {
		return map[string]time.Time{}, nil
	}
	return dates, nil
}

type blockingSource struct {
	os       string
	started  chan struct{}
	release  chan struct{}
	releases []Release
	once     sync.Once
}

func (b *blockingSource) OS() string { return b.os }

func (b *blockingSource) Releases(ctx context.Context) ([]Release, error) {
	b.once.Do(func() { close(b.started) })
	select {
	case <-b.release:
		return b.releases, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type slowSource struct{ os string }

func (s *slowSource) OS() string { return s.os }

func (s *slowSource) Releases(ctx context.Context) ([]Release, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

type memStore struct {
	mu    sync.Mutex
	snap  Snapshot
	saves int
	err   error
}

func (m *memStore) Load(ctx context.Context) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.snap.Releases) == 0 {
		return Snapshot{}, db.ErrNotFound
	}
	return cloneSnapshot(m.snap), nil
}

func (m *memStore) Save(ctx context.Context, snap Snapshot) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.snap = cloneSnapshot(snap)
	m.saves++
	return nil
}

func findRelease(releases []Release, osName, version string) (Release, bool) {
	for _, rel := range releases {
		if rel.OS == osName && rel.Version == version {
			return rel, true
		}
	}
	return Release{}, false
}

func openTestDB(t *testing.T) db.Database {
	t.Helper()
	t.Setenv("DB_PATH", t.TempDir())
	t.Setenv("DB_FILE", "catalog.db")
	cfg, err := config.LoadDefault()
	require.NoError(t, err)
	database, err := db.NewBoltDB(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func testClient(srv *httptest.Server) *http.Client {
	client := srv.Client()
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	client.Transport = headerTransport{base: base}
	return client
}

type headerTransport struct {
	base http.RoundTripper
}

func (h headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.Header.Set("X-Ignite-Test-Client", "1")
	return h.base.RoundTrip(cloned)
}

func newFixtureServer(t *testing.T, routes map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Ignite-Test-Client") != "1" {
			http.Error(w, "request did not use the injected client", http.StatusBadGateway)
			return
		}
		body, ok := routes[r.URL.RequestURI()]
		if !ok {
			body, ok = routes[r.URL.Path]
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}
