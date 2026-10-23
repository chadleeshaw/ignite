# Ignite

<p align="center">
  <img src="./public/http/img/flame-512.png" alt="Ignite" width="180"/>
</p>

Ignite is a powerful, all-in-one solution for network booting and server provisioning, implemented in Go. It simplifies the complexities of setting up network boot environments by integrating DHCP, TFTP, and a web-based management interface into a single, easy-to-use application.

## 🏆 Quality & Testing

Ignite features comprehensive test coverage across all components:

- **Comprehensive Unit Tests**: All packages include extensive test suites with mock implementations
- **Integration Testing**: VM-based testing with QEMU for end-to-end PXE boot validation  
- **Test Coverage**: Significant coverage across core packages (osimage, syslinux, handlers, routes, vmtest)
- **Quality Assurance**: Automated formatting (`gofmt`) and static analysis (`go vet`) validation

## Overview

Designed for developers, system administrators, and hobbyists, Ignite provides a streamlined workflow for managing PXE boot environments. Whether you're deploying operating systems, running diagnostics, or managing server configurations, Ignite offers the tools you need to get the job done efficiently.

## Features

- **Integrated DHCP Server**: Dynamically assign IP addresses to network devices.
- **Built-in TFTP Server**: Serve boot files (e.g., PXELinux, iPXE) to booting clients.
- **Web-Based Management UI**: A user-friendly interface to manage DHCP leases, configure boot menus, and monitor server status.
- **Dynamic PXE Boot Menu Templating**: Automatically generate PXE boot menus for different operating systems based on DHCP leases. Supported templates include:
  - Ubuntu
  - NixOS
  - Red Hat
- **Redfish API Integration**: Remotely manage servers and initiate PXE boot processes via the Redfish API.
- **Cloud-Init & Kickstart Templating**: Simplify automated OS installations with dynamic templating for cloud-init and Kickstart configurations.
- **Advanced File Management**: Template and configuration file browser with delete functionality and clear type distinction.
- **Enhanced Security**: Path validation, input sanitization, and secure file operations throughout the application.
- **Comprehensive Error Handling**: Detailed error messages, validation feedback, and graceful error recovery.

## Screenshots

The nav is Overview, DHCP, TFTP, OS Images, Syslinux, and Provision. Cozy is the default density. Console is the same layout in a monospace, tighter rhythm. Service health that used to live on a separate Status page is on Overview. Both Overview shots are 1280 pixels wide.

### **Overview**
Service tiles for HTTP, TFTP, and DHCP, plus the lease and pool summary slot.

<table>
  <tr>
    <td width="50%" align="center"><strong>Cozy</strong><br><img src="./public/http/img/main_page.png" alt="Overview in Cozy" width="100%"></td>
    <td width="50%" align="center"><strong>Console</strong><br><img src="./public/http/img/console_page.png" alt="Overview in Console" width="100%"></td>
  </tr>
</table>

### **Other pages**

- DHCP: [Cozy](./public/http/img/dhcp_page.png) · [Console](./public/http/img/dhcp_console.png)
- DHCP detail: [Cozy](./public/http/img/dhcp_detail.png) · [Console](./public/http/img/dhcp_detail_console.png)
- TFTP: [Cozy](./public/http/img/tftp_page.png) · [Console](./public/http/img/tftp_console.png)
- OS Images: [Cozy](./public/http/img/osimage_page.png) · [Console](./public/http/img/osimage_console.png)
- Syslinux: [Cozy](./public/http/img/syslinux_page.png) · [Console](./public/http/img/syslinux_console.png)
- Provision: [Cozy](./public/http/img/provision_page.png) · [Console](./public/http/img/provision_console.png)
- Login: [Cozy](./public/http/img/login_page.png) · [Console](./public/http/img/login_console.png)

## Getting Started

### Prerequisites

- **Go**: Version 1.21 or higher (tested with Go 1.23.4).
- **Node.js & npm**: For managing frontend dependencies.
- **Tailwind CSS**: For styling the web interface.
- **QEMU** (optional): For running integration tests (`qemu-system-x86_64`, `qemu-img`).
- **curl**: For API testing and health checks.
- A basic understanding of network booting concepts (DHCP, TFTP, PXE).

### Setup

1. **Clone the repository**:

   ```bash
   git clone https://github.com/your-username/ignite.git
   cd ignite
   ```

2. **Build and Run**:

   The easiest way to get started is by using the provided Makefile:

   ```bash
   make run
   ```

   This command will handle installing dependencies, building the CSS, and running the application.

3. **Access the Web UI**:

   Once the server is running, open your browser and navigate to `http://localhost:8080`.

### Development & Testing

**Running Tests**:

```bash
# Run all unit tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run vmtest integration tests (requires QEMU)
cd vmtest
go run vmtest.go 1    # Test boot files with QEMU built-in TFTP
go run vmtest.go 2    # Test ignite integration
go run vmtest.go logs # Show test logs
go run vmtest.go clean # Clean up test files
```

**README screenshots**:

Regenerate the images under `public/http/img/` with headless Chrome. TCP 8080 and UDP 69 must be free. The target builds Ignite, seeds a temporary database (DHCP leases, OS images, and Syslinux versions), and writes a Cozy and a Console capture of each nav page, the login page, and the DHCP host-detail state. Overview is captured at 1280×800.

```bash
make screenshots
```

`CHROME_PATH` overrides the Chrome binary. `scripts/capture.sh` is the same flow without Make.

**Code Quality**:

```bash
# Format code
gofmt -w .

# Run static analysis
go vet ./...
```

**CLI Options**:

```bash
# Populate with mock data for testing
./ignite -mock-data

# Clear all data
./ignite -clear-data
```

### Configuration

Ignite can be configured via environment variables. Here are the available options:

| Variable    | Description                                    | Default            |
|-------------|------------------------------------------------|--------------------|
| `DB_PATH`   | Path to store the database file. The Syslinux cache defaults to a `syslinux` directory under this path. | `./`               |
| `DB_FILE`   | Name of the database file.                     | `ignite.db`        |
| `DB_BUCKET` | Database bucket name.                          | `dhcp`             |
| `TFTP_DIR`  | Directory for the TFTP server to serve files from. Created on startup. The release image sets `/app/public/tftp`. | `./public/tftp`   |
| `HTTP_DIR`  | Directory for the HTTP server to serve files from. | `./public/http`   |
| `HTTP_PORT` | TCP port for the HTTP server.                   | `8080`             |
| `TFTP_PORT` | UDP port for the TFTP server. Ports below 1024 need extra privileges. Set this above 1024 to run TFTP as the unprivileged `ignite` user. | `69` |
| `DHCP_ENABLED` | When `false`, `0`, `no`, or `off`, no DHCP listener starts. Every server stays `configured` and does not make overall health `partial` or `down`. Starting one from the UI or API returns 409. Unset and any other value leave DHCP enabled. | `true` |
| `SYSLINUX_CACHE_DIR` | Download and extract cache for Syslinux. Created on startup. Empty uses `$DB_PATH/syslinux` (`/app/data/syslinux` in the image). | `$DB_PATH/syslinux` |
| `PROV_DIR`  | Directory for provisioning templates.          | `./public/provision` |
| `BMC_TLS_VERIFY` | Check the Redfish BMC certificate. Unset skips verification, which is what a self-signed homelab BMC needs. `strconv.ParseBool` values turn it on (`1`, `t`, `T`, `TRUE`, `true`, `True`) or off (`0`, `f`, `F`, `FALSE`, `false`, `False`). Any other value logs a warning and stays off. | `false` |
| `BMC_SYSTEM_ID` | Redfish ComputerSystem `Id` when a BMC exposes more than one system. A `system_id` form field on `POST /pxe/submit_ipmi` overrides this. | first and only system |
| `COOKIE_SECURE` | `Secure` flag on `ignite_session` and `ignite_mode`. `auto` sets it only when the request is TLS or `X-Forwarded-Proto` is `https`. `true` always sets it. `false` never does. Any other value is `auto`. The CSRF token is not a cookie. | `auto` |
| `OS_CATALOG_REFRESH` | How often Ignite reloads the OS release catalog. A Go duration, for example `12h` or `30m`. Empty, invalid, and non-positive values use 24h. | `24h` |
| `OS_CATALOG_OFFLINE` | `strconv.ParseBool` values (`1`, `t`, `T`, `TRUE`, `true`, `True`) pin the catalog to the list shipped with Ignite and skip release sources. Unset and invalid values stay online. | `false` |
| `GITHUB_TOKEN` | Optional token for the NixOS GitHub releases API. Empty uses the unauthenticated API. A 403 keeps the last saved NixOS releases. The token is not logged. | empty |

Startup logs the resolved TFTP root (`TFTP root: ...`), which is the absolute directory the server reads and writes. `TFTP_DIR` overrides the default. The directory is created if it is missing. When that directory cannot be created, Ignite falls back to a temporary directory named `ignite-tftp.*` under the system temp dir and logs a warning that boot files, OS images, and per-MAC configs will not survive a restart. The same warning is logged when `TFTP_DIR` itself points at a temp directory; the override is still used. The TFTP page shows the configured `TFTP_DIR` as given (`./public/tftp`, or `/app/public/tftp` in the image). When startup had to serve a temporary directory, the page shows that temporary path instead.

Both container images declare `VOLUME /app/public/tftp` after `./public` is copied in, and `VOLUME /app/data` after that directory is created and given to the `ignite` user (uid 1001). A fresh TFTP volume starts with the files shipped in the image (`boot.ipxe` and the OS directories). A fresh data volume starts empty and owned by uid 1001, which is who writes `/app/data/ignite.db` and `/app/data/syslinux`. Mount both by name so the database, the Syslinux cache, boot files, downloaded OS images, and per-MAC configs survive a container recreate:

```bash
docker run -d \
  --name ignite \
  -p 8080:8080 \
  -p 69:69/udp \
  -v ignite-data:/app/data \
  -v ignite-tftp:/app/public/tftp \
  ghcr.io/chadleeshaw/ignite:v3.0.0
```

Compose and a Swarm stack use the same syntax (`docker stack deploy -c docker-compose.yml ignite`):

```yaml
services:
  ignite:
    image: ghcr.io/chadleeshaw/ignite:v3.0.0
    ports:
      - "8080:8080"
      - "69:69/udp"
    volumes:
      - ignite-data:/app/data
      - ignite-tftp:/app/public/tftp
volumes:
  ignite-data:
  ignite-tftp:
```

Startup logs the Syslinux cache (`Syslinux cache: ...`). `SYSLINUX_CACHE_DIR` overrides the default. Versions downloaded before this change lived in `/tmp/syslinux-downloads`, which is not copied. Download those versions again.

A bind mount of an empty host directory (`-v /path/on/host:/app/public/tftp` when `/path/on/host` is empty) hides those shipped files. Seed the host directory from the image, give it to the `ignite` user (uid 1001), then start Ignite with the bind mount. `docker cp` onto the host writes the files as root, and the app cannot create boot files in a directory it does not own. A named volume keeps the `ignite` ownership on its own.

```bash
mkdir -p /path/on/host
docker create --name ignite-seed ghcr.io/chadleeshaw/ignite:v3.0.0
docker cp ignite-seed:/app/public/tftp/. /path/on/host
docker rm ignite-seed
sudo chown -R 1001:1001 /path/on/host
```

Upgrading a v2.x container that was started without these mounts: copy the database and the TFTP directory out before removing the container, recreate it with both named volumes, and restore the files. `docker cp` into the new container writes as root, so return ownership to the `ignite` user (uid 1001) afterward. Syslinux archives extracted under `/tmp/syslinux-downloads` are not in either copy. Download those versions again after the upgrade. Redfish skips BMC certificate checks unless `BMC_TLS_VERIFY=true`.

Downloaded OS images stay at `TFTP_DIR/os-images/<os>/<version>/vmlinuz` and `initrd.img` for every distribution. Ignite serves that directory over HTTP at `/os-images/<os>/<version>/`, which is the path the iPXE menu uses. Images already stored under those names do not need to be downloaded again. Regenerate the iPXE config after upgrading (`POST /ipxe/update`, or the button on the OS Images page) so `boot.ipxe` points at `/os-images/...` instead of `/<os>/<version>/<upstream file name>`.

The download picker reads a release catalog. Ignite refreshes it in the background at startup and then every `OS_CATALOG_REFRESH` (24h unless you set another duration). `GET /osimages/catalog` reports `source` as `live` after a refresh, `cached` when this process loaded the last saved snapshot, or `bundled` when it is using the list shipped with the binary. `OS_CATALOG_OFFLINE=true` stays on that bundled list. A source that fails keeps that OS's previous releases and records the error. A refresh that cannot reach any source leaves the saved catalog in place. End-of-life releases stay out of the picker. An existing download can still be resolved after its end-of-life date, and the image keeps an EOL badge. Refreshing the catalog does not delete downloaded images. Ubuntu offers LTS cycles only, Fedora skips test releases, and openSUSE offers Leap Stable plus Tumbleweed. Debian publishes per-file SHA256SUMS, and Fedora and CentOS Stream publish them in `.treeinfo`. Ignite checks those digests on download. When a release publishes none, Ignite records the SHA256 it computed.

```bash
docker cp <container>:/app/data/ignite.db ./ignite.db
docker cp <container>:/app/public/tftp ./tftp-backup
# remove the old container, then start the new one with
# -v ignite-data:/app/data -v ignite-tftp:/app/public/tftp
docker cp ./ignite.db <container>:/app/data/ignite.db
docker cp ./tftp-backup/. <container>:/app/public/tftp
docker exec -u root <container> chown -R ignite:ignite /app/data /app/public/tftp
```

## API Reference

Ignite exposes a set of RESTful APIs to control and manage the server.

### GET Routes

| Endpoint                | Description                              |
|-------------------------|------------------------------------------|
| `/`                     | Serves the main index page.              |
| `/status`               | Redirects to Overview (`/`).             |
| `/status/content`       | Status fragment Overview polls.          |
| `/open_modal`           | Opens a modal dialog.                    |
| `/close_modal`          | Closes a modal dialog.                   |
| `/dhcp`                 | Serves the DHCP management page.         |
| `/dhcp/servers`         | DHCP server list. HTML fragment for htmx; JSON when `?format=json` or `Accept` is `application/json` without `text/html`. |
| `/provision`            | Serves the provisioning page.            |
| `/tftp`                 | Serves the TFTP management page.         |
| `/tftp/open`            | Opens a file in the TFTP directory.      |
| `/tftp/download`        | Downloads a file from the TFTP directory.|
| `/tftp/view`            | Views a file from the TFTP directory.    |
| `/tftp/serve`           | Serves a file from the TFTP directory.   |
| `/os-images/<os>/<version>/vmlinuz` and `.../initrd.img` | Kernel and initrd for iPXE. No session. The path matches the file under `TFTP_DIR`. |
| `/prov/gettemplates`    | Retrieves provisioning template options. |
| `/prov/loadtemplate`    | Loads a provisioning template.           |
| `/prov/getconfigs`      | Retrieves configuration options.         |
| `/prov/loadconfig`      | Loads a configuration file.              |
| `/prov/getfilename`     | Updates the filename for a template.     |
| `/provision/load-file`  | Loads file content via API.              |
| `/provision/gallery`    | Retrieves template gallery items.        |

`GET /dhcp/servers` returns an HTML fragment (`text/html`) for browsers and
htmx. Add `?format=json`, or send `Accept: application/json` without
`text/html`, for JSON:

```json
{
  "servers": [
    {
      "id": "server-id",
      "tftpip": "192.168.1.1",
      "status": "running",
      "lease_range": 50,
      "used": 1,
      "free": 49,
      "in_flight": 0,
      "failed": 0,
      "leases": [
        {
          "mac": "aa:bb:cc:dd:ee:ff",
          "ip": "192.168.1.100",
          "static": false,
          "state": "assigned",
          "state_badge_class": "badge-info",
          "state_display_name": "Assigned"
        }
      ]
    }
  ]
}
```

`status` is the listener state, and the same value on `/status/content`,
`/dashboard/summary` (`pools[].status`), and the DHCP page:

| Value | Meaning |
|-------|---------|
| `running` | The listener is bound and serving. |
| `stopped` | It was started and then stopped, or the last start failed. |
| `configured` | It is defined and has not been started since this process booted. |

A `configured` server does not make overall health `partial`. `used` and
`free` follow `dhcp.PoolOccupancy` (reserved or unexpired). `in_flight` and
`failed` are separate from that partition, so a lease can be active,
reserved, or expired and also in flight or failed. `in_flight` counts
`pxe_requested`, `booting`, `imaging`, `imaged`, and `configuring`.
`failed` counts `failed`. Both are on `GET /dashboard/summary` (the totals
next to `active`, `expired`, and `reserved`, and each `pools[]` entry) and
on each server from `GET /dhcp/servers`. Each lease
includes `menu_name` (a friendly label such as `ubuntu 24.04`, or `""`
when no menu is assigned) plus `menu`, `ipmi`, and `last_seen`. `menu`
does not include a filesystem path.

### POST Routes

| Endpoint                  | Description                                   |
|---------------------------|-----------------------------------------------|
| `/dhcp/create_server`     | Creates a new DHCP server configuration.      |
| `/dhcp/start`             | Starts a DHCP server.                         |
| `/dhcp/stop`              | Stops a DHCP server.                          |
| `/dhcp/delete`            | Deletes a DHCP server.                        |
| `/dhcp/submit_dhcp`       | Submits a new DHCP server configuration.      |
| `/dhcp/submit_reserve`    | Reserves a DHCP lease.                        |
| `/dhcp/remove_reserve`    | Removes a DHCP lease reservation.             |
| `/dhcp/delete_lease`      | Deletes a DHCP lease.                         |
| `/tftp/delete_file`       | Deletes a file from the TFTP directory.       |
| `/tftp/upload_file`       | Uploads a file to the TFTP directory.         |
| `/pxe/submit_menu`        | Submits a new PXE boot menu.                  |
| `/pxe/submit_ipmi`        | Redfish power and one-time PXE boot. Skips TLS verification unless `BMC_TLS_VERIFY=true`. Powers On when the host is off and ForceRestart only when it is on. With more than one system and no `system_id` / `BMC_SYSTEM_ID`, responds `409`. |
| `/prov/newtemplate`       | Creates a new provisioning template.          |
| `/prov/save`              | Saves a provisioning file.                    |
| `/provision/save-file`    | Saves file content via API.                   |
| `/provision/delete-file`  | Deletes a file from provision directories.    |
| `/api/syslinux/activate/{version}` | Activates an already-downloaded SYSLINUX version. |

`POST /api/syslinux/activate/{version}` does not download the archive. The
version must already be downloaded. Activation removes the boot files
currently in the TFTP directories and installs that version's extracted
BIOS and EFI files. Other downloaded versions stay downloaded.

Success is `200` with:

```json
{"success": true, "message": "Syslinux 6.03 is active", "version": "6.03"}
```

| Status | When |
|--------|------|
| `409`  | The version exists but has not been downloaded. `error.user_message` is `This version is not downloaded. Download it before activating.` |
| `404`  | No stored version has that number. |
| `500`  | Boot files are missing or activation failed. |

## CSRF

Session-authenticated `POST`, `PUT`, and `DELETE` requests must send the
per-session token and a same-host `Origin`. This includes logout,
change-password, and every htmx POST. A missing or wrong token is `403`
with `error.user_message` `CSRF token mismatch`. A missing `Origin`, a
foreign `Origin`, or `Sec-Fetch-Site: cross-site` is `403` with
`Cross-site requests are not allowed`. `POST /auth/login` and the
per-lease host callbacks (`POST /dhcp/lease/{mac}/heartbeat` and
`POST /dhcp/lease/{mac}/state`) are exempt. `GET` does not require a token.

`ignite_mode` is an HttpOnly cookie. Page theme comes from `.Mode` in the
HTML, not from reading that cookie in JavaScript.

Every HTML render that uses the base page data includes `.CSRFToken`. The
frontend sends it in either place:

- Header `X-CSRF-Token` (htmx and fetch)
- Form field `csrf_token`

Add this to the base document, without storing the token in a cookie:

```html
<meta name="csrf-token" content="{{.CSRFToken}}">
```

```html
<body hx-headers='{"X-CSRF-Token": "{{.CSRFToken}}"}'>
```

State-changing forms also need:

```html
<input type="hidden" name="csrf_token" value="{{.CSRFToken}}">
```

Fetch clients that do not use htmx read the meta tag:

```js
headers: { "X-CSRF-Token": document.querySelector('meta[name="csrf-token"]').content }
```

`POST /auth/logout` needs the token when a session cookie is still present.

## Architecture

Ignite follows a clean, modular architecture with clear separation of concerns:

### Core Packages

- **`app/`**: Application lifecycle, dependency injection, and service orchestration
- **`dhcp/`**: DHCP server implementation with lease management and reservation support
- **`tftp/`**: TFTP server for serving boot files and OS images
- **`handlers/`**: HTTP request handlers with dependency injection for web API endpoints
- **`routes/`**: HTTP routing configuration and middleware setup
- **`config/`**: Configuration management with environment variable support
- **`db/`**: Database abstraction layer using BoltDB for persistent storage

### Specialized Components

- **`osimage/`**: OS image management with download tracking and file operations
- **`syslinux/`**: SYSLINUX boot file management and menu generation
- **`ipxe/`**: iPXE configuration generation and template rendering
- **`vmtest/`**: Integration testing framework using QEMU for end-to-end PXE testing

### Testing Strategy

- **Unit Tests**: Comprehensive test coverage with mock implementations for all external dependencies
- **Integration Tests**: Real-world testing using QEMU VMs to validate PXE boot workflows
- **Mock Services**: Testify-based mocks for database operations, file system interactions, and external services

## vmtest Integration Testing

The `vmtest/` package provides comprehensive integration testing capabilities:

```bash
cd vmtest
./setup-boot-files.sh    # One-time setup: download SYSLINUX boot files

# Test boot files with QEMU built-in TFTP (validates PXE menu display)
go run vmtest.go 1

# Test full ignite integration (DHCP server, API endpoints, VM boot)
go run vmtest.go 2

# View test results and VM serial output
go run vmtest.go logs

# Clean up test artifacts
go run vmtest.go clean
```

Integration tests validate:
- SYSLINUX boot file functionality
- DHCP server API endpoints
- Ignite web interface responsiveness  
- End-to-end PXE boot workflows
- VM network boot scenarios

## Deployment Options

### **Docker Deployment (Recommended)**

#### **Option 1: Pre-built Image**
```bash
docker run -d \
  --name ignite \
  -p 8080:8080 \
  -p 69:69/udp \
  -v ignite-data:/app/data \
  -v ignite-tftp:/app/public/tftp \
  ghcr.io/chadleeshaw/ignite:v3.0.0
```

Compose and a Swarm stack (`docker stack deploy -c docker-compose.yml ignite`) mount the same named volumes. `ignite-data` is `/app/data` (`ignite.db` and the Syslinux cache). `ignite-tftp` is `/app/public/tftp`. Generated provisioning files stay in `/app/public/provision`. The image does not declare a volume for that path.

```yaml
services:
  ignite:
    image: ghcr.io/chadleeshaw/ignite:v3.0.0
    ports:
      - "8080:8080"
      - "69:69/udp"
    volumes:
      - ignite-data:/app/data
      - ignite-tftp:/app/public/tftp
volumes:
  ignite-data:
  ignite-tftp:
```

A bind mount of an empty host directory onto `/app/public/tftp` hides the boot files shipped in the image. Seed that directory from the image first, as described next to `TFTP_DIR` above. The same section has the v2.x upgrade steps: copy `ignite.db` and the TFTP directory out, recreate with `ignite-data` and `ignite-tftp`, copy them back, and `chown` both to uid 1001.

#### **Option 2: Build from Source**
```bash
# Full build (requires Go toolchain in container)
docker build -t ignite:local .

# Quick build using release binary (faster, smaller image)
docker build -f Dockerfile.release -t ignite:release .
docker run -d --name ignite -p 8080:8080 -p 69:69/udp -v ignite-data:/app/data -v ignite-tftp:/app/public/tftp ignite:release
```

### **Binary Installation**
```bash
# Download for your platform
wget https://github.com/chadleeshaw/ignite/releases/download/v3.0.0/ignite-linux-amd64
chmod +x ignite-linux-amd64
./ignite-linux-amd64

# Or use the x86_64 variant
wget https://github.com/chadleeshaw/ignite/releases/download/v3.0.0/ignite-x86_64
chmod +x ignite-x86_64
./ignite-x86_64
```

### **Development Setup**
```bash
git clone https://github.com/chadleeshaw/ignite.git
cd ignite
go mod download
go run . -mock-data    # Start with sample data
```

## Recent Improvements ✅

- ✅ **Enhanced error handling and user feedback** - Comprehensive error messages and validation throughout the application
- ✅ **IP address validation for DHCP configurations** - Input sanitization and validation for network configurations
- ✅ **TFTP directory path security improvements** - Path traversal protection and secure file operations
- ✅ **Interface standardization across services** - Consistent API patterns and dependency injection
- ✅ **Additional OS template support** - Extended template gallery with Docker, Kubernetes, and more
- ✅ **File management enhancements** - Delete functionality and template vs config distinction
- ✅ **Code optimization** - Removed unused code, optimized performance, fixed deprecated functions

## Security Model

Ignite uses a simple, network-focused security approach suitable for lab and infrastructure environments:

- **Single Admin Authentication**: Shared login credentials (`admin/admin` by default, changeable via web interface)
- **Network-Level Security**: Designed for deployment on isolated management networks (VLANs, private networks)
- **Session-Based Access**: Simple session management with HTTP-only cookies
- **Path Security**: File operations restricted to designated directories with path traversal protection

## Future Enhancements

- **Advanced monitoring** - Metrics collection and alerting for system health
- **API versioning** - Backward compatibility and gradual migration support
- **Plugin architecture** - Extensible framework for custom integrations
- **Configuration templating** - Variable substitution and environment-specific configs

## Contributing

Contributions are welcome! Please ensure all contributions include:

1. **Comprehensive tests** for new functionality
2. **Code formatting** using `gofmt -w .`
3. **Static analysis** compliance via `go vet ./...`
4. **Documentation updates** for new features or API changes

### Development Workflow

```bash
# Format and validate code
gofmt -w .
go vet ./...

# Run full test suite
go test ./...

# Run integration tests (if QEMU available)
cd vmtest && go test -v

# Build and test the application
go build -o ignite .
./ignite -mock-data  # Test with sample data
```

## License

This project is licensed under the MIT License. See the `LICENSE` file for details.

## Acknowledgments

- Inspired by GoPXE.
- User interface built with Tailwind CSS and daisyUI.
- Frontend help from Google Jules
- Backend help from Claude