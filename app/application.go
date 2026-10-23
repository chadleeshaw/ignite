package app

import (
	"context"
	"embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"ignite/config"
	"ignite/dhcp"
	"ignite/handlers"
	"ignite/routes"
	"ignite/tftp"
)

// Application represents the main application with embedded static files
type Application struct {
	container        *Container
	handlerContainer *handlers.Container
	httpServer       *http.Server
	httpErrCh        chan error
	tftpServer       *tftp.Server
	staticFS         embed.FS
	staticHandler    *handlers.StaticHandlers

	// rootCancel cancels the process-wide context passed to background work.
	// Stop calls it so the lease maintainer observes cancellation.
	rootCancel context.CancelFunc
	maintainer *dhcp.LeaseMaintainer
}

// NewApplicationWithStatic creates a new application instance with embedded static files
func NewApplicationWithStatic(staticFS embed.FS) (*Application, error) {
	container, err := NewContainer()
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	// Create static file handler
	staticHandler := handlers.NewStaticHandlers(staticFS, container.Config.HTTP.Dir)

	// Build the handler container once and reuse it for Start/GetContainer.
	handlerContainer := &handlers.Container{
		ServerService:   container.ServerService,
		LeaseService:    container.LeaseService,
		OSImageService:  container.OSImageService,
		Catalog:         container.Catalog,
		SyslinuxService: container.SyslinuxService,
		IPXEService:     container.IPXEService,
		Config:          container.Config,
		Prefs:           container.Prefs,
	}
	handlers.SetPreferenceStore(container.Prefs)
	handlers.SetPageHealth(func(r *http.Request) string {
		return handlers.NewStatusHandlers(handlerContainer).CurrentOverallStatus(r)
	})

	return &Application{
		container:        container,
		handlerContainer: handlerContainer,
		httpErrCh:        make(chan error, 1),
		staticFS:         staticFS,
		staticHandler:    staticHandler,
	}, nil
}

// Start starts all application services including static file serving
func (a *Application) Start() error {
	rootCtx, rootCancel := context.WithCancel(context.Background())
	a.rootCancel = rootCancel
	a.maintainer = dhcp.NewLeaseMaintainer(
		a.container.LeaseService,
		dhcp.DefaultMaintenanceInterval,
		dhcp.DefaultOfflineThreshold,
	)
	a.maintainer.Start(rootCtx)

	// Start TFTP server. Record the outcome so status never stays "unknown"
	// after a start attempt, including a failed bind.
	tftpRoot, err := config.ResolveTFTPRoot(a.container.Config.TFTP.Dir)
	if err != nil {
		handlers.SetTFTPRunning(false)
		a.stopMaintenance()
		return fmt.Errorf("failed to resolve TFTP root: %w", err)
	}
	for _, line := range tftpRoot.StartupLines() {
		log.Print(line)
	}
	a.container.Config.TFTP.Dir = tftpRoot.Path
	handlers.SetTFTPRoot(tftpRoot.Path)
	handlers.SetTFTPDisplayRoot(tftpRoot.Display)
	if a.container.SyslinuxService != nil {
		slCfg := a.container.SyslinuxService.GetConfig()
		slCfg.TFTPDir = tftpRoot.Path
		_ = a.container.SyslinuxService.UpdateConfig(rootCtx, slCfg)
		log.Printf("Syslinux cache: %s", slCfg.TempDir)
	}
	tftpPort, err := strconv.Atoi(a.container.Config.TFTP.Port)
	if err != nil {
		handlers.SetTFTPRunning(false)
		a.stopMaintenance()
		return fmt.Errorf("invalid TFTP port %q: %w", a.container.Config.TFTP.Port, err)
	}
	a.tftpServer = tftp.NewServerWithPort(tftpRoot.Path, tftpPort)
	if err := a.tftpServer.Start(); err != nil {
		handlers.SetTFTPRunning(false)
		a.stopMaintenance()
		return fmt.Errorf("failed to start TFTP server: %w", err)
	}
	handlers.SetTFTPRunning(true)
	log.Printf("TFTP server started on port %d, serving from %s", tftpPort, a.container.Config.TFTP.Dir)
	if a.container.Config.DHCP.Enabled {
		dhcp.SetListenersEnabled(true)
	} else {
		dhcp.SetListenersEnabled(false)
		log.Print("DHCP is disabled")
	}
	log.Print(config.BMCTLSModeLog(a.container.Config.BMC.TLSVerify))

	// Every page gets the same overall service word as the Overview pill.
	handlers.SetPageHealth(func(r *http.Request) string {
		return handlers.NewStatusHandlers(a.handlerContainer).CurrentOverallStatus(r)
	})

	// Create HTTP router with injected dependencies and static file handling
	router := routes.SetupWithContainerAndStatic(a.handlerContainer, a.staticHandler)
	log.Printf("Embedded HTTP server configured")

	// Create HTTP server
	a.httpServer = &http.Server{
		Addr:    ":" + a.container.Config.HTTP.Port,
		Handler: router,
	}

	// Start HTTP server. Failures are propagated via httpErrCh instead of
	// killing the process from inside the goroutine.
	go func() {
		log.Printf("HTTP API server started on port %s", a.container.Config.HTTP.Port)
		if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			select {
			case a.httpErrCh <- err:
			default:
			}
		}
	}()

	return nil
}

// stopMaintenance cancels the application context and waits for the lease
// maintainer to finish. It is safe to call more than once.
func (a *Application) stopMaintenance() {
	if a.rootCancel != nil {
		a.rootCancel()
		a.rootCancel = nil
	}
	if a.maintainer != nil {
		a.maintainer.Stop()
		a.maintainer = nil
	}
}

func (a *Application) Stop() error {
	// Cancel background work first and wait for the lease maintainer to
	// leave the database alone before the rest of shutdown closes it.
	a.stopMaintenance()

	// Stop any running DHCP servers first
	if a.container.ServerService != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		servers, err := a.container.ServerService.GetAllServers(ctx)
		if err != nil {
			log.Printf("Error listing DHCP servers for shutdown: %v", err)
		} else {
			for _, server := range servers {
				if server.Started {
					if err := a.container.ServerService.StopServer(ctx, server.ID); err != nil {
						log.Printf("Error stopping DHCP server %s: %v", server.ID, err)
					}
				}
			}
		}
		cancel()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if a.httpServer != nil {
		if err := a.httpServer.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down HTTP server: %v", err)
		}
	}

	if a.tftpServer != nil {
		a.tftpServer.Stop()
		handlers.SetTFTPRunning(false)
	}

	if err := a.container.Close(); err != nil {
		log.Printf("Error closing container: %v", err)
	}

	return nil
}

func (a *Application) Run() error {
	if err := a.Start(); err != nil {
		return fmt.Errorf("failed to start application: %w", err)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	log.Println("Application started. Press Ctrl+C to stop.")
	select {
	case <-quit:
		log.Println("Shutting down application...")
	case err := <-a.httpErrCh:
		// The HTTP server died on its own; shut everything else down and
		// report the failure instead of log.Fatalf-ing from a goroutine.
		if stopErr := a.Stop(); stopErr != nil {
			log.Printf("Error during shutdown after HTTP failure: %v", stopErr)
		}
		return fmt.Errorf("HTTP server failed: %w", err)
	}

	if err := a.Stop(); err != nil {
		return fmt.Errorf("failed to stop application: %w", err)
	}

	log.Println("Application stopped")
	return nil
}

// GetContainer returns the application's shared handler container
func (a *Application) GetContainer() *handlers.Container {
	return a.handlerContainer
}
