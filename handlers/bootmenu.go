package handlers

import (
	"context"
	"fmt"
	"html/template"
	"ignite/dhcp"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"ignite/config"
)

// BootMenuHandlers handles boot menu-related requests
type BootMenuHandlers struct {
	container *Container
}

// NewBootMenuHandlers creates a new BootMenuHandlers instance
func NewBootMenuHandlers(container *Container) *BootMenuHandlers {
	return &BootMenuHandlers{container: container}
}

// BootMenuData holds the data used for generating a PXE boot menu configuration.
type BootMenuData struct {
	Name    string
	Kernel  string
	Initrd  string
	Options string
}

// SubmitBootMenu handles boot menu submission
func (h *BootMenuHandlers) SubmitBootMenu(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}

	formData := map[string]string{
		"tftpip":        r.Form.Get("tftpip"),
		"mac":           r.Form.Get("mac"),
		"os":            r.Form.Get("os"),
		"typeSelect":    r.Form.Get("typeSelect"),
		"template_name": r.Form.Get("template_name"),
		"hostname":      r.Form.Get("hostname"),
		"ip":            r.Form.Get("ip"),
		"subnet":        r.Form.Get("subnet"),
		"gateway":       r.Form.Get("gateway"),
		"dns":           r.Form.Get("dns"),
	}

	for key, value := range formData {
		if value == "" {
			http.Error(w, fmt.Sprintf("Missing required field: %s", key), http.StatusBadRequest)
			return
		}
	}

	// Load configuration to get provision directory
	cfg, err := config.LoadDefault()
	if err != nil {
		http.Error(w, "Failed to load configuration", http.StatusInternalServerError)
		return
	}

	// Build PXE file paths
	buildpxe := fmt.Sprintf("pxelinux.cfg/01-%s", strings.ReplaceAll(formData["mac"], ":", "-"))
	pxefile := filepath.Join(TFTPDir, buildpxe)
	pxetempl := filepath.Join(cfg.Provision.Dir, "templates/bootmenu/default.templ")

	// Create BootMenu struct
	bootMenu := dhcp.BootMenu{
		Filename:     pxefile,
		OS:           formData["os"],
		TemplateType: formData["typeSelect"],
		TemplateName: formData["template_name"],
		Hostname:     formData["hostname"],
		IP:           net.ParseIP(formData["ip"]),
		Subnet:       net.ParseIP(formData["subnet"]),
		Gateway:      net.ParseIP(formData["gateway"]),
		DNS:          net.ParseIP(formData["dns"]),
	}

	// Build config file paths
	buildconfig := fmt.Sprintf("configs/%s/%s", formData["typeSelect"], strings.ReplaceAll(formData["mac"], ":", "-"))
	configFile := filepath.Join(cfg.Provision.Dir, buildconfig)
	templBuild := fmt.Sprintf("templates/%s/%s", formData["typeSelect"], formData["template_name"])
	configTempl := filepath.Join(cfg.Provision.Dir, templBuild)

	// Generate boot data
	pxedata := h.generateBootData(formData, configFile)

	// Ensure directories exist
	if err := os.MkdirAll(filepath.Dir(pxefile), 0755); err != nil {
		http.Error(w, fmt.Sprintf("Failed to create PXE directory: %v", err), http.StatusInternalServerError)
		return
	}

	if err := os.MkdirAll(filepath.Dir(configFile), 0755); err != nil {
		http.Error(w, fmt.Sprintf("Failed to create config directory: %v", err), http.StatusInternalServerError)
		return
	}

	// Write PXE template to disk
	if err := h.writeTemplateToDisk(pxetempl, pxefile, pxedata); err != nil {
		http.Error(w, fmt.Sprintf("Error writing PXE menu: %v", err), http.StatusInternalServerError)
		return
	}

	// Write config template to disk
	if err := h.writeTemplateToDisk(configTempl, configFile, formData); err != nil {
		http.Error(w, fmt.Sprintf("Error writing config file: %v", err), http.StatusInternalServerError)
		return
	}

	// Update DHCP lease
	if err := h.updateDHCPLease(formData["tftpip"], formData["mac"], bootMenu); err != nil {
		fmt.Printf("Failed to update DHCP lease: %v\n", err)
	}

	// Redirect to DHCP page
	http.Redirect(w, r, "/dhcp", http.StatusSeeOther)
}

// generateBootData constructs the boot configuration data based on provided OS and network details.
func (h *BootMenuHandlers) generateBootData(formData map[string]string, configFile string) BootMenuData {
	options := h.getBootOptions(formData["os"], formData["dns"], formData["tftpip"], configFile)

	return BootMenuData{
		Name:    h.osToName(formData["os"]),
		Kernel:  h.osToKernel(formData["os"]),
		Initrd:  h.osToInitrd(formData["os"]),
		Options: options,
	}
}

// getBootOptions returns the appropriate boot options string based on the operating system.
func (h *BootMenuHandlers) getBootOptions(os, dns, tftpip, configFile string) string {
	switch os {
	case "Ubuntu", "NixOS":
		return fmt.Sprintf(`url=http://%s/%s autoinstall ds=nocloud-net;s=http://%s/ nameserver=%s`,
			tftpip, configFile, tftpip, dns)
	case "Redhat":
		return fmt.Sprintf(`ks=http://%s/%s nameserver=%s`,
			tftpip, configFile, dns)
	}
	return ""
}

// osToName maps the OS name to a standardized name used in file paths.
func (h *BootMenuHandlers) osToName(os string) string {
	switch os {
	case "Ubuntu":
		return "ubuntu"
	case "NixOS":
		return "nixos"
	case "Redhat":
		return "redhat"
	}
	return ""
}

// osToKernel constructs the kernel file path for the given OS.
func (h *BootMenuHandlers) osToKernel(os string) string {
	return fmt.Sprintf("%s/vmlinuz", h.osToName(os))
}

// osToInitrd constructs the initrd file path for the given OS.
func (h *BootMenuHandlers) osToInitrd(os string) string {
	return fmt.Sprintf("%s/initrd.img", h.osToName(os))
}

// writeTemplateToDisk writes the template to disk.
func (h *BootMenuHandlers) writeTemplateToDisk(templpath string, filepath string, data interface{}) error {
	templates := map[string]*template.Template{
		"templ": template.Must(template.ParseFiles(templpath)),
	}

	file, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	if err := templates["templ"].Execute(file, data); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	return nil
}

// updateDHCPLease updates the DHCP lease with new boot menu data.
func (h *BootMenuHandlers) updateDHCPLease(tftpip, mac string, menu dhcp.BootMenu) error {
	ctx := context.Background()

	// Find server by IP to get server ID
	networkIP := net.ParseIP(tftpip)
	if networkIP == nil {
		return fmt.Errorf("invalid network IP: %s", tftpip)
	}

	servers, err := h.container.ServerService.GetAllServers(ctx)
	if err != nil {
		return fmt.Errorf("failed to get servers: %w", err)
	}

	var serverID string
	for _, server := range servers {
		if server.IP.Equal(networkIP) {
			serverID = server.ID
			break
		}
	}

	if serverID == "" {
		return fmt.Errorf("server not found for IP: %s", tftpip)
	}

	// Get lease by MAC
	lease, err := h.container.LeaseService.GetLeaseByMAC(ctx, mac)
	if err != nil || lease == nil {
		return fmt.Errorf("lease not found for MAC %s: %w", mac, err)
	}

	// Update lease with boot menu
	lease.Menu = menu

	// Save the updated lease using the lease service
	if err := h.container.LeaseService.UpdateLease(ctx, lease); err != nil {
		return fmt.Errorf("failed to save lease with boot menu: %w", err)
	}

	return nil
}
