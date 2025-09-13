package app

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
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

	// Clean up
	container.Close()
}
