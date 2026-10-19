package app

import (
	"embed"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewApplicationWithStatic(t *testing.T) {
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
