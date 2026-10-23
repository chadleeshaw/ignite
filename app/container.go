package app

import (
	"fmt"
	"ignite/config"
	"ignite/db"
	"ignite/dhcp"
	"ignite/handlers"
	"ignite/ipxe"
	"ignite/osimage"
	"ignite/osimage/catalog"
	"ignite/syslinux"
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
	Catalog            catalog.Catalog
	SyslinuxRepo       syslinux.Repository
	SyslinuxService    syslinux.Service
	IPXEService        *ipxe.Service
	Prefs              handlers.PreferenceStore
}

// NewContainer creates and wires up all dependencies
func NewContainer() (c *Container, err error) {
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
	// Don't leak the database if a later construction step fails.
	defer func() {
		if err != nil {
			_ = database.Close()
		}
	}()

	// Create repositories
	serverRepo := dhcp.NewBoltServerRepository(database, cfg.DB.Bucket+"_servers")
	leaseRepo := dhcp.NewBoltLeaseRepository(database, cfg.DB.Bucket+"_leases")
	osImageRepo, err := osimage.NewOSImageRepository(database)
	if err != nil {
		return nil, fmt.Errorf("failed to create os image repository: %w", err)
	}
	downloadStatusRepo, err := osimage.NewDownloadStatusRepository(database)
	if err != nil {
		return nil, fmt.Errorf("failed to create download status repository: %w", err)
	}
	syslinuxRepo, err := syslinux.NewBoltRepository(database)
	if err != nil {
		return nil, fmt.Errorf("failed to create syslinux repository: %w", err)
	}

	// Create services
	serverService := dhcp.NewDHCPServerService(serverRepo, leaseRepo)
	leaseService := dhcp.NewDHCPLeaseService(leaseRepo, serverRepo)
	httpClient := catalog.NewClient()
	store, err := catalog.NewBoltStore(database)
	if err != nil {
		return nil, fmt.Errorf("failed to create OS catalog store: %w", err)
	}
	cat, err := catalog.New(catalog.Options{
		Sources:   catalog.DefaultSources(httpClient),
		Lifecycle: catalog.EndOfLife{Client: httpClient},
		Store:     store,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load OS catalog: %w", err)
	}
	osImageService := osimage.NewOSImageService(osImageRepo, downloadStatusRepo, cfg, cat)
	cacheDir, err := config.ResolveSyslinuxCacheDir(cfg.DB.DBPath, cfg.Syslinux.CacheDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve syslinux cache: %w", err)
	}
	cfg.Syslinux.CacheDir = cacheDir
	slCfg := syslinux.GetDefaultConfig()
	slCfg.TFTPDir = cfg.TFTP.Dir
	slCfg.TempDir = cacheDir
	syslinuxService := syslinux.NewService(syslinuxRepo, slCfg)
	ipxeService := ipxe.NewService(cfg, osImageService)
	ipxeService.UseCatalog(cat)
	prefs, err := handlers.NewBoltPreferenceStore(database, cfg.DB.Bucket+"_prefs")
	if err != nil {
		return nil, fmt.Errorf("failed to create preference store: %w", err)
	}

	c = &Container{
		Config:             cfg,
		Database:           database,
		ServerRepo:         serverRepo,
		LeaseRepo:          leaseRepo,
		ServerService:      serverService,
		LeaseService:       leaseService,
		OSImageRepo:        osImageRepo,
		DownloadStatusRepo: downloadStatusRepo,
		OSImageService:     osImageService,
		Catalog:            cat,
		SyslinuxRepo:       syslinuxRepo,
		SyslinuxService:    syslinuxService,
		IPXEService:        ipxeService,
		Prefs:              prefs,
	}
	return c, nil
}

// Close closes all resources held by the container
func (c *Container) Close() error {
	if c.Database != nil {
		return c.Database.Close()
	}
	return nil
}
