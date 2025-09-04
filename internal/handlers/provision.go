package handlers

import (
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"ignite/internal/errors"
	"ignite/internal/validation"
)

var Filename string

// HomeHandler serves the main page for the provisioning section.
func (h *Handlers) HomeHandler(w http.ResponseWriter, r *http.Request) {
	templates := LoadTemplates()

	w.Header().Set("Content-Type", "text/html")
	if err := templates["provision"].Execute(w, nil); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("execute_template", err))
	}
}

// HandleFileOptions renders a select dropdown with file options.
func (h *Handlers) HandleFileOptions(w http.ResponseWriter, r *http.Request) {
	Type := r.FormValue("typeSelect")

	if err := validation.ValidateRequired("typeSelect", Type); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("validate_type", err))
		return
	}

	files := h.listFiles("templates", Type, h.GetProvisionDir())

	tmpl := template.Must(template.New("select").Parse(`
	{{range .}}
	<option value="{{.}}">{{.}}</option>
	{{end}}
	`))

	if err := tmpl.Execute(w, files); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("execute_template", err))
	}
}

// HandleConfigOptions renders a select dropdown with config file options.
func (h *Handlers) HandleConfigOptions(w http.ResponseWriter, r *http.Request) {
	Type := r.FormValue("configTypeSelect")
	if err := validation.ValidateRequired("configTypeSelect", Type); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("validate_type", err))
		return
	}

	var files []string
	if Type == "bootmenu" {
		files = h.listFiles("pxelinux.cfg", "", h.GetTFTPDir())
	} else {
		files = h.listFiles("configs", Type, h.GetProvisionDir())
	}

	tmpl := template.Must(template.New("select").Parse(`
	{{range .}}
	<option value="{{.}}">{{.}}</option>
	{{end}}
	`))

	if err := tmpl.Execute(w, files); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("execute_template", err))
	}
}

// listFiles returns a slice of file names in the directories joined.
func (h *Handlers) listFiles(parentFolder, childFolder string, rootDir string) []string {
	osDir := filepath.Join(rootDir, parentFolder, childFolder)
	files, err := os.ReadDir(osDir)
	if err != nil {
		h.Logger.Warn("Error reading directory", "path", osDir, "error", err)
		return nil
	}

	var fileList []string
	for _, file := range files {
		if !file.IsDir() {
			fileList = append(fileList, file.Name())
		}
	}
	return fileList
}

// LoadTemplate loads a template file and returns its content.
func (h *Handlers) LoadTemplate(w http.ResponseWriter, r *http.Request) {
	templateType := r.FormValue("typeSelect")
	name := r.FormValue("templateSelect")

	if err := validation.ValidateRequired("typeSelect", templateType); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("validate_type", err))
		return
	}
	if err := validation.ValidateRequired("templateSelect", name); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("validate_name", err))
		return
	}

	filename := filepath.Join(h.GetProvisionDir(), "templates", templateType, name)

	// Validate file path
	safePath, err := validation.ValidateFilePath(h.GetProvisionDir(), filename)
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

	Filename = filename // for global reference

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")

	if _, err := io.Copy(w, file); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewFileSystemError("read_file", err))
	}
}

// LoadConfig loads a config file and returns its content.
func (h *Handlers) LoadConfig(w http.ResponseWriter, r *http.Request) {
	configType := r.FormValue("configTypeSelect")
	name := r.FormValue("configSelect")

	if err := validation.ValidateRequired("configTypeSelect", configType); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("validate_type", err))
		return
	}
	if err := validation.ValidateRequired("configSelect", name); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("validate_name", err))
		return
	}

	var filename string
	var baseDir string
	if configType == "bootmenu" {
		baseDir = h.GetTFTPDir()
		filename = filepath.Join(baseDir, "pxelinux.cfg", name)
	} else {
		baseDir = h.GetProvisionDir()
		filename = filepath.Join(baseDir, "configs", configType, name)
	}

	safePath, err := validation.ValidateFilePath(baseDir, filename)
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

	Filename = filename // for global reference

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if _, err := io.Copy(w, file); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewFileSystemError("read_file", err))
	}
}

// UpdateFilename returns the global variable Filename.
func (h *Handlers) UpdateFilename(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(fmt.Sprintf("Filename: %v", Filename)))
}

// HandleNewTemplate writes a new file to the templates folder.
func (h *Handlers) HandleNewTemplate(w http.ResponseWriter, r *http.Request) {
	Type := r.FormValue("saveTypeSelect")
	name := r.FormValue("filenameInput")

	if err := validation.ValidateRequired("saveTypeSelect", Type); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("validate_type", err))
		return
	}
	if err := validation.ValidateRequired("filenameInput", name); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("validate_name", err))
		return
	}

	filePath := filepath.Join(h.GetProvisionDir(), "templates", Type, name)
	safePath, err := validation.ValidateFilePath(h.GetProvisionDir(), filePath)
	if err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("validate_path", err))
		return
	}

	if _, err := os.Stat(safePath); err == nil {
		err := fmt.Errorf("file already exists: %s", name)
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("file_exists", err))
		return
	}

	file, err := os.Create(safePath)
	if err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewFileSystemError("create_file", err))
		return
	}
	defer file.Close()

	Filename = filePath

	w.WriteHeader(http.StatusCreated)
}

// HandleSave saves content to the currently active file.
func (h *Handlers) HandleSave(w http.ResponseWriter, r *http.Request) {
	if Filename == "" {
		err := fmt.Errorf("no file is currently being edited")
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("no_file_active", err))
		return
	}

	content := r.FormValue("codeContent")
	if err := validation.ValidateRequired("codeContent", content); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("validate_content", err))
		return
	}

	cleanPath := filepath.Clean(Filename)

	// Re-validate the path before writing
	// This is important because the Filename is a global variable.
	// A better solution would avoid this global state.
	// For now, we validate against both potential base directories.
	provDir := h.GetProvisionDir()
	tftpDir := h.GetTFTPDir()

	isSafeInProv, _ := validation.ValidateFilePath(provDir, cleanPath)
	isSafeInTftp, _ := validation.ValidateFilePath(tftpDir, cleanPath)

	if isSafeInProv != cleanPath && isSafeInTftp != cleanPath {
		err := fmt.Errorf("invalid file path for saving: %s", cleanPath)
		errors.HandleHTTPError(w, h.Logger, errors.NewValidationError("validate_save_path", err))
		return
	}

	if err := os.WriteFile(cleanPath, []byte(content), 0644); err != nil {
		errors.HandleHTTPError(w, h.Logger, errors.NewFileSystemError("write_file", err))
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "File saved successfully!")
}
