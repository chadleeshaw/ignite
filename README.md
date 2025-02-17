
![Ignite Small](https://platformops.s3.us-west-2.amazonaws.com/images/Ignite_small.png)

# ignite

An all-in-one Golang implementation for network booting with DHCP, TFTP, Web Server, and backend APIs.

## Overview

`ignite` is designed to simplify the process of network booting by integrating:

- **DHCP Server**: Manages IP address assignments.
- **TFTP Server**: Serves files for PXE booting.
- **Web Server**: Provides an interface for managing DHCP leases and boot options.
- **Backend APIs**: Supports operations for server management and configuration.

### Features

- **PXE Boot Menu Templating**: Automatically templates boot menus based on DHCP leases for:
  - Ubuntu
  - NixOS
  - Redhat

- **Redfish API Integration**: Allows for remote server reboot into PXE boot mode.

### Inspiration

This project was inspired by [GoPXE](https://github.com/ppetko/GoPXE), but extends the functionality with additional features for a more comprehensive network boot solution.

## Usage

### Prerequisites

- Golang installed on your system
- Basic understanding of network booting concepts

### Setup

1. **Clone the repository:**
   ```sh
   git clone git@github.com:your-username/ignite.git
   cd ignite
   ```

2. **Build the project:**
   ```sh
   go build -o ignite main.go
   ```

3. **Run the server:**
   ```sh
   ./ignite
   ```

## Configuration


- Configuration files are located in the `config/` directory. Adjust as necessary for your network setup.
- You can also configure `ignite` using environment variables:

  - `DB_PATH`: Path to store the database file. Default is "./"
  - `DB_FILE`: Name of the database file. Default is "ignite.db"
  - `DB_BUCKET`: Database bucket name. Default is "dhcp"
  - `TFTP_DIR`: Directory where TFTP server serves files from. Default is "./public/tftp"
  - `HTTP_DIR`: Directory where http server serves files from.  Default is "./public/http"
  - `HTTP_PORT`: Port on which the HTTP server listens. Default is "8080"
  - `PROV_DIR`: Directory where provisioning raw and rendered templates exist.  Default is "./public/provision"

  Example of setting environment variables in a Unix-like system:
  
  ```bash
  export DB_PATH=./
  export DB_FILE=ignite.db
  export DB_BUCKET=ignite
  export TFTP_DIR=./public/tftp
  export HTTP_DIR=./public/http
  export HTTP_PORT=8080
  export PROV_DIR=./public/provision
  ```

## Contributing

ignite is an open-source project, and contributions are welcome! Here's how you can help:

- Submit Pull Requests: Fix bugs, add features, or improve documentation.
- Report Issues: Found a bug? Let us know through the issues page.
- Suggest Features: Have an idea? Open a new issue or discuss it in a current one.

Please ensure your pull request adheres to our coding standards:

- Use Go conventions for coding style.
- Write clear commit messages.
- Add tests for new code or bug fixes.

## License

MIT License - See the LICENSE file for details.

## Acknowledgments

- GoPXE (https://github.com/ppetko/GoPXE) for the initial inspiration.

---

Feel free to open an issue or join our discussions for any questions or suggestions regarding ignite.