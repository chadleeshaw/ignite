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
	container     *Container
	httpServer    *http.Server
	tftpServer    *tftp.Server
	staticFS      embed.FS
	staticHandler *handlers.StaticHandlers
}

// NewApplicationWithStatic creates a new application instance with embedded static files
func NewApplicationWithStatic(staticFS embed.FS) (*Application, error) {
	container, err := NewContainer()
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	// Create static file handler
	staticHandler := handlers.NewStaticHandlers(staticFS, container.Config.HTTP.Dir)

	return &Application{
		container:     container,
		staticFS:      staticFS,
		staticHandler: staticHandler,
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

	// Setup HTTP handlers with dependency injection
	handlerContainer := &handlers.Container{
		ServerService:  a.container.ServerService,
		LeaseService:   a.container.LeaseService,
		OSImageService: a.container.OSImageService,
		Config:         a.container.Config,
	}

	// Create HTTP router with injected dependencies and static file handling
	router := routes.SetupWithContainerAndStatic(handlerContainer, a.staticHandler)
	log.Printf("Embedded HTTP server configured")

	// Create HTTP server
	a.httpServer = &http.Server{
		Addr:    ":" + a.container.Config.HTTP.Port,
		Handler: router,
	}

	// Start HTTP server
	go func() {
		log.Printf("HTTP API server started on port %s", a.container.Config.HTTP.Port)
		if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	return nil
}

// Rest of the Application methods remain the same...
func (a *Application) Stop() error {
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
	<-quit
	log.Println("Shutting down application...")

	if err := a.Stop(); err != nil {
		return fmt.Errorf("failed to stop application: %w", err)
	}

	log.Println("Application stopped")
	return nil
}

// GetContainer returns the application's container for access to services
func (a *Application) GetContainer() *handlers.Container {
	return &handlers.Container{
		ServerService:  a.container.ServerService,
		LeaseService:   a.container.LeaseService,
		OSImageService: a.container.OSImageService,
		Config:         a.container.Config,
	}
}
