package handlers

import (
	"ignite/config"
	"ignite/dhcp"
	"ignite/ipxe"
	"ignite/osimage"
	"ignite/osimage/catalog"
	"ignite/syslinux"
)

// Container holds dependencies for handlers
type Container struct {
	ServerService   dhcp.ServerService
	LeaseService    dhcp.LeaseService
	OSImageService  osimage.OSImageService
	Catalog         catalog.Catalog
	SyslinuxService syslinux.Service
	IPXEService     *ipxe.Service
	Config          *config.Config
	Prefs           PreferenceStore
}
