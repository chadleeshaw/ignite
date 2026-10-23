package osimage

import (
	"fmt"
	"path"
	"strings"
)

const (
	// DirName is the directory under the TFTP root, and the HTTP URL prefix,
	// for every downloaded kernel and initrd.
	DirName = "os-images"

	// KernelName is the stored kernel file name for every distribution.
	// Upstream KernelFile names differ and are used only in the download URL.
	KernelName = "vmlinuz"

	// InitrdName is the stored initrd file name for every distribution.
	InitrdName = "initrd.img"
)

// RelPath is the slash-separated location of one stored boot file.
// It is relative to the TFTP root and to the HTTP origin, so the downloader,
// the iPXE menu, and the HTTP file server all share it.
//
//	os-images/<os>/<version>/vmlinuz
//	os-images/<os>/<version>/initrd.img
func RelPath(osName, version, file string) (string, error) {
	if err := pathSegment("os", osName); err != nil {
		return "", err
	}
	if err := pathSegment("version", version); err != nil {
		return "", err
	}
	if err := pathSegment("file", file); err != nil {
		return "", err
	}
	return path.Join(DirName, osName, version, file), nil
}

func pathSegment(label, value string) error {
	if value == "" || value == "." || value == ".." || strings.ContainsAny(value, `/\`) {
		return fmt.Errorf("invalid %s %q", label, value)
	}
	return nil
}
