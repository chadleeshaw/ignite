package app

import (
	"embed"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewApplicationWithStatic(t *testing.T) {
	// Create a simple empty embedded filesystem for testing
	testFS := embed.FS{}
	app, err := NewApplicationWithStatic(testFS)
	assert.NoError(t, err)
	assert.NotNil(t, app)
	assert.NotNil(t, app.GetContainer())
	assert.NotNil(t, app.GetContainer().ServerService)
	assert.NotNil(t, app.GetContainer().LeaseService)
	assert.NotNil(t, app.GetContainer().Config)
}
