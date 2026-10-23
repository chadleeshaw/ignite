package osimage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ignite/config"
	"ignite/dlstatus"
	"ignite/osimage/catalog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProcessDownloadIgnoresSharedExpectedChecksum records that one catalog
// digest is not applied to both files. The kernel and the initrd differ, so
// a single ExpectedChecksum cannot match both.
func TestProcessDownloadIgnoresSharedExpectedChecksum(t *testing.T) {
	const kernelBody = "kernel-bytes"
	const initrdBody = "initrd-bytes-are-different"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/linux"):
			_, _ = w.Write([]byte(kernelBody))
		case strings.HasSuffix(r.URL.Path, "/initrd.gz"):
			_, _ = w.Write([]byte(initrdBody))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	kernelSum := sha256.Sum256([]byte(kernelBody))
	initrdSum := sha256.Sum256([]byte(initrdBody))
	kernelHex := hex.EncodeToString(kernelSum[:])
	initrdHex := hex.EncodeToString(initrdSum[:])
	require.NotEqual(t, kernelHex, initrdHex)

	images := &memImages{}
	statuses := newMemStatus()
	id := "dl-checksum"
	require.NoError(t, statuses.Save(context.Background(), &DownloadStatus{
		ID:      id,
		Status:  dlstatus.StatusQueued,
		OS:      "debian",
		Version: "12",
	}))

	svc := &osImageServiceImpl{
		repo:         images,
		downloadRepo: statuses,
		config:       &config.Config{TFTP: config.TFTPConfig{Dir: t.TempDir()}},
		httpClient:   srv.Client(),
		cancels:      map[string]context.CancelFunc{},
	}
	svc.processDownload(downloadRequest{
		ctx: context.Background(),
		id:  id,
		osConfig: OSImageConfig{
			OS:           "debian",
			Version:      "12",
			Architecture: "x86_64",
		},
		release: catalog.Release{
			OS: "debian", Version: "12", Arch: "x86_64",
			KernelURL: srv.URL + "/netboot/linux",
			InitrdURL: srv.URL + "/netboot/initrd.gz",
		},
	})

	status, err := statuses.Get(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, dlstatus.StatusCompleted, status.Status, status.ErrorMessage)
	require.NotNil(t, images.saved)
	assert.Equal(t, kernelHex+":"+initrdHex, images.saved.Checksum)
	assert.Equal(t, int64(len(kernelBody)), images.saved.KernelSize)
	assert.Equal(t, int64(len(initrdBody)), images.saved.InitrdSize)
}

func TestProcessDownloadRejectsChecksumMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("kernel-bytes"))
	}))
	t.Cleanup(srv.Close)

	images := &memImages{}
	statuses := newMemStatus()
	id := "dl-mismatch"
	require.NoError(t, statuses.Save(context.Background(), &DownloadStatus{
		ID: id, Status: dlstatus.StatusQueued, OS: "debian", Version: "12",
	}))
	svc := &osImageServiceImpl{
		repo:         images,
		downloadRepo: statuses,
		config:       &config.Config{TFTP: config.TFTPConfig{Dir: t.TempDir()}},
		httpClient:   srv.Client(),
		cancels:      map[string]context.CancelFunc{},
	}
	svc.processDownload(downloadRequest{
		ctx:      context.Background(),
		id:       id,
		osConfig: OSImageConfig{OS: "debian", Version: "12", Architecture: "x86_64"},
		release: catalog.Release{
			OS: "debian", Version: "12", Arch: "x86_64",
			KernelURL:    srv.URL + "/linux",
			InitrdURL:    srv.URL + "/initrd.gz",
			KernelSHA256: "deadbeef",
		},
	})

	status, err := statuses.Get(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, dlstatus.StatusFailed, status.Status)
	assert.Contains(t, status.ErrorMessage, "checksum mismatch")
	assert.Nil(t, images.saved)
}
