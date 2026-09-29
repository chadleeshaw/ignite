package dlstatus

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatusValues(t *testing.T) {
	assert.Equal(t, Status("queued"), StatusQueued)
	assert.Equal(t, Status("downloading"), StatusDownloading)
	assert.Equal(t, Status("extracting"), StatusExtracting)
	assert.Equal(t, Status("installing"), StatusInstalling)
	assert.Equal(t, Status("completed"), StatusCompleted)
	assert.Equal(t, Status("failed"), StatusFailed)
	assert.Equal(t, Status("cancelled"), StatusCancelled)
}

func TestStatusActive(t *testing.T) {
	for _, s := range []Status{StatusQueued, StatusDownloading, StatusExtracting, StatusInstalling} {
		assert.True(t, s.Active(), "status %q should be active", s)
	}
	for _, s := range []Status{StatusCompleted, StatusFailed, StatusCancelled} {
		assert.False(t, s.Active(), "status %q should not be active", s)
	}
}

func TestStatusTerminal(t *testing.T) {
	for _, s := range []Status{StatusCompleted, StatusFailed, StatusCancelled} {
		assert.True(t, s.Terminal(), "status %q should be terminal", s)
	}
	for _, s := range []Status{StatusQueued, StatusDownloading, StatusExtracting, StatusInstalling} {
		assert.False(t, s.Terminal(), "status %q should not be terminal", s)
	}
}

func TestActiveAndTerminalAreComplementary(t *testing.T) {
	all := []Status{
		StatusQueued, StatusDownloading, StatusExtracting, StatusInstalling,
		StatusCompleted, StatusFailed, StatusCancelled,
	}
	for _, s := range all {
		assert.NotEqual(t, s.Active(), s.Terminal(), "status %q must be exactly one of active/terminal", s)
	}
}

func TestDownloadStatusStruct(t *testing.T) {
	ds := DownloadStatus{
		ID:       "test-id",
		Status:   StatusDownloading,
		Progress: 42,
	}
	assert.Equal(t, "test-id", ds.ID)
	assert.True(t, ds.Status.Active())
	assert.False(t, ds.Status.Terminal())
}
