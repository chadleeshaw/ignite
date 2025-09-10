package main

import (
	"embed"
	"ignite/app"
	"log"
)

// Embed static files at compile time
//
//go:embed public/http/*
var staticFS embed.FS

func main() {
	// Create application with embedded static files
	application, err := app.NewApplicationWithStatic(staticFS)
	if err != nil {
		log.Fatalf("Failed to create application: %v", err)
	}

	if err := application.Run(); err != nil {
		log.Fatalf("Application failed: %v", err)
	}
}
