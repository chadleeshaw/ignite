package handlers

import (
	"context"
	"fmt"
	"html/template"
	"ignite/dhcp"
	"log"
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
		"tftpip":         r.Form.Get("tftpip"),
		"mac":            r.Form.Get("mac"),
		"os":             r.Form.Get("os"),
		"version":        r.Form.Get("version"),
		"typeSelect":     r.Form.Get("typeSelect"),
		"template_name":  r.Form.Get("template_name"),
		"hostname":       r.Form.Get("hostname"),
		"ip":             r.Form.Get("ip"),
		"subnet":         r.Form.Get("subnet"),
		"gateway":        r.Form.Get("gateway"),
		"dns":            r.Form.Get("dns"),
		"kernel_options": r.Form.Get("kernel_options"),
	}

	// Check required fields (kernel_options is optional)
	requiredFields := []string{"tftpip", "mac", "os", "version", "typeSelect", "template_name", "hostname", "ip", "subnet", "gateway", "dns"}
	for _, field := range requiredFields {
		if formData[field] == "" {
			http.Error(w, fmt.Sprintf("Missing required field: %s", field), http.StatusBadRequest)
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
		Filename:      pxefile,
		OS:            formData["os"],
		Version:       formData["version"],
		TemplateType:  formData["typeSelect"],
		TemplateName:  formData["template_name"],
		Hostname:      formData["hostname"],
		IP:            net.ParseIP(formData["ip"]),
		Subnet:        net.ParseIP(formData["subnet"]),
		Gateway:       net.ParseIP(formData["gateway"]),
		DNS:           net.ParseIP(formData["dns"]),
		KernelOptions: formData["kernel_options"],
	}

	// Build config file paths
	buildconfig := fmt.Sprintf("configs/%s/%s", formData["typeSelect"], strings.ReplaceAll(formData["mac"], ":", "-"))
	configFile := filepath.Join(cfg.Provision.Dir, buildconfig)
	templBuild := fmt.Sprintf("templates/%s/%s", formData["typeSelect"], formData["template_name"])
	configTempl := filepath.Join(cfg.Provision.Dir, templBuild)

	// Generate boot data (use buildconfig for HTTP URL, configFile for filesystem path)
	pxedata := h.generateBootData(formData, buildconfig)

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
		log.Printf("Failed to update DHCP lease: %v", err)
	}

	// Redirect to DHCP page
	http.Redirect(w, r, "/dhcp", http.StatusSeeOther)
}

// generateBootData constructs the boot configuration data based on provided OS and network details.
func (h *BootMenuHandlers) generateBootData(formData map[string]string, configFile string) BootMenuData {
	options := h.getBootOptions(formData["os"], formData["typeSelect"], formData["dns"], formData["tftpip"], configFile, formData["kernel_options"])

	return BootMenuData{
		Name:    h.osToName(formData["os"]),
		Kernel:  h.osToKernel(formData["os"], formData["version"]),
		Initrd:  h.osToInitrd(formData["os"], formData["version"]),
		Options: options,
	}
}

// getBootOptions returns the appropriate boot options string based on the operating system and template type.
func (h *BootMenuHandlers) getBootOptions(os, templateType, dns, tftpip, configFile, kernelOptions string) string {
	var baseOptions string

	// Determine boot parameters based on template type and OS
	switch templateType {
	case "cloud-init":
		baseOptions = fmt.Sprintf(`url=http://%s/%s autoinstall ds=nocloud-net;s=http://%s/ nameserver=%s`,
			tftpip, configFile, tftpip, dns)
	case "kickstart":
		baseOptions = fmt.Sprintf(`ks=http://%s/%s nameserver=%s`,
			tftpip, configFile, dns)
	case "preseed":
		baseOptions = fmt.Sprintf(`url=http://%s/%s auto=true priority=critical nameserver=%s`,
			tftpip, configFile, dns)
	case "autoyast":
		baseOptions = fmt.Sprintf(`autoyast=http://%s/%s nameserver=%s`,
			tftpip, configFile, dns)
	case "ipxe":
		baseOptions = fmt.Sprintf(`initrd=http://%s/%s nameserver=%s`,
			tftpip, configFile, dns)
	default:
		// Fallback to OS-based detection for backward compatibility
		switch os {
		case "ubuntu", "Ubuntu", "nixos", "NixOS":
			baseOptions = fmt.Sprintf(`url=http://%s/%s autoinstall ds=nocloud-net;s=http://%s/ nameserver=%s`,
				tftpip, configFile, tftpip, dns)
		case "debian", "Debian":
			baseOptions = fmt.Sprintf(`url=http://%s/%s auto=true priority=critical nameserver=%s`,
				tftpip, configFile, dns)
		case "redhat", "Redhat", "centos", "CentOS", "fedora", "Fedora":
			baseOptions = fmt.Sprintf(`ks=http://%s/%s nameserver=%s`,
				tftpip, configFile, dns)
		case "opensuse", "openSUSE", "suse", "SUSE":
			baseOptions = fmt.Sprintf(`autoyast=http://%s/%s nameserver=%s`,
				tftpip, configFile, dns)
		default:
			baseOptions = ""
		}
	}

	// Add additional kernel options if provided
	if kernelOptions != "" && baseOptions != "" {
		return baseOptions + " " + kernelOptions
	} else if kernelOptions != "" {
		return kernelOptions
	}

	return baseOptions
}

// osToName maps the OS name to a standardized name used in file paths.
func (h *BootMenuHandlers) osToName(os string) string {
	switch os {
	case "ubuntu", "Ubuntu":
		return "ubuntu"
	case "debian", "Debian":
		return "debian"
	case "fedora", "Fedora":
		return "fedora"
	case "centos", "CentOS":
		return "centos"
	case "opensuse", "openSUSE":
		return "opensuse"
	case "nixos", "NixOS":
		return "nixos"
	case "redhat", "Redhat":
		return "redhat"
	}
	return strings.ToLower(os)
}

// osToKernel constructs the kernel file path for the given OS and version.
// If a specific version is provided, it tries to find that version, otherwise uses the default version.
func (h *BootMenuHandlers) osToKernel(os, version string) string {
	ctx := context.Background()

	if h.container.OSImageService != nil {
		// If a specific version is requested, try to find that exact version
		if version != "" {
			if allImages, err := h.container.OSImageService.GetAllOSImages(ctx); err == nil {
				for _, image := range allImages {
					if image.OS == os && image.Version == version {
						return image.KernelPath
					}
				}
			}
		}

		// Fallback to default version
		if image, err := h.container.OSImageService.GetDefaultVersion(ctx, os); err == nil {
			return image.KernelPath
		}
	}

	// Final fallback to legacy path structure
	return fmt.Sprintf("%s/vmlinuz", h.osToName(os))
}

// osToInitrd constructs the initrd file path for the given OS and version.
// If a specific version is provided, it tries to find that version, otherwise uses the default version.
func (h *BootMenuHandlers) osToInitrd(os, version string) string {
	ctx := context.Background()

	if h.container.OSImageService != nil {
		// If a specific version is requested, try to find that exact version
		if version != "" {
			if allImages, err := h.container.OSImageService.GetAllOSImages(ctx); err == nil {
				for _, image := range allImages {
					if image.OS == os && image.Version == version {
						return image.InitrdPath
					}
				}
			}
		}

		// Fallback to default version
		if image, err := h.container.OSImageService.GetDefaultVersion(ctx, os); err == nil {
			return image.InitrdPath
		}
	}

	// Final fallback to legacy path structure
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
