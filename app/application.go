package app

import (
	"context"
	"embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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
		SyslinuxService: container.SyslinuxService,
		IPXEService:     container.IPXEService,
		Config:          container.Config,
	}

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
	// Start TFTP server
	a.tftpServer = tftp.NewServer(a.container.Config.TFTP.Dir)
	if err := a.tftpServer.Start(); err != nil {
		return fmt.Errorf("failed to start TFTP server: %w", err)
	}
	log.Printf("TFTP server started on port 69, serving from %s", a.container.Config.TFTP.Dir)

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

// Rest of the Application methods remain the same...
func (a *Application) Stop() error {
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
