package handlers

import (
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"ignite/config"
	"ignite/internal/errors"
	"ignite/internal/validation"
)

var ProvDir = config.Defaults.Provision.Dir
var Filename string

// Load page for provision
func HomeHandler(w http.ResponseWriter, r *http.Request) {
	templates := LoadTemplates()

	w.Header().Set("Content-Type", "text/html")
	if err := templates["provision"].Execute(w, nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Save content to a file on the os
func SaveHandler(w http.ResponseWriter, r *http.Request) {
	content := r.FormValue("content")
	filename := r.FormValue("filename")
	scriptType := r.FormValue("type")

	// Validate required fields
	if err := validation.ValidateRequired("filename", filename); err != nil {
		errors.HandleHTTPError(w, slog.Default(), errors.NewValidationError("validate_filename", err))
		return
	}
	if err := validation.ValidateRequired("type", scriptType); err != nil {
		errors.HandleHTTPError(w, slog.Default(), errors.NewValidationError("validate_type", err))
		return
	}
	if err := validation.ValidateRequired("content", content); err != nil {
		errors.HandleHTTPError(w, slog.Default(), errors.NewValidationError("validate_content", err))
		return
	}

	// Validate filename for safety
	if err := validation.ValidateFilename(filename); err != nil {
		errors.HandleHTTPError(w, slog.Default(), errors.NewValidationError("validate_filename", err))
		return
	}

	// Validate and secure the file path
	relativePath := filepath.Join(scriptType, filename)
	safePath, err := validation.ValidateFilePath(ProvDir, relativePath)
	if err != nil {
		errors.HandleHTTPError(w, slog.Default(), errors.NewValidationError("validate_path", err))
		return
	}

	// Ensure directory exists
	dir := filepath.Dir(safePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		errors.HandleHTTPError(w, slog.Default(), errors.NewFileSystemError("create_directory", err))
		return
	}

	if err := os.WriteFile(safePath, []byte(content), 0644); err != nil {
		errors.HandleHTTPError(w, slog.Default(), errors.NewFileSystemError("write_file", err))
		return
	}

	fmt.Fprintf(w, "File saved successfully")
}

// Load a rendered template into response writer
func LoadFile(w http.ResponseWriter, r *http.Request) {
	filename := r.FormValue("filename")
	scriptType := r.FormValue("type")

	// Validate required fields
	if err := validation.ValidateRequired("filename", filename); err != nil {
		errors.HandleHTTPError(w, slog.Default(), errors.NewValidationError("validate_filename", err))
		return
	}
	if err := validation.ValidateRequired("type", scriptType); err != nil {
		errors.HandleHTTPError(w, slog.Default(), errors.NewValidationError("validate_type", err))
		return
	}

	// Validate filename for safety
	if err := validation.ValidateFilename(filename); err != nil {
		errors.HandleHTTPError(w, slog.Default(), errors.NewValidationError("validate_filename", err))
		return
	}

	Filename = filename // for global reference

	// Validate and secure the file path
	relativePath := filepath.Join(scriptType, filename)
	safePath, err := validation.ValidateFilePath(ProvDir, relativePath)
	if err != nil {
		errors.HandleHTTPError(w, slog.Default(), errors.NewValidationError("validate_path", err))
		return
	}

	fileInfo, err := os.Stat(safePath)
	if os.IsNotExist(err) {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	if fileInfo.IsDir() {
		http.Error(w, "Requested path is a directory", http.StatusBadRequest)
		return
	}

	file, err := os.Open(safePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error opening file: %v", err), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")

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

func HandleFileOptions(w http.ResponseWriter, r *http.Request) {
	Type := r.FormValue("typeSelect")

	if Type == "" {
		http.Error(w, "Field is missing", http.StatusBadRequest)
		return
	}

	files := ListFiles("templates", Type)

	tmpl := template.Must(template.New("select").Parse(`
	{{range .}}
	<option value="{{.}}">{{.}}</option>
	{{end}}
	`))

	err := tmpl.Execute(w, files)
	if err != nil {
		http.Error(w, "Error rendering template", http.StatusInternalServerError)
		return
	}
}

func HandleConfigOptions(w http.ResponseWriter, r *http.Request) {
	Type := r.FormValue("configTypeSelect")
	var files []string

	if Type == "" {
		http.Error(w, "Field is missing", http.StatusBadRequest)
		return
	}

	if Type == "bootmenu" {
		files = ListFiles("pxelinux.cfg", "", config.Defaults.TFTP.Dir)
	} else {
		files = ListFiles("configs", Type)
	}

	tmpl := template.Must(template.New("select").Parse(`
	{{range .}}
	<option value="{{.}}">{{.}}</option>
	{{end}}
	`))

	err := tmpl.Execute(w, files)
	if err != nil {
		http.Error(w, "Error rendering template", http.StatusInternalServerError)
		return
	}
}

// listFiles returns a slice of file names in the directories joined
func ListFiles(parentFolder, childFolder string, rootDir ...string) []string {
	dir := ProvDir
	if len(rootDir) > 0 {
		dir = rootDir[0]
	}

	osDir := filepath.Join(dir, parentFolder, childFolder)
	files, err := os.ReadDir(osDir)
	if err != nil {
		fmt.Println("Error reading directory:", err)
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

// Load kickstart/cloud-init/bootmenu template and return file to response writer
func LoadTemplate(w http.ResponseWriter, r *http.Request) {
	templateType := r.FormValue("typeSelect")
	name := r.FormValue("templateSelect")

	if templateType == "" || name == "" {
		http.Error(w, "Field is missing", http.StatusBadRequest)
		return
	}

	filename := filepath.Join(ProvDir, "templates", templateType, name)

	_, err := os.Stat(filename)
	if os.IsNotExist(err) {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	file, err := os.Open(filename)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error opening file: %v", err), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	Filename = filename // for global reference

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")

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

// Load kickstart/cloud-init/bootmenu rendered config and return file to response writer
func LoadConfig(w http.ResponseWriter, r *http.Request) {
	configType := r.FormValue("configTypeSelect")
	name := r.FormValue("configSelect")

	if configType == "" || name == "" {
		http.Error(w, "Field is missing", http.StatusBadRequest)
		return
	}

	filename := filepath.Join(ProvDir, "configs", configType, name)
	if configType == "bootmenu" {
		filename = filepath.Join(config.Defaults.TFTP.Dir, "pxelinux.cfg", name)
	}

	_, err := os.Stat(filename)
	if os.IsNotExist(err) {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	file, err := os.Open(filename)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error opening file: %v", err), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	Filename = filename // for global reference

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

// UpdateFilename return global variable Filename
func UpdateFilename(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(fmt.Sprintf("Filename: %v", Filename)))
}

// HandleNewTemplate writes a new file to templates folder
func HandleNewTemplate(w http.ResponseWriter, r *http.Request) {
	Type := r.FormValue("saveTypeSelect")
	name := r.FormValue("filenameInput")

	if Type == "" || name == "" {
		http.Error(w, "Field is missing", http.StatusBadRequest)
		return
	}

	filePath := filepath.Join(ProvDir, "templates", Type, name)

	if _, err := os.Stat(filePath); err == nil {
		http.Error(w, "File already exists", http.StatusConflict)
		return
	}

	file, err := os.Create(filePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error creating file: %v", err), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	Filename = filePath

	_, err = file.WriteString("")
	if err != nil {
		http.Error(w, fmt.Sprintf("Error writing to file: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func HandleSave(w http.ResponseWriter, r *http.Request) {
	if Filename == "" {
		http.Error(w, "Error saving file, filename not set", http.StatusBadRequest)
		return
	}

	content := r.FormValue("codeContent")
	if content == "" {
		http.Error(w, "Error saving file, no content", http.StatusBadRequest)
		return
	}

	cleanPath := filepath.Clean(Filename)

	file, err := os.Create(cleanPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error creating file: %v", err), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	_, err = file.WriteString(content)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error writing to file: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "File saved successfully!")
}
