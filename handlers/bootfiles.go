package handlers

import (
	"net/http"
	"os"
	"path/filepath"

	"ignite/osimage"
)

// NewBootFileHandler serves downloaded kernels and initrds from the TFTP root.
// A request for /os-images/<os>/<version>/<file> reads
// <tftpDir>/os-images/<os>/<version>/<file>, the same relative path the
// downloader writes and the iPXE menu advertises. Directory listings are off.
func NewBootFileHandler(tftpDir string) http.Handler {
	root := filepath.Join(tftpDir, osimage.DirName)
	files := http.FileServer(bootFilesOnly{http.Dir(root)})
	return http.StripPrefix("/"+osimage.DirName+"/", files)
}

// bootFilesOnly hides directories so a client cannot list every image.
type bootFilesOnly struct {
	http.FileSystem
}

func (b bootFilesOnly) Open(name string) (http.File, error) {
	file, err := b.FileSystem.Open(name)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if info.IsDir() {
		_ = file.Close()
		return nil, os.ErrNotExist
	}
	return file, nil
}
