package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultTFTPDir is the TFTP root when TFTP_DIR is unset.
// The release image overrides it with TFTP_DIR=/app/public/tftp.
const DefaultTFTPDir = "./public/tftp"

// TFTPRoot is the directory the TFTP server will serve.
type TFTPRoot struct {
	Path string
	// Ephemeral is true when Path will not survive a process restart.
	Ephemeral bool
	// Warning is empty when Path is a normal persistent directory.
	Warning string
}

// ResolveTFTPRoot chooses the TFTP directory.
// A non-empty configured path (the TFTP_DIR override) is kept. An empty
// path uses DefaultTFTPDir. The directory is created. When it cannot be
// created, a temporary directory is used instead. A path under the system
// temp directory is served as given, and reported as ephemeral.
func ResolveTFTPRoot(configured string) (TFTPRoot, error) {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		configured = DefaultTFTPDir
	}
	if err := os.MkdirAll(configured, 0o755); err != nil {
		fallback, ferr := os.MkdirTemp("", "ignite-tftp")
		if ferr != nil {
			return TFTPRoot{}, fmt.Errorf("create TFTP directory %s: %w", configured, err)
		}
		return TFTPRoot{
			Path:      fallback,
			Ephemeral: true,
			Warning: fmt.Sprintf(
				"WARNING: TFTP directory %q could not be created (%v). Falling back to temporary directory %s. Boot files, OS images, and per-MAC configs in this directory will not survive a restart",
				configured, err, fallback,
			),
		}, nil
	}
	abs, err := filepath.Abs(configured)
	if err != nil {
		abs = configured
	}
	root := TFTPRoot{Path: abs}
	if ephemeralPath(abs) {
		root.Ephemeral = true
		root.Warning = fmt.Sprintf(
			"WARNING: TFTP root %s is under the system temp directory. Boot files, OS images, and per-MAC configs will not survive a restart",
			abs,
		)
	}
	return root, nil
}

// StartupLines is the log text for the resolved root.
// The warning line is included only when the directory is ephemeral.
func (r TFTPRoot) StartupLines() []string {
	lines := []string{"TFTP root: " + r.Path}
	if r.Warning != "" {
		lines = append(lines, r.Warning)
	}
	return lines
}

func ephemeralPath(path string) bool {
	temp, err := filepath.Abs(os.TempDir())
	if err != nil {
		temp = os.TempDir()
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	rel, err := filepath.Rel(filepath.Clean(temp), filepath.Clean(abs))
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
