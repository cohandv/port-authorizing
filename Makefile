.PHONY: all build clean test run-api run-cli install deps

# Build both API and CLI
all: build

# Build binaries
build:
	@echo "Building API server..."
	@mkdir -p bin
	@go build -o bin/port-authorizing-api ./cmd/api
	@echo "Building CLI client..."
	@go build -o bin/port-authorizing-cli ./cmd/cli
	@echo "✓ Build complete!"

# Install dependencies
deps:
	@echo "Installing dependencies..."
	@go get github.com/gorilla/mux
	@go get github.com/golang-jwt/jwt/v5
	@go get github.com/spf13/cobra
	@go get gopkg.in/yaml.v3
	@go get github.com/google/uuid
	@go mod tidy
	@echo "✓ Dependencies installed!"

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf bin/
	@rm -f audit.log
	@echo "✓ Clean complete!"

# Run Go tests
test:
	@echo "Running Go tests..."
	@go test -v ./...

# Run end-to-end tests with Docker
test-e2e:
	@echo "Running end-to-end tests with Docker..."
	@./test.sh

# Start Docker services
docker-up:
	@echo "Starting Docker services..."
	@docker-compose up -d
	@echo "Waiting for services to be healthy..."
	@sleep 5
	@docker-compose ps

# Stop Docker services
docker-down:
	@echo "Stopping Docker services..."
	@docker-compose down -v

# View Docker logs
docker-logs:
	@docker-compose logs -f

# Run API server
run-api:
	@./bin/port-authorizing-api --config config.yaml

# Install binaries to system
install: build
	@echo "Installing binaries to /usr/local/bin..."
	@cp bin/port-authorizing-api /usr/local/bin/
	@cp bin/port-authorizing-cli /usr/local/bin/
	@echo "✓ Installation complete!"

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...
	@echo "✓ Format complete!"

# Run linter
lint:
	@echo "Running linter..."
	@go vet ./...
	@echo "✓ Lint complete!"

# Development mode (with auto-restart)
dev:
	@echo "Starting development mode..."
	@go run ./cmd/api/main.go --config config.yaml

# Show help
help:
	@echo "Available targets:"
	@echo "  make build        - Build both API and CLI binaries"
	@echo "  make deps         - Install Go dependencies"
	@echo "  make clean        - Clean build artifacts"
	@echo "  make test         - Run Go unit tests"
	@echo "  make test-e2e     - Run end-to-end tests with Docker"
	@echo "  make docker-up    - Start Docker services (PostgreSQL + Nginx)"
	@echo "  make docker-down  - Stop Docker services"
	@echo "  make docker-logs  - View Docker logs"
	@echo "  make run-api      - Run the API server"
	@echo "  make install      - Install binaries to system"
	@echo "  make fmt          - Format code"
	@echo "  make lint         - Run linter"
	@echo "  make dev          - Run API in development mode"
	@echo "  make help         - Show this help message"


