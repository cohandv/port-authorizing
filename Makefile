.PHONY: all build build-linux build-darwin build-windows build-all build-release \
        build-docker clean test run-api run-cli install deps cross-compile

# Version info
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GitCommit=$(GIT_COMMIT)"

# Build output directory
BIN_DIR := bin

# Build both API and CLI
all: build

# Build binaries for current platform
build:
	@echo "Building API server..."
	@mkdir -p $(BIN_DIR)
	@go build $(LDFLAGS) -o $(BIN_DIR)/port-authorizing-api ./cmd/api
	@echo "Building CLI client..."
	@go build $(LDFLAGS) -o $(BIN_DIR)/port-authorizing-cli ./cmd/cli
	@echo "✓ Build complete!"

# Build with optimizations (release mode)
build-release:
	@echo "Building release binaries..."
	@mkdir -p $(BIN_DIR)
	@go build $(LDFLAGS) -trimpath -ldflags="-s -w" -o $(BIN_DIR)/port-authorizing-api ./cmd/api
	@go build $(LDFLAGS) -trimpath -ldflags="-s -w" -o $(BIN_DIR)/port-authorizing-cli ./cmd/cli
	@echo "✓ Release build complete!"

# Build for Linux
build-linux:
	@echo "Building for Linux (amd64)..."
	@mkdir -p $(BIN_DIR)/linux
	@GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BIN_DIR)/linux/port-authorizing-api ./cmd/api
	@GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BIN_DIR)/linux/port-authorizing-cli ./cmd/cli
	@echo "✓ Linux build complete!"

# Build for Linux ARM64
build-linux-arm64:
	@echo "Building for Linux (arm64)..."
	@mkdir -p $(BIN_DIR)/linux-arm64
	@GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BIN_DIR)/linux-arm64/port-authorizing-api ./cmd/api
	@GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BIN_DIR)/linux-arm64/port-authorizing-cli ./cmd/cli
	@echo "✓ Linux ARM64 build complete!"

# Build for macOS
build-darwin:
	@echo "Building for macOS (amd64)..."
	@mkdir -p $(BIN_DIR)/darwin
	@GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BIN_DIR)/darwin/port-authorizing-api ./cmd/api
	@GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BIN_DIR)/darwin/port-authorizing-cli ./cmd/cli
	@echo "✓ macOS build complete!"

# Build for macOS ARM64 (Apple Silicon)
build-darwin-arm64:
	@echo "Building for macOS (arm64)..."
	@mkdir -p $(BIN_DIR)/darwin-arm64
	@GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BIN_DIR)/darwin-arm64/port-authorizing-api ./cmd/api
	@GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BIN_DIR)/darwin-arm64/port-authorizing-cli ./cmd/cli
	@echo "✓ macOS ARM64 build complete!"

# Build for Windows
build-windows:
	@echo "Building for Windows (amd64)..."
	@mkdir -p $(BIN_DIR)/windows
	@GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BIN_DIR)/windows/port-authorizing-api.exe ./cmd/api
	@GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BIN_DIR)/windows/port-authorizing-cli.exe ./cmd/cli
	@echo "✓ Windows build complete!"

# Build for all platforms
build-all: build-linux build-linux-arm64 build-darwin build-darwin-arm64 build-windows
	@echo "✓ All platform builds complete!"

# Cross-compile for all platforms
cross-compile: build-all
	@echo "Creating archives..."
	@cd $(BIN_DIR)/linux && tar -czf ../port-authorizing-linux-amd64.tar.gz *
	@cd $(BIN_DIR)/linux-arm64 && tar -czf ../port-authorizing-linux-arm64.tar.gz *
	@cd $(BIN_DIR)/darwin && tar -czf ../port-authorizing-darwin-amd64.tar.gz *
	@cd $(BIN_DIR)/darwin-arm64 && tar -czf ../port-authorizing-darwin-arm64.tar.gz *
	@cd $(BIN_DIR)/windows && zip -q ../port-authorizing-windows-amd64.zip *
	@echo "✓ Cross-compilation complete! Archives in $(BIN_DIR)/"

# Build Docker image
build-docker:
	@echo "Building Docker image..."
	@docker build -t port-authorizing:$(VERSION) .
	@docker tag port-authorizing:$(VERSION) port-authorizing:latest
	@echo "✓ Docker image built: port-authorizing:$(VERSION)"

# Install dependencies
deps:
	@echo "Installing dependencies..."
	@go mod download
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

# Show version
version:
	@echo "Version:    $(VERSION)"
	@echo "Build Time: $(BUILD_TIME)"
	@echo "Git Commit: $(GIT_COMMIT)"

# Show help
help:
	@echo "Port Authorizing - Makefile Commands"
	@echo ""
	@echo "Build Commands:"
	@echo "  make build              - Build for current platform"
	@echo "  make build-release      - Build optimized release binaries"
	@echo "  make build-linux        - Build for Linux (amd64)"
	@echo "  make build-linux-arm64  - Build for Linux (arm64)"
	@echo "  make build-darwin       - Build for macOS (amd64)"
	@echo "  make build-darwin-arm64 - Build for macOS (arm64/Apple Silicon)"
	@echo "  make build-windows      - Build for Windows (amd64)"
	@echo "  make build-all          - Build for all platforms"
	@echo "  make cross-compile      - Build for all platforms and create archives"
	@echo "  make build-docker       - Build Docker image"
	@echo ""
	@echo "Development Commands:"
	@echo "  make deps               - Install Go dependencies"
	@echo "  make clean              - Clean build artifacts"
	@echo "  make fmt                - Format code"
	@echo "  make lint               - Run linter"
	@echo "  make dev                - Run API in development mode"
	@echo "  make run-api            - Run the API server"
	@echo ""
	@echo "Testing Commands:"
	@echo "  make test               - Run Go unit tests"
	@echo "  make test-e2e           - Run end-to-end tests with Docker"
	@echo ""
	@echo "Docker Commands:"
	@echo "  make docker-up          - Start Docker services (PostgreSQL, Nginx, Keycloak, LDAP)"
	@echo "  make docker-down        - Stop Docker services"
	@echo "  make docker-logs        - View Docker logs"
	@echo ""
	@echo "Installation Commands:"
	@echo "  make install            - Install binaries to system (/usr/local/bin)"
	@echo ""
	@echo "Utility Commands:"
	@echo "  make version            - Show version information"
	@echo "  make help               - Show this help message"
	@echo ""
	@echo "Environment Variables:"
	@echo "  VERSION=x.y.z           - Override version (default: git describe)"
	@echo "  BIN_DIR=path            - Override output directory (default: bin)"


