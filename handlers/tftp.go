package handlers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// TFTPHandlers handles TFTP-related requests
type TFTPHandlers struct {
	container *Container
}

// NewTFTPHandlers creates a new TFTPHandlers instance
func NewTFTPHandlers(container *Container) *TFTPHandlers {
	return &TFTPHandlers{container: container}
}

// HandleTFTPPage serves the TFTP management page
func (h *TFTPHandlers) HandleTFTPPage(w http.ResponseWriter, r *http.Request) {
	templates := LoadTemplates()
	var data *TFTPData
	var err error

	if dir := r.URL.Query().Get("dir"); dir != "" {
		data, err = h.getTFTPDir(dir)
	} else {
		data, err = h.getTFTP()
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data.ServerDirectory = strings.TrimPrefix(strings.TrimPrefix(data.ServerDirectory, TFTPDir), string(filepath.Separator))
	data.PrevDirectory = removeLastDir(TFTPDir, data.ServerDirectory)
	if err := templates["tftp"].Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// HandleDownload handles file downloads
func (h *TFTPHandlers) HandleDownload(w http.ResponseWriter, r *http.Request) {
	fileName := r.URL.Query().Get("file")
	if fileName == "" {
		http.Error(w, "File parameter is required", http.StatusBadRequest)
		return
	}

	filePath := filepath.Join(TFTPDir, fileName)
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "File not found", http.StatusNotFound)
		} else {
			http.Error(w, "Error opening file", http.StatusInternalServerError)
		}
		return
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		http.Error(w, "Error getting file info", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filepath.Base(fileName)))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", fileInfo.Size()))

	if _, err := io.Copy(w, file); err != nil {
		http.Error(w, "Error serving file", http.StatusInternalServerError)
	}
}

// ViewFile views file content
func (h *TFTPHandlers) ViewFile(w http.ResponseWriter, r *http.Request) {
	fileName := r.URL.Query().Get("file")
	if fileName == "" {
		http.Error(w, "File parameter is required", http.StatusBadRequest)
		return
	}

	filePath := filepath.Join(TFTPDir, fileName)
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "File not found", http.StatusNotFound)
		} else {
			http.Error(w, "Error opening file", http.StatusInternalServerError)
		}
		return
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		http.Error(w, "Error getting file info", http.StatusInternalServerError)
		return
	}

	if fileInfo.IsDir() {
		http.Error(w, "Cannot view directory", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", filepath.Base(fileName)))

	if _, err := io.Copy(w, file); err != nil {
		http.Error(w, "Error reading file", http.StatusInternalServerError)
	}
}

// ServeFile serves files
func (h *TFTPHandlers) ServeFile(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Serve file not implemented", http.StatusNotImplemented)
}

// HandleDelete handles file deletion
func (h *TFTPHandlers) HandleDelete(w http.ResponseWriter, r *http.Request) {
	fileName := r.URL.Query().Get("file")
	if fileName == "" {
		http.Error(w, "File parameter is required", http.StatusBadRequest)
		return
	}

	filePath := filepath.Join(TFTPDir, fileName)
	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "File not found", http.StatusNotFound)
		} else {
			http.Error(w, "Error deleting file", http.StatusInternalServerError)
		}
		return
	}

	SetNoCacheHeaders(w)
	dir := filepath.Dir(fileName)
	if dir == "." || dir == "/" {
		http.Redirect(w, r, "/tftp/open", http.StatusSeeOther)
	} else {
		http.Redirect(w, r, fmt.Sprintf("/tftp/open?dir=%s", dir), http.StatusSeeOther)
	}
}

// HandleUpload handles file uploads
func (h *TFTPHandlers) HandleUpload(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form with 32MB max memory
	err := r.ParseMultipartForm(32 << 20)
	if err != nil {
		http.Error(w, "Failed to parse upload form", http.StatusBadRequest)
		return
	}

	file, handler, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Failed to get uploaded file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Create the uploads directory if it doesn't exist
	tftpDir := h.container.Config.TFTP.Dir
	if tftpDir == "" {
		tftpDir = "./public/tftp"
	}
	
	if err := os.MkdirAll(tftpDir, 0755); err != nil {
		http.Error(w, "Failed to create upload directory", http.StatusInternalServerError)
		return
	}

	// Create the destination file
	dst, err := os.Create(filepath.Join(tftpDir, handler.Filename))
	if err != nil {
		http.Error(w, "Failed to create destination file", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	// Copy the uploaded file to the destination
	_, err = io.Copy(dst, file)
	if err != nil {
		http.Error(w, "Failed to save uploaded file", http.StatusInternalServerError)
		return
	}

	// Return success response
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("File uploaded successfully"))
}

// FileInfo represents file metadata for display
type FileInfo struct {
	Name         string
	Size         string
	LastModified string
	IsDir        bool
}

// TFTPData holds information for rendering the TFTP management page
type TFTPData struct {
	Title           string
	ServerRunning   bool
	ServerDirectory string
	PrevDirectory   string
	Files           []FileInfo
}

// getTFTP retrieves file information for the root TFTP directory.
func (h *TFTPHandlers) getTFTP() (*TFTPData, error) {
	return h.getFileInfo("./")
}

// getTFTPDir retrieves file information for a specified directory within the TFTP server.
func (h *TFTPHandlers) getTFTPDir(dir string) (*TFTPData, error) {
	return h.getFileInfo(dir)
}

// getFileInfo reads the directory and returns file information for display.
func (h *TFTPHandlers) getFileInfo(dir string) (*TFTPData, error) {
	entries, err := os.ReadDir(filepath.Join(TFTPDir, dir))
	if err != nil {
		return nil, err
	}

	var fileInfos []FileInfo
	for _, entry := range entries {
		fileInfo, err := entry.Info()
		if err != nil {
			continue
		}

		relativePath := entry.Name()
		if dir != "." && dir != "./" {
			subDirPath := strings.TrimPrefix(dir, TFTPDir)
			subDirPath = strings.TrimPrefix(subDirPath, string(filepath.Separator))
			relativePath = subDirPath + string(filepath.Separator) + entry.Name()
		}

		fileInfos = append(fileInfos, FileInfo{
			Name:         relativePath,
			Size:         humanReadableSize(fileInfo.Size()),
			LastModified: fileInfo.ModTime().Format("2006-01-02 15:04:05"),
			IsDir:        fileInfo.IsDir(),
		})
	}

	currentDir := strings.TrimPrefix(strings.TrimPrefix(dir, TFTPDir), string(filepath.Separator))

	return &TFTPData{
		Title:           "TFTP Server Management",
		ServerRunning:   true,
		ServerDirectory: currentDir,
		PrevDirectory:   removeLastDir(TFTPDir, dir),
		Files:           fileInfos,
	}, nil
}

// humanReadableSize converts bytes to a human-readable format.
func humanReadableSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// removeLastDir removes the last directory from a path.
func removeLastDir(base, path string) string {
	if !strings.HasPrefix(path, base) {
		return ""
	}

	relPath := strings.TrimPrefix(path, base)
	if relPath == "" || relPath == string(filepath.Separator) {
		return ""
	}

	parts := strings.Split(strings.Trim(relPath, string(filepath.Separator)), string(filepath.Separator))
	if len(parts) <= 1 {
		return ""
	}

	return strings.Join(parts[:len(parts)-1], string(filepath.Separator))
}
