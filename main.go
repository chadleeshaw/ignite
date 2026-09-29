package main

import (
	"embed"
	"ignite/app"
	"ignite/cli"
	"log"
)

// Embed static files at compile time
//
//go:embed public/http/*
var staticFS embed.FS

func main() {
	// Parse command line flags
	config := cli.ParseFlags()

	// Create application with embedded static files
	application, err := app.NewApplicationWithStatic(staticFS)
	if err != nil {
		log.Fatalf("Failed to create application: %v", err)
	}

	// Handle any CLI data operations (mock data, clear data, etc.)
	if !cli.HandleDataOperations(config, application) {
		return // Exit if only data operations were requested
	}

	// Start the application
	if err := application.Run(); err != nil {
		log.Fatalf("Application failed: %v", err)
	}
}
