# Build Guide

This document describes how to build Port Authorizing for various platforms.

## Prerequisites

- Go 1.21 or later
- Make (optional, but recommended)
- Git (for version information)

## Quick Build

```bash
# Build for current platform
make build

# Binaries will be in bin/
./bin/port-authorizing-api --help
./bin/port-authorizing-cli --help
```

## Build Variations

### Development Build

Standard build with debug information:

```bash
make build
```

### Release Build

Optimized build with stripped symbols (smaller binaries):

```bash
make build-release
```

### Platform-Specific Builds

Build for a specific target platform:

```bash
# Linux
make build-linux          # Linux amd64
make build-linux-arm64    # Linux ARM64 (Raspberry Pi, ARM servers)

# macOS
make build-darwin         # macOS Intel (x86_64)
make build-darwin-arm64   # macOS Apple Silicon (M1/M2/M3)

# Windows
make build-windows        # Windows amd64
```

Output directories:
- `bin/linux/` - Linux binaries
- `bin/darwin/` - macOS binaries
- `bin/windows/` - Windows binaries (`.exe` files)

### Build All Platforms

```bash
# Build for all platforms at once
make build-all
```

### Cross-Compilation with Archives

Build for all platforms and create distribution archives:

```bash
make cross-compile
```

This creates:
- `bin/port-authorizing-linux-amd64.tar.gz`
- `bin/port-authorizing-linux-arm64.tar.gz`
- `bin/port-authorizing-darwin-amd64.tar.gz`
- `bin/port-authorizing-darwin-arm64.tar.gz`
- `bin/port-authorizing-windows-amd64.zip`

### Docker Image

```bash
make build-docker
```

This creates:
- `port-authorizing:latest`
- `port-authorizing:<git-version>`

## Version Information

Build version is automatically determined from git:

```bash
# Show version info
make version
```

Override version:

```bash
VERSION=1.0.0 make build
```

Version information is embedded in the binaries:
- `Version` - Git tag or commit hash
- `BuildTime` - Build timestamp (UTC)
- `GitCommit` - Short commit hash

## Build Flags

The following LDFLAGS are automatically applied:

```bash
-X main.Version=$(VERSION)
-X main.BuildTime=$(BUILD_TIME)
-X main.GitCommit=$(GIT_COMMIT)
```

Release builds add:
- `-trimpath` - Remove file system paths from binary
- `-s` - Omit symbol table
- `-w` - Omit DWARF debug info

## Custom Build Directory

```bash
# Custom output directory
BIN_DIR=/tmp/build make build

# Results in /tmp/build/port-authorizing-*
```

## Manual Build (without Make)

### API Server

```bash
go build -o bin/port-authorizing-api ./cmd/api
```

### CLI Client

```bash
go build -o bin/port-authorizing-cli ./cmd/cli
```

### With Version Information

```bash
VERSION=$(git describe --tags --always --dirty)
BUILD_TIME=$(date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT=$(git rev-parse --short HEAD)

go build \
  -ldflags "-X main.Version=$VERSION -X main.BuildTime=$BUILD_TIME -X main.GitCommit=$GIT_COMMIT" \
  -o bin/port-authorizing-api \
  ./cmd/api
```

### Cross-Compilation

```bash
# For Linux
GOOS=linux GOARCH=amd64 go build -o bin/port-authorizing-api-linux ./cmd/api

# For macOS ARM64
GOOS=darwin GOARCH=arm64 go build -o bin/port-authorizing-api-darwin-arm64 ./cmd/api

# For Windows
GOOS=windows GOARCH=amd64 go build -o bin/port-authorizing-api.exe ./cmd/api
```

## Supported Platforms

| OS      | Architecture | Make Target             | GOOS/GOARCH        |
|---------|-------------|-------------------------|--------------------|
| Linux   | amd64       | `make build-linux`      | `linux/amd64`      |
| Linux   | arm64       | `make build-linux-arm64`| `linux/arm64`      |
| macOS   | amd64       | `make build-darwin`     | `darwin/amd64`     |
| macOS   | arm64       | `make build-darwin-arm64`| `darwin/arm64`    |
| Windows | amd64       | `make build-windows`    | `windows/amd64`    |

Additional platforms can be built manually using Go's cross-compilation:

```bash
# FreeBSD
GOOS=freebsd GOARCH=amd64 go build -o bin/port-authorizing-api-freebsd ./cmd/api

# Linux ARM (32-bit)
GOOS=linux GOARCH=arm GOARM=7 go build -o bin/port-authorizing-api-arm ./cmd/api
```

## Dependencies

Install/update dependencies:

```bash
make deps
```

This runs:
- `go mod download` - Download dependencies
- `go mod tidy` - Clean up unused dependencies

## Clean Build Artifacts

```bash
make clean
```

This removes:
- `bin/` directory
- `audit.log` file

## Build Size Optimization

### Release Build

Release builds are ~30% smaller due to symbol stripping:

```bash
make build-release
```

### Compression

Further reduce size with UPX:

```bash
# Install UPX (example for macOS)
brew install upx

# Compress binary
upx --best bin/port-authorizing-api
```

**Note:** Some antivirus software may flag UPX-compressed binaries.

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Build

on: [push, pull_request]

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'

      - name: Build
        run: make build-all

      - name: Test
        run: make test
```

### GitLab CI Example

```yaml
build:
  image: golang:1.21
  script:
    - make deps
    - make build-all
    - make test
  artifacts:
    paths:
      - bin/
```

## Troubleshooting

### Module Download Issues

```bash
# Clear module cache
go clean -modcache

# Re-download
make deps
```

### Cross-Compilation Issues

```bash
# Ensure CGO is disabled for pure Go builds
CGO_ENABLED=0 make build-linux
```

### Git Version Not Found

If `git describe` fails:

```bash
# Use explicit version
VERSION=dev make build
```

## Testing Builds

```bash
# Run tests before building
make test

# Run end-to-end tests
make test-e2e

# Lint code
make lint

# Format code
make fmt
```

## Development Workflow

Typical development build cycle:

```bash
# 1. Install dependencies
make deps

# 2. Format and lint
make fmt
make lint

# 3. Run tests
make test

# 4. Build
make build

# 5. Test locally
./bin/port-authorizing-api --config config.yaml
```

## Production Release Checklist

1. **Update version**: Tag release in git
   ```bash
   git tag -a v1.0.0 -m "Release v1.0.0"
   git push origin v1.0.0
   ```

2. **Clean build**: Remove old artifacts
   ```bash
   make clean
   ```

3. **Build all platforms**: Create distribution binaries
   ```bash
   make cross-compile
   ```

4. **Test**: Verify binaries on target platforms
   ```bash
   # Test each platform
   ./bin/linux/port-authorizing-api --version
   ```

5. **Create checksums**: For security verification
   ```bash
   cd bin
   sha256sum *.tar.gz *.zip > checksums.txt
   ```

6. **Upload artifacts**: To GitHub releases or artifact server

## Support

For build issues:
- Check Go version: `go version` (need 1.21+)
- Check Make: `make --version`
- Review build logs for errors
- Open an issue on GitHub

