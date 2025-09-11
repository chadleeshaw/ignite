package testdata

import (
	"flag"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseFlags(t *testing.T) {
	// Reset flag state for testing
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	// Test with no flags
	os.Args = []string{"test"}
	config := ParseFlags()
	assert.False(t, config.MockData)
	assert.False(t, config.ClearData)
}

func TestParseFlagsWithMockData(t *testing.T) {
	// Reset flag state for testing
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	// Test with mock-data flag
	os.Args = []string{"test", "-mock-data"}
	config := ParseFlags()
	assert.True(t, config.MockData)
	assert.False(t, config.ClearData)
}

func TestParseFlagsWithClearData(t *testing.T) {
	// Reset flag state for testing
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	// Test with clear-data flag
	os.Args = []string{"test", "-clear-data"}
	config := ParseFlags()
	assert.False(t, config.MockData)
	assert.True(t, config.ClearData)
}

func TestHandleDataOperationsNoOps(t *testing.T) {
	config := &Config{
		MockData:  false,
		ClearData: false,
	}

	// Should return true (continue running) when no operations requested
	result := HandleDataOperations(config, nil)
	assert.True(t, result)
}
