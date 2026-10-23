package dhcp

import (
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// MenuName is the friendly OS image or menu label for a lease.
// An empty string means no menu is assigned. The value is never a filesystem path.
func MenuName(menu BootMenu) string {
	osName := strings.TrimSpace(menu.OS)
	version := strings.TrimSpace(menu.Version)
	switch {
	case osName != "" && version != "" && !looksLikePath(osName) && !looksLikePath(version):
		return osName + " " + version
	case osName != "" && !looksLikePath(osName):
		return osName
	case version != "" && !looksLikePath(version):
		return version
	}
	label := strings.TrimSpace(menu.TemplateName)
	if label != "" && !looksLikePath(label) {
		return label
	}
	return ""
}

// MenuNameForLease returns the stored friendly name, or reads the current
// TFTP pxelinux.cfg file for mac when the lease has no menu fields.
// filename is consulted only when it stays inside tftpRoot. A path from an
// older temporary root is ignored, so a restart cannot surface it.
func MenuNameForLease(menu BootMenu, tftpRoot, mac, filename string) string {
	if name := MenuName(menu); name != "" {
		return name
	}
	if name := menuNameFromFile(filepath.Join(tftpRoot, "pxelinux.cfg", pxeMACFile(mac))); name != "" {
		return name
	}
	if filename != "" && pathInside(tftpRoot, filename) {
		return menuNameFromFile(filename)
	}
	return ""
}

// MenuNameFromPXE reads a PXE config body and returns a display name.
// A kernel path such as ubuntu/24.04/vmlinuz becomes "ubuntu 24.04".
// MENU LABEL and LABEL are the fallback. Paths are not returned.
func MenuNameFromPXE(content string) string {
	var kernel, menuLabel, label string
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		switch strings.ToUpper(fields[0]) {
		case "KERNEL":
			kernel = fields[1]
		case "LABEL":
			if label == "" {
				label = fields[1]
			}
		case "MENU":
			if strings.EqualFold(fields[1], "LABEL") && len(fields) > 2 && menuLabel == "" {
				menuLabel = strings.Join(fields[2:], " ")
			}
		}
	}
	if name := nameFromKernelPath(kernel); name != "" {
		return name
	}
	menuLabel = strings.TrimSpace(strings.TrimPrefix(menuLabel, "Install "))
	menuLabel = strings.TrimSpace(strings.TrimPrefix(menuLabel, "install "))
	if menuLabel != "" && !looksLikePath(menuLabel) {
		return menuLabel
	}
	if label != "" && !looksLikePath(label) {
		return label
	}
	return ""
}

func menuNameFromFile(path string) string {
	if path == "" {
		return ""
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return MenuNameFromPXE(string(body))
}

func nameFromKernelPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	parts := strings.Split(filepath.ToSlash(path), "/")
	if len(parts) < 3 {
		return ""
	}
	version := parts[len(parts)-2]
	name := parts[len(parts)-3]
	if name == "" || looksLikePath(name) || !containsDigit(version) {
		return ""
	}
	return name + " " + version
}

func pxeMACFile(mac string) string {
	mac = strings.ToLower(strings.TrimSpace(mac))
	mac = strings.ReplaceAll(mac, ":", "-")
	return "01-" + mac
}

func looksLikePath(value string) bool {
	if value == "" {
		return false
	}
	if strings.ContainsAny(value, `/\`) || strings.Contains(value, "..") {
		return true
	}
	return filepath.IsAbs(value)
}

func containsDigit(value string) bool {
	for _, r := range value {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func pathInside(root, path string) bool {
	if root == "" || path == "" {
		return false
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, pathAbs)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
