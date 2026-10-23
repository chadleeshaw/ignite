package osimage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelPath(t *testing.T) {
	kernel, err := RelPath("debian", "12", KernelName)
	require.NoError(t, err)
	assert.Equal(t, "os-images/debian/12/vmlinuz", kernel)

	initrd, err := RelPath("nixos", "26.05", InitrdName)
	require.NoError(t, err)
	assert.Equal(t, "os-images/nixos/26.05/initrd.img", initrd)

	for _, bad := range [][3]string{
		{"../debian", "12", KernelName},
		{"debian", "..", InitrdName},
		{"debian", "12", "../vmlinuz"},
		{"deb/ian", "12", KernelName},
		{"", "12", KernelName},
		{"debian", "", KernelName},
	} {
		_, err := RelPath(bad[0], bad[1], bad[2])
		assert.Error(t, err, "%q %q %q", bad[0], bad[1], bad[2])
	}
}
