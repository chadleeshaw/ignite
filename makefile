.PHONY: all build run dev test clean css help

# Default target
all: test build

# Build the application
build: css
	@echo "Building application..."
	@go build -o bin/ignite main.go

# Run the application (builds first)
run: build
	@./bin/ignite

# Development mode (no build, just run)
dev:
	@echo "Starting development server..."
	@go run main.go

# Run tests
test:
	@echo "Running tests..."
	@go test ./...

# Build CSS assets
css:
	@echo "Building CSS..."
	@npm install --prefix ./public --silent
	@./public/node_modules/.bin/tailwindcss -i ./public/http/css/includes.css -o ./public/http/css/tailwind.css --minify

# Clean build artifacts
clean:
	@echo "Cleaning up..."
	@rm -rf bin/ public/http/css/tailwind.css ignite.db

# Database operations
db-mock:
	@echo "Adding mock data..."
	@go run main.go -mock-data

db-clear:
	@echo "Clearing database..."
	@go run main.go -clear-data

db-reset: db-clear db-mock
	@echo "Database reset complete"

# Help
help:
	@echo "Available targets:"
	@echo "  build    - Build the application"
	@echo "  run      - Build and run the application"  
	@echo "  dev      - Run in development mode"
	@echo "  test     - Run tests"
	@echo "  css      - Build CSS assets"
	@echo "  clean    - Clean build artifacts"
	@echo "  db-mock  - Add mock data to database"
	@echo "  db-clear - Clear database"
	@echo "  db-reset - Reset database with mock data"
	@echo "  help     - Show this help"