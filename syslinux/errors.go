package syslinux

import "errors"

var (
	// ErrVersionNotFound is returned when no stored syslinux version matches
	// the requested number.
	ErrVersionNotFound = errors.New("syslinux version not found")

	// ErrVersionNotDownloaded is returned when activation is requested for a
	// version that has not been downloaded. Callers should surface this so
	// the UI can offer download again.
	ErrVersionNotDownloaded = errors.New("syslinux version is not downloaded")
)
