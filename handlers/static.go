package handlers

import (
	"embed"
	"io"
	"net/http"
	"path"
	"strings"
)

// StaticHandlers manages static file serving
type StaticHandlers struct {
	staticFS embed.FS
	httpDir  string
}

// NewStaticHandlers creates a new static file handler
func NewStaticHandlers(staticFS embed.FS, httpDir string) *StaticHandlers {
	return &StaticHandlers{
		staticFS: staticFS,
		httpDir:  httpDir,
	}
}

// GetFS returns the embedded filesystem
func (h *StaticHandlers) GetFS() embed.FS {
	return h.staticFS
}

// ServeStatic serves static files from embedded filesystem
func (h *StaticHandlers) ServeStatic(w http.ResponseWriter, r *http.Request) {
	// Clean the path to prevent directory traversal
	cleanPath := path.Clean(r.URL.Path)

	// The URL path is "/public/http/css/style.css"
	// We need to map this directly to the embedded file path "public/http/css/style.css"
	// Strip the leading slash to get the embedded file path
	embeddedPath := strings.TrimPrefix(cleanPath, "/")

	// Try to open the file from embedded filesystem
	file, err := h.staticFS.Open(embeddedPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()

	// Get file info for content type detection
	fileInfo, err := file.Stat()
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Serve directory listings are disabled for security
	if fileInfo.IsDir() {
		http.NotFound(w, r)
		return
	}

	// Set appropriate content type based on file extension
	contentType := getContentType(embeddedPath)
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}

	// Serve the file
	http.ServeContent(w, r, fileInfo.Name(), fileInfo.ModTime(), file.(io.ReadSeeker))
}

// getContentType returns the appropriate content type for a file
func getContentType(filename string) string {
	ext := path.Ext(filename)
	switch ext {
	case ".css":
		return "text/css"
	case ".js":
		return "application/javascript"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".ico":
		return "image/x-icon"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	case ".ttf":
		return "font/ttf"
	case ".otf":
		return "font/otf"
	default:
		return ""
	}
}
