# Port Authorizing

Enterprise-grade secure proxy system with multi-provider authentication, role-based access control, and comprehensive audit logging. Control and monitor access to PostgreSQL databases, HTTP APIs, and TCP services.

## Features

### 🔐 Multi-Provider Authentication
- **Local** - Username/password authentication
- **OIDC** - OpenID Connect (Keycloak, Auth0, Okta, Google, Azure AD)
- **LDAP** - Active Directory / OpenLDAP integration
- **SAML2** - SAML 2.0 (framework ready)

### 🛡️ Role-Based Access Control (RBAC)
- **Tag-Based Policies** - Connections tagged by environment, team, type
- **Flexible Whitelisting** - Per-role query/request patterns
- **Multi-Tag Matching** - ANY or ALL matching modes
- **Environment Isolation** - Separate dev/staging/production access

### 📊 Protocol Support
- **PostgreSQL** - Transparent proxy with credential substitution
- **HTTP/HTTPS** - REST API and web service proxying
- **TCP** - Generic TCP stream proxying (Redis, MongoDB, etc.)

### 🔍 Security & Monitoring
- **Audit Logging** - Complete trail of all operations
- **Connection Timeouts** - Automatic session expiration
- **Query Whitelisting** - Regex-based validation
- **LLM Analysis** - Optional AI risk detection (coming soon)

## Architecture

```
┌─────────┐      ┌─────────┐      ┌──────────────┐
│  User   │──────│   CLI   │──────│  API Server  │
│ (psql)  │      │ (proxy) │      │   (AuthN)    │
└─────────┘      └─────────┘      │   (AuthZ)    │
                                   │   (Audit)    │
                                   └──────┬───────┘
                                          │
                    ┌─────────────────────┼─────────────────────┐
                    │                     │                     │
             ┌──────▼──────┐     ┌───────▼──────┐     ┌───────▼──────┐
             │ PostgreSQL  │     │ HTTP Services│     │ TCP Services │
             │  (prod/dev) │     │   (APIs)     │     │ (Redis, etc) │
             └─────────────┘     └──────────────┘     └──────────────┘
                    │                     │                     │
                    └─────────────────────┴─────────────────────┘
                              Authentication Providers
                           (Keycloak, LDAP, Local Users)
```

## Quick Start

### Build

```bash
# Build for current platform
make build

# Build for all platforms
make build-all

# Build optimized release
make build-release
```

### Configure

Create `config.yaml` with your connections and authentication:

```yaml
auth:
  jwt_secret: "your-secret-key"

  # Authentication providers
  providers:
    - name: keycloak
      type: oidc
      enabled: true
      config:
        issuer: "http://localhost:8180/realms/portauth"
        client_id: "port-authorizing"
        client_secret: "your-secret"

  # Local users (backward compatible)
  users:
    - username: admin
      password: admin123
      roles: [admin]

connections:
  - name: postgres-prod
    type: postgres
    host: db.example.com
    port: 5432
    tags: [env:production, type:database]
    backend_username: "dbuser"
    backend_password: "dbpass"

policies:
  - name: admin-full-access
    roles: [admin]
    tags: [env:production]
    whitelist: [".*"]
```

### Run

```bash
# Start Docker test environment
make docker-up

# Start API server
./bin/port-authorizing-api

# Login
./bin/port-authorizing-cli login -u admin -p admin123

# List available connections
./bin/port-authorizing-cli list

# Connect to PostgreSQL
./bin/port-authorizing-cli connect postgres-prod -l 5433

# Use with psql
psql -h localhost -p 5433 -U anyuser
```

## Detailed Usage

### API Server

```bash
# Start the API server
./bin/port-authorizing-api

# With custom config
./bin/port-authorizing-api --config /path/to/config.yaml

# Development mode (auto-reload)
make dev
```

### CLI Client

```bash
# Login with local auth
./bin/port-authorizing-cli login -u admin -p admin123

# Login with OIDC (if configured)
./bin/port-authorizing-cli login -u alice -p password123

# List available connections (filtered by role)
./bin/port-authorizing-cli list

# Connect to PostgreSQL
./bin/port-authorizing-cli connect postgres-prod -l 5433 -d 1h

# Connect to HTTP API
./bin/port-authorizing-cli connect api-gateway -l 8080 -d 2h

# Use proxied connections
psql -h localhost -p 5433 -U anyuser
curl http://localhost:8080/api/endpoint
```

## Configuration

### Complete Example (`config.yaml`)

```yaml
server:
  port: 8080
  max_connection_duration: 2h

auth:
  jwt_secret: "change-this-in-production"
  token_expiry: 24h

  # Multiple authentication providers
  providers:
    # OIDC (Keycloak, Auth0, Okta, etc.)
    - name: keycloak
      type: oidc
      enabled: true
      config:
        issuer: "http://localhost:8180/realms/portauth"
        client_id: "port-authorizing"
        client_secret: "your-client-secret"
        redirect_url: "http://localhost:8080/auth/callback/oidc"
        roles_claim: "roles"
        username_claim: "preferred_username"

    # LDAP / Active Directory
    - name: corporate-ldap
      type: ldap
      enabled: false
      config:
        url: "ldap.example.com:389"
        bind_dn: "cn=admin,dc=example,dc=com"
        bind_password: "adminpass"
        user_base_dn: "ou=users,dc=example,dc=com"
        user_filter: "(uid=%s)"
        group_base_dn: "ou=groups,dc=example,dc=com"

  # Local users (backward compatible)
  users:
    - username: admin
      password: admin123
      roles: [admin]
    - username: developer
      password: dev123
      roles: [developer]

# Connections with tags
connections:
  - name: postgres-prod
    type: postgres
    host: prod-db.example.com
    port: 5432
    tags:
      - env:production
      - type:database
      - team:backend
    backend_username: "produser"
    backend_password: "prodpass"
    backend_database: "app"
    metadata:
      description: "Production database"

  - name: postgres-dev
    type: postgres
    host: dev-db.example.com
    port: 5432
    tags:
      - env:dev
      - type:database
      - team:backend
    backend_username: "devuser"
    backend_password: "devpass"

  - name: api-gateway
    type: http
    host: api.internal.example.com
    port: 443
    scheme: https
    tags:
      - env:production
      - type:api

# Role-based access policies
policies:
  # Admins have full access
  - name: admin-all
    roles: [admin]
    tags: [env:production, env:dev]
    tag_match: any
    whitelist: [".*"]

  # Developers: full dev, read-only prod
  - name: dev-full-dev
    roles: [developer]
    tags: [env:dev]
    tag_match: any
    whitelist: [".*"]

  - name: dev-readonly-prod
    roles: [developer]
    tags: [env:production]
    tag_match: any
    whitelist:
      - "^SELECT.*"
      - "^EXPLAIN.*"
      - "^GET .*"

security:
  enable_llm_analysis: false

logging:
  audit_log_path: "audit.log"
  log_level: "info"
```

See [AUTHENTICATION_GUIDE.md](AUTHENTICATION_GUIDE.md) for detailed configuration guide.

## Feature Highlights

### Authentication
- ✅ Multiple authentication providers (Local, OIDC, LDAP, SAML2)
- ✅ JWT token-based sessions
- ✅ Role extraction from auth providers
- ✅ Provider failover and chaining

### Authorization
- ✅ Role-based access control (RBAC)
- ✅ Tag-based connection policies
- ✅ Flexible whitelist patterns (SQL, HTTP, TCP)
- ✅ Multi-tag matching (any/all)
- ✅ Environment isolation (dev/staging/prod)

### Protocols
- ✅ PostgreSQL with credential substitution
- ✅ HTTP/HTTPS with full request proxying
- ✅ Generic TCP streaming
- ✅ Protocol-aware query logging
- ✅ Transparent connection handling

### Security
- ✅ Comprehensive audit logging
- ✅ Automatic connection timeouts
- ✅ Query/request whitelisting
- ✅ Connection ownership verification
- ⏳ LLM-based risk analysis
- ⏳ Rate limiting
- ⏳ IP-based restrictions

## Testing

### Quick End-to-End Test

```bash
# Run comprehensive test suite
./test.sh

# Or using make
make test-e2e
```

This will:
1. Start all Docker services (PostgreSQL, Nginx, Keycloak, LDAP)
2. Start the API server
3. Test local authentication
4. Test OIDC authentication (Keycloak)
5. Test LDAP authentication
6. Test role-based connection filtering
7. Test HTTP and PostgreSQL proxying
8. Verify audit logging
9. Clean up

### Docker Test Environment

```bash
# Start all services
make docker-up

# This starts:
# - PostgreSQL (5432)
# - Nginx (8888)
# - Keycloak (8180) - OIDC/SAML2 provider
# - OpenLDAP (389) - LDAP server
# - phpLDAPadmin (8181) - LDAP UI
```

**Test Users** (password: `password123`):
- **alice** - developer role
- **bob** - admin role
- **charlie** - qa role

### Manual Testing

```bash
# Start services
make docker-up

# Start API
./bin/port-authorizing-api &

# Test local auth
./bin/port-authorizing-cli login -u admin -p admin123

# Test OIDC (Keycloak)
./bin/port-authorizing-cli login -u alice -p password123

# List connections (role-based filtering)
./bin/port-authorizing-cli list

# Connect to services
./bin/port-authorizing-cli connect postgres-test -l 5433
psql -h localhost -p 5433 -U testuser testdb

# Test HTTP proxy
./bin/port-authorizing-cli connect nginx-server -l 9090
curl http://localhost:9090/

# Check audit log
tail -f audit.log | jq
```

### Unit Tests

```bash
# Run Go unit tests
make test

# With coverage
go test -cover ./...
```

See [DOCKER_TESTING.md](DOCKER_TESTING.md) and [AUTHENTICATION_GUIDE.md](AUTHENTICATION_GUIDE.md) for detailed guides.

## Development

### Building

```bash
# Build for current platform
make build

# Build optimized release
make build-release

# Build for specific platforms
make build-linux          # Linux amd64
make build-linux-arm64    # Linux ARM64
make build-darwin         # macOS Intel
make build-darwin-arm64   # macOS Apple Silicon
make build-windows        # Windows amd64

# Build for all platforms
make build-all

# Cross-compile and create archives
make cross-compile

# Build Docker image
make build-docker
```

### Development Workflow

```bash
# Install dependencies
make deps

# Format code
make fmt

# Run linter
make lint

# Run in development mode (auto-reload)
make dev

# Run tests
make test

# Run E2E tests
make test-e2e
```

### Docker Services

```bash
# Start all services
make docker-up

# Stop services
make docker-down

# View logs
make docker-logs
```

### Environment Variables

```bash
# Custom version
VERSION=1.0.0 make build

# Custom output directory
BIN_DIR=/tmp/build make build
```

## Security Best Practices

### Authentication
- ✅ Use OIDC or LDAP in production (not local users)
- ✅ Rotate JWT secrets regularly
- ✅ Set appropriate token expiry times
- ✅ Use strong passwords/secrets

### Authorization
- ✅ Apply least privilege principle
- ✅ Use specific whitelist patterns (avoid `.*`)
- ✅ Tag connections consistently
- ✅ Review policies regularly

### Deployment
- ✅ Use TLS for API server
- ✅ Rotate backend database credentials
- ✅ Store secrets in environment variables or secret managers
- ✅ Enable audit logging
- ✅ Monitor audit logs for anomalies
- ✅ Set appropriate connection timeouts

### Network
- ✅ Run API behind reverse proxy (nginx, traefik)
- ✅ Use firewall rules to restrict access
- ✅ Enable network segmentation
- ✅ Use VPN for remote access

## Documentation

- [AUTHENTICATION_GUIDE.md](AUTHENTICATION_GUIDE.md) - Complete auth/authz guide
- [AUTH_UPGRADE_SUMMARY.md](AUTH_UPGRADE_SUMMARY.md) - Migration and upgrade guide
- [ARCHITECTURE.md](ARCHITECTURE.md) - System architecture
- [DOCKER_TESTING.md](DOCKER_TESTING.md) - Docker testing guide
- [QUICK_REFERENCE.md](QUICK_REFERENCE.md) - Quick reference
- `config.example.yaml` - Comprehensive configuration example

## Contributing

Contributions welcome! Please read the architecture docs and follow the existing code style.

```bash
# Fork and clone
git clone https://github.com/yourusername/port-authorizing
cd port-authorizing

# Create feature branch
git checkout -b feature/my-feature

# Make changes and test
make test
make test-e2e

# Format and lint
make fmt
make lint

# Commit and push
git commit -m "Add my feature"
git push origin feature/my-feature
```

## License

MIT

## Support

For issues, questions, or contributions:
- Open an issue on GitHub
- Check documentation in `AUTHENTICATION_GUIDE.md`
- Review example config in `config.example.yaml`

