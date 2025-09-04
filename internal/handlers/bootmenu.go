package handlers

import (
	"fmt"
	"html/template"
	"ignite/dhcp"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// BootMenuData holds the data used for generating a PXE boot menu configuration.
type BootMenuData struct {
	Name    string
	Kernel  string
	Initrd  string
	Options string
}

// SubmitBootMenu handles the submission of PXE boot menu configurations.
func (h *Handlers) SubmitBootMenu(w http.ResponseWriter, r *http.Request) {
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

	buildpxe := fmt.Sprintf("pxelinux.cfg/01-%s", strings.ReplaceAll(formData["mac"], ":", "-"))
	pxefile := filepath.Join(h.GetTFTPDir(), buildpxe)
	pxetempl := fmt.Sprintf("%s/templates/bootmenu/default.templ", h.GetProvisionDir())

	bootMenu := dhcp.BootMenu{
		Filename:      pxefile,
		OS:            formData["os"],
		Template_Type: formData["typeSelect"],
		Template_Name: formData["template_name"],
		Hostname:      formData["hostname"],
		IP:            net.ParseIP(formData["ip"]),
		Subnet:        net.ParseIP(formData["subnet"]),
		Gateway:       net.ParseIP(formData["gateway"]),
		DNS:           net.ParseIP(formData["dns"]),
	}

	buildconfig := fmt.Sprintf("configs/%s/%s", formData["typeSelect"], strings.ReplaceAll(formData["mac"], ":", "-"))
	configFile := filepath.Join(h.GetProvisionDir(), buildconfig)
	templBuild := fmt.Sprintf("templates/%s/%s", formData["typeSelect"], formData["template_name"])
	configTempl := filepath.Join(h.GetProvisionDir(), templBuild)

	pxedata := generateBootData(formData, configFile)

	if err := WriteTemplateToDisk(pxetempl, pxefile, pxedata); err != nil {
		http.Error(w, fmt.Sprintf("Error writing PXE menu: %v", err), http.StatusInternalServerError)
		return
	}

	if err := WriteTemplateToDisk(configTempl, configFile, formData); err != nil {
		http.Error(w, fmt.Sprintf("Error writing config file: %v", err), http.StatusInternalServerError)
		return
	}

	if err := updateDHCPLease(formData["tftpip"], formData["mac"], bootMenu); err != nil {
		fmt.Printf("Failed to update DHCP lease: %v\n", err)
	}

	http.Redirect(w, r, "/dhcp", http.StatusSeeOther)
}

// generateBootData constructs the boot configuration data based on provided OS and network details.
func generateBootData(formData map[string]string, configFile string) BootMenuData {
	options := getBootOptions(formData["os"], formData["dns"], formData["tftpip"], configFile)

	return BootMenuData{
		Name:    osToName(formData["os"]),
		Kernel:  osToKernel(formData["os"]),
		Initrd:  osToInitrd(formData["os"]),
		Options: options,
	}
}

// getBootOptions returns the appropriate boot options string based on the operating system.
func getBootOptions(os, dns, tftpip, configFile string) string {
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
func osToName(os string) string {
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
func osToKernel(os string) string {
	return fmt.Sprintf("%s/vmlinuz", osToName(os))
}

// osToInitrd constructs the initrd file path for the given OS.
func osToInitrd(os string) string {
	return fmt.Sprintf("%s/initrd.img", osToName(os))
}

// WriteTemplateToDisk writes the template to disk.
func WriteTemplateToDisk(templpath string, filepath string, data interface{}) error {
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
func updateDHCPLease(tftpip, mac string, menu dhcp.BootMenu) error {
	dhcpHandler, err := dhcp.GetDHCPServer(tftpip)
	if err != nil {
		return fmt.Errorf("unable to get DHCP server: %w", err)
	}

	for i, lease := range dhcpHandler.Leases {
		if lease.MAC == mac {
			lease.Menu = menu
			dhcpHandler.Leases[i] = lease
			return dhcpHandler.UpdateDBState()
		}
	}
	return fmt.Errorf("lease not found for MAC %s", mac)
}
