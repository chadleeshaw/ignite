#!/bin/bash

# Setup Boot Files for Ignite PXE Server
# Downloads and configures necessary boot files for BIOS and UEFI PXE boot

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log() { echo -e "${BLUE}[INFO]${NC} $1"; }
success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Configuration - local TFTP directory for vmtest
TFTP_DIR="../public/tftp"
SYSLINUX_VERSION="6.03"
SYSLINUX_URL="https://mirrors.kernel.org/pub/linux/utils/boot/syslinux/syslinux-${SYSLINUX_VERSION}.tar.gz"

# Create directories
setup_directories() {
    log "Setting up directory structure..."
    
    mkdir -p "$TFTP_DIR/boot-bios"
    mkdir -p "$TFTP_DIR/boot-efi"
    mkdir -p "$TFTP_DIR/pxelinux.cfg"
    
    success "Directory structure created"
}

# Download and extract SYSLINUX
download_syslinux() {
    if [ -f "/tmp/syslinux-${SYSLINUX_VERSION}.tar.gz" ]; then
        log "SYSLINUX archive already exists"
    else
        log "Downloading SYSLINUX ${SYSLINUX_VERSION}..."
        curl -L -o "/tmp/syslinux-${SYSLINUX_VERSION}.tar.gz" "$SYSLINUX_URL"
    fi
    
    log "Extracting SYSLINUX..."
    tar -xzf "/tmp/syslinux-${SYSLINUX_VERSION}.tar.gz" -C /tmp/
    
    success "SYSLINUX downloaded and extracted"
}

# Setup BIOS boot files
setup_bios_files() {
    log "Setting up BIOS boot files..."
    
    SYSLINUX_DIR="/tmp/syslinux-${SYSLINUX_VERSION}"
    
    # Copy BIOS boot files
    cp "$SYSLINUX_DIR/bios/core/pxelinux.0" "$TFTP_DIR/boot-bios/"
    cp "$SYSLINUX_DIR/bios/com32/elflink/ldlinux/ldlinux.c32" "$TFTP_DIR/boot-bios/"
    cp "$SYSLINUX_DIR/bios/com32/lib/libcom32.c32" "$TFTP_DIR/boot-bios/"
    cp "$SYSLINUX_DIR/bios/com32/libutil/libutil.c32" "$TFTP_DIR/boot-bios/"
    cp "$SYSLINUX_DIR/bios/com32/menu/vesamenu.c32" "$TFTP_DIR/boot-bios/"
    cp "$SYSLINUX_DIR/bios/com32/menu/menu.c32" "$TFTP_DIR/boot-bios/"
    
    success "BIOS boot files setup complete"
}

# Setup EFI boot files
setup_efi_files() {
    log "Setting up EFI boot files..."
    
    SYSLINUX_DIR="/tmp/syslinux-${SYSLINUX_VERSION}"
    
    # Copy EFI boot files
    cp "$SYSLINUX_DIR/efi64/efi/syslinux.efi" "$TFTP_DIR/boot-efi/"
    cp "$SYSLINUX_DIR/efi64/com32/elflink/ldlinux/ldlinux.e64" "$TFTP_DIR/boot-efi/"
    cp "$SYSLINUX_DIR/efi64/com32/lib/libcom32.c32" "$TFTP_DIR/boot-efi/"
    cp "$SYSLINUX_DIR/efi64/com32/libutil/libutil.c32" "$TFTP_DIR/boot-efi/"
    cp "$SYSLINUX_DIR/efi64/com32/menu/vesamenu.c32" "$TFTP_DIR/boot-efi/"
    cp "$SYSLINUX_DIR/efi64/com32/menu/menu.c32" "$TFTP_DIR/boot-efi/"
    
    success "EFI boot files setup complete"
}

# Create default PXE configuration
create_pxe_config() {
    log "Creating PXE boot configuration..."
    
    cat > "$TFTP_DIR/pxelinux.cfg/default" << 'EOF'
DEFAULT vesamenu.c32
TIMEOUT 300
ONTIMEOUT local

MENU TITLE Ignite PXE Boot Menu
MENU BACKGROUND pxelinux.cfg/background.png
MENU COLOR border 0 #ffffffff #00000000 std
MENU COLOR title 0 #ffffffff #00000000 std

LABEL local
    MENU LABEL Boot from local disk
    LOCALBOOT 0

LABEL test
    MENU LABEL Test - Memory Test
    KERNEL memtest86+
    INITRD memtest86+.bin

LABEL ubuntu
    MENU LABEL Ubuntu 22.04 Live
    KERNEL ubuntu/vmlinuz
    APPEND initrd=ubuntu/24.04/initrd.img boot=casper netboot=nfs nfsroot=172.20.0.1:/path/to/ubuntu quiet splash

LABEL centos
    MENU LABEL CentOS Stream 9
    KERNEL centos/vmlinuz
    APPEND initrd=centos/9/initrd.img inst.repo=http://172.20.0.1:8080/centos/ quiet

LABEL nixos
    MENU LABEL NixOS
    KERNEL nixos/bzImage
    APPEND initrd=nixos/24.05/initrd init=/nix/store/.../init boot.shell_on_fail console=ttyS0

EOF
    
    success "PXE configuration created"
}

# Create iPXE configuration for advanced features
create_ipxe_config() {
    log "Creating iPXE configuration..."
    
    cat > "$TFTP_DIR/boot.ipxe" << 'EOF'
#!ipxe

# iPXE Boot Script for Ignite
dhcp
set server-ip 172.20.0.1
set base-url http://${server-ip}:8080

# Show network info
echo Network configuration:
echo IP: ${ip}
echo Gateway: ${gateway}
echo DNS: ${dns}
echo

:menu
menu Ignite PXE Boot Menu
item --gap -- Operating Systems:
item ubuntu Ubuntu 22.04 Live
item centos CentOS Stream 9  
item nixos NixOS
item --gap -- Tools:
item memtest Memory Test
item local Boot from local disk
item shell iPXE Shell
item exit Exit to BIOS
choose selected || goto exit

goto ${selected}

:ubuntu
echo Booting Ubuntu 22.04...
kernel ${base-url}/ubuntu/24.04/vmlinuz initrd=initrd.img boot=casper netboot=nfs nfsroot=${server-ip}:/path/to/ubuntu quiet splash
initrd ${base-url}/ubuntu/24.04/initrd.img
boot

:centos
echo Booting CentOS Stream 9...
kernel ${base-url}/centos/9/vmlinuz initrd=initrd.img inst.repo=http://${server-ip}:8080/centos/ quiet
initrd ${base-url}/centos/9/initrd.img
boot

:nixos
echo Booting NixOS...
kernel ${base-url}/nixos/24.05/bzImage initrd=initrd init=/nix/store/.../init boot.shell_on_fail console=ttyS0
initrd ${base-url}/nixos/24.05/initrd
boot

:memtest
echo Loading memory test...
kernel ${base-url}/tools/memtest86+
boot

:local
echo Booting from local disk...
sanboot --no-describe --drive 0x80

:shell
echo Entering iPXE shell...
shell

:exit
exit
EOF
    
    success "iPXE configuration created"
}

# Verify boot file setup
verify_setup() {
    log "Verifying boot file setup..."
    
    local files_ok=true
    
    # Check BIOS files
    if [ ! -f "$TFTP_DIR/boot-bios/pxelinux.0" ]; then
        error "Missing: boot-bios/pxelinux.0"
        files_ok=false
    fi
    
    # Check EFI files
    if [ ! -f "$TFTP_DIR/boot-efi/syslinux.efi" ]; then
        error "Missing: boot-efi/syslinux.efi"
        files_ok=false
    fi
    
    # Check config
    if [ ! -f "$TFTP_DIR/pxelinux.cfg/default" ]; then
        error "Missing: pxelinux.cfg/default"
        files_ok=false
    fi
    
    if [ "$files_ok" = true ]; then
        success "All boot files are properly set up"
        log "File sizes:"
        du -sh "$TFTP_DIR/boot-bios"/* "$TFTP_DIR/boot-efi"/* 2>/dev/null | head -10
    else
        error "Some boot files are missing"
        return 1
    fi
}

# Show usage
show_help() {
    cat << EOF
Setup Boot Files for Ignite PXE Server

Usage: $0 [OPTIONS]

Options:
    -h, --help     Show this help message
    -f, --force    Force re-download of files
    --skip-efi     Skip EFI setup (BIOS only)
    --skip-bios    Skip BIOS setup (EFI only)

This script will:
1. Download SYSLINUX ${SYSLINUX_VERSION}
2. Extract and copy BIOS boot files (pxelinux.0, etc.)
3. Extract and copy EFI boot files (syslinux.efi, etc.)
4. Create default PXE boot configuration
5. Create iPXE boot script

EOF
}

# Parse arguments
FORCE=false
SKIP_EFI=false
SKIP_BIOS=false

while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            show_help
            exit 0
            ;;
        -f|--force)
            FORCE=true
            shift
            ;;
        --skip-efi)
            SKIP_EFI=true
            shift
            ;;
        --skip-bios)
            SKIP_BIOS=true
            shift
            ;;
        *)
            warn "Unknown option: $1"
            shift
            ;;
    esac
done

# Main execution
log "Starting boot files setup for Ignite PXE server..."

# Clean up if forced
if [ "$FORCE" = true ]; then
    log "Force mode: cleaning up existing files..."
    rm -f "/tmp/syslinux-${SYSLINUX_VERSION}.tar.gz"
    rm -rf "/tmp/syslinux-${SYSLINUX_VERSION}"
fi

# Setup
setup_directories
download_syslinux

if [ "$SKIP_BIOS" != true ]; then
    setup_bios_files
fi

if [ "$SKIP_EFI" != true ]; then
    setup_efi_files
fi

create_pxe_config
create_ipxe_config
verify_setup

success "Boot files setup completed successfully!"
