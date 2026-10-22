package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTFTPVolumeFollowsPublicCopy checks both images declare the TFTP
// volume after ./public is copied, and that no later instruction rewrites
// that tree. A fresh named volume is seeded from the image only when the
// files are already in the image at the VOLUME line.
func TestTFTPVolumeFollowsPublicCopy(t *testing.T) {
	root := repoRoot(t)
	_, err := os.Stat(filepath.Join(root, "public", "tftp", "boot.ipxe"))
	require.NoError(t, err)

	for _, name := range []string{"dockerfile", "Dockerfile.release"} {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(root, name))
			require.NoError(t, err)
			instructions := dockerfileInstructions(string(raw))

			copyIdx, volIdx := -1, -1
			for i, line := range instructions {
				if strings.Contains(line, "COPY --chown=ignite:ignite ./public ./public") {
					copyIdx = i
				}
				if line == "VOLUME /app/public/tftp" {
					require.Equal(t, -1, volIdx, "duplicate TFTP VOLUME")
					volIdx = i
				}
			}
			require.GreaterOrEqual(t, copyIdx, 0, "public COPY missing")
			require.GreaterOrEqual(t, volIdx, 0, "VOLUME /app/public/tftp missing")
			require.Greater(t, volIdx, copyIdx, "VOLUME must follow the public COPY")

			for _, line := range instructions[volIdx+1:] {
				op := strings.ToUpper(strings.Fields(line)[0])
				switch op {
				case "RUN", "COPY", "ADD":
					t.Fatalf("instruction %q after VOLUME does not persist in the volume", line)
				}
			}
		})
	}
}

// TestDataVolumeFollowsChown checks both images declare VOLUME /app/data
// after the directory is created and given to the ignite user. A fresh
// named volume then starts owned by uid 1001, which is who writes ignite.db
// and the Syslinux cache.
func TestDataVolumeFollowsChown(t *testing.T) {
	root := repoRoot(t)
	for _, name := range []string{"dockerfile", "Dockerfile.release"} {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(root, name))
			require.NoError(t, err)
			instructions := dockerfileInstructions(string(raw))

			chownIdx, volIdx := -1, -1
			for i, line := range instructions {
				if strings.Contains(line, "mkdir -p /app/data") && strings.Contains(line, "chown ignite:ignite /app/data") {
					require.Equal(t, -1, chownIdx, "duplicate data directory setup")
					chownIdx = i
				}
				if line == "VOLUME /app/data" {
					require.Equal(t, -1, volIdx, "duplicate data VOLUME")
					volIdx = i
				}
			}
			require.GreaterOrEqual(t, chownIdx, 0, "data directory chown missing")
			require.GreaterOrEqual(t, volIdx, 0, "VOLUME /app/data missing")
			require.Greater(t, volIdx, chownIdx, "VOLUME /app/data must follow the chown")
		})
	}
}

func dockerfileInstructions(text string) []string {
	var instructions []string
	var b strings.Builder
	for _, line := range strings.Split(text, "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		continued := strings.HasSuffix(trim, `\`)
		if continued {
			trim = strings.TrimSpace(strings.TrimSuffix(trim, `\`))
		}
		b.WriteString(trim)
		if !continued {
			instructions = append(instructions, b.String())
			b.Reset()
		}
	}
	if b.Len() > 0 {
		instructions = append(instructions, b.String())
	}
	return instructions
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, parent, dir, "go.mod not found")
		dir = parent
	}
}
