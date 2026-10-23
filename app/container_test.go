package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewContainer(t *testing.T) {
	// Create a temporary directory for test database
	tempDir := t.TempDir()

	// Set environment variable to use temp directory for database
	oldPath := os.Getenv("DB_PATH")
	defer func() {
		if oldPath != "" {
			os.Setenv("DB_PATH", oldPath)
		} else {
			os.Unsetenv("DB_PATH")
		}
	}()

	os.Setenv("DB_PATH", tempDir)

	container, err := NewContainer()
	assert.NoError(t, err)
	assert.NotNil(t, container)
	assert.NotNil(t, container.Database)
	assert.NotNil(t, container.ServerService)
	assert.NotNil(t, container.LeaseService)
	assert.NotNil(t, container.Config)
	wantCache, err := filepath.Abs(filepath.Join(tempDir, "syslinux"))
	require.NoError(t, err)
	assert.Equal(t, wantCache, container.SyslinuxService.GetConfig().TempDir)
	info, err := os.Stat(wantCache)
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	// Clean up
	container.Close()
}
