.PHONY: help setup sqlc build run clean test

help: ## Show this help message
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-15s %s\n", $$1, $$2}'

setup: ## Install required tools (sqlc)
	@echo "Installing sqlc..."
	@go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
	@echo "Done! Make sure $(go env GOPATH)/bin is in your PATH"

sqlc: ## Generate Go code from SQL queries using sqlc
	@echo "Generating sqlc code..."
	@sqlc generate
	@echo "Done!"

deps: ## Download Go dependencies
	@echo "Downloading dependencies..."
	@go mod download
	@echo "Done!"

build: sqlc ## Build the example application
	@echo "Building example application..."
	@go build -o bin/example ./cmd/example
	@echo "Done! Binary created at bin/example"

run: sqlc ## Run the example application
	@echo "Running example application..."
	@go run ./cmd/example/main.go

clean: ## Clean generated files and build artifacts
	@echo "Cleaning up..."
	@rm -rf bin/
	@rm -f auth.db
	@rm -f cookies.txt
	@echo "Done!"

test: sqlc ## Run tests
	@echo "Running tests..."
	@go test -v ./...

demo: sqlc ## Run demo with curl examples
	@echo "Starting demo..."
	@echo ""
	@echo "Starting server in background..."
	@go run ./cmd/example/main.go &
	@sleep 2
	@echo ""
	@echo "=== Testing login as alice ==="
	@curl -s -X POST http://localhost:8080/login -d 'username=alice&password=password123' -c cookies.txt
	@echo ""
	@echo ""
	@echo "=== Testing profile endpoint ==="
	@curl -s http://localhost:8080/profile -b cookies.txt
	@echo ""
	@echo ""
	@echo "=== Testing documents list ==="
	@curl -s http://localhost:8080/documents/list -b cookies.txt
	@echo ""
	@echo ""
	@echo "=== Testing admin endpoint (alice has admin) ==="
	@curl -s http://localhost:8080/documents/admin -b cookies.txt
	@echo ""
	@echo ""
	@echo "=== Testing logout ==="
	@curl -s http://localhost:8080/logout -b cookies.txt
	@echo ""
	@killall example || true
	@rm -f cookies.txt

.DEFAULT_GOAL := help
