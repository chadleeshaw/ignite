// Package dlstatus provides a single, shared download-status type for the
// whole application. The osimage and syslinux packages previously each
// defined their own DownloadStatus struct with free-form status strings;
// everything now uses this one struct and the typed Status constants below.
package dlstatus

import "time"

// Status is the lifecycle state of a file download.
type Status string

// Lifecycle states for a download.
const (
	StatusQueued      Status = "queued"
	StatusDownloading Status = "downloading"
	StatusExtracting  Status = "extracting"
	StatusInstalling  Status = "installing"
	StatusCompleted   Status = "completed"
	StatusFailed      Status = "failed"
	StatusCancelled   Status = "cancelled"
)

// Active reports whether the download is still in progress (not terminal).
func (s Status) Active() bool {
	switch s {
	case StatusQueued, StatusDownloading, StatusExtracting, StatusInstalling:
		return true
	default:
		return false
	}
}

// Terminal reports whether the status is a final state that must never be
// overwritten by stale worker updates.
func (s Status) Terminal() bool {
	switch s {
	case StatusCompleted, StatusFailed, StatusCancelled:
		return true
	default:
		return false
	}
}

// DownloadStatus tracks the progress of a file download initiated through
// the OS image or syslinux services.
type DownloadStatus struct {
	ID           string     `json:"id"`
	OS           string     `json:"os,omitempty"`
	Version      string     `json:"version"`
	Status       Status     `json:"status"`
	Progress     int        `json:"progress"` // 0-100
	ErrorMessage string     `json:"error_message,omitempty"`
	StartedAt    time.Time  `json:"started_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}
