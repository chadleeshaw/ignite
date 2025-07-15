package handlers

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

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

// HandleTFTPPage serves the TFTP management page, displaying files in the specified directory.
func HandleTFTPPage(w http.ResponseWriter, r *http.Request) {
	templates := LoadTemplates()
	var data *TFTPData
	var err error

	if dir := r.URL.Query().Get("dir"); dir != "" {
		data, err = getTFTPDir(dir)
	} else {
		data, err = getTFTP()
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

// HandleDelete removes a specified file from the TFTP server directory.
func HandleDelete(w http.ResponseWriter, r *http.Request) {
	fileName := r.URL.Query().Get("file")
	if err := deleteFile(filepath.Join(TFTPDir, fileName)); err != nil {
		log.Printf("error deleting file: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	SetNoCacheHeaders(w)
	http.Redirect(w, r, fmt.Sprintf("/tftp/open?file=%s", fileName), http.StatusSeeOther)
}

// HandleDownload allows for downloading files from the TFTP server.
func HandleDownload(w http.ResponseWriter, r *http.Request) {
	fileName := r.URL.Query().Get("file")
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
	w.Header().Set("Content-Type", "application/octet-stream") // or specific MIME type if known
	w.Header().Set("Content-Length", fmt.Sprintf("%d", fileInfo.Size()))

	if _, err := io.Copy(w, file); err != nil {
		http.Error(w, "Error serving file", http.StatusInternalServerError)
	}
}

// HandleUpload processes file uploads to the TFTP server directory.
func HandleUpload(w http.ResponseWriter, r *http.Request) {
	file, handler, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Error retrieving file", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	dir := r.URL.Query().Get("dir")
	if dir == "" {
		http.Error(w, "Directory parameter is required", http.StatusBadRequest)
		return
	}

	filePath := filepath.Join(TFTPDir, dir, filepath.Base(handler.Filename))
	if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
		http.Error(w, "Error creating directory", http.StatusInternalServerError)
		return
	}

	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		http.Error(w, "Error saving file", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	if _, err := io.Copy(f, file); err != nil {
		http.Error(w, "Error copying file", http.StatusInternalServerError)
		return
	}

	path := fmt.Sprintf("/tftp/open?dir=%s", dir)
	SetNoCacheHeaders(w)
	http.Redirect(w, r, path, http.StatusSeeOther)
}

// getTFTP retrieves file information for the root TFTP directory.
func getTFTP() (*TFTPData, error) {
	return getFileInfo("./")
}

// getTFTPDir retrieves file information for a specified directory within the TFTP server.
func getTFTPDir(dir string) (*TFTPData, error) {
	return getFileInfo(dir)
}

// getFileInfo reads the directory and returns file information for display.
func getFileInfo(dir string) (*TFTPData, error) {
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

// deleteFile removes the specified file from the filesystem.
func deleteFile(filePath string) error {
	return os.Remove(filePath)
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

// ViewFile redirects to serve a file for viewing.
func ViewFile(w http.ResponseWriter, r *http.Request) {
	fileName := r.URL.Query().Get("file")
	if fileName == "" {
		http.Error(w, "File parameter is missing", http.StatusBadRequest)
		return
	}

	filePath := filepath.Join(TFTPDir, fileName)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	SetNoCacheHeaders(w)
	http.Redirect(w, r, fmt.Sprintf("/tftp/serve?file=%s", fileName), http.StatusSeeOther)
}

// ServeFile streams the content of a file to the HTTP response.
func ServeFile(w http.ResponseWriter, r *http.Request) {
	fileName := r.URL.Query().Get("file")
	if fileName == "" {
		http.Error(w, "File parameter is missing", http.StatusBadRequest)
		return
	}

	filePath := filepath.Join(TFTPDir, fileName)
	fileInfo, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	if fileInfo.IsDir() {
		http.Error(w, "Requested path is a directory", http.StatusBadRequest)
		return
	}

	file, err := os.Open(filePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error opening file: %v", err), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	content, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error reading file: %v", err), http.StatusInternalServerError)
		return
	}

	_, err = w.Write(content)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error writing content to response: %v", err), http.StatusInternalServerError)
	}
}

// Generate map for upload modal containing current directory
func NewUploadModal(w http.ResponseWriter, r *http.Request) map[string]any {
	dir := "./"

	if queryDir := r.URL.Query().Get("dir"); queryDir != "" {
		dir = queryDir
	}

	currentDir := strings.TrimPrefix(strings.TrimPrefix(dir, TFTPDir), string(filepath.Separator))
	return map[string]any{
		"Directory": currentDir,
	}
}
