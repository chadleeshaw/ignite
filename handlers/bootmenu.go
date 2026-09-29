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
	if !requireConfig(w, r, h.container) {
		return
	}
	if err := r.ParseForm(); err != nil {
		HandleError(w, r, NewValidationError("Failed to parse form data", "The submitted form could not be parsed"))
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
			HandleError(w, r, NewValidationError(
				fmt.Sprintf("Missing required field: %s", field),
				fmt.Sprintf("The %s field is required", field),
			))
			return
		}
	}

	// The MAC address becomes part of file names — reject anything that is
	// not a real MAC so it cannot smuggle path separators.
	if _, err := net.ParseMAC(formData["mac"]); err != nil {
		HandleError(w, r, NewValidationError(
			fmt.Sprintf("Invalid MAC address: %v", err),
			"The MAC address is invalid",
		))
		return
	}
	macDashes := strings.ReplaceAll(formData["mac"], ":", "-")

	provisionDir := h.container.Config.Provision.Dir

	// Build PXE file paths (join-then-check: user input stays inside the roots).
	pxefile, err := safeJoin(TFTPDir, filepath.Join("pxelinux.cfg", "01-"+macDashes))
	if err != nil {
		HandleError(w, r, NewValidationError(
			fmt.Sprintf("Invalid PXE file path: %v", err),
			"The PXE file path is not allowed",
		))
		return
	}
	pxetempl := filepath.Join(provisionDir, "templates", "bootmenu", "default.templ")

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

	// Build config file paths (join-then-check).
	configFile, err := safeJoin(provisionDir, filepath.Join("configs", formData["typeSelect"], macDashes))
	if err != nil {
		HandleError(w, r, NewValidationError(
			fmt.Sprintf("Invalid config file path: %v", err),
			"The config file path is not allowed",
		))
		return
	}
	configTempl, err := safeJoin(provisionDir, filepath.Join("templates", formData["typeSelect"], formData["template_name"]))
	if err != nil {
		HandleError(w, r, NewValidationError(
			fmt.Sprintf("Invalid template path: %v", err),
			"The template path is not allowed",
		))
		return
	}

	// Generate boot data (use buildconfig for HTTP URL, configFile for filesystem path)
	buildconfig := fmt.Sprintf("configs/%s/%s", formData["typeSelect"], macDashes)
	pxedata := h.generateBootData(formData, buildconfig)

	// Ensure directories exist
	if err := os.MkdirAll(filepath.Dir(pxefile), 0755); err != nil {
		HandleError(w, r, NewInternalError(
			fmt.Sprintf("Failed to create PXE directory: %v", err),
			"Unable to prepare the PXE directory",
		))
		return
	}

	if err := os.MkdirAll(filepath.Dir(configFile), 0755); err != nil {
		HandleError(w, r, NewInternalError(
			fmt.Sprintf("Failed to create config directory: %v", err),
			"Unable to prepare the config directory",
		))
		return
	}

	// Write PXE template to disk
	if err := h.writeTemplateToDisk(pxetempl, pxefile, pxedata); err != nil {
		HandleError(w, r, NewInternalError(
			fmt.Sprintf("Error writing PXE menu: %v", err),
			"Unable to write the PXE boot menu",
		))
		return
	}

	// Write config template to disk
	if err := h.writeTemplateToDisk(configTempl, configFile, formData); err != nil {
		HandleError(w, r, NewInternalError(
			fmt.Sprintf("Error writing config file: %v", err),
			"Unable to write the config file",
		))
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

// writeTemplateToDisk parses the template file and writes the rendered
// output to disk. Parse failures are returned as errors — never panics.
func (h *BootMenuHandlers) writeTemplateToDisk(templpath string, filepath string, data interface{}) error {
	tmpl, err := template.ParseFiles(templpath)
	if err != nil {
		return fmt.Errorf("failed to parse template %s: %w", templpath, err)
	}

	file, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	if err := tmpl.Execute(file, data); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	return nil
}

// updateDHCPLease updates the DHCP lease with new boot menu data.
func (h *BootMenuHandlers) updateDHCPLease(tftpip, mac string, menu dhcp.BootMenu) error {
	ctx := context.Background()

	// Find server by IP to get server ID via the shared lookup helper.
	if _, err := findServerByIP(ctx, h.container.ServerService, tftpip); err != nil {
		return err
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
