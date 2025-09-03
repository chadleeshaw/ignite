package main

import (
	"embed"
	"io/fs"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"ignite/config"
	"ignite/internal/app"

	"github.com/gorilla/mux"
)

//go:embed public/http/*
var staticFS embed.FS

func main() {
	// Initialize application
	igniteApp, err := app.NewApp()
	if err != nil {
		log.Fatalf("Failed to initialize application: %v", err)
	}

	// Setup static file serving
	if err := setupStaticFiles(igniteApp.Router, igniteApp.GetConfig()); err != nil {
		log.Fatalf("Failed to setup static files: %v", err)
	}

	// Setup graceful shutdown
	setupGracefulShutdown(igniteApp)

	// Start the application
	igniteApp.GetLogger().Info("Starting Ignite server")
	if err := igniteApp.Start(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// setupStaticFiles configures static file serving for the HTTP server
func setupStaticFiles(router *mux.Router, cfg *config.Config) error {
	// Setup embedded static files
	publicFS, err := fs.Sub(staticFS, "public/http")
	if err != nil {
		return err
	}
	router.PathPrefix("/public/http/").Handler(http.StripPrefix("/public/http/", http.FileServer(http.FS(publicFS))))

	// Setup provision files (these need to be on the filesystem for editing)
	provDir := cfg.Provision.Dir
	configsPath := filepath.Join(provDir, "configs")
	templatesPath := filepath.Join(provDir, "templates")

	router.PathPrefix("/public/provision/configs/").Handler(http.StripPrefix("/public/provision/configs/", http.FileServer(http.Dir(configsPath))))
	router.PathPrefix("/public/provision/templates/").Handler(http.StripPrefix("/public/provision/templates/", http.FileServer(http.Dir(templatesPath))))

	return nil
}

// setupGracefulShutdown configures graceful shutdown handling
func setupGracefulShutdown(igniteApp *app.App) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		igniteApp.GetLogger().Info("Received shutdown signal")
		if err := igniteApp.Stop(); err != nil {
			igniteApp.GetLogger().Error("Error during shutdown", slog.String("error", err.Error()))
			os.Exit(1)
		}
		os.Exit(0)
	}()
}



