package catalog

import (
	"strings"
)

// debianBootSums reads SHA256SUMS. Only the non-GTK netboot paths match,
// so a gtk image that ends with the same file name is ignored.
func debianBootSums(body []byte) (kernel, initrd string) {
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		path := strings.TrimPrefix(fields[1], "./")
		switch path {
		case "netboot/debian-installer/amd64/linux":
			kernel = fields[0]
		case "netboot/debian-installer/amd64/initrd.gz":
			initrd = fields[0]
		}
	}
	return kernel, initrd
}

// pxeBootSums reads the [checksums] section of a .treeinfo file.
// Values may be bare hex or sha256:<hex>.
func pxeBootSums(body []byte) (kernel, initrd string) {
	inChecksums := false
	for _, line := range strings.Split(string(body), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			inChecksums = trimmed == "[checksums]"
			continue
		}
		if !inChecksums {
			continue
		}
		key, value, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		sum := strings.TrimSpace(value)
		sum = strings.TrimPrefix(sum, "sha256:")
		sum = strings.TrimPrefix(sum, "SHA256:")
		switch strings.TrimSpace(key) {
		case "images/pxeboot/vmlinuz":
			kernel = sum
		case "images/pxeboot/initrd.img":
			initrd = sum
		}
	}
	return kernel, initrd
}
