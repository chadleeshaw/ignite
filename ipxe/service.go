package ipxe

import (
	"bytes"
	"context"
	"fmt"
	"ignite/config"
	"ignite/osimage"
	"ignite/osimage/catalog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"
)

// Service handles iPXE configuration generation
type Service struct {
	config         *config.Config
	osImageService osimage.OSImageService
	catalog        catalog.Catalog
}

// UseCatalog supplies release names for the boot menu. A nil catalog keeps
// the built-in names.
func (s *Service) UseCatalog(cat catalog.Catalog) {
	if s == nil {
		return
	}
	s.catalog = cat
}

// NewService creates a new iPXE service
func NewService(cfg *config.Config, osImageService osimage.OSImageService) *Service {
	return &Service{
		config:         cfg,
		osImageService: osImageService,
	}
}

// iPXEConfig represents the data for iPXE template
type iPXEConfig struct {
	ServerIP string
	HTTPPort string
	BaseURL  string
	OSImages []OSImageEntry
}

// OSImageEntry represents an OS image for iPXE menu
type OSImageEntry struct {
	ID          string
	Name        string
	DisplayName string
	KernelPath  string
	InitrdPath  string
	KernelArgs  string
}

// GenerateConfig creates iPXE configuration based on available OS images
func (s *Service) GenerateConfig(ctx context.Context) (string, error) {
	// Get server IP (try to detect from network interfaces)
	serverIP, err := s.getServerIP()
	if err != nil {
		// Fallback to localhost
		serverIP = "127.0.0.1"
	}

	// Get available OS images
	osImages, err := s.osImageService.GetAllOSImages(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get OS images: %w", err)
	}

	// Convert to iPXE menu entries
	var entries []OSImageEntry
	for _, img := range osImages {
		kernelRel, err := osimage.RelPath(img.OS, img.Version, osimage.KernelName)
		if err != nil {
			return "", fmt.Errorf("os image %s %s: %w", img.OS, img.Version, err)
		}
		initrdRel, err := osimage.RelPath(img.OS, img.Version, osimage.InitrdName)
		if err != nil {
			return "", fmt.Errorf("os image %s %s: %w", img.OS, img.Version, err)
		}
		entry := OSImageEntry{
			ID:          menuID(img.OS, img.Version),
			Name:        strings.ToLower(img.OS),
			DisplayName: s.menuLabel(ctx, img.OS, img.Version),
			KernelPath:  kernelRel,
			InitrdPath:  initrdRel,
			KernelArgs:  s.getKernelArgs(img.OS, serverIP),
		}
		entries = append(entries, entry)
	}

	// Create template data
	data := iPXEConfig{
		ServerIP: serverIP,
		HTTPPort: s.config.HTTP.Port,
		BaseURL:  fmt.Sprintf("http://%s:%s", serverIP, s.config.HTTP.Port),
		OSImages: entries,
	}

	// Generate iPXE script
	return s.renderTemplate(data)
}

// menuID builds a unique, iPXE-label-safe menu ID from OS name and version.
// IDs are unique per OS+version so multiple versions of the same OS each get
// their own menu entry; anything outside [a-z0-9] is replaced with "-".
func menuID(os, version string) string {
	sanitize := func(s string) string {
		s = strings.ToLower(s)
		var b strings.Builder
		for _, r := range s {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				b.WriteRune(r)
			} else {
				b.WriteRune('-')
			}
		}
		return strings.Trim(b.String(), "-")
	}
	id := sanitize(os)
	if v := sanitize(version); v != "" {
		id += "-" + v
	}
	if id == "" {
		id = "os"
	}
	return id
}

// getServerIP attempts to detect the server's IP address.
// The SERVER_IP environment variable, when set, takes precedence.
func (s *Service) getServerIP() (string, error) {
	// Explicit SERVER_IP environment variable wins, then the configured
	// value, then interface auto-detection.
	if ip := strings.TrimSpace(os.Getenv("SERVER_IP")); ip != "" {
		return ip, nil
	}
	if s.config != nil {
		if ip := strings.TrimSpace(s.config.ServerIP); ip != "" {
			return ip, nil
		}
	}

	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil {
					return ipnet.IP.String(), nil
				}
			}
		}
	}

	return "", fmt.Errorf("no suitable network interface found")
}

// menuLabel prefers the catalog display name. The OS name is prefixed when
// the release name does not already include it, and past-EOL releases gain
// an (EOL) suffix. A miss falls back to the built-in names.
func (s *Service) menuLabel(ctx context.Context, osName, version string) string {
	fallback := s.getDisplayName(osName, version)
	if s == nil || s.catalog == nil {
		return fallback
	}
	rel, ok := s.catalog.Lookup(ctx, osName, version, "")
	if !ok {
		return fallback
	}
	name := rel.DisplayName
	if name == "" {
		name = version
	}
	osDisplay := ""
	if s.config != nil {
		if def, ok := s.config.OSImages.Sources[osName]; ok {
			osDisplay = def.DisplayName
		}
	}
	if osDisplay != "" && !strings.Contains(name, osDisplay) {
		name = osDisplay + " " + name
	}
	if rel.EOL(time.Now()) {
		name += " (EOL)"
	}
	return name
}

// getDisplayName returns a user-friendly display name for OS/version
func (s *Service) getDisplayName(os, version string) string {
	if os == "" {
		if version == "" {
			return "Unknown OS"
		}
		return version
	}
	switch os {
	case "ubuntu":
		return fmt.Sprintf("Ubuntu %s", version)
	case "centos":
		return fmt.Sprintf("CentOS Stream %s", version)
	case "nixos":
		return fmt.Sprintf("NixOS %s", version)
	default:
		return fmt.Sprintf("%s %s", strings.ToUpper(string(os[0]))+os[1:], version)
	}
}

// getKernelArgs returns appropriate kernel arguments for an OS.
// The HTTP port comes from configuration, never a hardcoded value.
func (s *Service) getKernelArgs(os, serverIP string) string {
	port := s.config.HTTP.Port
	switch os {
	case "ubuntu":
		return "boot=casper netboot=url fetch=http://" + serverIP + ":" + port + "/ubuntu/ quiet splash"
	case "centos":
		return "inst.repo=http://" + serverIP + ":" + port + "/centos/ quiet"
	case "nixos":
		return "init=/nix/store/.../init boot.shell_on_fail console=ttyS0"
	default:
		return "quiet"
	}
}

// renderTemplate renders the iPXE configuration template
func (s *Service) renderTemplate(data iPXEConfig) (string, error) {
	tmpl := `#!ipxe

# iPXE Boot Script for Ignite (Auto-generated)
dhcp
set server-ip {{.ServerIP}}
set base-url {{.BaseURL}}

# Show network info
echo Network configuration:
echo IP: ${ip}
echo Gateway: ${gateway}
echo DNS: ${dns}
echo

:menu
menu Ignite PXE Boot Menu
item --gap -- Operating Systems:
{{range .OSImages}}item {{.ID}} {{.DisplayName}}
{{end}}item --gap -- Tools:
item memtest Memory Test
item local Boot from local disk
item shell iPXE Shell
item exit Exit to BIOS
choose selected || goto exit

goto ${selected}

{{range .OSImages}}
:{{.ID}}
echo Booting {{.DisplayName}}...
kernel ${base-url}/{{.KernelPath}} initrd={{.InitrdPath}} {{.KernelArgs}}
initrd ${base-url}/{{.InitrdPath}}
boot

{{end}}
:memtest
echo Loading memory test...
kernel ${base-url}/tools/memtest86+
boot

:local
echo Booting from local disk...
sanboot --no-describe --drive 0x80

:shell
echo Entering iPXE shell...
shell

:exit
exit
`

	t, err := template.New("ipxe").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// WriteConfigToFile generates and writes iPXE config to the TFTP directory
func (s *Service) WriteConfigToFile(ctx context.Context) error {
	config, err := s.GenerateConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to generate iPXE config: %w", err)
	}

	filePath := filepath.Join(s.config.TFTP.Dir, "boot.ipxe")

	if err := os.WriteFile(filePath, []byte(config), 0644); err != nil {
		return fmt.Errorf("failed to write iPXE config to %s: %w", filePath, err)
	}

	return nil
}
