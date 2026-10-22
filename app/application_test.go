package app

import (
	"embed"
	"path/filepath"
	"testing"

	"ignite/handlers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewApplicationWithStatic(t *testing.T) {
	t.Setenv("DB_PATH", t.TempDir())
	// Create a simple empty embedded filesystem for testing
	testFS := embed.FS{}
	app, err := NewApplicationWithStatic(testFS)
	require.NoError(t, err)
	require.NotNil(t, app)
	t.Cleanup(func() { _ = app.Stop() })
	assert.NotNil(t, app.GetContainer())
	assert.NotNil(t, app.GetContainer().ServerService)
	assert.NotNil(t, app.GetContainer().LeaseService)
	assert.NotNil(t, app.GetContainer().Config)
}

func TestStartAndStopRecordTFTPStatus(t *testing.T) {
	t.Setenv("DB_PATH", t.TempDir())
	t.Setenv("DB_FILE", "tftp-status.db")
	t.Setenv("HTTP_PORT", "18081")
	t.Setenv("TFTP_DIR", filepath.Join(t.TempDir(), "tftp"))

	app, err := NewApplicationWithStatic(embed.FS{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = app.Stop() })

	err = app.Start()
	if err != nil {
		require.Equal(t, "stopped", handlers.TFTPStatus())
		return
	}
	require.Equal(t, "running", handlers.TFTPStatus())
	require.NoError(t, app.Stop())
	require.Equal(t, "stopped", handlers.TFTPStatus())
}
