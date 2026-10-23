package catalog

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDebianFedoraCentOSSourcesUseInjectedClient(t *testing.T) {
	const debianStable = "Origin: Debian\nSuite: stable\nVersion: 13.7\nCodename: trixie\n"
	const debianOld = "Origin: Debian\nSuite: oldstable\nVersion: 12.15\nCodename: bookworm\n"
	const fedora = `[
		{"version":"45 Beta","arch":"x86_64","variant":"Everything","subvariant":"Everything"},
		{"version":"44","arch":"aarch64","variant":"Everything","subvariant":"Everything"},
		{"version":"44","arch":"x86_64","variant":"Server","subvariant":"Server"},
		{"version":"44","arch":"x86_64","variant":"Everything","subvariant":"Everything"},
		{"version":"43","arch":"x86_64","variant":"Everything","subvariant":"Everything"},
		{"version":"42","arch":"x86_64","variant":"Everything","subvariant":"Everything"}
	]`
	const centos = `<html><a href="SIGs/">SIGs</a><a href="9-stream/">9-stream/</a><a href="10-stream/">10-stream/</a><a href="9-stream/">dup</a></html>`

	srv := newFixtureServer(t, map[string]string{
		"/debian/dists/stable/Release":    debianStable,
		"/debian/dists/oldstable/Release": debianOld,
		"/releases.json":                  fedora,
		"/":                               centos,
		"/centos-stream.json":             `[{"cycle":"10","eol":"2030-05-31"},{"cycle":"9","eol":"2027-05-31"}]`,
	})
	client := testClient(srv)

	debian, err := Debian{Client: client, Mirror: srv.URL + "/debian"}.Releases(context.Background())
	require.NoError(t, err)
	require.Len(t, debian, 2)
	assert.Equal(t, "13", debian[0].Version)
	assert.Equal(t, "13 (Trixie)", debian[0].DisplayName)
	assert.Equal(t, srv.URL+"/debian/dists/trixie/main/installer-amd64/current/images/netboot/debian-installer/amd64/linux", debian[0].KernelURL)
	assert.Contains(t, debian[0].InitrdURL, "/initrd.gz")
	assert.Equal(t, "12", debian[1].Version)
	assert.Equal(t, "12 (Bookworm)", debian[1].DisplayName)

	fedoraReleases, err := Fedora{
		Client:       client,
		ReleasesURL:  srv.URL + "/releases.json",
		DownloadBase: "https://download.fedoraproject.org/pub/fedora/linux/releases",
	}.Releases(context.Background())
	require.NoError(t, err)
	require.Len(t, fedoraReleases, 3)
	assert.Equal(t, []string{"42", "43", "44"}, []string{fedoraReleases[0].Version, fedoraReleases[1].Version, fedoraReleases[2].Version})
	assert.Contains(t, fedoraReleases[2].KernelURL, "/44/Everything/x86_64/os/images/pxeboot/vmlinuz")
	assert.Contains(t, fedoraReleases[2].InitrdURL, "/initrd.img")

	centosReleases, err := CentOS{Client: client, Mirror: srv.URL}.Releases(context.Background())
	require.NoError(t, err)
	require.Len(t, centosReleases, 2)
	assert.Equal(t, "9", centosReleases[0].Version)
	assert.Equal(t, "9 Stream", centosReleases[0].DisplayName)
	assert.Equal(t, "10", centosReleases[1].Version)
	assert.Contains(t, centosReleases[1].KernelURL, "/10-stream/BaseOS/x86_64/os/images/pxeboot/vmlinuz")

	dates, err := (EndOfLife{Client: client, BaseURL: srv.URL}).EOLDates(context.Background(), "centos-stream")
	require.NoError(t, err)
	assert.Equal(t, "2030-05-31", FormatDay(dates["10"]))

	_, err = (Debian{Mirror: srv.URL + "/debian"}).Releases(context.Background())
	require.ErrorContains(t, err, "http client is nil")
}

func TestDefaultSourcesCoverPhase1(t *testing.T) {
	sources := DefaultSources(NewClient())
	require.Len(t, sources, 3)
	assert.Equal(t, "debian", sources[0].OS())
	assert.Equal(t, "fedora", sources[1].OS())
	assert.Equal(t, "centos", sources[2].OS())
}

func TestStartRefreshesWithoutBlocking(t *testing.T) {
	called := make(chan struct{}, 1)
	src := &fakeSource{os: "debian", releases: []Release{{
		OS: "debian", Version: "13", Arch: "x86_64", DisplayName: "13 (Trixie)",
	}}}
	src.onCall = func() {
		select {
		case called <- struct{}{}:
		default:
		}
	}
	svc, err := New(Options{
		Sources:   []ReleaseSource{src},
		Lifecycle: fakeEOL{},
		Interval:  time.Hour,
	})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	returned := make(chan struct{})
	go func() {
		svc.Start(ctx)
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("Start blocked")
	}
	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("startup refresh did not run")
	}
}
