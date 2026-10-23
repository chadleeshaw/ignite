package osimage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"ignite/config"
	"ignite/dlstatus"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBootFileURLCollapsesSlashes(t *testing.T) {
	tests := []struct {
		base string
		name string
		want string
	}{
		{"http://example.com/foo/", "vmlinuz", "http://example.com/foo/vmlinuz"},
		{"http://example.com/foo", "vmlinuz", "http://example.com/foo/vmlinuz"},
		{"http://example.com/foo/", "/vmlinuz", "http://example.com/foo/vmlinuz"},
		{"http://example.com/foo//", "initrd.img", "http://example.com/foo/initrd.img"},
	}
	for _, test := range tests {
		got, err := bootFileURL(test.base, test.name)
		require.NoError(t, err)
		assert.Equal(t, test.want, got)
	}

	_, err := bootFileURL("", "vmlinuz")
	assert.Error(t, err)
	_, err = bootFileURL("http://example.com/foo/", "")
	assert.Error(t, err)
	_, err = bootFileURL("http://example.com/foo/", "a/b")
	assert.Error(t, err)
}

// TestCatalogBootFileURLs asserts the download URL of every bundled release.
func TestCatalogBootFileURLs(t *testing.T) {
	cfg, err := config.NewConfigBuilder().Build()
	require.NoError(t, err)

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

	require.Len(t, cfg.OSImages.Sources, len(want))
	for osName, def := range cfg.OSImages.Sources {
		versions, ok := want[osName]
		require.True(t, ok, "unexpected OS %s", osName)
		require.Len(t, def.Versions, len(versions), osName)
		for version, info := range def.Versions {
			urls, ok := versions[version]
			require.True(t, ok, "unexpected %s %s", osName, version)
			kernel, err := bootFileURL(info.BaseURL, def.KernelFile)
			require.NoError(t, err)
			initrd, err := bootFileURL(info.BaseURL, def.InitrdFile)
			require.NoError(t, err)
			assert.Equal(t, urls[0], kernel, "%s %s kernel", osName, version)
			assert.Equal(t, urls[1], initrd, "%s %s initrd", osName, version)
			assert.NotContains(t, kernel, "/x86_64/vmlinuz")
			assert.NotContains(t, initrd, "/x86_64/initrd.img")
		}
	}
}

func TestProcessDownloadUsesCatalogFileNames(t *testing.T) {
	var mu sync.Mutex
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = append(got, r.URL.Path)
		mu.Unlock()
		_, _ = w.Write([]byte(r.URL.Path))
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	images := &memImages{}
	statuses := newMemStatus()
	svc := &osImageServiceImpl{
		repo:         images,
		downloadRepo: statuses,
		config:       &config.Config{TFTP: config.TFTPConfig{Dir: dir}},
		httpClient:   srv.Client(),
		cancels:      map[string]context.CancelFunc{},
	}
	id := "dl-1"
	require.NoError(t, statuses.Save(context.Background(), &DownloadStatus{
		ID:      id,
		OS:      "debian",
		Version: "12",
		Status:  dlstatus.StatusQueued,
	}))

	svc.processDownload(downloadRequest{
		ctx: context.Background(),
		id:  id,
		osConfig: OSImageConfig{
			OS:           "debian",
			Version:      "12",
			Architecture: "x86_64",
		},
		versionInfo: config.OSVersion{
			BaseURL:       srv.URL + "/debian/dists/bookworm/main/installer-amd64/current/images/netboot/debian-installer/amd64/",
			Architectures: []string{"x86_64"},
		},
		kernelFile: "linux",
		initrdFile: "initrd.gz",
	})

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{
		"/debian/dists/bookworm/main/installer-amd64/current/images/netboot/debian-installer/amd64/linux",
		"/debian/dists/bookworm/main/installer-amd64/current/images/netboot/debian-installer/amd64/initrd.gz",
	}, got)

	kernelPath := filepath.Join(dir, "os-images", "debian", "12", "vmlinuz")
	initrdPath := filepath.Join(dir, "os-images", "debian", "12", "initrd.img")
	kernel, err := os.ReadFile(kernelPath)
	require.NoError(t, err)
	initrd, err := os.ReadFile(initrdPath)
	require.NoError(t, err)
	assert.Equal(t, got[0], string(kernel))
	assert.Equal(t, got[1], string(initrd))

	require.NotNil(t, images.saved)
	assert.Equal(t, kernelPath, images.saved.KernelPath)
	assert.Equal(t, initrdPath, images.saved.InitrdPath)

	status, err := statuses.Get(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, dlstatus.StatusCompleted, status.Status)
}

type memImages struct {
	saved *OSImage
}

func (m *memImages) Save(_ context.Context, image *OSImage) error {
	cp := *image
	m.saved = &cp
	return nil
}

func (m *memImages) Get(context.Context, string) (*OSImage, error) { return nil, os.ErrNotExist }
func (m *memImages) GetAll(context.Context) ([]*OSImage, error)    { return nil, nil }
func (m *memImages) GetByOS(context.Context, string) ([]*OSImage, error) {
	return nil, nil
}
func (m *memImages) GetByOSAndVersion(context.Context, string, string) (*OSImage, error) {
	return nil, os.ErrNotExist
}
func (m *memImages) GetDefaultVersion(context.Context, string) (*OSImage, error) {
	return nil, os.ErrNotExist
}
func (m *memImages) Delete(context.Context, string) error { return nil }
func (m *memImages) SetDefault(context.Context, string) error {
	return nil
}

type memStatus struct {
	mu   sync.Mutex
	rows map[string]*DownloadStatus
}

func newMemStatus() *memStatus {
	return &memStatus{rows: map[string]*DownloadStatus{}}
}

func (m *memStatus) Save(_ context.Context, status *DownloadStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *status
	m.rows[status.ID] = &cp
	return nil
}

func (m *memStatus) Get(_ context.Context, id string) (*DownloadStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.rows[id]
	if !ok {
		return nil, os.ErrNotExist
	}
	cp := *row
	return &cp, nil
}

func (m *memStatus) GetActive(context.Context) ([]*DownloadStatus, error) { return nil, nil }
func (m *memStatus) Delete(context.Context, string) error                 { return nil }
