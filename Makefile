.PHONY: all build build-linux build-darwin build-windows build-all build-release \
        build-docker clean test run-server install deps cross-compile help

# Version info
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GitCommit=$(GIT_COMMIT)"

# Build output directory
BIN_DIR := bin

# Build unified binary for current platform
all: build

build:
	@echo "Building port-authorizing..."
	@mkdir -p $(BIN_DIR)
	@go build $(LDFLAGS) -o $(BIN_DIR)/port-authorizing ./cmd/port-authorizing
	@echo "✓ Build complete!"

# Build with optimizations (release mode)
build-release:
	@echo "Building release binary..."
	@mkdir -p $(BIN_DIR)
	@go build -trimpath -ldflags="-s -w $(LDFLAGS)" -o $(BIN_DIR)/port-authorizing ./cmd/port-authorizing
	@echo "✓ Release build complete!"

# Build for Linux
build-linux:
	@echo "Building for Linux (amd64)..."
	@mkdir -p $(BIN_DIR)/linux
	@GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BIN_DIR)/linux/port-authorizing ./cmd/port-authorizing
	@echo "✓ Linux build complete!"

# Build for Linux ARM64
build-linux-arm64:
	@echo "Building for Linux (arm64)..."
	@mkdir -p $(BIN_DIR)/linux-arm64
	@GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BIN_DIR)/linux-arm64/port-authorizing ./cmd/port-authorizing
	@echo "✓ Linux ARM64 build complete!"

# Build for macOS
build-darwin:
	@echo "Building for macOS (amd64)..."
	@mkdir -p $(BIN_DIR)/darwin
	@GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BIN_DIR)/darwin/port-authorizing ./cmd/port-authorizing
	@echo "✓ macOS build complete!"

# Build for macOS ARM64 (Apple Silicon)
build-darwin-arm64:
	@echo "Building for macOS (arm64)..."
	@mkdir -p $(BIN_DIR)/darwin-arm64
	@GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BIN_DIR)/darwin-arm64/port-authorizing ./cmd/port-authorizing
	@echo "✓ macOS ARM64 build complete!"

# Build for Windows
build-windows:
	@echo "Building for Windows (amd64)..."
	@mkdir -p $(BIN_DIR)/windows
	@GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BIN_DIR)/windows/port-authorizing.exe ./cmd/port-authorizing
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
	@docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		-t cohandv/port-authorizing:$(VERSION) \
		-t cohandv/port-authorizing:latest \
		.
	@echo "✓ Docker image built: cohandv/port-authorizing:$(VERSION)"

# Push Docker image
push-docker: build-docker
	@echo "Pushing Docker image..."
	@docker push cohandv/port-authorizing:$(VERSION)
	@docker push cohandv/port-authorizing:latest
	@echo "✓ Docker image pushed!"

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
	@rm -f audit.log api.log
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

# Run server
run-server:
	@./bin/port-authorizing server --config config.yaml

# Install binary to system
install: build
	@echo "Installing binary to /usr/local/bin..."
	@cp bin/port-authorizing /usr/local/bin/
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
	@go run ./cmd/port-authorizing/main.go server --config config.yaml

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
	@echo "  make build              - Build unified binary for current platform"
	@echo "  make build-release      - Build optimized release binary"
	@echo "  make build-linux        - Build for Linux (amd64)"
	@echo "  make build-linux-arm64  - Build for Linux (arm64)"
	@echo "  make build-darwin       - Build for macOS (amd64)"
	@echo "  make build-darwin-arm64 - Build for macOS (arm64/Apple Silicon)"
	@echo "  make build-windows      - Build for Windows (amd64)"
	@echo "  make build-all          - Build for all platforms"
	@echo "  make cross-compile      - Build for all platforms and create archives"
	@echo "  make build-docker       - Build Docker image"
	@echo "  make push-docker        - Build and push Docker image to Docker Hub"
	@echo ""
	@echo "Usage Commands:"
	@echo "  port-authorizing server             - Start API server"
	@echo "  port-authorizing login              - Login (opens browser for OIDC)"
	@echo "  port-authorizing list               - List available connections"
	@echo "  port-authorizing connect <name>     - Connect to a service"
	@echo "  port-authorizing version            - Show version"
	@echo ""
	@echo "Development Commands:"
	@echo "  make deps               - Install Go dependencies"
	@echo "  make clean              - Clean build artifacts"
	@echo "  make fmt                - Format code"
	@echo "  make lint               - Run linter"
	@echo "  make dev                - Run server in development mode"
	@echo "  make run-server         - Run the server"
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
	@echo "  make install            - Install binary to system (/usr/local/bin)"
	@echo ""
	@echo "Utility Commands:"
	@echo "  make version            - Show version information"
	@echo "  make help               - Show this help message"
	@echo ""
	@echo "Environment Variables:"
	@echo "  VERSION=x.y.z           - Override version (default: git describe)"
	@echo "  BIN_DIR=path            - Override output directory (default: bin)"
