package handlers

import (
	"ignite/config"
	"ignite/dhcp"
	"ignite/osimage"
)

// Container holds dependencies for handlers
type Container struct {
	ServerService  dhcp.ServerService
	LeaseService   dhcp.LeaseService
	OSImageService osimage.OSImageService
	Config         *config.Config
}
