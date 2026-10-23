package handlers

import (
	"net"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"ignite/dhcp"

	"github.com/stretchr/testify/require"
)

func TestFormatRFC3339HasOffset(t *testing.T) {
	when := time.Date(2026, 10, 19, 22, 30, 54, 0, time.FixedZone("IST", 5*3600+30*60))
	got := formatRFC3339(when)
	require.Equal(t, "2026-10-19T22:30:54+05:30", got)
	parsed, err := time.Parse(time.RFC3339, got)
	require.NoError(t, err)
	require.True(t, parsed.Equal(when))
	require.Empty(t, formatRFC3339(time.Time{}))
}

func TestTFTPLastModifiedKeepsTextAndRFC3339(t *testing.T) {
	dir := t.TempDir()
	prev := TFTPDir
	SetTFTPRoot(dir)
	t.Cleanup(func() { SetTFTPRoot(prev) })

	when := time.Date(2026, 10, 19, 22, 30, 54, 0, time.FixedZone("IST", 5*3600+30*60))
	path := filepath.Join(dir, "boot.ipxe")
	require.NoError(t, os.WriteFile(path, []byte("#!ipxe\n"), 0o644))
	require.NoError(t, os.Chtimes(path, when, when))

	data, err := (&TFTPHandlers{}).getFileInfo("./")
	require.NoError(t, err)
	var found *FileInfo
	for i := range data.Files {
		if data.Files[i].Name == "boot.ipxe" {
			found = &data.Files[i]
		}
	}
	require.NotNil(t, found)
	require.Equal(t, when.Local().Format("2006-01-02 15:04:05"), found.LastModified)
	parsed, err := time.Parse(time.RFC3339, found.LastModifiedAt)
	require.NoError(t, err)
	require.True(t, parsed.Equal(when))
	require.Regexp(t, regexp.MustCompile(`(?:Z|[+-]\d{2}:\d{2})$`), found.LastModifiedAt)
}

func TestLeaseLastSeenAtIsRFC3339(t *testing.T) {
	when := time.Date(2026, 10, 19, 22, 30, 54, 0, time.UTC)
	h := &DHCPHandlers{}
	views := h.convertLeasesToViews([]*dhcp.Lease{{
		MAC:      "aa:bb:cc:dd:ee:ff",
		IP:       net.ParseIP("10.0.0.5"),
		LastSeen: when,
		State:    dhcp.StateAssigned,
	}})
	require.Len(t, views, 1)
	require.Equal(t, "2026-10-19T22:30:54Z", views[0].LastSeenAt)
	require.True(t, views[0].LastSeen.Equal(when))

	unset := h.convertLeasesToViews([]*dhcp.Lease{{
		MAC:   "aa:bb:cc:dd:ee:ff",
		IP:    net.ParseIP("10.0.0.6"),
		State: dhcp.StateAssigned,
	}})
	require.Empty(t, unset[0].LastSeenAt)
}

func TestUTCLabelUsesTheValuesZone(t *testing.T) {
	utc := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	require.Equal(t, "2026-01-02 03:04:05 UTC", utcLabel(utc, "2006-01-02 15:04:05"))
	require.Equal(t, "UTC", zoneName(utc))

	ist := time.Date(2026, 10, 19, 22, 30, 54, 0, time.FixedZone("IST", 5*3600+30*60))
	got := utcLabel(ist, "2006-01-02 15:04:05")
	require.Equal(t, "2026-10-19 22:30:54 IST", got)
	require.NotContains(t, got, "UTC")
	require.Equal(t, "IST", zoneName(ist))
	require.Empty(t, utcLabel(time.Time{}, ""))
}
