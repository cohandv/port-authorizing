# Port Authorizing

> **📦 Docker Hub** | 🔗 **[GitHub Repository](https://github.com/cohandv/port-authorizing)** | 📖 **[Documentation](https://github.com/cohandv/port-authorizing/tree/main/docs)** | 🐛 **[Issues](https://github.com/cohandv/port-authorizing/issues)** | 🚀 **[Releases](https://github.com/cohandv/port-authorizing/releases)**

**Secure proxy for any service with authentication, authorization, and audit logging.**

Port Authorizing provides time-limited, audited access to any service (PostgreSQL, HTTP, TCP) with centralized authentication (OIDC/LDAP/SAML2), role-based access control, and protocol-specific filtering.

## 📊 Protocol Maturity

| Protocol | Status | Features |
|----------|--------|----------|
| **PostgreSQL** | ✅ Mature | Full authentication, query whitelisting, username validation, audit logging |
| **HTTP/HTTPS** | ✅ Mature | Transparent proxying, authentication, full request/response handling |
| **TCP** | 🚧 Beta | Basic proxying with authentication, limited protocol awareness |

## 🚀 Quick Start

### Run Server

```bash
docker run -d \
  -p 8080:8080 \
  -v $(pwd)/config.yaml:/app/config.yaml \
  --name port-authorizing \
  cohandv/port-authorizing:latest
```

### Verify Server

```bash
curl http://localhost:8080/api/health
```

### Use as Client

The Docker image can also be used as a CLI client:

```bash
# Login to server
docker run --rm -v ~/.port-auth:/home/portauth/.port-auth \
  cohandv/port-authorizing:latest \
  login -u admin -p password --api-url http://your-server:8080

# List available connections
docker run --rm -v ~/.port-auth:/home/portauth/.port-auth \
  cohandv/port-authorizing:latest \
  list --api-url http://your-server:8080

# Check version
docker run --rm cohandv/port-authorizing:latest --version
```

**Note:** Client mode requires network access to your Port Authorizing server and volume mount for storing the auth token.

## 🐳 Docker Compose

```yaml
version: '3.8'

services:
  port-authorizing:
    image: cohandv/port-authorizing:latest
    ports:
      - "8080:8080"
    volumes:
      - ./config.yaml:/app/config.yaml
      - ./data:/app/data
      - ./logs:/app/logs
    environment:
      - JWT_SECRET=${JWT_SECRET}
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "--spider", "-q", "http://localhost:8080/api/health"]
      interval: 30s
      timeout: 3s
      retries: 3
```

## 📦 Available Tags

| Tag | Description |
|-----|-------------|
| `latest` | Latest stable release from main branch |
| `v2.0.0` | Specific version (semantic versioning) |
| `main` | Latest build from main branch (development) |

## 🏗️ Multi-Architecture Support

Images are available for:
- `linux/amd64` (x86_64)
- `linux/arm64` (ARM64 / Apple Silicon)

Docker will automatically pull the correct architecture.

## ⚙️ Configuration

### Minimum Config (config.yaml)

```yaml
server:
  port: 8080

auth:
  jwt_secret: "change-this-secret"

  # Local users
  users:
    - username: admin
      password: admin123
      roles: [admin]

connections:
  - name: postgres-prod
    type: postgres
    host: postgres.internal
    port: 5432
    backend_username: app_user
    backend_password: app_pass
    tags:
      - env:production

policies:
  - name: admin-full
    roles: [admin]
    tags: [env:production]
    whitelist: [".*"]
```

### Environment Variables

Override config values with environment variables:

```bash
docker run -d \
  -p 8080:8080 \
  -v $(pwd)/config.yaml:/app/config.yaml \
  -e JWT_SECRET=your-secret-key \
  -e SERVER_PORT=8080 \
  cohandv/port-authorizing:latest
```

## 🔐 OIDC Authentication (Keycloak)

```yaml
auth:
  providers:
    - name: keycloak
      type: oidc
      enabled: true
      config:
        issuer: "https://keycloak.example.com/realms/myapp"
        client_id: "port-authorizing"
        client_secret: "${KEYCLOAK_CLIENT_SECRET}"
        redirect_url: "https://api.example.com/api/auth/oidc/callback"
        roles_claim: "roles"
        username_claim: "preferred_username"
```

## 📖 Usage Examples

### 1. Complete Stack with PostgreSQL

```yaml
version: '3.8'

services:
  postgres:
    image: postgres:14
    environment:
      POSTGRES_PASSWORD: dbpass

  port-authorizing:
    image: cohandv/port-authorizing:latest
    ports:
      - "8080:8080"
    volumes:
      - ./config.yaml:/app/config.yaml
    depends_on:
      - postgres
```

### 2. With Keycloak

```yaml
version: '3.8'

services:
  keycloak:
    image: quay.io/keycloak/keycloak:23.0
    environment:
      KEYCLOAK_ADMIN: admin
      KEYCLOAK_ADMIN_PASSWORD: admin
    ports:
      - "8180:8080"
    command: start-dev

  port-authorizing:
    image: cohandv/port-authorizing:latest
    ports:
      - "8080:8080"
    volumes:
      - ./config.yaml:/app/config.yaml
    depends_on:
      - keycloak
```

### 3. Client Usage

The same image can run as a client:

```bash
# Login (opens browser for OIDC)
docker run --rm -it \
  --network host \
  -v ~/.port-auth:/root/.port-auth \
  cohandv/port-authorizing:latest \
  login --api-url http://localhost:8080

# List connections
docker run --rm -it \
  --network host \
  -v ~/.port-auth:/root/.port-auth \
  cohandv/port-authorizing:latest \
  list --api-url http://localhost:8080

# Connect to service (PostgreSQL example)
docker run --rm -it \
  --network host \
  -v ~/.port-auth:/root/.port-auth \
  cohandv/port-authorizing:latest \
  connect postgres-prod -l 5433 --api-url http://localhost:8080
```

Or install the binary locally:

```bash
# Extract binary from image
docker create --name temp cohandv/port-authorizing:latest
docker cp temp:/usr/local/bin/port-authorizing ./port-authorizing
docker rm temp

# Use locally
./port-authorizing login
./port-authorizing connect postgres-prod -l 5433
```

## 🔒 Security Features

- ✅ **Multi-provider authentication** - OIDC, LDAP, SAML2, local users
- ✅ **Role-based access control** - Different permissions per role
- ✅ **Query whitelisting** - Regex-based SQL filtering
- ✅ **Credential hiding** - Users never see backend passwords
- ✅ **Time-limited access** - Connections expire automatically
- ✅ **Full audit logging** - Every action logged with user identity
- ✅ **Username enforcement** - Prevents user impersonation

## 📊 Common Use Cases

### Temporary Production Database Access

Give developers read-only access to production for debugging:

```yaml
policies:
  - name: developer-readonly-prod
    roles: [developer]
    tags: [env:production]
    whitelist:
      - "^SELECT.*"
      - "^EXPLAIN.*"
```

Connect:
```bash
# Developer logs in via OIDC
port-authorizing login

# Gets 30-minute connection to prod
port-authorizing connect postgres-prod -l 5433

# Can only run SELECT queries
psql -h localhost -p 5433 -U alice -d myapp
```

### Different Access Per Environment

```yaml
policies:
  # Full access to test
  - name: dev-full-test
    roles: [developer]
    tags: [env:test]
    whitelist: [".*"]

  # Read-only in production
  - name: dev-readonly-prod
    roles: [developer]
    tags: [env:production]
    whitelist: ["^SELECT.*"]
```

### Audit All Database Access

All queries are logged with user identity:

```json
{
  "timestamp": "2025-10-04T15:30:00Z",
  "username": "alice",
  "action": "postgres_query",
  "resource": "postgres-prod",
  "metadata": {
    "query": "SELECT * FROM users WHERE id = 1",
    "allowed": true,
    "connection_id": "abc123"
  }
}
```

## 🔧 Configuration Reference

### Server Settings

```yaml
server:
  port: 8080                        # API port
  max_connection_duration: 2h       # Max connection time
```

### Logging

```yaml
logging:
  audit_log_path: "/app/logs/audit.log"  # Audit log location
  enable_llm_analysis: false              # Optional LLM analysis
```

### Connections

```yaml
connections:
  - name: postgres-prod              # Connection name
    type: postgres                    # Type: postgres, http, tcp
    host: db.internal                 # Backend host
    port: 5432                        # Backend port
    duration: 30m                     # Connection timeout
    tags:                             # Tags for policy matching
      - env:production
      - team:platform
    backend_username: app_user        # Real DB credentials
    backend_password: "${DB_PASS}"    # From environment
    backend_database: myapp
```

## 📝 Health Check

The container includes a built-in health check:

```bash
docker inspect --format='{{.State.Health.Status}}' port-authorizing
```

Manual check:
```bash
curl http://localhost:8080/api/health
# Response: {"status":"healthy"}
```

## 🐛 Troubleshooting

### Container won't start

```bash
# Check logs
docker logs port-authorizing

# Common issues:
# - Invalid config.yaml
# - Port 8080 already in use
# - Missing volume mounts
```

### Can't connect to backend database

```bash
# Test from container
docker exec port-authorizing wget -O- http://backend-db:5432

# Check network
docker network inspect bridge
```

### Authentication fails

```bash
# Verify OIDC configuration
curl http://localhost:8080/api/health

# Check issuer is reachable
curl https://keycloak.example.com/realms/myapp/.well-known/openid-configuration
```

## 📚 Full Documentation

- **GitHub Repository**: https://github.com/yourusername/port-authorizing
- **Getting Started**: [Documentation](https://github.com/yourusername/port-authorizing/blob/main/docs/guides/getting-started.md)
- **OIDC Setup Guide**: [OIDC Guide](https://github.com/yourusername/port-authorizing/blob/main/docs/guides/oidc-setup.md)
- **Configuration Reference**: [Config Guide](https://github.com/yourusername/port-authorizing/blob/main/docs/guides/configuration.md)

## 📄 License

[Your License]

## 🤝 Contributing

Issues and PRs welcome! See [GitHub repository](https://github.com/yourusername/port-authorizing)

## 💬 Support

- **Issues**: https://github.com/yourusername/port-authorizing/issues
- **Discussions**: https://github.com/yourusername/port-authorizing/discussions

---

**Image Size**: ~30MB (Alpine-based)
**Base Image**: `alpine:latest`
**Build**: Multi-stage with Go 1.21
**Security**: Runs as non-root user

