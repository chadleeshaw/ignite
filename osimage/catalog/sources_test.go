package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
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
		"/debian/dists/trixie/main/installer-amd64/current/images/SHA256SUMS": "" +
			"aaaa  ./netboot/gtk/debian-installer/amd64/linux\n" +
			"bbbb  ./netboot/debian-installer/amd64/linux\n" +
			"cccc  ./netboot/debian-installer/amd64/initrd.gz\n",
		"/44/Everything/x86_64/os/.treeinfo":    "[checksums]\nimages/pxeboot/vmlinuz = sha256:dddd\nimages/pxeboot/initrd.img = sha256:eeee\n",
		"/10-stream/BaseOS/x86_64/os/.treeinfo": "[checksums]\nimages/pxeboot/vmlinuz = sha256:ffff\nimages/pxeboot/initrd.img = SHA256:1111\n",
		"/releases.json":                        fedora,
		"/":                                     centos,
		"/centos-stream.json":                   `[{"cycle":"10","eol":"2030-05-31"},{"cycle":"9","eol":"2027-05-31"}]`,
	})
	client := testClient(srv)

	debian, err := Debian{Client: client, Mirror: srv.URL + "/debian"}.Releases(context.Background())
	require.NoError(t, err)
	require.Len(t, debian, 2)
	assert.Equal(t, "13", debian[0].Version)
	assert.Equal(t, "13 (Trixie)", debian[0].DisplayName)
	assert.Equal(t, "bbbb", debian[0].KernelSHA256)
	assert.Equal(t, "cccc", debian[0].InitrdSHA256)
	assert.NotEqual(t, "aaaa", debian[0].KernelSHA256)
	assert.Equal(t, srv.URL+"/debian/dists/trixie/main/installer-amd64/current/images/netboot/debian-installer/amd64/linux", debian[0].KernelURL)
	assert.Contains(t, debian[0].InitrdURL, "/initrd.gz")
	assert.Equal(t, "12", debian[1].Version)
	assert.Equal(t, "12 (Bookworm)", debian[1].DisplayName)

	fedoraReleases, err := Fedora{
		Client:       client,
		ReleasesURL:  srv.URL + "/releases.json",
		DownloadBase: srv.URL,
	}.Releases(context.Background())
	require.NoError(t, err)
	require.Len(t, fedoraReleases, 3)
	assert.Equal(t, []string{"42", "43", "44"}, []string{fedoraReleases[0].Version, fedoraReleases[1].Version, fedoraReleases[2].Version})
	assert.Contains(t, fedoraReleases[2].KernelURL, "/44/Everything/x86_64/os/images/pxeboot/vmlinuz")
	assert.Contains(t, fedoraReleases[2].InitrdURL, "/initrd.img")
	assert.Equal(t, "dddd", fedoraReleases[2].KernelSHA256)
	assert.Equal(t, "eeee", fedoraReleases[2].InitrdSHA256)
	assert.Empty(t, fedoraReleases[0].KernelSHA256)

	centosReleases, err := CentOS{Client: client, Mirror: srv.URL}.Releases(context.Background())
	require.NoError(t, err)
	require.Len(t, centosReleases, 2)
	assert.Equal(t, "9", centosReleases[0].Version)
	assert.Equal(t, "9 Stream", centosReleases[0].DisplayName)
	assert.Equal(t, "10", centosReleases[1].Version)
	assert.Contains(t, centosReleases[1].KernelURL, "/10-stream/BaseOS/x86_64/os/images/pxeboot/vmlinuz")
	assert.Equal(t, "ffff", centosReleases[1].KernelSHA256)
	assert.Equal(t, "1111", centosReleases[1].InitrdSHA256)

	dates, err := (EndOfLife{Client: client, BaseURL: srv.URL}).EOLDates(context.Background(), "centos-stream")
	require.NoError(t, err)
	assert.Equal(t, "2030-05-31", FormatDay(dates["10"]))

	_, err = (Debian{Mirror: srv.URL + "/debian"}).Releases(context.Background())
	require.ErrorContains(t, err, "http client is nil")
}

func TestDefaultSourcesCoverSupportedOSes(t *testing.T) {
	sources := DefaultSources(NewClient(), "token")
	require.Len(t, sources, 6)
	assert.Equal(t, "debian", sources[0].OS())
	assert.Equal(t, "fedora", sources[1].OS())
	assert.Equal(t, "centos", sources[2].OS())
	assert.Equal(t, "ubuntu", sources[3].OS())
	assert.Equal(t, "nixos", sources[4].OS())
	assert.Equal(t, "opensuse", sources[5].OS())
	nix, ok := sources[4].(NixOS)
	require.True(t, ok)
	assert.Equal(t, "token", nix.Token)
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

func TestUbuntuKeepsLTSCycles(t *testing.T) {
	const doc = `
endpoints:
  ubuntu-netboot-24.10-amd64:
    path: /ubuntu-squash/releases/download/24.10.1-aaaa/
  ubuntu-netboot-26.10-amd64:
    path: /ubuntu-squash/releases/download/26.10-bbbb/
  ubuntu-netboot-hwe-24.04-amd64:
    path: /ubuntu-squash/releases/download/hwe/
  ubuntu-netboot-24.04-amd64:
    path: /ubuntu-squash/releases/download/24.04.4-494663bf/
    version: 24.04.4
  ubuntu-netboot-22.04-amd64:
    path: ubuntu-squash/releases/download/22.04.5-cccc
  ubuntu-24.04-default-squash:
    path: /ubuntu-squash/releases/download/other/
`
	srv := newFixtureServer(t, map[string]string{"/endpoints.yml": doc})
	releases, err := Ubuntu{Client: testClient(srv), EndpointsURL: srv.URL + "/endpoints.yml"}.Releases(context.Background())
	require.NoError(t, err)
	require.Len(t, releases, 2)
	assert.Equal(t, "22.04", releases[0].Version)
	assert.Equal(t, "24.04", releases[1].Version)
	assert.Equal(t, "24.04 LTS", releases[1].DisplayName)
	assert.Equal(t, "x86_64", releases[1].Arch)
	assert.Equal(t, "https://github.com/netbootxyz/ubuntu-squash/releases/download/24.04.4-494663bf/vmlinuz", releases[1].KernelURL)
	assert.Equal(t, "https://github.com/netbootxyz/ubuntu-squash/releases/download/24.04.4-494663bf/initrd", releases[1].InitrdURL)
	assert.Equal(t, "https://github.com/netbootxyz/ubuntu-squash/releases/download/22.04.5-cccc/vmlinuz", releases[0].KernelURL)
}

func TestNixOSPagesTokenAndForbidden(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Ignite-Test-Client") != "1" {
			http.Error(w, "request did not use the injected client", http.StatusBadGateway)
			return
		}
		if r.Header.Get("Accept") != "application/vnd.github+json" {
			http.Error(w, "missing accept", http.StatusBadRequest)
			return
		}
		switch r.URL.Path {
		case "/rel":
			if r.Header.Get("Authorization") != "Bearer secret" {
				http.Error(w, "missing token", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Link", "<"+srv.URL+"/page2>; rel=\"next\"")
			_, _ = w.Write([]byte(`[
				{"tag_name":"nixos-26.05","draft":false,"prerelease":false,"assets":[
					{"name":"bzImage-x86_64-linux","browser_download_url":"https://example.test/bz-26"},
					{"name":"initrd-x86_64-linux","browser_download_url":"https://example.test/initrd-26"},
					{"name":"bzImage-aarch64-linux","browser_download_url":"https://example.test/arm"}
				]},
				{"tag_name":"nixos-25.11","draft":false,"prerelease":true,"assets":[
					{"name":"bzImage-x86_64-linux","browser_download_url":"https://example.test/skip"},
					{"name":"initrd-x86_64-linux","browser_download_url":"https://example.test/skip"}
				]},
				{"tag_name":"nixos-24.11","draft":true,"prerelease":false,"assets":[]}
			]`))
		case "/page2":
			_, _ = w.Write([]byte(`[
				{"tag_name":"nixos-25.05","draft":false,"prerelease":false,"assets":[
					{"name":"bzImage-x86_64-linux","browser_download_url":"https://example.test/bz-25"},
					{"name":"initrd-x86_64-linux","browser_download_url":"https://example.test/initrd-25"}
				]},
				{"tag_name":"not-a-release","draft":false,"prerelease":false,"assets":[]}
			]`))
		case "/denied":
			http.Error(w, "rate limit", http.StatusForbidden)
		default:
			http.Error(w, r.URL.Path, http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	releases, err := NixOS{Client: testClient(srv), APIURL: srv.URL + "/rel", Token: "secret"}.Releases(context.Background())
	require.NoError(t, err)
	require.Len(t, releases, 2)
	assert.Equal(t, "26.05", releases[0].Version)
	assert.Equal(t, "25.05", releases[1].Version)
	assert.Equal(t, "https://example.test/bz-26", releases[0].KernelURL)
	assert.Equal(t, "https://example.test/initrd-26", releases[0].InitrdURL)
	assert.Equal(t, "x86_64", releases[0].Arch)

	_, err = NixOS{Client: testClient(srv), APIURL: srv.URL + "/denied"}.Releases(context.Background())
	require.ErrorContains(t, err, "HTTP 403")
}

func TestOpenSUSEKeepsStableLeapAndTumbleweed(t *testing.T) {
	const body = `{
		"Leap": [
			{"name":"openSUSE Leap","version":"16.1","state":"RC"},
			{"name":"openSUSE Leap","version":"16.0","state":"Stable"},
			{"name":"openSUSE Leap","version":"15.6","state":"Stable"},
			{"name":"openSUSE Leap","version":"15.5","state":"EOL"}
		],
		"LeapMicro": [{"name":"openSUSE Leap Micro","version":"6.2","state":"Stable"}],
		"Tumbleweed": [{"name":"openSUSE Tumbleweed","version":"20261021"}]
	}`
	srv := newFixtureServer(t, map[string]string{"/distributions.json": body})
	releases, err := OpenSUSE{Client: testClient(srv), DistributionsURL: srv.URL + "/distributions.json"}.Releases(context.Background())
	require.NoError(t, err)
	require.Len(t, releases, 3)
	assert.Equal(t, "15.6", releases[0].Version)
	assert.Equal(t, "16.0", releases[1].Version)
	assert.Equal(t, "tumbleweed", releases[2].Version)
	assert.Equal(t, "Tumbleweed (Rolling)", releases[2].DisplayName)
	assert.Equal(t, "http://download.opensuse.org/distribution/leap/16.0/repo/oss/boot/x86_64/loader/linux", releases[1].KernelURL)
	assert.Equal(t, "http://download.opensuse.org/tumbleweed/repo/oss/boot/x86_64/loader/initrd", releases[2].InitrdURL)
	for _, rel := range releases {
		assert.NotContains(t, rel.Version, "6.2")
		assert.NotEqual(t, "16.1", rel.Version)
		assert.NotEqual(t, "15.5", rel.Version)
	}
}
