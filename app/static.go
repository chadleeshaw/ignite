package app

import (
	"embed"
	"io/fs"
	"net/http"
)

// StaticFileSystem wraps embed.FS for serving static files
type StaticFileSystem struct {
	embedFS embed.FS
	prefix  string
}

// NewStaticFileSystem creates a new static file system
func NewStaticFileSystem(embedFS embed.FS, prefix string) *StaticFileSystem {
	return &StaticFileSystem{
		embedFS: embedFS,
		prefix:  prefix,
	}
}

// Open implements fs.FS interface
func (sfs *StaticFileSystem) Open(name string) (fs.File, error) {
	return sfs.embedFS.Open(sfs.prefix + "/" + name)
}

// HTTPHandler returns an http.Handler for serving static files
func (sfs *StaticFileSystem) HTTPHandler() http.Handler {
	return http.FileServer(http.FS(sfs))
}

// StripPrefix returns a handler that strips the prefix from the URL path
func (sfs *StaticFileSystem) StripPrefixHandler(prefix string) http.Handler {
	return http.StripPrefix(prefix, sfs.HTTPHandler())
}
