// vmtest-based PXE testing for ignite server
// Provides OS-independent testing using Go vmtest library

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
)

type TestConfig struct {
	TFTPDir     string
	IgniteBin   string
	TestDisk    string
	MACAddress  string
	Memory      string
}

func logInfo(msg string) {
	fmt.Printf("%s[INFO]%s %s\n", ColorBlue, ColorReset, msg)
}

func logSuccess(msg string) {
	fmt.Printf("%s[SUCCESS]%s %s\n", ColorGreen, ColorReset, msg)
}

func logWarn(msg string) {
	fmt.Printf("%s[WARN]%s %s\n", ColorYellow, ColorReset, msg)
}

func logError(msg string) {
	fmt.Printf("%s[ERROR]%s %s\n", ColorRed, ColorReset, msg)
}

func NewTestConfig() *TestConfig {
	return &TestConfig{
		TFTPDir:    "./public/tftp",
		IgniteBin:  "./ignite",
		TestDisk:   "vmtest-disk.img",
		MACAddress: "52:54:00:12:34:56",
		Memory:     "512M",
	}
}

func (tc *TestConfig) CheckPrerequisites() error {
	logInfo("Checking prerequisites...")

	// Check QEMU
	if _, err := exec.LookPath("qemu-system-x86_64"); err != nil {
		return fmt.Errorf("qemu-system-x86_64 not found: %v", err)
	}

	// Check TFTP directory
	if _, err := os.Stat(tc.TFTPDir); os.IsNotExist(err) {
		return fmt.Errorf("TFTP directory not found: %s", tc.TFTPDir)
	}

	// Check boot files
	pxelinuxPath := filepath.Join(tc.TFTPDir, "boot-bios", "pxelinux.0")
	if _, err := os.Stat(pxelinuxPath); os.IsNotExist(err) {
		return fmt.Errorf("PXE boot files not found. Run ./setup-boot-files.sh first")
	}

	// Check or build ignite
	if _, err := os.Stat(tc.IgniteBin); os.IsNotExist(err) {
		logInfo("Building ignite binary...")
		cmd := exec.Command("go", "build", "-o", "ignite", ".")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to build ignite: %v", err)
		}
	}

	logSuccess("Prerequisites OK")
	return nil
}

func (tc *TestConfig) CreateTestDisk() error {
	if _, err := os.Stat(tc.TestDisk); !os.IsNotExist(err) {
		return nil // Already exists
	}

	logInfo("Creating test disk image...")
	cmd := exec.Command("qemu-img", "create", "-f", "qcow2", tc.TestDisk, "100M")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create test disk: %v", err)
	}

	return nil
}

func (tc *TestConfig) TestBootFilesOnly(ctx context.Context) error {
	logInfo("TEST 1: Boot files with QEMU built-in TFTP")
	logInfo("Testing PXE menu display without ignite server")

	if err := tc.CreateTestDisk(); err != nil {
		return err
	}

	logInfo("Starting QEMU with built-in TFTP...")
	logInfo("Expected: PXE boot menu should appear")

	args := []string{
		"-m", tc.Memory,
		"-netdev", fmt.Sprintf("user,id=net0,tftp=%s,bootfile=boot-bios/pxelinux.0", tc.TFTPDir),
		"-device", fmt.Sprintf("e1000,netdev=net0,mac=%s", tc.MACAddress),
		"-boot", "order=nc",
		"-drive", fmt.Sprintf("file=%s,format=qcow2", tc.TestDisk),
		"-nographic",
		"-serial", "file:vmtest-serial.log",
		"-monitor", "none",
	}

	// Run QEMU in background for testing
	cmd := exec.CommandContext(ctx, "qemu-system-x86_64", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	logInfo("Starting QEMU VM (will run for 30 seconds)...")
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start QEMU: %v", err)
	}

	// Wait for context timeout or process completion
	go func() {
		time.Sleep(30 * time.Second)
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	}()

	err := cmd.Wait()
	if err != nil && err.Error() != "signal: killed" {
		return fmt.Errorf("QEMU test failed: %v", err)
	}

	logSuccess("Boot files test completed")
	return nil
}

func (tc *TestConfig) TestIgniteIntegration(ctx context.Context) error {
	logInfo("TEST 2: Integration with ignite server")
	logInfo("Testing DHCP/TFTP from ignite server")

	if err := tc.CreateTestDisk(); err != nil {
		return err
	}

	// Start ignite server
	logInfo("Starting ignite server...")
	igniteCmd := exec.CommandContext(ctx, tc.IgniteBin)
	igniteLogFile, err := os.Create("vmtest-ignite.log")
	if err != nil {
		return fmt.Errorf("failed to create ignite log file: %v", err)
	}
	defer igniteLogFile.Close()

	igniteCmd.Stdout = igniteLogFile
	igniteCmd.Stderr = igniteLogFile

	if err := igniteCmd.Start(); err != nil {
		return fmt.Errorf("failed to start ignite server: %v", err)
	}
	defer igniteCmd.Process.Kill()

	// Give ignite time to start
	time.Sleep(3 * time.Second)

	logSuccess("Ignite server started")

	// Start QEMU VM with console logging
	logInfo("Starting QEMU VM...")
	
	// Create console log file
	consoleLogFile, err := os.Create("vmtest-console.log")
	if err != nil {
		return fmt.Errorf("failed to create console log file: %v", err)
	}
	defer consoleLogFile.Close()
	
	args := []string{
		"-m", tc.Memory,
		"-netdev", "user,id=net0", // No built-in DHCP/TFTP
		"-device", fmt.Sprintf("e1000,netdev=net0,mac=%s", tc.MACAddress),
		"-boot", "order=nc",
		"-drive", fmt.Sprintf("file=%s,format=qcow2", tc.TestDisk),
		"-nographic",
		"-serial", "file:vmtest-integration-serial.log",
		"-monitor", "none",
	}

	qemuCmd := exec.CommandContext(ctx, "qemu-system-x86_64", args...)
	
	// Capture both stdout and stderr to console log
	qemuCmd.Stdout = consoleLogFile
	qemuCmd.Stderr = consoleLogFile

	logInfo("VM will attempt to get DHCP from ignite server...")
	logInfo("Console output will be saved to vmtest-console.log")
	
	if err := qemuCmd.Start(); err != nil {
		return fmt.Errorf("failed to start QEMU: %v", err)
	}

	// Monitor for 30 seconds with periodic status updates
	for i := 0; i < 30; i++ {
		time.Sleep(1 * time.Second)
		if i%5 == 0 {
			logInfo(fmt.Sprintf("VM running... (%d/30 seconds)", i+1))
		}
	}
	
	logInfo("Stopping VM...")
	qemuCmd.Process.Kill()
	qemuCmd.Wait()

	logSuccess("Integration test completed")
	logInfo("Check vmtest-ignite.log for ignite server activity")
	return nil
}

func (tc *TestConfig) ShowLogs() {
	logInfo("Showing test logs...")

	logFiles := []string{
		"vmtest-serial.log",
		"vmtest-integration-serial.log",
		"vmtest-console.log",
		"vmtest-ignite.log",
	}

	for _, logFile := range logFiles {
		if _, err := os.Stat(logFile); err == nil {
			logInfo(fmt.Sprintf("=== %s ===", logFile))
			content, err := os.ReadFile(logFile)
			if err != nil {
				logError(fmt.Sprintf("Failed to read %s: %v", logFile, err))
				continue
			}
			
			// Show last 500 characters
			if len(content) > 500 {
				content = content[len(content)-500:]
			}
			fmt.Print(string(content))
			fmt.Println()
		}
	}
}

func (tc *TestConfig) Cleanup() {
	logInfo("Cleaning up test files...")
	
	files := []string{
		tc.TestDisk,
		"vmtest-serial.log",
		"vmtest-integration-serial.log",
		"vmtest-console.log",
		"vmtest-ignite.log",
	}

	for _, file := range files {
		if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
			logWarn(fmt.Sprintf("Failed to remove %s: %v", file, err))
		}
	}

	logSuccess("Cleanup completed")
}

func showHelp() {
	fmt.Printf(`
Go vmtest-style PXE Boot Testing for Ignite

Usage: go run vmtest_go.go [TEST_NUMBER]

Tests:
  1    Boot files with QEMU built-in TFTP (recommended first)
  2    Integration test with ignite server
  logs Show test logs
  clean Clean up test files

This provides OS-independent PXE boot testing using Go and QEMU.
Works on macOS (including Apple Silicon), Linux, and Windows.

Requirements:
  - Go 1.21+
  - QEMU installed
  - Boot files setup (./setup-boot-files.sh)

Examples:
  go run vmtest_go.go 1      # Test boot files
  go run vmtest_go.go 2      # Test with ignite server
  go run vmtest_go.go logs   # Show logs
  go run vmtest_go.go clean  # Cleanup

`)
}

func main() {
	var testNum int
	flag.IntVar(&testNum, "test", 1, "Test number to run (1 or 2)")
	flag.Parse()

	args := flag.Args()
	if len(args) > 0 {
		switch args[0] {
		case "help", "-h", "--help":
			showHelp()
			return
		case "logs":
			config := NewTestConfig()
			config.ShowLogs()
			return
		case "clean":
			config := NewTestConfig()
			config.Cleanup()
			return
		case "1":
			testNum = 1
		case "2":
			testNum = 2
		default:
			logError(fmt.Sprintf("Unknown command: %s", args[0]))
			showHelp()
			return
		}
	}

	config := NewTestConfig()

	if err := config.CheckPrerequisites(); err != nil {
		logError(err.Error())
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var err error
	switch testNum {
	case 1:
		err = config.TestBootFilesOnly(ctx)
	case 2:
		err = config.TestIgniteIntegration(ctx)
	default:
		logError(fmt.Sprintf("Invalid test number: %d", testNum))
		showHelp()
		return
	}

	if err != nil {
		logError(fmt.Sprintf("Test %d failed: %v", testNum, err))
		os.Exit(1)
	}

	logSuccess(fmt.Sprintf("Test %d completed successfully", testNum))
}