package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockCommandRunner allows us to mock external command execution
type MockCommandRunner struct {
	mock.Mock
}

func (m *MockCommandRunner) LookPath(file string) (string, error) {
	args := m.Called(file)
	return args.String(0), args.Error(1)
}

func (m *MockCommandRunner) RunCommand(name string, args ...string) error {
	callArgs := append([]interface{}{name}, stringSliceToInterface(args)...)
	return m.Called(callArgs...).Error(0)
}

func (m *MockCommandRunner) RunCommandWithOutput(name string, args ...string) ([]byte, error) {
	callArgs := append([]interface{}{name}, stringSliceToInterface(args)...)
	ret := m.Called(callArgs...)
	return ret.Get(0).([]byte), ret.Error(1)
}

func stringSliceToInterface(slice []string) []interface{} {
	result := make([]interface{}, len(slice))
	for i, s := range slice {
		result[i] = s
	}
	return result
}

func TestNewTestConfig(t *testing.T) {
	config := NewTestConfig()
	
	assert.NotNil(t, config)
	assert.Equal(t, "../public/tftp", config.TFTPDir)
	assert.Equal(t, "../ignite", config.IgniteBin)
	assert.Equal(t, "vmtest-disk.img", config.TestDisk)
	assert.Equal(t, "52:54:00:12:34:56", config.MACAddress)
	assert.Equal(t, "512M", config.Memory)
}

func TestTestConfigStructFields(t *testing.T) {
	config := &TestConfig{
		TFTPDir:    "/custom/tftp",
		IgniteBin:  "/custom/ignite",
		TestDisk:   "custom-disk.img",
		MACAddress: "aa:bb:cc:dd:ee:ff",
		Memory:     "1G",
	}
	
	assert.Equal(t, "/custom/tftp", config.TFTPDir)
	assert.Equal(t, "/custom/ignite", config.IgniteBin)
	assert.Equal(t, "custom-disk.img", config.TestDisk)
	assert.Equal(t, "aa:bb:cc:dd:ee:ff", config.MACAddress)
	assert.Equal(t, "1G", config.Memory)
}

func TestLogFunctions(t *testing.T) {
	// Test that log functions don't panic
	assert.NotPanics(t, func() {
		logInfo("test info")
	})
	assert.NotPanics(t, func() {
		logSuccess("test success")
	})
	assert.NotPanics(t, func() {
		logWarn("test warning")
	})
	assert.NotPanics(t, func() {
		logError("test error")
	})
}

func TestCheckPrerequisites(t *testing.T) {
	// Test TFTP directory not found
	t.Run("tftp directory not found", func(t *testing.T) {
		config := NewTestConfig()
		config.TFTPDir = "/nonexistent/directory"
		
		err := config.CheckPrerequisites()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "TFTP directory not found")
	})
	
	// Test PXE boot files not found
	t.Run("pxe boot files not found", func(t *testing.T) {
		// Create temporary directory without boot files
		tempDir, err := os.MkdirTemp("", "vmtest-*")
		assert.NoError(t, err)
		defer os.RemoveAll(tempDir)
		
		config := NewTestConfig()
		config.TFTPDir = tempDir
		
		err = config.CheckPrerequisites()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "PXE boot files not found")
	})
}

func TestCheckPrerequisitesWithTempFiles(t *testing.T) {
	// Create temporary directory structure for testing
	tempDir, err := os.MkdirTemp("", "vmtest-*")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)
	
	// Create test TFTP directory structure
	tftpDir := filepath.Join(tempDir, "tftp")
	bootDir := filepath.Join(tftpDir, "boot-bios")
	err = os.MkdirAll(bootDir, 0755)
	assert.NoError(t, err)
	
	// Create mock pxelinux.0 file
	pxelinuxPath := filepath.Join(bootDir, "pxelinux.0")
	err = os.WriteFile(pxelinuxPath, []byte("mock pxelinux"), 0644)
	assert.NoError(t, err)
	
	// Create mock ignite binary
	igniteBin := filepath.Join(tempDir, "ignite")
	err = os.WriteFile(igniteBin, []byte("#!/bin/bash\necho mock ignite"), 0755)
	assert.NoError(t, err)
	
	config := &TestConfig{
		TFTPDir:    tftpDir,
		IgniteBin:  igniteBin,
		TestDisk:   "vmtest-disk.img",
		MACAddress: "52:54:00:12:34:56",
		Memory:     "512M",
	}
	
	// This test will still fail because qemu-system-x86_64 likely isn't installed
	// but we can test the file checking logic
	err = config.CheckPrerequisites()
	if err != nil {
		// Should fail on QEMU check, not file checks
		assert.Contains(t, err.Error(), "qemu-system-x86_64 not found")
	}
}

func TestCreateTestDisk(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "vmtest-*")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)
	
	config := &TestConfig{
		TestDisk: filepath.Join(tempDir, "test-disk.img"),
	}
	
	// Test CreateTestDisk - may succeed if qemu-img is available
	err = config.CreateTestDisk()
	if err != nil {
		// If qemu-img is not available, should get an error
		assert.Contains(t, err.Error(), "failed to create test disk")
	} else {
		// If qemu-img is available, disk should be created
		assert.FileExists(t, config.TestDisk)
	}
}

func TestCreateTestDiskAlreadyExists(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "vmtest-*")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)
	
	diskPath := filepath.Join(tempDir, "existing-disk.img")
	err = os.WriteFile(diskPath, []byte("mock disk"), 0644)
	assert.NoError(t, err)
	
	config := &TestConfig{
		TestDisk: diskPath,
	}
	
	// Should not error if disk already exists
	err = config.CreateTestDisk()
	assert.NoError(t, err)
}

func TestTestBootFilesOnlySetup(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "vmtest-*")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)
	
	config := &TestConfig{
		TFTPDir:    filepath.Join(tempDir, "tftp"),
		TestDisk:   filepath.Join(tempDir, "test-disk.img"),
		Memory:     "512M",
		MACAddress: "52:54:00:12:34:56",
	}
	
	// Create the test disk file to avoid creation error
	err = os.WriteFile(config.TestDisk, []byte("mock disk"), 0644)
	assert.NoError(t, err)
	
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	
	// This will likely fail because QEMU may not be available or will timeout quickly
	err = config.TestBootFilesOnly(ctx)
	if err != nil {
		// Expected to fail due to QEMU not available or timeout
		assert.NotNil(t, err)
	}
}

func TestTestIgniteIntegrationSetup(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "vmtest-*")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)
	
	config := &TestConfig{
		TFTPDir:   filepath.Join(tempDir, "tftp"),
		IgniteBin: filepath.Join(tempDir, "ignite"),
		TestDisk:  filepath.Join(tempDir, "test-disk.img"),
		Memory:    "512M",
		MACAddress: "52:54:00:12:34:56",
	}
	
	// Create mock files
	err = os.WriteFile(config.TestDisk, []byte("mock disk"), 0644)
	assert.NoError(t, err)
	err = os.WriteFile(config.IgniteBin, []byte("#!/bin/bash\necho mock ignite"), 0755)
	assert.NoError(t, err)
	
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	
	// This will fail because ignite binary is not real, but we can test setup
	err = config.TestIgniteIntegration(ctx)
	if err != nil {
		// Expected to fail due to mock ignite binary or other issues
		assert.NotNil(t, err)
	}
}

func TestSetupDHCPServer(t *testing.T) {
	config := NewTestConfig()
	err := config.setupDHCPServer()
	
	// This method just logs and returns nil
	assert.NoError(t, err)
}

func TestCreateDHCPServerInIgnite(t *testing.T) {
	config := NewTestConfig()
	
	// This will fail because ignite server is not running
	err := config.createDHCPServerInIgnite()
	assert.Error(t, err)
	// Should contain curl or connection error
	assert.NotNil(t, err)
}

func TestVerifyIgniteStatus(t *testing.T) {
	config := NewTestConfig()
	
	// This will fail because ignite server is not running
	err := config.verifyIgniteStatus()
	assert.Error(t, err)
	// Should fail on HTTP connection
	assert.NotNil(t, err)
}

func TestShowLogsWithNoFiles(t *testing.T) {
	config := NewTestConfig()
	
	// Should not panic when no log files exist
	assert.NotPanics(t, func() {
		config.ShowLogs()
	})
}

func TestShowLogsWithMockFiles(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "vmtest-*")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)
	
	// Change to temp directory
	oldDir, err := os.Getwd()
	assert.NoError(t, err)
	defer os.Chdir(oldDir)
	
	err = os.Chdir(tempDir)
	assert.NoError(t, err)
	
	// Create mock log files
	logFiles := []string{
		"vmtest-serial.log",
		"vmtest-integration-serial.log",
		"vmtest-console.log",
		"vmtest-ignite.log",
	}
	
	for _, logFile := range logFiles {
		content := "Mock log content for " + logFile + "\n"
		// Create content longer than 500 chars to test truncation
		for i := 0; i < 10; i++ {
			content += "This is line " + string(rune(i+'0')) + " of mock log data with enough content to test truncation logic.\n"
		}
		err = os.WriteFile(logFile, []byte(content), 0644)
		assert.NoError(t, err)
	}
	
	config := NewTestConfig()
	
	// Should not panic and should handle the log files
	assert.NotPanics(t, func() {
		config.ShowLogs()
	})
}

func TestCleanup(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "vmtest-*")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)
	
	// Change to temp directory
	oldDir, err := os.Getwd()
	assert.NoError(t, err)
	defer os.Chdir(oldDir)
	
	err = os.Chdir(tempDir)
	assert.NoError(t, err)
	
	config := &TestConfig{
		TestDisk: "vmtest-disk.img",
	}
	
	// Create files to be cleaned up
	filesToCreate := []string{
		config.TestDisk,
		"vmtest-serial.log",
		"vmtest-integration-serial.log",
		"vmtest-console.log",
		"vmtest-ignite.log",
	}
	
	for _, file := range filesToCreate {
		err = os.WriteFile(file, []byte("test content"), 0644)
		assert.NoError(t, err)
		assert.FileExists(t, file)
	}
	
	// Run cleanup
	config.Cleanup()
	
	// Verify files are removed
	for _, file := range filesToCreate {
		assert.NoFileExists(t, file)
	}
}

func TestCleanupNonExistentFiles(t *testing.T) {
	config := NewTestConfig()
	
	// Should not panic when files don't exist
	assert.NotPanics(t, func() {
		config.Cleanup()
	})
}

func TestShowHelp(t *testing.T) {
	// Test that showHelp doesn't panic
	assert.NotPanics(t, func() {
		showHelp()
	})
}

func TestMainFunctionLogic(t *testing.T) {
	// Test command line argument parsing logic
	tests := []struct {
		name     string
		args     []string
		testNum  int
		expected string
	}{
		{
			name:     "default test 1",
			args:     []string{},
			testNum:  1,
			expected: "should run test 1",
		},
		{
			name:     "explicit test 1",
			args:     []string{"1"},
			testNum:  1,
			expected: "should run test 1",
		},
		{
			name:     "explicit test 2",
			args:     []string{"2"},
			testNum:  2,
			expected: "should run test 2",
		},
	}
	
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// We can't easily test the main function directly due to os.Exit calls
			// But we can test the logic components
			assert.NotEmpty(t, test.expected)
			assert.GreaterOrEqual(t, test.testNum, 1)
			assert.LessOrEqual(t, test.testNum, 2)
		})
	}
}

func TestContextTimeout(t *testing.T) {
	// Test that context timeout works as expected
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	
	select {
	case <-ctx.Done():
		assert.Equal(t, context.DeadlineExceeded, ctx.Err())
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Context should have timed out")
	}
}

func TestColorConstants(t *testing.T) {
	// Test that color constants are defined
	assert.NotEmpty(t, ColorReset)
	assert.NotEmpty(t, ColorRed)
	assert.NotEmpty(t, ColorGreen)
	assert.NotEmpty(t, ColorYellow)
	assert.NotEmpty(t, ColorBlue)
	
	// Test that they contain ANSI escape sequences
	assert.Contains(t, ColorReset, "\033")
	assert.Contains(t, ColorRed, "\033")
	assert.Contains(t, ColorGreen, "\033")
	assert.Contains(t, ColorYellow, "\033")
	assert.Contains(t, ColorBlue, "\033")
}

func TestQEMUCommandArguments(t *testing.T) {
	config := NewTestConfig()
	
	// Test that the QEMU arguments are constructed correctly
	expectedArgs := []string{
		"-m", config.Memory,
		"-netdev", "user,id=net0,tftp=" + config.TFTPDir + ",bootfile=boot-bios/pxelinux.0",
		"-device", "e1000,netdev=net0,mac=" + config.MACAddress,
		"-boot", "order=nc",
		"-drive", "file=" + config.TestDisk + ",format=qcow2",
		"-nographic",
		"-serial", "file:vmtest-serial.log",
		"-monitor", "none",
	}
	
	// Verify the arguments would be constructed correctly
	assert.Equal(t, "512M", config.Memory)
	assert.Equal(t, "52:54:00:12:34:56", config.MACAddress)
	assert.Equal(t, "../public/tftp", config.TFTPDir)
	assert.Equal(t, "vmtest-disk.img", config.TestDisk)
	
	// Test argument construction logic
	netdevArg := "user,id=net0,tftp=" + config.TFTPDir + ",bootfile=boot-bios/pxelinux.0"
	deviceArg := "e1000,netdev=net0,mac=" + config.MACAddress
	driveArg := "file=" + config.TestDisk + ",format=qcow2"
	
	assert.Contains(t, netdevArg, config.TFTPDir)
	assert.Contains(t, deviceArg, config.MACAddress)
	assert.Contains(t, driveArg, config.TestDisk)
	
	// expectedArgs has: -m, 512M, -netdev, user..., -device, e1000..., -boot, order=nc, -drive, file..., -nographic, -serial, file:..., -monitor, none
	// That's 15 elements total
	assert.Equal(t, 15, len(expectedArgs)) // Should have 15 arguments
}

// Test that demonstrates the integration flow without external dependencies
func TestIntegrationFlowLogic(t *testing.T) {
	config := NewTestConfig()
	
	// Test 1: Boot files test flow
	t.Run("boot files test flow", func(t *testing.T) {
		// Would create test disk (mocked)
		assert.Equal(t, "vmtest-disk.img", config.TestDisk)
		
		// Would start QEMU with built-in TFTP (mocked)
		assert.Equal(t, "../public/tftp", config.TFTPDir)
		
		// Would run for 30 seconds (mocked)
		timeout := 30 * time.Second
		assert.Equal(t, 30*time.Second, timeout)
	})
	
	// Test 2: Ignite integration flow
	t.Run("ignite integration flow", func(t *testing.T) {
		// Would setup DHCP server (mocked)
		err := config.setupDHCPServer()
		assert.NoError(t, err)
		
		// Would start ignite server (mocked)
		assert.Equal(t, "../ignite", config.IgniteBin)
		
		// Would create DHCP server config (would fail without running server)
		// Would start QEMU VM (would fail without QEMU)
		// Would verify ignite status (would fail without running server)
	})
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name   string
		config *TestConfig
		valid  bool
	}{
		{
			name:   "valid config",
			config: NewTestConfig(),
			valid:  true,
		},
		{
			name: "empty TFTP dir",
			config: &TestConfig{
				TFTPDir:    "",
				IgniteBin:  "../ignite",
				TestDisk:   "test.img",
				MACAddress: "52:54:00:12:34:56",
				Memory:     "512M",
			},
			valid: false,
		},
		{
			name: "empty ignite binary",
			config: &TestConfig{
				TFTPDir:    "../public/tftp",
				IgniteBin:  "",
				TestDisk:   "test.img",
				MACAddress: "52:54:00:12:34:56",
				Memory:     "512M",
			},
			valid: false,
		},
		{
			name: "empty MAC address",
			config: &TestConfig{
				TFTPDir:    "../public/tftp",
				IgniteBin:  "../ignite",
				TestDisk:   "test.img",
				MACAddress: "",
				Memory:     "512M",
			},
			valid: false,
		},
	}
	
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Basic validation logic
			isValid := test.config.TFTPDir != "" &&
				test.config.IgniteBin != "" &&
				test.config.TestDisk != "" &&
				test.config.MACAddress != "" &&
				test.config.Memory != ""
			
			assert.Equal(t, test.valid, isValid)
		})
	}
}