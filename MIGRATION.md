# Migration Guide

## Major Changes

### 1. Unified Binary

**Before:** Two separate binaries
- `port-authorizing-api` - Server
- `port-authorizing-cli` - Client

**After:** Single unified binary with subcommands
- `port-authorizing server` - Start server
- `port-authorizing login` - Client login
- `port-authorizing list` - Client list
- `port-authorizing connect` - Client connect

### 2. Documentation Organization

**Before:** Multiple markdown files in root directory
- Many `*_SUMMARY.md` and `*_UPDATE.md` files
- Scattered documentation

**After:** Organized docs/ folder
```
docs/
├── README.md (index)
├── guides/
│   ├── getting-started.md
│   ├── authentication.md
│   ├── configuration.md
│   ├── oidc-setup.md
│   ├── quick-reference.md
│   └── testing.md
├── architecture/
│   ├── ARCHITECTURE.md
│   └── transparent-proxy.md
├── deployment/
│   ├── building.md
│   └── docker-testing.md
└── security/
    └── security-improvements.md
```

### 3. Docker Hub Publishing

**New:** Automatic Docker image publishing via GitHub Actions
- Pushes to `main` branch → `cohandv/port-authorizing:latest`
- Git tags `v*` → `cohandv/port-authorizing:v1.0.0`
- Multi-arch support: `linux/amd64`, `linux/arm64`

## Migration Steps

### For Server Deployments

#### Docker

**Before:**
```bash
docker run -p 8080:8080 cohandv/port-authorizing:latest /usr/local/bin/port-authorizing-api --config /app/config.yaml
```

**After:**
```bash
docker run -p 8080:8080 cohandv/port-authorizing:latest
# Default command is already: server --config /app/config.yaml
```

#### Systemd Service

Update your systemd service file:

**Before:**
```ini
[Service]
ExecStart=/usr/local/bin/port-authorizing-api --config /etc/port-authorizing/config.yaml
```

**After:**
```ini
[Service]
ExecStart=/usr/local/bin/port-authorizing server --config /etc/port-authorizing/config.yaml
```

#### Manual

**Before:**
```bash
./port-authorizing-api --config config.yaml
```

**After:**
```bash
./port-authorizing server --config config.yaml
```

### For Client Usage

#### Login

**Before:**
```bash
./port-authorizing-cli login
```

**After:**
```bash
./port-authorizing login
```

#### List Connections

**Before:**
```bash
./port-authorizing-cli list
```

**After:**
```bash
./port-authorizing list
```

#### Connect to Service

**Before:**
```bash
./port-authorizing-cli connect postgres-prod -l 5433
```

**After:**
```bash
./port-authorizing connect postgres-prod -l 5433
```

### Installation

#### From Binary

**Before:**
```bash
# Install both binaries
sudo install -m 755 port-authorizing-api /usr/local/bin/
sudo install -m 755 port-authorizing-cli /usr/local/bin/
```

**After:**
```bash
# Install single binary
sudo install -m 755 port-authorizing /usr/local/bin/
```

#### Using Make

**Before:**
```bash
make install  # Installed both binaries
```

**After:**
```bash
make install  # Installs unified binary
```

### Building from Source

#### Regular Build

**Before:**
```bash
make build  # Built both binaries
```

**After:**
```bash
make build  # Builds unified binary
```

#### Docker Build

**Before:**
```bash
make build-docker  # Tagged as port-authorizing:latest
```

**After:**
```bash
make build-docker  # Tags as cohandv/port-authorizing:latest
```

## Breaking Changes

### Binary Names

If you have scripts referencing the old binary names, update them:

```bash
# Old
/path/to/port-authorizing-api
/path/to/port-authorizing-cli

# New
/path/to/port-authorizing server
/path/to/port-authorizing login
/path/to/port-authorizing list
/path/to/port-authorizing connect
```

### Docker Image Entrypoint

If you were overriding the entrypoint:

**Before:**
```yaml
command: ["/usr/local/bin/port-authorizing-api", "--config", "/app/config.yaml"]
```

**After:**
```yaml
command: ["server", "--config", "/app/config.yaml"]
# Or use default (no command needed)
```

## Configuration Changes

No configuration file changes required! Your existing `config.yaml` works as-is.

## Rollback

If you need to rollback to the old separate binaries:

```bash
# Checkout previous commit
git checkout <previous-commit>

# Build
make build

# This will create:
# - bin/port-authorizing-api
# - bin/port-authorizing-cli
```

## Benefits of Unified Binary

1. **Simpler Distribution**
   - Single binary to download/install
   - Smaller total size

2. **Consistent CLI Experience**
   - All commands under one binary
   - Shared global flags

3. **Easier Deployment**
   - One binary to manage
   - Clearer command structure

4. **Better Help System**
   - `port-authorizing --help` shows all commands
   - Autocomplete support

## Getting Help

If you encounter issues during migration:

1. Check the [Documentation](docs/README.md)
2. Run `port-authorizing --help` or `port-authorizing <command> --help`
3. Open an issue on GitHub

## Timeline

- **Old binaries**: Deprecated but still in `bin/` folder
- **Removal**: Old binaries will be removed in next major version
- **Docker images**: Old images remain available but won't receive updates

## Examples

### Complete Migration Example

```bash
# Stop old server
systemctl stop port-authorizing-api

# Download new binary
wget https://github.com/yourusername/port-authorizing/releases/download/v2.0.0/port-authorizing-linux-amd64.tar.gz
tar xzf port-authorizing-linux-amd64.tar.gz
sudo install -m 755 port-authorizing /usr/local/bin/

# Update systemd service
sudo nano /etc/systemd/system/port-authorizing.service
# Change ExecStart to: /usr/local/bin/port-authorizing server --config /etc/port-authorizing/config.yaml

# Reload and start
sudo systemctl daemon-reload
sudo systemctl start port-authorizing
sudo systemctl status port-authorizing

# Verify
port-authorizing version
curl http://localhost:8080/api/health

# Update client on developer machines
port-authorizing login
port-authorizing list
```

