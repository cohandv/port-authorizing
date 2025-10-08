# Port Authorizing

[![Go 1.24+](https://img.shields.io/badge/Go-1.24+-00ADD8.svg?logo=go)](https://go.dev/)
[![Docker Hub](https://img.shields.io/docker/v/cohandv/port-authorizing?label=docker&logo=docker)](https://hub.docker.com/r/cohandv/port-authorizing)
[![Release](https://github.com/cohandv/port-authorizing/actions/workflows/release.yml/badge.svg)](https://github.com/cohandv/port-authorizing/actions/workflows/release.yml)
[![Tests](https://github.com/cohandv/port-authorizing/actions/workflows/test.yml/badge.svg)](https://github.com/cohandv/port-authorizing/actions/workflows/test.yml)
[![Docker Build](https://github.com/cohandv/port-authorizing/actions/workflows/docker-publish.yml/badge.svg)](https://github.com/cohandv/port-authorizing/actions/workflows/docker-publish.yml)

**Secure proxy for any service with authentication, authorization, and audit logging.**

Port Authorizing acts as a transparent proxy between clients and backend services (PostgreSQL, HTTP, TCP, etc.), providing centralized authentication, role-based authorization, protocol-specific filtering, and comprehensive audit logging.

## Tools

- **`port-authorizing`** - Main API server
- **`port-authorizing-cli`** - CLI client for connecting through proxies
- **`mock-approval-server`** - Testing tool for approval workflows (see `tools/mock-approval-server/`)

## Features

- 🔐 **Multi-Provider Authentication** - Local users, OIDC (Keycloak), LDAP, SAML2
- 🛡️ **Role-Based Access Control** - Tag-based policies with different access per role
- 📝 **Protocol-Specific Filtering** - SQL query whitelisting for PostgreSQL, request filtering for HTTP
- 🔬 **SQL Semantic Analysis** - Table-level permissions with PostgreSQL parser (prevents injection)
- 🔒 **Credential Hiding** - Users never see backend credentials
- 🌐 **Transparent Proxying** - Works with standard clients (psql, curl, etc.)
- ⏱️ **Time-Limited Access** - Connections expire automatically
- 📊 **Full Audit Logging** - All actions logged with user attribution
- ✅ **Command Approval Workflow** - Human-in-the-loop for dangerous operations (Webhook + Slack)
- ✨ **Resilient Startup** - Server starts even if auth providers (OIDC/LDAP/SAML2) are temporarily unavailable

## Protocol Maturity

| Protocol | Status | Features | Notes |
|----------|--------|----------|-------|
| PostgreSQL | ✅ **Mature** | Authentication, SQL parsing, table-level permissions, query whitelisting, approval workflow, audit logging | Fully protocol-aware with semantic analysis |
| HTTP/HTTPS | ✅ **Mature** | Transparent proxying, request whitelisting, CORS, approval workflow, audit logging | Full request/response handling |
| TCP | 🚧 **Beta** | Basic proxying, authentication | Limited protocol awareness, suitable for simple services |

## Quick Start

### Installation

**Using install script (recommended):**
```bash
curl -fsSL https://raw.githubusercontent.com/davidcohan/port-authorizing/main/scripts/install.sh | bash
```

**Manual download:**
```bash
# Download from GitHub releases
wget https://github.com/davidcohan/port-authorizing/releases/latest/download/port-authorizing-linux-amd64
chmod +x port-authorizing-linux-amd64
sudo mv port-authorizing-linux-amd64 /usr/local/bin/port-authorizing
```

**Using Docker:**
```bash
docker pull cohandv/port-authorizing:latest
```

**Build from source:**
```bash
git clone https://github.com/davidcohan/port-authorizing.git
cd port-authorizing
make build
```

### Basic Usage

```bash
# Start server
port-authorizing server --config config.yaml

# Login (opens browser for OIDC)
port-authorizing login

# List available connections
port-authorizing list

# Connect to service (PostgreSQL example)
port-authorizing connect postgres-prod -l 5433

# Use standard client
psql -h localhost -p 5433 -U your-username -d database

# Or connect to HTTP service
port-authorizing connect api-server -l 8080
curl http://localhost:8080/api/users
```

## Architecture

```
┌─────────┐         ┌──────────────┐         ┌──────────┐
│  Client │────────▶│ Port Auth    │────────▶│ Backend  │
│ (psql)  │         │ Proxy        │         │ Postgres │
└─────────┘         └──────────────┘         └──────────┘
                     │
                     ├─ JWT Authentication
                     ├─ Role Authorization
                     ├─ Query Validation
                     └─ Audit Logging
```

## Configuration Example

```yaml
server:
  port: 8080

auth:
  providers:
    - name: keycloak
      type: oidc
      enabled: true
      config:
        issuer: "https://keycloak.example.com/realms/myapp"
        client_id: "port-authorizing"
        roles_claim: "roles"

connections:
  - name: postgres-prod
    type: postgres
    host: prod-db.internal
    port: 5432
    tags:
      - env:production
    backend_username: "app_user"
    backend_password: "${DB_PASSWORD}"

policies:
  - name: developer-readonly
    roles:
      - developer
    tags:
      - env:production
    whitelist:
      - "^SELECT.*"
      - "^EXPLAIN.*"
```

## Documentation

📚 **[Full Documentation](docs/README.md)**

- [Getting Started Guide](docs/guides/getting-started.md)
- [Authentication Setup](docs/guides/authentication.md)
- [Configuration Reference](docs/guides/configuration.md)
- [Architecture](docs/architecture/ARCHITECTURE.md)
- [Deployment Guide](docs/deployment/building.md)

## Use Cases

### Secure Production Database Access

Give developers temporary SELECT-only access to production databases without sharing credentials:

```bash
# Developer workflow
port-authorizing login  # Authenticates via OIDC
port-authorizing connect postgres-prod -l 5433
psql -h localhost -p 5433 -U alice -d myapp

# Can execute: SELECT, EXPLAIN
# Cannot execute: UPDATE, DELETE, DROP
# All queries logged with username
```

### Time-Limited Access

Connections automatically expire:

```yaml
connections:
  - name: postgres-prod
    duration: 30m  # Access expires after 30 minutes
```

### Multi-Environment Access Control

Different users have different access per environment:

```yaml
policies:
  - name: dev-full-test
    roles: [developer]
    tags: [env:test]
    whitelist: [".*"]  # Full access to test

  - name: dev-readonly-prod
    roles: [developer]
    tags: [env:production]
    whitelist: ["^SELECT.*", "^EXPLAIN.*"]  # Read-only in prod
```

## Development

```bash
# Install dependencies
make deps

# Run tests
make test

# Build for all platforms
make build-all

# Run locally
make dev
```

## Docker Compose (Testing)

```bash
# Start all services (PostgreSQL, Keycloak, LDAP)
docker-compose up -d

# Setup Keycloak
./docker/setup-keycloak.sh setup

# Stop services
docker-compose down
```

## Security

- ✅ **No credential sharing** - Backend passwords never exposed to users
- ✅ **Username enforcement** - Users can only connect as themselves
- ✅ **Query validation** - All queries checked against whitelist before execution
- ✅ **Audit trail** - Every action logged with user identity
- ✅ **Time-bound access** - Connections expire automatically
- ✅ **JWT-based auth** - Cryptographically signed tokens

See [Security Improvements](docs/security/security-improvements.md) for details.

## Contributing

We welcome contributions! Port Authorizing uses **automatic versioning** based on conventional commits.

**Quick start:**
```bash
# Fork and clone
git clone https://github.com/YOUR_USERNAME/port-authorizing.git

# Create feature branch
git checkout -b feat/my-feature

# Commit using conventional commits
git commit -m "feat: add awesome feature"

# Push and create PR
git push origin feat/my-feature
```

**Commit format:**
- `feat: ...` → Minor version bump (new features)
- `fix: ...` → Patch version bump (bug fixes)
- `feat!: ...` or `BREAKING CHANGE:` → Major version bump

See [CONTRIBUTING.md](CONTRIBUTING.md) for detailed guidelines.

## Versioning

This project uses **fully automated semantic versioning**:

- Every push to `main` triggers automatic version analysis
- Version is determined from commit messages
- Releases are created automatically with binaries
- See [docs/development/VERSIONING.md](docs/development/VERSIONING.md)

## License

MIT License - see LICENSE file for details.

## Support

- **Documentation**: [docs/](docs/)
- **GitHub**: [davidcohan/port-authorizing](https://github.com/davidcohan/port-authorizing)
- **Docker Hub**: [cohandv/port-authorizing](https://hub.docker.com/r/cohandv/port-authorizing)
- **Releases**: [GitHub Releases](https://github.com/davidcohan/port-authorizing/releases)
- **Changelog**: [CHANGELOG.md](CHANGELOG.md)
