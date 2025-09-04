package handlers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"ignite/internal/errors"
	"ignite/internal/validation"
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
func (h *Handlers) HandleTFTPPage(w http.ResponseWriter, r *http.Request) {
	templates := LoadTemplates()
	var data *TFTPData
	var err error

	baseDir := h.GetTFTPDir()

	if dir := r.URL.Query().Get("dir"); dir != "" {
		data, err = h.getTFTPDir(dir)
	} else {
		data, err = h.getTFTP()
	}

	if err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewFileSystemError("get_tftp_data", err))
		return
	}

	data.ServerDirectory = strings.TrimPrefix(strings.TrimPrefix(data.ServerDirectory, baseDir), string(filepath.Separator))
	data.PrevDirectory = removeLastDir(baseDir, data.ServerDirectory)
	if err := templates["tftp"].Execute(w, data); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("execute_template", err))
	}
}

// HandleDelete removes a specified file from the TFTP server directory.
func (h *Handlers) HandleDelete(w http.ResponseWriter, r *http.Request) {
	fileName := r.URL.Query().Get("file")
	if err := validation.ValidateRequired("file", fileName); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("validate_filename", err))
		return
	}

	// Validate and secure the file path
	safePath, err := validation.ValidateFilePath(h.GetTFTPDir(), fileName)
	if err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("validate_path", err))
		return
	}

	if err := deleteFile(safePath); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewFileSystemError("delete_file", err))
		return
	}

	SetNoCacheHeaders(w)
	http.Redirect(w, r, fmt.Sprintf("/tftp/open?file=%s", fileName), http.StatusSeeOther)
}

// HandleDownload allows for downloading files from the TFTP server.
func (h *Handlers) HandleDownload(w http.ResponseWriter, r *http.Request) {
	fileName := r.URL.Query().Get("file")
	if err := validation.ValidateRequired("file", fileName); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("validate_filename", err))
		return
	}

	// Validate and secure the file path
	safePath, err := validation.ValidateFilePath(h.GetTFTPDir(), fileName)
	if err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("validate_path", err))
		return
	}

	file, err := os.Open(safePath)
	if err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewFileSystemError("open_file", err))
		return
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewFileSystemError("stat_file", err))
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filepath.Base(fileName)))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", fileInfo.Size()))

	if _, err := io.Copy(w, file); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewFileSystemError("serve_file", err))
	}
}

// HandleUpload processes file uploads to the TFTP server directory.
func (h *Handlers) HandleUpload(w http.ResponseWriter, r *http.Request) {
	file, handler, err := r.FormFile("file")
	if err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("retrieve_file", err))
		return
	}
	defer file.Close()

	dir := r.URL.Query().Get("dir")
	if err := validation.ValidateRequired("dir", dir); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("validate_dir_param", err))
		return
	}

	// Validate filename
	filename := filepath.Base(handler.Filename)
	if err := validation.ValidateFilename(filename); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("validate_filename", err))
		return
	}

	// Validate directory path
	baseDir := h.GetTFTPDir()
	safeDir, err := validation.ValidateFilePath(baseDir, dir)
	if err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("validate_dir_path", err))
		return
	}

	// Create the safe file path
	filePath := filepath.Join(safeDir, filename)

	// Ensure the file is still within the safe directory after joining
	if !strings.HasPrefix(filePath, baseDir) {
		err := fmt.Errorf("path outside allowed directory")
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("validate_final_path", err))
		return
	}

	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewFileSystemError("create_directory", err))
		return
	}

	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewFileSystemError("create_file", err))
		return
	}
	defer f.Close()

	if _, err := io.Copy(f, file); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewFileSystemError("copy_file", err))
		return
	}

	path := fmt.Sprintf("/tftp/open?dir=%s", dir)
	SetNoCacheHeaders(w)
	http.Redirect(w, r, path, http.StatusSeeOther)
}

// getTFTP retrieves file information for the root TFTP directory.
func (h *Handlers) getTFTP() (*TFTPData, error) {
	return h.getFileInfo("./")
}

// getTFTPDir retrieves file information for a specified directory within the TFTP server.
func (h *Handlers) getTFTPDir(dir string) (*TFTPData, error) {
	return h.getFileInfo(dir)
}

// getFileInfo reads the directory and returns file information for display.
func (h *Handlers) getFileInfo(dir string) (*TFTPData, error) {
	baseDir := h.GetTFTPDir()
	entries, err := os.ReadDir(filepath.Join(baseDir, dir))
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
			subDirPath := strings.TrimPrefix(dir, baseDir)
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

	currentDir := strings.TrimPrefix(strings.TrimPrefix(dir, baseDir), string(filepath.Separator))

	return &TFTPData{
		Title:           "TFTP Server Management",
		ServerRunning:   true,
		ServerDirectory: currentDir,
		PrevDirectory:   removeLastDir(baseDir, dir),
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
func (h *Handlers) ViewFile(w http.ResponseWriter, r *http.Request) {
	fileName := r.URL.Query().Get("file")
	if err := validation.ValidateRequired("file", fileName); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("validate_filename", err))
		return
	}

	filePath := filepath.Join(h.GetTFTPDir(), fileName)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		errors.HandleHTTPError(w, h.Logger, errors.NewFileSystemError("stat_file", err))
		return
	}

	SetNoCacheHeaders(w)
	http.Redirect(w, r, fmt.Sprintf("/tftp/serve?file=%s", fileName), http.StatusSeeOther)
}

// ServeFile streams the content of a file to the HTTP response.
func (h *Handlers) ServeFile(w http.ResponseWriter, r *http.Request) {
	fileName := r.URL.Query().Get("file")
	if err := validation.ValidateRequired("file", fileName); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("validate_filename", err))
		return
	}

	filePath := filepath.Join(h.GetTFTPDir(), fileName)
	fileInfo, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		errors.HandleHTTPError(w, h.Logger, errors.NewFileSystemError("stat_file", err))
		return
	}
	if fileInfo.IsDir() {
		err := fmt.Errorf("requested path is a directory")
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("path_is_dir", err))
		return
	}

	file, err := os.Open(filePath)
	if err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewFileSystemError("open_file", err))
		return
	}
	defer file.Close()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	content, err := io.ReadAll(file)
	if err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewFileSystemError("read_file", err))
		return
	}

	_, err = w.Write(content)
	if err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewFileSystemError("write_response", err))
	}
}
