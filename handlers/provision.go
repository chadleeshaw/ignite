package handlers

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ProvisionHandlers handles provisioning-related requests
type ProvisionHandlers struct {
	container *Container
}

// FileInfo represents file metadata for the provision system
type ProvisionFileInfo struct {
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	Size        int64     `json:"size"`
	ModTime     time.Time `json:"mod_time"`
	IsDir       bool      `json:"is_dir"`
	Type        string    `json:"type"`     // template, config, bootmenu
	Category    string    `json:"category"` // cloud-init, kickstart, etc.
	Language    string    `json:"language"` // yaml, ini, cfg
	Description string    `json:"description,omitempty"`
}

// ProvisionData holds data for the provision page
type ProvisionData struct {
	Title       string               `json:"title"`
	CurrentFile *ProvisionFileInfo   `json:"current_file,omitempty"`
	Files       []*ProvisionFileInfo `json:"files"`
	Categories  []string             `json:"categories"`
	Types       []string             `json:"types"`
}

// TemplateGallery represents pre-built templates
type TemplateGalleryItem struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Language    string   `json:"language"`
	Content     string   `json:"content"`
	Tags        []string `json:"tags"`
}

// NewProvisionHandlers creates a new ProvisionHandlers instance
func NewProvisionHandlers(container *Container) *ProvisionHandlers {
	return &ProvisionHandlers{container: container}
}

// HomeHandler serves the provision page
func (h *ProvisionHandlers) HomeHandler(w http.ResponseWriter, r *http.Request) {
	if !requireConfig(w, r, h.container) {
		return
	}
	data := h.getProvisionData()

	renderCachedTemplate(w, r, "provision", data, "Unable to render the provision page")
}

// getProvisionData collects all provision files and metadata
func (h *ProvisionHandlers) getProvisionData() *ProvisionData {
	provisionDir := h.container.Config.Provision.Dir

	data := &ProvisionData{
		Title:      "Provisioning Scripts",
		Files:      []*ProvisionFileInfo{},
		Categories: []string{"autoyast", "bootmenu", "cloud-init", "ipxe", "kickstart", "preseed"},
		Types:      []string{"templates", "configs"},
	}

	// Scan templates directory
	h.scanDirectory(provisionDir, filepath.Join(provisionDir, "templates"), "template", data)

	// Scan configs directory
	h.scanDirectory(provisionDir, filepath.Join(provisionDir, "configs"), "config", data)

	// Sort files by name
	sort.Slice(data.Files, func(i, j int) bool {
		return data.Files[i].Name < data.Files[j].Name
	})

	return data
}

// scanDirectory recursively scans a directory for provision files.
// root is the provision root; file paths are stored relative to it so
// absolute filesystem paths are never exposed.
func (h *ProvisionHandlers) scanDirectory(root, dir, fileType string, data *ProvisionData) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return
	}

	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Continue on error
		}

		// Skip root directory
		if path == dir {
			return nil
		}

		// Determine category from directory structure
		relPath, _ := filepath.Rel(dir, path)
		pathParts := strings.Split(relPath, string(os.PathSeparator))
		category := ""
		if len(pathParts) > 0 && !info.IsDir() {
			category = pathParts[0]
		}

		// Skip directories since we're showing all files flattened
		if info.IsDir() {
			return nil
		}

		// Expose the path relative to the provision root, never absolute.
		relToRoot, err := filepath.Rel(root, path)
		if err != nil {
			relToRoot = info.Name()
		}

		fileInfo := &ProvisionFileInfo{
			Name:     info.Name(),
			Path:     relToRoot,
			Size:     info.Size(),
			ModTime:  info.ModTime(),
			IsDir:    false, // We only include files now
			Type:     fileType,
			Category: category,
			Language: h.detectLanguage(path),
		}

		data.Files = append(data.Files, fileInfo)
		return nil
	})
}

// detectLanguage determines the syntax highlighting language from file extension or content
func (h *ProvisionHandlers) detectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".yaml", ".yml":
		return "yaml"
	case ".cfg", ".conf":
		return "ini"
	case ".ks":
		return "kickstart"
	case ".xml":
		return "xml"
	case ".ipxe":
		return "ipxe"
	default:
		// Check directory or filename for hints
		if strings.Contains(path, "cloud-init") {
			return "yaml"
		}
		if strings.Contains(path, "kickstart") {
			return "kickstart"
		}
		if strings.Contains(path, "preseed") {
			return "preseed"
		}
		if strings.Contains(path, "autoyast") {
			return "xml"
		}
		if strings.Contains(path, "ipxe") {
			return "ipxe"
		}
		if strings.Contains(path, "bootmenu") || strings.Contains(path, "pxe") {
			return "ini"
		}
		return "text"
	}
}

// renderOptions renders a list of <option> elements for the file and
// template pickers. The template is parsed per request with normal error
// handling instead of panicking.
func renderOptions(w http.ResponseWriter, r *http.Request, files []string) {
	tmpl, err := template.New("options").Parse(
		"{{range .}}<option value=\"{{.}}\">{{.}}</option>{{end}}",
	)
	if err != nil {
		HandleError(w, r, NewInternalError(
			fmt.Sprintf("Failed to parse options template: %v", err),
			"Unable to render options",
		))
		return
	}

	w.Header().Set("Content-Type", "text/html")
	if err := tmpl.Execute(w, files); err != nil {
		HandleError(w, r, NewInternalError(
			fmt.Sprintf("Failed to render options template: %v", err),
			"Unable to render options",
		))
	}
}

// HandleFileOptions handles file options - returns available files for a given type
func (h *ProvisionHandlers) HandleFileOptions(w http.ResponseWriter, r *http.Request) {
	if !requireConfig(w, r, h.container) {
		return
	}
	fileType := r.FormValue("typeSelect")
	if fileType == "" {
		HandleError(w, r, NewValidationError("Missing typeSelect", "typeSelect parameter is required"))
		return
	}

	provisionDir := h.container.Config.Provision.Dir
	// Resolve the requested type directory inside templates (join-then-check).
	typeDir, err := safeJoin(filepath.Join(provisionDir, "templates"), fileType)
	if err != nil {
		HandleError(w, r, NewValidationError(
			fmt.Sprintf("Invalid file type: %v", err),
			"The requested file type is not allowed",
		))
		return
	}
	files := h.listFiles(typeDir)

	renderOptions(w, r, files)
}

// LoadTemplate loads a template file content
func (h *ProvisionHandlers) LoadTemplate(w http.ResponseWriter, r *http.Request) {
	if !requireConfig(w, r, h.container) {
		return
	}
	templateType := r.FormValue("typeSelect")
	templateName := r.FormValue("templateSelect")

	if templateType == "" || templateName == "" {
		HandleError(w, r, NewValidationError("Missing parameters", "typeSelect and templateSelect parameters are required"))
		return
	}

	provisionDir := h.container.Config.Provision.Dir
	// Resolve inside the templates root (join-then-check).
	filePath, err := safeJoin(filepath.Join(provisionDir, "templates"), filepath.Join(templateType, templateName))
	if err != nil {
		HandleError(w, r, NewValidationError(
			fmt.Sprintf("Invalid template path: %v", err),
			"The requested template path is not allowed",
		))
		return
	}

	content, err := h.loadFileContent(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			HandleError(w, r, NewNotFoundError("Template not found", "The requested template does not exist"))
		} else {
			HandleError(w, r, NewInternalError(
				fmt.Sprintf("Error loading template: %v", err),
				"Unable to load the template",
			))
		}
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(content))
}

// HandleConfigOptions handles config options - returns available configs for a given type
func (h *ProvisionHandlers) HandleConfigOptions(w http.ResponseWriter, r *http.Request) {
	if !requireConfig(w, r, h.container) {
		return
	}
	configType := r.FormValue("configTypeSelect")
	if configType == "" {
		HandleError(w, r, NewValidationError("Missing configTypeSelect", "configTypeSelect parameter is required"))
		return
	}

	var files []string

	if configType == "bootmenu" {
		// Boot menu configs are in TFTP directory
		files = h.listFiles(filepath.Join(h.container.Config.TFTP.Dir, "pxelinux.cfg"))
	} else {
		// Other configs are in provision configs directory — resolve
		// the type directory inside configs (join-then-check).
		provisionDir := h.container.Config.Provision.Dir
		configDir, err := safeJoin(filepath.Join(provisionDir, "configs"), configType)
		if err != nil {
			HandleError(w, r, NewValidationError(
				fmt.Sprintf("Invalid config type: %v", err),
				"The requested config type is not allowed",
			))
			return
		}
		files = h.listFiles(configDir)
	}

	renderOptions(w, r, files)
}

// LoadConfig loads a config file content
func (h *ProvisionHandlers) LoadConfig(w http.ResponseWriter, r *http.Request) {
	if !requireConfig(w, r, h.container) {
		return
	}
	configType := r.FormValue("configTypeSelect")
	configName := r.FormValue("configSelect")

	if configType == "" || configName == "" {
		HandleError(w, r, NewValidationError("Missing parameters", "configTypeSelect and configSelect parameters are required"))
		return
	}

	var filePath string
	var err error

	if configType == "bootmenu" {
		filePath, err = safeJoin(filepath.Join(h.container.Config.TFTP.Dir, "pxelinux.cfg"), configName)
	} else {
		provisionDir := h.container.Config.Provision.Dir
		filePath, err = safeJoin(filepath.Join(provisionDir, "configs"), filepath.Join(configType, configName))
	}
	if err != nil {
		HandleError(w, r, NewValidationError(
			fmt.Sprintf("Invalid config path: %v", err),
			"The requested config path is not allowed",
		))
		return
	}

	content, err := h.loadFileContent(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			HandleError(w, r, NewNotFoundError("Config not found", "The requested config does not exist"))
		} else {
			HandleError(w, r, NewInternalError(
				fmt.Sprintf("Error loading config: %v", err),
				"Unable to load the config",
			))
		}
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(content))
}

// UpdateFilename returns current filename for display
func (h *ProvisionHandlers) UpdateFilename(w http.ResponseWriter, r *http.Request) {
	filename := r.URL.Query().Get("filename")
	if filename == "" {
		filename = "untitled"
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "Filename: %s", filename)
}

// HandleNewTemplate creates a new template file
func (h *ProvisionHandlers) HandleNewTemplate(w http.ResponseWriter, r *http.Request) {
	if !requireConfig(w, r, h.container) {
		return
	}
	templateType := r.FormValue("saveTypeSelect")
	filename := r.FormValue("filenameInput")

	if templateType == "" || filename == "" {
		HandleError(w, r, NewValidationError("Missing parameters", "saveTypeSelect and filenameInput parameters are required"))
		return
	}

	provisionDir := h.container.Config.Provision.Dir
	// Resolve the new file inside the templates root (join-then-check).
	filePath, err := safeJoin(filepath.Join(provisionDir, "templates"), filepath.Join(templateType, filename))
	if err != nil {
		HandleError(w, r, NewValidationError(
			fmt.Sprintf("Invalid template path: %v", err),
			"The requested template path is not allowed",
		))
		return
	}

	// Create directories if they don't exist
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		HandleError(w, r, NewInternalError(
			fmt.Sprintf("Error creating directory: %v", err),
			"Unable to create the template directory",
		))
		return
	}

	// Check if file already exists
	if _, err := os.Stat(filePath); err == nil {
		HandleError(w, r, NewAppError(ErrorTypeConflict, "File already exists", "A file with that name already exists", http.StatusConflict))
		return
	}

	// Create empty file
	file, err := os.Create(filePath)
	if err != nil {
		HandleError(w, r, NewInternalError(
			fmt.Sprintf("Error creating file: %v", err),
			"Unable to create the template file",
		))
		return
	}
	defer file.Close()

	// Quote the filename for safe interpolation into the JavaScript response.
	w.Header().Set("Content-Type", "application/javascript")
	fmt.Fprintf(w, `
		document.getElementById('currentFilename').textContent = %s;
		document.getElementById('editableTextarea').value = '';
		alert('Template created successfully!');
	`, strconv.Quote("Filename: "+filename))
}

// HandleSave saves file content
func (h *ProvisionHandlers) HandleSave(w http.ResponseWriter, r *http.Request) {
	if !requireConfig(w, r, h.container) {
		return
	}
	content := r.FormValue("codeContent")
	filename := r.FormValue("filename")
	fileType := r.FormValue("type")
	category := r.FormValue("category")

	if content == "" {
		HandleError(w, r, NewValidationError("Missing codeContent", "codeContent parameter is required"))
		return
	}

	if filename == "" || filename == "untitled" {
		HandleError(w, r, NewValidationError("Missing filename", "Please specify a filename before saving"))
		return
	}

	provisionDir := h.container.Config.Provision.Dir

	if fileType == "" {
		fileType = "templates"
	}
	if category == "" {
		category = "cloud-init" // default category
	}

	// Resolve the target inside the provision root (join-then-check).
	filePath, err := safeJoin(provisionDir, filepath.Join(fileType, category, filename))
	if err != nil {
		HandleError(w, r, NewValidationError(
			fmt.Sprintf("Invalid file path: %v", err),
			"The requested file path is not allowed",
		))
		return
	}

	// Create directories if they don't exist
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		HandleError(w, r, NewInternalError(
			fmt.Sprintf("Error creating directory: %v", err),
			"Unable to create the target directory",
		))
		return
	}

	// Write file
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		HandleError(w, r, NewInternalError(
			fmt.Sprintf("Error saving file: %v", err),
			"Unable to save the file",
		))
		return
	}

	w.Header().Set("Content-Type", "application/javascript")
	fmt.Fprintf(w, `alert("File saved successfully!");`)
}

// Helper functions

// listFiles returns a slice of filenames in a directory
func (h *ProvisionHandlers) listFiles(dir string) []string {
	files, err := os.ReadDir(dir)
	if err != nil {
		return []string{}
	}

	var fileList []string
	for _, file := range files {
		if !file.IsDir() {
			fileList = append(fileList, file.Name())
		}
	}

	sort.Strings(fileList)
	return fileList
}

// loadFileContent reads and returns file content
func (h *ProvisionHandlers) loadFileContent(filePath string) (string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

// isPathAllowed checks if a file path is within allowed directories
func (h *ProvisionHandlers) isPathAllowed(filePath string, allowedDirs ...string) bool {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return false
	}

	for _, allowedDir := range allowedDirs {
		absAllowedDir, err := filepath.Abs(allowedDir)
		if err != nil {
			continue
		}

		relPath, err := filepath.Rel(absAllowedDir, absPath)
		if err != nil {
			continue
		}

		// Check if the relative path doesn't start with ".." (which would indicate it's outside the allowed directory)
		if !strings.HasPrefix(relPath, "..") && !filepath.IsAbs(relPath) {
			return true
		}
	}

	return false
}

// Additional API endpoints for the new interface

// resolveProvisionPath resolves a user-supplied path to an absolute path
// inside the provision or TFTP roots. Relative paths are joined-then-checked
// against each root; absolute paths are honored only when already contained in
// an allowed root (legacy clients send absolute paths).
func (h *ProvisionHandlers) resolveProvisionPath(userPath string) (string, error) {
	if strings.TrimSpace(userPath) == "" {
		return "", fmt.Errorf("path parameter is required")
	}

	var bases []string
	if dir := h.container.Config.Provision.Dir; dir != "" {
		bases = append(bases, dir)
	}
	if dir := h.container.Config.TFTP.Dir; dir != "" {
		bases = append(bases, dir)
	}

	// Relative paths: join-then-check against each allowed root.
	if !filepath.IsAbs(userPath) {
		for _, base := range bases {
			if p, err := safeJoin(base, userPath); err == nil {
				return p, nil
			}
		}
		return "", fmt.Errorf("path is not within an allowed directory")
	}

	// Absolute paths: only honor when already inside an allowed root.
	if h.isPathAllowed(userPath, bases...) {
		return filepath.Clean(userPath), nil
	}
	return "", fmt.Errorf("path is not within an allowed directory")
}

// writeJSONError writes a generic JSON error without leaking internals.
func writeJSONError(w http.ResponseWriter, status int, userMessage string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": false,
		"error":   userMessage,
	})
}

// LoadFileContent loads file content via API
func (h *ProvisionHandlers) LoadFileContent(w http.ResponseWriter, r *http.Request) {
	if !requireConfig(w, r, h.container) {
		return
	}
	userPath := r.URL.Query().Get("path")
	if userPath == "" {
		HandleError(w, r, NewValidationError("Missing path", "path parameter is required"))
		return
	}

	filePath, err := h.resolveProvisionPath(userPath)
	if err != nil {
		HandleError(w, r, NewValidationError(
			fmt.Sprintf("Invalid path: %v", err),
			"The requested path is not allowed",
		))
		return
	}

	content, err := h.loadFileContent(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			HandleError(w, r, NewNotFoundError("File not found", "The requested file does not exist"))
		} else {
			HandleError(w, r, NewInternalError(
				fmt.Sprintf("Error loading file: %v", err),
				"Unable to load the file",
			))
		}
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(content))
}

// SaveFileContent saves file content via API
func (h *ProvisionHandlers) SaveFileContent(w http.ResponseWriter, r *http.Request) {
	if !requireConfig(w, r, h.container) {
		return
	}
	userPath := r.FormValue("path")
	content := r.FormValue("content")

	if userPath == "" {
		writeJSONError(w, http.StatusBadRequest, "path parameter is required")
		return
	}

	if content == "" {
		writeJSONError(w, http.StatusBadRequest, "content parameter is required")
		return
	}

	filePath, err := h.resolveProvisionPath(userPath)
	if err != nil {
		writeJSONError(w, http.StatusForbidden, "The requested path is not allowed")
		return
	}

	// Create directories if they don't exist
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Unable to create the target directory")
		return
	}

	// Write file
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Unable to save the file")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "File saved successfully",
	})
}

// DeleteFile deletes a file via API
func (h *ProvisionHandlers) DeleteFile(w http.ResponseWriter, r *http.Request) {
	if !requireConfig(w, r, h.container) {
		return
	}
	userPath := r.FormValue("path")

	if userPath == "" {
		writeJSONError(w, http.StatusBadRequest, "path parameter is required")
		return
	}

	// Security check: resolve inside the allowed provision/TFTP roots.
	filePath, err := h.resolveProvisionPath(userPath)
	if err != nil {
		writeJSONError(w, http.StatusForbidden, "File deletion not allowed outside of provision directories")
		return
	}

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		writeJSONError(w, http.StatusNotFound, "File not found")
		return
	}

	// Delete file
	if err := os.Remove(filePath); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Unable to delete the file")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "File deleted successfully",
	})
}

// GetTemplateGallery returns pre-built templates
func (h *ProvisionHandlers) GetTemplateGallery(w http.ResponseWriter, r *http.Request) {
	gallery := h.getTemplateGallery()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(gallery); err != nil {
		HandleError(w, r, NewInternalError(
			fmt.Sprintf("Error encoding gallery: %v", err),
			"Unable to load the template gallery",
		))
	}
}

// getTemplateGallery returns pre-built templates
func (h *ProvisionHandlers) getTemplateGallery() []TemplateGalleryItem {
	return []TemplateGalleryItem{
		{
			Name:        "Ubuntu Server Cloud-Init",
			Description: "Basic Ubuntu server setup with user creation and package installation",
			Category:    "cloud-init",
			Language:    "yaml",
			Tags:        []string{"ubuntu", "server", "basic"},
			Content: `#cloud-config
hostname: ubuntu-server
manage_etc_hosts: true

users:
  - name: admin
    groups: [adm, sudo]
    shell: /bin/bash
    sudo: ALL=(ALL) NOPASSWD:ALL
    ssh_authorized_keys:
      - ssh-rsa AAAAB3N... # Add your SSH key here

packages:
  - curl
  - wget
  - vim
  - git
  - htop
  - unzip

package_update: true
package_upgrade: true

runcmd:
  - systemctl enable ssh
  - ufw --force enable
  - ufw allow ssh

final_message: "Ubuntu server setup complete!"`,
		},
		{
			Name:        "CentOS Kickstart",
			Description: "Automated CentOS installation with custom partitioning",
			Category:    "kickstart",
			Language:    "kickstart",
			Tags:        []string{"centos", "rhel", "automated"},
			Content: `#version=DEVEL
# System authorization information
auth --enableshadow --passalgo=sha512

# Use CDROM installation media
cdrom

# Use graphical install
graphical

# Run the Setup Agent on first boot
firstboot --enable

ignoredisk --only-use=sda

# Keyboard layouts
keyboard --vckeymap=us --xlayouts='us'

# System language
lang en_US.UTF-8

# Network information
network  --bootproto=dhcp --device=enp0s3 --onboot=off --ipv6=auto
network  --hostname=centos.localdomain

# Root password
rootpw --iscrypted $6$...

# System services
services --enabled="chronyd"

# System timezone
timezone America/New_York --isUtc

# System bootloader configuration
bootloader --append=" crashkernel=auto" --location=mbr --boot-drive=sda

# Partition clearing information
clearpart --none --initlabel

# Disk partitioning information
part /boot --fstype="ext4" --ondisk=sda --size=1024
part pv.157 --fstype="lvmpv" --ondisk=sda --size=51199
volgroup centos --pesize=4096 pv.157
logvol /  --fstype="ext4" --size=46080 --name=root --vgname=centos
logvol swap  --fstype="swap" --size=5119 --name=swap --vgname=centos

%packages
@^minimal
@core
chrony
kexec-tools

%end

%addon com_redhat_kdump --enable --reserve-mb='auto'

%end

%anaconda
pwpolicy root --minlen=6 --minquality=1 --notstrict --nochanges --notempty
pwpolicy user --minlen=6 --minquality=1 --notstrict --nochanges --emptyok
pwpolicy luks --minlen=6 --minquality=1 --notstrict --nochanges --notempty
%end`,
		},
		{
			Name:        "PXE Boot Menu",
			Description: "Standard PXE boot configuration with multiple OS options",
			Category:    "bootmenu",
			Language:    "ini",
			Tags:        []string{"pxe", "boot", "menu"},
			Content: `DEFAULT menu.c32
PROMPT 0
TIMEOUT 300
ONTIMEOUT local

MENU TITLE Network Boot Menu
MENU BACKGROUND splash.png

LABEL local
    MENU LABEL Boot from local disk
    MENU DEFAULT
    LOCALBOOT 0

LABEL ubuntu2004
    MENU LABEL Ubuntu 20.04 LTS Server
    KERNEL ubuntu-20.04/vmlinuz
    APPEND initrd=ubuntu-20.04/initrd.img ip=dhcp url=http://archive.ubuntu.com/ubuntu/
    
LABEL centos8
    MENU LABEL CentOS 8 Stream
    KERNEL centos8/vmlinuz
    APPEND initrd=centos8/initrd.img ip=dhcp repo=http://mirror.centos.org/centos/8-stream/BaseOS/x86_64/os/

LABEL memtest
    MENU LABEL Memory Test
    KERNEL memtest86+/memtest.bin

MENU SEPARATOR

LABEL reboot
    MENU LABEL Reboot
    COM32 reboot.c32

LABEL poweroff
    MENU LABEL Power Off
    COM32 poweroff.c32`,
		},
		{
			Name:        "Docker Host Setup",
			Description: "Cloud-init configuration for Docker host with compose",
			Category:    "cloud-init",
			Language:    "yaml",
			Tags:        []string{"docker", "containers", "compose"},
			Content: `#cloud-config
hostname: docker-host
manage_etc_hosts: true

users:
  - name: docker
    groups: [adm, sudo, docker]
    shell: /bin/bash
    sudo: ALL=(ALL) NOPASSWD:ALL
    ssh_authorized_keys:
      - ssh-rsa AAAAB3N... # Add your SSH key here

packages:
  - apt-transport-https
  - ca-certificates
  - curl
  - gnupg
  - lsb-release
  - vim
  - htop

package_update: true
package_upgrade: true

runcmd:
  - curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg
  - echo "deb [arch=amd64 signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
  - apt-get update
  - apt-get install -y docker-ce docker-ce-cli containerd.io
  - curl -L "https://github.com/docker/compose/releases/download/v2.20.0/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
  - chmod +x /usr/local/bin/docker-compose
  - systemctl enable docker
  - systemctl start docker
  - usermod -aG docker docker

write_files:
  - path: /home/docker/docker-compose.yml
    owner: docker:docker
    permissions: '0644'
    content: |
      version: '3.8'
      services:
        nginx:
          image: nginx:alpine
          ports:
            - "80:80"
          volumes:
            - ./html:/usr/share/nginx/html
          restart: unless-stopped

final_message: "Docker host setup complete! Access via SSH and run 'docker --version' to verify."`,
		},
		{
			Name:        "Kubernetes Node",
			Description: "Prepare Ubuntu node for Kubernetes cluster",
			Category:    "cloud-init",
			Language:    "yaml",
			Tags:        []string{"kubernetes", "k8s", "cluster"},
			Content: `#cloud-config
hostname: k8s-node
manage_etc_hosts: true

users:
  - name: k8s
    groups: [adm, sudo]
    shell: /bin/bash
    sudo: ALL=(ALL) NOPASSWD:ALL
    ssh_authorized_keys:
      - ssh-rsa AAAAB3N... # Add your SSH key here

packages:
  - apt-transport-https
  - ca-certificates
  - curl
  - gpg
  - vim

package_update: true
package_upgrade: true

runcmd:
  # Disable swap
  - swapoff -a
  - sed -i '/ swap / s/^\(.*\)$/#\1/g' /etc/fstab
  
  # Load kernel modules
  - modprobe overlay
  - modprobe br_netfilter
  
  # Install containerd
  - curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg
  - echo "deb [arch=amd64 signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null
  - apt-get update
  - apt-get install -y containerd.io
  
  # Configure containerd
  - mkdir -p /etc/containerd
  - containerd config default | tee /etc/containerd/config.toml
  - systemctl restart containerd
  - systemctl enable containerd
  
  # Install Kubernetes
  - curl -fsSL https://packages.cloud.google.com/apt/doc/apt-key.gpg | gpg --dearmor -o /usr/share/keyrings/kubernetes-archive-keyring.gpg
  - echo "deb [signed-by=/usr/share/keyrings/kubernetes-archive-keyring.gpg] https://apt.kubernetes.io/ kubernetes-xenial main" | tee /etc/apt/sources.list.d/kubernetes.list
  - apt-get update
  - apt-get install -y kubelet kubeadm kubectl
  - apt-mark hold kubelet kubeadm kubectl

write_files:
  - path: /etc/modules-load.d/k8s.conf
    content: |
      overlay
      br_netfilter
      
  - path: /etc/sysctl.d/k8s.conf
    content: |
      net.bridge.bridge-nf-call-iptables  = 1
      net.bridge.bridge-nf-call-ip6tables = 1
      net.ipv4.ip_forward                 = 1

final_message: "Kubernetes node ready! Use 'kubeadm join' to add to cluster."`,
		},
	}
}
