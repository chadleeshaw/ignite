// Seed a database with the sample DHCP, OS image, and Syslinux records the
// README screenshots are taken from. The server must not have this database
// open. Configuration comes from the usual environment variables (DB_PATH,
// DB_FILE, TFTP_DIR, PROV_DIR).
package main

import (
	"context"
	"fmt"
	"net"
	"time"

	"ignite/app"
	"ignite/cli"
	"ignite/dhcp"
	"ignite/osimage"
	"ignite/syslinux"
)

func main() {
	ctx := context.Background()
	container, err := app.NewContainer()
	if err != nil {
		fatal(err)
	}
	defer container.Close()

	mock := cli.NewMockDataService(container.ServerService, container.LeaseService)
	if err := mock.ClearAllData(ctx); err != nil {
		fatal(err)
	}
	if err := mock.PopulateMockData(ctx); err != nil {
		fatal(err)
	}
	if err := decorateLeases(ctx, container.LeaseService, container.LeaseRepo); err != nil {
		fatal(err)
	}
	if err := seedOSImages(ctx, container.OSImageRepo); err != nil {
		fatal(err)
	}
	if err := seedSyslinux(ctx, container.SyslinuxRepo); err != nil {
		fatal(err)
	}
	fmt.Println("seeded screenshot database")
}

func fatal(err error) {
	panic(err)
}

func decorateLeases(ctx context.Context, leases dhcp.LeaseService, repo dhcp.LeaseRepository) error {
	type paint struct {
		reserved bool
		state    dhcp.LeaseState
		seen     time.Duration
		menu     dhcp.BootMenu
		ipmi     dhcp.IPMI
	}
	now := time.Now()
	looks := map[string]paint{
		"00:11:22:33:44:55": {
			reserved: false,
			state:    dhcp.StatePXERequested,
			seen:     3 * time.Minute,
			menu: dhcp.BootMenu{
				Filename: "ubuntu/24.04/vmlinuz", OS: "ubuntu", Version: "24.04",
				TemplateType: "cloud-init", TemplateName: "default", Hostname: "workstation-01",
			},
		},
		"AA:BB:CC:DD:EE:FF": {
			reserved: true,
			state:    dhcp.StateImaging,
			seen:     45 * time.Second,
			menu: dhcp.BootMenu{
				Filename: "ubuntu/22.04/vmlinuz", OS: "ubuntu", Version: "22.04",
				TemplateType: "autoinstall", Hostname: "lab-02",
			},
			ipmi: dhcp.IPMI{Username: "root", IP: net.ParseIP("192.168.1.201"), PXEBoot: true},
		},
		"00:11:22:33:44:66": {
			reserved: true,
			state:    dhcp.StateComplete,
			seen:     2 * time.Hour,
			menu: dhcp.BootMenu{
				Filename: "centos/stream9/vmlinuz", OS: "centos", Version: "9",
				TemplateType: "kickstart", Hostname: "db-server",
			},
			ipmi: dhcp.IPMI{Username: "ipmi-admin", IP: net.ParseIP("192.168.1.202")},
		},
		"11:22:33:44:55:66": {
			reserved: false,
			state:    dhcp.StateAssigned,
			seen:     12 * time.Minute,
		},
		"77:88:99:AA:BB:CC": {
			reserved: true,
			state:    dhcp.StateOffline,
			seen:     36 * time.Hour,
		},
	}
	for mac, paint := range looks {
		lease, err := leases.GetLeaseByMAC(ctx, mac)
		if err != nil {
			return fmt.Errorf("lease %s: %w", mac, err)
		}
		lease.Reserved = paint.reserved
		lease.State = paint.state
		lease.StateUpdatedAt = now.Add(-paint.seen)
		lease.LastSeen = now.Add(-paint.seen)
		lease.Menu = paint.menu
		lease.IPMI = paint.ipmi
		// UpdateLease keeps a newer LastSeen, so the painted heartbeat would
		// be discarded and every row would read "just now". The repository
		// write is the seed's stored row.
		if err := repo.Save(ctx, lease); err != nil {
			return err
		}
	}
	return nil
}

func seedOSImages(ctx context.Context, repo osimage.OSImageRepository) error {
	when := time.Now().Add(-72 * time.Hour)
	images := []*osimage.OSImage{
		{
			ID: "ubuntu-2404", OS: "ubuntu", Version: "24.04", Architecture: "x86_64",
			KernelPath: "ubuntu/24.04/vmlinuz", InitrdPath: "ubuntu/24.04/initrd",
			KernelSize: 14_680_064, InitrdSize: 81_264_640, Active: true, CreatedAt: when,
		},
		{
			ID: "debian-12", OS: "debian", Version: "12.6", Architecture: "x86_64",
			KernelPath: "debian/12/vmlinuz", InitrdPath: "debian/12/initrd.gz",
			KernelSize: 8_126_464, InitrdSize: 52_428_800, CreatedAt: when.Add(6 * time.Hour),
		},
		{
			ID: "fedora-40", OS: "fedora", Version: "40", Architecture: "x86_64",
			KernelPath: "fedora/40/vmlinuz", InitrdPath: "fedora/40/initrd.img",
			KernelSize: 13_631_488, InitrdSize: 104_857_600, CreatedAt: when.Add(24 * time.Hour),
			EOL: true, EOLDate: "2025-05-13",
		},
	}
	for _, image := range images {
		if err := repo.Save(ctx, image); err != nil {
			return err
		}
	}
	return nil
}

func seedSyslinux(ctx context.Context, repo syslinux.Repository) error {
	downloaded := time.Now().Add(-10 * 24 * time.Hour)
	versions := []*syslinux.SyslinuxVersion{
		{
			ID: "syslinux-6.03", Version: "6.03", Size: 7_340_032,
			Downloaded: true, Active: true, DownloadedAt: &downloaded, CreatedAt: downloaded,
		},
		{
			ID: "syslinux-6.04", Version: "6.04-pre1", Size: 7_512_064,
			Downloaded: true, DownloadedAt: &downloaded,
			CreatedAt: time.Now().Add(-48 * time.Hour),
		},
	}
	for _, version := range versions {
		if err := repo.SaveVersion(ctx, version); err != nil {
			return err
		}
	}
	return nil
}
