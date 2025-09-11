package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewContainer(t *testing.T) {
	container, err := NewContainer()
	assert.NoError(t, err)
	assert.NotNil(t, container)
	assert.NotNil(t, container.Database)
	assert.NotNil(t, container.ServerService)
	assert.NotNil(t, container.LeaseService)
	assert.NotNil(t, container.Config)
}
