# Test Data for Ignite

This package provides mock data functionality for testing the Ignite DHCP management UI.

## Usage

### Using Makefile (Recommended)

The easiest way to work with mock data is through the Makefile targets:

```bash
# Populate mock data for UI testing
make db-mock

# Clear all data from database
make db-clear

# Reset database with fresh mock data
make db-reset

# See all available commands
make help
```

### Direct Command Line Usage

You can also use the commands directly:

```bash
# Populate mock data
go run main.go -mock-data

# Clear all data
go run main.go -clear-data

# Reset and repopulate
go run main.go -clear-data -mock-data
```

### Quick Development Workflow

For the fastest development setup:

```bash
make db-reset   # Reset database with mock data
make dev        # Runs the server in development mode
```

This will create:
- 2 DHCP servers with different network configurations
- Multiple leases with various configurations including boot menus and IPMI settings
- Both reserved and dynamic leases for testing different scenarios

## Mock Data Details

### DHCP Server 1
- Network: 192.168.1.2/24
- Gateway: 192.168.1.1
- DNS: 8.8.8.8
- Lease Range: 192.168.1.100-149 (50 IPs)
- Lease Duration: 2 hours

**Leases:**
- `00:11:22:33:44:55` → 192.168.1.100 (Dynamic, Ubuntu 20.04 PXE boot)
- `AA:BB:CC:DD:EE:FF` → 192.168.1.101 (Reserved, Windows 11 PXE boot)
- `00:11:22:33:44:66` → 192.168.1.102 (Reserved, CentOS 8 PXE boot)

### DHCP Server 2
- Network: 10.0.1.2/24
- Gateway: 10.0.1.1
- DNS: 1.1.1.1
- Lease Range: 10.0.1.100-124 (25 IPs)
- Lease Duration: 4 hours

**Leases:**
- `11:22:33:44:55:66` → 10.0.1.100 (Dynamic)
- `77:88:99:AA:BB:CC` → 10.0.1.101 (Reserved)

## Testing with the UI

After populating mock data:

1. Start the application: `go run main.go`
2. Navigate to the DHCP page at `http://localhost:8080/dhcp`
3. Verify servers are displayed with correct status badges
4. Test server start/stop/delete operations
5. Test lease reservations and boot menu configurations
6. Verify IPMI settings are displayed correctly

## Integration Tests

The mock data setup replaces the old `main_test.go` manual testing approach with a more maintainable solution that:

- Works with the current service-based architecture
- Provides realistic test scenarios
- Can be easily run before UI testing sessions
- Supports both setup and teardown operations