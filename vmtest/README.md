# VMTest - Ignite PXE Testing

Self-contained PXE boot testing for the Ignite server using Go and QEMU.

## Overview

This directory contains everything needed to test PXE boot functionality for Ignite in a virtualized environment. It's completely self-contained and only requires QEMU and Go.

## Requirements

- Go 1.21+
- QEMU (specifically `qemu-system-x86_64`)
- Internet connection for downloading boot files

## Quick Start

1. Navigate to the vmtest directory:
   ```bash
   cd vmtest
   ```

2. Set up boot files (one-time setup):
   ```bash
   ./setup-boot-files.sh
   ```

3. Run tests:
   ```bash
   # Test basic boot files with QEMU's built-in TFTP
   go run vmtest.go 1
   
   # Test full integration with Ignite server
   go run vmtest.go 2
   ```

## Commands

- `go run vmtest.go 1` - Test boot files only (recommended first)
- `go run vmtest.go 2` - Full integration test with Ignite server
- `go run vmtest.go logs` - Show test logs
- `go run vmtest.go clean` - Clean up test files
- `go run vmtest.go help` - Show help

## What It Does

### Test 1: Boot Files Only  
- Uses QEMU's built-in TFTP server
- Tests if PXE boot menu appears correctly
- Verifies boot file integrity and SYSLINUX configuration
- **This test works completely and is recommended for basic verification**

### Test 2: Ignite Integration
- Builds and starts the Ignite server
- Tests web API responsiveness
- Creates a DHCP server configuration via API calls
- Starts a VM to attempt PXE boot (with networking limitations)
- **Note**: Full DHCP testing is limited by QEMU's user networking mode
- **Purpose**: Verifies ignite server functionality, not end-to-end PXE/DHCP

## Networking Limitations

Test 2 has networking limitations due to QEMU's user networking mode:
- The VM cannot act as a real DHCP client to the host ignite server
- This is a limitation of QEMU's default networking, not ignite itself
- For full PXE/DHCP testing, you would need:
  - Real hardware, or
  - QEMU with bridge/tap networking (requires root/admin privileges), or  
  - Advanced virtualization setup

The integration test focuses on verifying that:
1. Ignite server starts successfully
2. Web interface responds to requests  
3. DHCP server can be created via API
4. Basic VM functionality works

## Files Created

The testing process creates several temporary files:
- `vmtest-disk.img` - Virtual disk for testing
- `vmtest-*.log` - Various log files from tests
- `public/tftp/` - Local TFTP directory with boot files

## Directory Structure

```
vmtest/
├── README.md              # This file
├── go.mod                 # Go module (self-contained)
├── vmtest.go              # Main test program
├── setup-boot-files.sh    # Boot files setup script
└── public/                # Created by setup script
    └── tftp/              # TFTP files for testing
```

## Troubleshooting

1. **QEMU not found**: Install QEMU for your platform
2. **Boot files missing**: Run `./setup-boot-files.sh`
3. **Permission errors**: Ensure scripts are executable: `chmod +x setup-boot-files.sh`
4. **Test failures**: Check log files with `go run vmtest.go logs`

## Architecture Support

Currently supports x86_64 architecture. The tests work on:
- Linux (x86_64)
- macOS (Intel and Apple Silicon via emulation)
- Windows (with QEMU installed)