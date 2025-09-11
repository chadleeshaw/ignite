package app

import (
	"fmt"
	"ignite/config"
	"ignite/db"
	"ignite/dhcp"
	"ignite/osimage"
)

// Container holds all application dependencies
type Container struct {
	Config             *config.Config
	Database           db.Database
	ServerRepo         dhcp.ServerRepository
	LeaseRepo          dhcp.LeaseRepository
	ServerService      dhcp.ServerService
	LeaseService       dhcp.LeaseService
	OSImageRepo        osimage.OSImageRepository
	DownloadStatusRepo osimage.DownloadStatusRepository
	OSImageService     osimage.OSImageService
}

// NewContainer creates and wires up all dependencies
func NewContainer() (*Container, error) {
	// Load configuration
	cfg, err := config.LoadDefault()
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	// Initialize database
	database, err := db.NewBoltDB(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	// Create repositories
	serverRepo := dhcp.NewBoltServerRepository(database, cfg.DB.Bucket+"_servers")
	leaseRepo := dhcp.NewBoltLeaseRepository(database, cfg.DB.Bucket+"_leases")
	osImageRepo := osimage.NewOSImageRepository(database)
	downloadStatusRepo := osimage.NewDownloadStatusRepository(database)

	// Create services
	serverService := dhcp.NewDHCPServerService(serverRepo, leaseRepo)
	leaseService := dhcp.NewDHCPLeaseService(leaseRepo, serverRepo)
	osImageService := osimage.NewOSImageService(osImageRepo, downloadStatusRepo, cfg)

	return &Container{
		Config:             cfg,
		Database:           database,
		ServerRepo:         serverRepo,
		LeaseRepo:          leaseRepo,
		ServerService:      serverService,
		LeaseService:       leaseService,
		OSImageRepo:        osImageRepo,
		DownloadStatusRepo: downloadStatusRepo,
		OSImageService:     osImageService,
	}, nil
}

// Close closes all resources held by the container
func (c *Container) Close() error {
	if c.Database != nil {
		return c.Database.Close()
	}
	return nil
}
