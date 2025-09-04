package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"ignite/config"
	"ignite/db"
	"ignite/dhcp"
	"ignite/internal/errors"
	"ignite/internal/handlers"
	"ignite/internal/middleware"
	"ignite/routes"
	"ignite/tftp"

	"github.com/gorilla/mux"
)

// DatabaseStore interface for better testability
type DatabaseStore interface {
	GetKV(bucket string, key []byte) ([]byte, error)
	PutKV(bucket string, key, value []byte) error
	DeleteKV(bucket string, key []byte) error
	GetAllKV(bucket string) (map[string][]byte, error)
	Close() error
}

// DHCPService interface
type DHCPService interface {
	Start() error
	Stop() error
	GetAllLeases() (map[string]dhcp.Lease, error)
}

// TFTPService interface
type TFTPService interface {
	Start() error
	Stop()
}

// App represents the main application with all dependencies
type App struct {
	Config     *config.Config
	DB         DatabaseStore
	Logger     *slog.Logger
	HTTPServer *http.Server
	TFTPServer TFTPService
	Router     *mux.Router
	Handlers   *handlers.Handlers
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewApp creates a new application instance with all dependencies injected
func NewApp() (*App, error) {
	// Initialize logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Load and validate configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, errors.Wrap(err, "load_config")
	}

	logger.Info("Configuration loaded successfully")

	// Initialize database
	database, err := initDatabase(cfg, logger)
	if err != nil {
		return nil, errors.Wrap(err, "init_database")
	}

	logger.Info("Database initialized successfully")

	// Initialize TFTP server
	tftpServer := tftp.NewServer(cfg.TFTP.Dir, logger)

	ctx, cancel := context.WithCancel(context.Background())

	app := &App{
		Config:     cfg,
		DB:         database,
		Logger:     logger,
		TFTPServer: tftpServer,
		ctx:        ctx,
		cancel:     cancel,
	}

	// Initialize handlers with dependencies
	app.Handlers = handlers.NewHandlers(database, cfg, logger, app)

	// Setup HTTP server and routes
	if err := app.setupHTTPServer(); err != nil {
		cancel()
		return nil, errors.Wrap(err, "setup_http_server")
	}

	return app, nil
}

// Start initializes and starts all services
func (a *App) Start() error {
	a.Logger.Info("Starting Ignite application")

	// Start TFTP server
	if err := a.TFTPServer.Start(); err != nil {
		return errors.NewNetworkError("start_tftp", err)
	}
	a.Logger.Info("TFTP server started", slog.String("directory", a.Config.TFTP.Dir))

	// Initialize DHCP servers from database
	if err := a.initializeDHCPServers(); err != nil {
		a.Logger.Warn("Failed to initialize DHCP servers", slog.String("error", err.Error()))
	}

	// Start HTTP server
	a.Logger.Info("Starting HTTP server", slog.String("port", a.Config.HTTP.Port))
	if err := a.HTTPServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return errors.NewNetworkError("start_http", err)
	}

	return nil
}

// Stop gracefully shuts down the application
func (a *App) Stop() error {
	a.Logger.Info("Shutting down application")
	a.cancel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop HTTP server
	if err := a.HTTPServer.Shutdown(ctx); err != nil {
		a.Logger.Error("Error shutting down HTTP server", slog.String("error", err.Error()))
		return err
	}

	// Stop TFTP server
	a.TFTPServer.Stop()

	// Close database
	if err := a.DB.Close(); err != nil {
		a.Logger.Error("Error closing database", slog.String("error", err.Error()))
		return err
	}

	a.Logger.Info("Application shutdown complete")
	return nil
}

// setupHTTPServer configures the HTTP server with middleware and routes
func (a *App) setupHTTPServer() error {
	// Setup routes with handlers
	router := routes.Setup(a.Handlers)
	
	// Apply middleware
	middlewares := middleware.DefaultMiddleware(a.Logger)
	handler := middleware.ChainMiddleware(middlewares...)(router)
	
	// Configure server with proper timeouts and limits
	a.HTTPServer = &http.Server{
		Addr:           ":" + a.Config.HTTP.Port,
		Handler:        handler,
		ReadTimeout:    15 * time.Second,
		WriteTimeout:   15 * time.Second,
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1MB
	}

	a.Router = router
	return nil
}

// initializeDHCPServers retrieves DHCP server configurations from database and initializes them
func (a *App) initializeDHCPServers() error {
	bucket := a.Config.DB.Bucket
	kv, err := a.DB.GetAllKV(bucket)
	if err != nil {
		return errors.NewDatabaseError("get_dhcp_servers", err)
	}

	if len(kv) == 0 {
		a.Logger.Info("No DHCP servers found in database")
		return nil
	}

	a.Logger.Info("Initializing DHCP servers", slog.Int("count", len(kv)))
	
	// TODO: Implement DHCP server initialization with proper error handling
	// This would require refactoring the DHCP package to work with the new structure
	
	return nil
}

// GetLogger returns the application logger
func (a *App) GetLogger() *slog.Logger {
	return a.Logger
}

// GetConfig returns the application configuration
func (a *App) GetConfig() *config.Config {
	return a.Config
}

// GetDatabase returns the database interface
func (a *App) GetDatabase() DatabaseStore {
	return a.DB
}

// GetDhcpServer returns a DHCP handler for a given TFTP IP
func (a *App) GetDhcpServer(tftpip string) (*dhcp.DHCPHandler, error) {
	return dhcp.GetDHCPServer(tftpip)
}

// initDatabase initializes the database connection
func initDatabase(cfg *config.Config, logger *slog.Logger) (DatabaseStore, error) {
	database, err := db.Init()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	logger.Info("Database connection established", 
		slog.String("path", cfg.GetDatabasePath()),
		slog.String("bucket", cfg.DB.Bucket),
	)

	return database, nil
}
