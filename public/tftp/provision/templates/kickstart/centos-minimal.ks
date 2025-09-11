#version=DEVEL
# CentOS 8 Minimal Installation

# System authorization information
auth --enableshadow --passalgo=sha512

# Use network installation
url --url="http://mirror.centos.org/centos/8/BaseOS/x86_64/os/"

# Use text install
text

# Run the Setup Agent on first boot
firstboot --enable

# Keyboard layouts
keyboard --vckeymap=us --xlayouts='us'

# System language
lang en_US.UTF-8

# Network information
network --bootproto=dhcp --device=link --onboot=on --ipv6=auto --activate

# Root password (change this!)
rootpw --iscrypted $6$SALT$HASH

# System timezone
timezone America/New_York --isUtc

# System bootloader configuration
bootloader --location=mbr

# Clear the Master Boot Record
zerombr

# Partition clearing information
clearpart --all --initlabel

# Disk partitioning information
autopart

%packages
@^minimal-environment
@core

%end

%post
# Configure SSH
systemctl enable sshd
firewall-cmd --add-service=ssh --permanent

# Install additional packages
dnf install -y vim htop curl git

%end
