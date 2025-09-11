package handlers

import (
	"ignite/config"
	"ignite/dhcp"
)

// Container holds dependencies for handlers
type Container struct {
	ServerService dhcp.ServerService
	LeaseService  dhcp.LeaseService
	Config        *config.Config
}
