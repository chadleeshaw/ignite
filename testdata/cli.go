package testdata

import (
	"context"
	"flag"
	"ignite/app"
	"log"
	"time"
)

// Config holds CLI configuration
type Config struct {
	MockData  bool
	ClearData bool
}

// ParseFlags parses command line flags and returns CLI config
func ParseFlags() *Config {
	config := &Config{}
	flag.BoolVar(&config.MockData, "mock-data", false, "Populate database with mock data for UI testing")
	flag.BoolVar(&config.ClearData, "clear-data", false, "Clear all data from database")
	flag.Parse()
	return config
}

// HandleDataOperations handles mock data operations if requested
// Returns true if the application should continue running, false if it should exit
func HandleDataOperations(config *Config, application *app.Application) bool {
	if !config.ClearData && !config.MockData {
		return true // No data operations requested, continue with normal startup
	}

	container := application.GetContainer()
	mockService := NewMockDataService(container.ServerService, container.LeaseService)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if config.ClearData {
		log.Println("Clearing all data from database...")
		if err := mockService.ClearAllData(ctx); err != nil {
			log.Fatalf("Failed to clear mock data: %v", err)
		}
		log.Println("All data cleared successfully")
	}

	if config.MockData {
		log.Println("Populating database with mock data...")
		if err := mockService.PopulateMockData(ctx); err != nil {
			log.Fatalf("Failed to populate mock data: %v", err)
		}
		log.Println("Mock data populated successfully")
	}

	// If only data operations were requested (no additional args), exit
	// Return false to exit if data operations performed and no additional args
	return flag.NArg() > 0
}
