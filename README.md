# Port Authorizing

**Secure database access proxy with authentication, authorization, and query whitelisting.**

Port Authorizing acts as a transparent proxy between clients and backend services (PostgreSQL, HTTP, TCP), providing centralized authentication, role-based authorization, and SQL query filtering.

## Features

- 🔐 **Multi-Provider Authentication** - Local users, OIDC (Keycloak), LDAP, SAML2
- 🛡️ **Role-Based Access Control** - Tag-based policies with different access per role
- 📝 **Query Whitelisting** - Regex-based SQL filtering with audit logging
- 🔒 **Credential Hiding** - Users never see backend credentials
- 🌐 **Transparent Proxying** - Works with standard clients (psql, curl, etc.)
- ⏱️ **Time-Limited Access** - Connections expire automatically
- 📊 **Full Audit Logging** - All actions logged with user attribution

## Quick Start

### Installation

```bash
# Clone repository
git clone https://github.com/yourusername/port-authorizing.git
cd port-authorizing

# Build
make build

# Or use Docker
docker pull cohandv/port-authorizing:latest
```

### Basic Usage

```bash
# Start server
port-authorizing server --config config.yaml

# Login (opens browser for OIDC)
port-authorizing login

# List available connections
port-authorizing list

# Connect to database
port-authorizing connect postgres-prod -l 5433

# Use standard client
psql -h localhost -p 5433 -U your-username -d database
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

Contributions welcome! Please read our contributing guidelines and submit PRs.

## License

[Your License]

## Support

- **Documentation**: [docs/](docs/)
- **Issues**: [GitHub Issues](https://github.com/yourusername/port-authorizing/issues)
- **Discussions**: [GitHub Discussions](https://github.com/yourusername/port-authorizing/discussions)
