package handlers

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// maxUploadBytes caps TFTP upload request bodies (512 MB).
const maxUploadBytes = 512 << 20

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
	var data *TFTPData
	var err error

	if dir := r.URL.Query().Get("dir"); dir != "" {
		data, err = h.getTFTPDir(dir)
	} else {
		data, err = h.getTFTP()
	}

	if err != nil {
		HandleError(w, r, NewInternalError(
			fmt.Sprintf("failed to list TFTP directory: %v", err),
			"Unable to list the TFTP directory",
		))
		return
	}

	renderCachedTemplate(w, r, "tftp", data, "Unable to render the TFTP page")
}

// HandleDownload handles file downloads
func (h *TFTPHandlers) HandleDownload(w http.ResponseWriter, r *http.Request) {
	fileName := r.URL.Query().Get("file")
	if fileName == "" {
		HandleError(w, r, NewValidationError("Missing file parameter", "File parameter is required"))
		return
	}

	// Join-then-check: resolve the user-supplied name inside the TFTP root
	// and reject anything that escapes it.
	filePath, err := safeJoin(TFTPDir, fileName)
	if err != nil {
		HandleError(w, r, NewValidationError(
			fmt.Sprintf("Invalid file path: %v", err),
			"The requested file path is not allowed",
		))
		return
	}

	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			HandleError(w, r, NewNotFoundError(
				fmt.Sprintf("File not found: %s", fileName),
				"The requested file does not exist",
			))
		} else {
			HandleError(w, r, NewInternalError(
				fmt.Sprintf("Error opening file %s: %v", fileName, err),
				"Unable to open the requested file",
			))
		}
		return
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		HandleError(w, r, NewInternalError("Error getting file info", "Unable to read the requested file"))
		return
	}
	if fileInfo.IsDir() {
		HandleError(w, r, NewValidationError("Path is a directory", "The requested path is a directory, not a file"))
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", sanitizeDispositionFilename(fileName)))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", fileInfo.Size()))

	if _, err := io.Copy(w, file); err != nil {
		// Response already started; log only.
		log.Printf("Error serving file %s: %v", fileName, err)
	}
}

// ViewFile views file content
func (h *TFTPHandlers) ViewFile(w http.ResponseWriter, r *http.Request) {
	fileName := r.URL.Query().Get("file")
	if fileName == "" {
		HandleError(w, r, NewValidationError("Missing file parameter", "File parameter is required"))
		return
	}

	// Join-then-check: keep the resolved path inside the TFTP root.
	filePath, err := safeJoin(TFTPDir, fileName)
	if err != nil {
		HandleError(w, r, NewValidationError(
			fmt.Sprintf("Invalid file path: %v", err),
			"The requested file path is not allowed",
		))
		return
	}

	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			HandleError(w, r, NewNotFoundError(
				fmt.Sprintf("File not found: %s", fileName),
				"The requested file does not exist",
			))
		} else {
			HandleError(w, r, NewInternalError(
				fmt.Sprintf("Error opening file %s: %v", fileName, err),
				"Unable to open the requested file",
			))
		}
		return
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		HandleError(w, r, NewInternalError("Error getting file info", "Unable to read the requested file"))
		return
	}

	if fileInfo.IsDir() {
		HandleError(w, r, NewValidationError("Cannot view directory", "Directories cannot be previewed"))
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", sanitizeDispositionFilename(fileName)))

	if _, err := io.Copy(w, file); err != nil {
		log.Printf("Error streaming file %s: %v", fileName, err)
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
		HandleError(w, r, NewValidationError("Missing file parameter", "File parameter is required"))
		return
	}

	// Join-then-check: the deleted path must stay inside the TFTP root.
	filePath, err := safeJoin(TFTPDir, fileName)
	if err != nil {
		HandleError(w, r, NewValidationError(
			fmt.Sprintf("Invalid file path: %v", err),
			"The requested file path is not allowed",
		))
		return
	}

	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			HandleError(w, r, NewNotFoundError(
				fmt.Sprintf("File not found: %s", fileName),
				"The requested file does not exist",
			))
		} else {
			HandleError(w, r, NewInternalError(
				fmt.Sprintf("Error deleting file %s: %v", fileName, err),
				"Unable to delete the requested file",
			))
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
	if !requireConfig(w, r, h.container) {
		return
	}

	// Hard cap on the upload body before parsing the multipart form.
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)

	// Parse multipart form with 32MB max memory
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		HandleError(w, r, NewValidationError(
			fmt.Sprintf("Failed to parse upload form: %v", err),
			"The upload was rejected (too large or malformed)",
		))
		return
	}

	file, handler, err := r.FormFile("file")
	if err != nil {
		HandleError(w, r, NewValidationError("Missing file in upload", "No file was included in the upload"))
		return
	}
	defer file.Close()

	// Validate the upload against the TFTP security policy.
	validator := NewTFTPSecurityValidator(TFTPDir)
	if err := validator.ValidateTFTPUpload(handler.Filename, handler.Size); err != nil {
		HandleError(w, r, NewValidationError(
			fmt.Sprintf("Upload rejected: %v", err),
			"The uploaded file is not allowed",
		))
		return
	}

	// Create the uploads directory if it doesn't exist
	tftpDir := h.container.Config.TFTP.Dir
	if tftpDir == "" {
		tftpDir = "./public/tftp"
	}

	if err := os.MkdirAll(tftpDir, 0755); err != nil {
		HandleError(w, r, NewInternalError(
			fmt.Sprintf("Failed to create upload directory: %v", err),
			"Unable to prepare the upload directory",
		))
		return
	}

	// Resolve the destination inside the TFTP root (join-then-check).
	dstPath, err := safeJoin(tftpDir, handler.Filename)
	if err != nil {
		HandleError(w, r, NewValidationError(
			fmt.Sprintf("Invalid upload filename: %v", err),
			"The uploaded file name is not allowed",
		))
		return
	}

	// Create the destination file
	dst, err := os.Create(dstPath)
	if err != nil {
		HandleError(w, r, NewInternalError(
			fmt.Sprintf("Failed to create destination file: %v", err),
			"Unable to save the uploaded file",
		))
		return
	}
	defer dst.Close()

	// Copy the uploaded file to the destination
	if _, err := io.Copy(dst, file); err != nil {
		HandleError(w, r, NewInternalError(
			fmt.Sprintf("Failed to save uploaded file: %v", err),
			"Unable to save the uploaded file",
		))
		return
	}

	// Return success response
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("File uploaded successfully"))
}

// FileInfo represents file metadata for display.
// Name is the root-relative path used by open, download, and delete.
// Base is the entry name shown in the table. ModTime is the filesystem
// time; the page formats it in the browser's local zone.
type FileInfo struct {
	Name           string
	Base           string
	Size           string
	ModTime        time.Time
	LastModified   string
	LastModifiedAt string
	IsDir          bool
}

// TFTPCrumb is one step of the directory path under the TFTP root.
type TFTPCrumb struct {
	Name string
	Dir  string
}

// TFTPData holds information for rendering the TFTP management page.
// Mode is the server UI preference. Empty renders as cozy.
type TFTPData struct {
	Title           string
	Mode            string
	Page            string
	CSRFToken       string
	OverallStatus   string
	ServerRunning   bool
	ServerStatus    string
	ServerDirectory string
	PrevDirectory   string
	Root            string
	Breadcrumbs     []TFTPCrumb
	Files           []FileInfo
	FileCount       int
	DirCount        int
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
// The requested directory is resolved inside the TFTP root (join-then-check).
func (h *TFTPHandlers) getFileInfo(dir string) (*TFTPData, error) {
	fullDir, err := safeJoin(TFTPDir, dir)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(fullDir)
	if err != nil {
		return nil, err
	}

	// Path of the listed directory relative to the TFTP root ("" for root).
	relDir, err := filepath.Rel(mustAbs(TFTPDir), fullDir)
	if err != nil {
		return nil, err
	}
	if relDir == "." {
		relDir = ""
	}

	var fileInfos []FileInfo
	for _, entry := range entries {
		fileInfo, err := entry.Info()
		if err != nil {
			continue
		}

		relativePath := entry.Name()
		if relDir != "" {
			relativePath = relDir + string(filepath.Separator) + entry.Name()
		}

		fileInfos = append(fileInfos, FileInfo{
			Name:           relativePath,
			Base:           entry.Name(),
			Size:           humanReadableSize(fileInfo.Size()),
			ModTime:        fileInfo.ModTime(),
			LastModified:   fileInfo.ModTime().Format("2006-01-02 15:04:05"),
			LastModifiedAt: formatRFC3339(fileInfo.ModTime()),
			IsDir:          fileInfo.IsDir(),
		})
	}
	orderTFTPFiles(fileInfos)

	fileCount, dirCount := 0, 0
	for _, info := range fileInfos {
		if info.IsDir {
			dirCount++
		} else {
			fileCount++
		}
	}

	return &TFTPData{
		Title:           "TFTP",
		Page:            "tftp",
		ServerRunning:   isTFTPRunning(),
		ServerStatus:    TFTPStatus(),
		ServerDirectory: relDir,
		PrevDirectory:   removeLastDir(relDir),
		Root:            tftpPageRoot(),
		Files:           fileInfos,
		FileCount:       fileCount,
		DirCount:        dirCount,
		Breadcrumbs:     tftpCrumbs(relDir),
	}, nil
}

// orderTFTPFiles lists directories before files, then by display name.
func orderTFTPFiles(files []FileInfo) {
	sort.SliceStable(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		return strings.ToLower(tftpDisplayName(files[i])) < strings.ToLower(tftpDisplayName(files[j]))
	})
}

func tftpDisplayName(f FileInfo) string {
	if f.Base != "" {
		return f.Base
	}
	return f.Name
}

// tftpCrumbs splits a root-relative directory into linked steps.
func tftpCrumbs(rel string) []TFTPCrumb {
	rel = strings.Trim(rel, `/\`)
	if rel == "" || rel == "." {
		return nil
	}
	parts := strings.FieldsFunc(rel, func(r rune) bool {
		return r == '/' || r == '\\'
	})
	crumbs := make([]TFTPCrumb, 0, len(parts))
	acc := ""
	for _, part := range parts {
		if acc == "" {
			acc = part
		} else {
			acc = acc + "/" + part
		}
		crumbs = append(crumbs, TFTPCrumb{Name: part, Dir: acc})
	}
	return crumbs
}

// tftpPageRoot is the path the TFTP page prints. File operations keep using
// the absolute TFTPDir. The value is the configured TFTP_DIR when one was set.
func tftpPageRoot() string {
	if TFTPDisplayRoot != "" {
		return TFTPDisplayRoot
	}
	return TFTPDir
}

// mustAbs returns the absolute form of p, falling back to p itself on error.
func mustAbs(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
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

// removeLastDir removes the last directory from a base-relative path,
// e.g. "a/b" -> "a", "a" -> "". It accepts paths with or without the TFTP
// root prefix still attached.
func removeLastDir(path string) string {
	// Tolerate callers that pass a full path: strip the TFTP root first.
	path = strings.TrimPrefix(path, TFTPDir)
	path = strings.Trim(path, string(filepath.Separator))
	if path == "" || path == "." {
		return ""
	}

	parts := strings.Split(path, string(filepath.Separator))
	if len(parts) <= 1 {
		return ""
	}

	return strings.Join(parts[:len(parts)-1], string(filepath.Separator))
}
