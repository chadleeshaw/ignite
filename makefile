.PHONY: all build run clean css

all: clean build test run

test: # Run unit tests
	@go test ./...

build: css # Build the Go application
	@go build -o bin/app main.go

run: build # Run the compiled Go application
	@./bin/app

clean: # Clean up the build artifacts
	@rm -rf bin public/http/css/tailwind.css public/http/js/* ignite.db

css: # Compile Tailwind CSS
	@tailwindcss -i public/http/css/includes.css -o public/http/css/tailwind.css

# ts: # Compile TypeScript
# 	@tsc --project tsconfig.json

help: ## Display this help screen
	@grep -h -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'