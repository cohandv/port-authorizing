# Architecture

## Overview

Port Authorizing is a secure proxy system that acts as a central hub for authenticated and audited access to protected services. It consists of two main components: an API server and a CLI client.

## System Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                         User Layer                          │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐   │
│  │   psql   │  │   curl   │  │ redis-cli│  │  custom  │   │
│  └─────┬────┘  └─────┬────┘  └─────┬────┘  └─────┬────┘   │
└────────┼─────────────┼─────────────┼─────────────┼─────────┘
         │             │             │             │
         └─────────────┴─────────────┴─────────────┘
                       │
         ┌─────────────▼─────────────┐
         │     CLI Local Proxy       │
         │   (localhost:XXXX)        │
         └─────────────┬─────────────┘
                       │ JWT Auth
                       │ Connection ID
         ┌─────────────▼─────────────┐
         │       API Server          │
         │   ┌───────────────────┐   │
         │   │  Authentication   │   │
         │   │     (JWT)         │   │
         │   └───────────────────┘   │
         │   ┌───────────────────┐   │
         │   │  Authorization    │   │
         │   │ (Connection-based)│   │
         │   └───────────────────┘   │
         │   ┌───────────────────┐   │
         │   │ Connection Manager│   │
         │   │  (Timeouts/Track) │   │
         │   └───────────────────┘   │
         │   ┌───────────────────┐   │
         │   │Security Validation│   │
         │   │  (Whitelist/LLM)  │   │
         │   └───────────────────┘   │
         │   ┌───────────────────┐   │
         │   │  Audit Logger     │   │
         │   └───────────────────┘   │
         │   ┌───────────────────┐   │
         │   │ Protocol Handlers │   │
         │   │ HTTP │ PG │ TCP   │   │
         │   └───────────────────┘   │
         └─────────────┬─────────────┘
                       │
         ┌─────────────▼─────────────┐
         │   Protected Services      │
         │  ┌──────┐  ┌──────┐      │
         │  │  PG  │  │ HTTP │      │
         │  └──────┘  └──────┘      │
         │  ┌──────┐  ┌──────┐      │
         │  │ TCP  │  │Redis │      │
         │  └──────┘  └──────┘      │
         └───────────────────────────┘
```

## Component Details

### API Server

**Entry Point:** `cmd/api/main.go`

#### Packages:

1. **`internal/api`** - HTTP server and handlers
   - `server.go` - HTTP server setup with Gorilla Mux
   - `auth.go` - JWT authentication logic
   - `handlers.go` - API endpoint handlers

2. **`internal/config`** - Configuration management
   - `config.go` - YAML configuration parsing
   - Defines connection endpoints, users, security settings

3. **`internal/proxy`** - Protocol handlers
   - `manager.go` - Connection lifecycle management
   - `protocol.go` - Protocol interface definition
   - `http.go` - HTTP/HTTPS proxy implementation
   - `postgres.go` - PostgreSQL proxy (simplified)
   - `tcp.go` - Raw TCP proxy

4. **`internal/security`** - Security validation
   - `whitelist.go` - Regex-based query validation
   - LLM risk analysis integration (placeholder)

5. **`internal/audit`** - Audit logging
   - `logger.go` - JSON-formatted audit log writer

### CLI Client

**Entry Point:** `cmd/cli/main.go`

#### Commands:

1. **`login`** - Authenticate with API server
   - Stores JWT token in `~/.port-auth/config.json`
   - Token used for all subsequent requests

2. **`list`** - List available connections
   - Queries API for configured endpoints
   - Displays connection metadata

3. **`connect`** - Establish proxy connection
   - Opens local TCP port
   - Forwards all traffic through API to target
   - Maintains connection until timeout or interrupt

## Request Flow

### 1. Login Flow

```
User → CLI login → API /api/login → JWT Token → Store locally
```

1. User provides username/password
2. CLI sends credentials to API
3. API validates against config
4. API generates JWT token
5. CLI stores token in config file

### 2. List Connections Flow

```
User → CLI list → API /api/connections (with JWT) → Connection list
```

1. CLI reads stored JWT token
2. Sends GET request with Authorization header
3. API validates JWT
4. API returns list of configured connections
5. CLI displays formatted list

### 3. Connect Flow

```
User → CLI connect → API /api/connect/{name} → Connection ID
    → CLI starts local proxy → User uses client → Proxy forwards
```

1. CLI requests connection from API with desired duration
2. API validates JWT and connection name
3. API creates connection record with unique ID
4. API returns connection ID and proxy URL
5. CLI opens local TCP port
6. User connects standard client to local port
7. CLI forwards all data to API proxy endpoint
8. API forwards to actual target service
9. API validates queries (whitelist, LLM)
10. API logs all requests with username

### 4. Proxy Request Flow

```
psql → localhost:5433 → CLI proxy → API /api/proxy/{connID}
    → Security validation → Target service → Response back
```

For each request:
1. Data arrives at CLI local port
2. CLI wraps in HTTP POST with JWT
3. API validates JWT and connection ID
4. API checks if connection expired
5. API validates query against whitelist
6. API (optionally) performs LLM risk analysis
7. API logs request with username
8. API forwards to target via protocol handler
9. Response flows back through chain

## Security Model

### Authentication (JWT)

- Users defined in config file
- Login generates JWT with configurable expiry (default 24h)
- JWT contains username and expiration
- All API requests require valid JWT

### Authorization

- Per-connection access control
- Connection ownership tracked
- Only creator can use their connections
- Connections auto-expire after configured duration

### Validation

1. **Whitelist** - Regex patterns for allowed queries
   ```yaml
   whitelist:
     - "^SELECT.*"
     - "^INSERT INTO logs.*"
   ```

2. **LLM Analysis** (optional) - AI-based risk detection
   - Analyzes query patterns
   - Detects SQL injection attempts
   - Identifies suspicious operations

### Audit Trail

Every operation logged with:
- Timestamp
- Username
- Action (login, connect, proxy_request)
- Resource (connection name)
- Metadata (connection ID, query details)

Format: JSON lines in `audit.log`

## Protocol Handlers

### Interface

```go
type Protocol interface {
    HandleRequest(w http.ResponseWriter, r *http.Request) error
    Close() error
}
```

### HTTP Handler

- Forward HTTP/HTTPS requests
- Copy headers and body
- Support all HTTP methods
- Handle TLS connections

### PostgreSQL Handler

- Parse PostgreSQL wire protocol (TODO: full implementation)
- Validate queries against whitelist
- Forward to PostgreSQL server
- Return results in wire protocol format

### TCP Handler

- Raw TCP data forwarding
- Bidirectional stream
- No protocol-specific logic

### Extensibility

Add new protocols by:
1. Create new file in `internal/proxy/`
2. Implement `Protocol` interface
3. Add case in `NewProtocol()` function
4. Update config with new type

Example:
```go
// internal/proxy/mysql.go
type MySQLProxy struct { ... }

func (p *MySQLProxy) HandleRequest(...) error {
    // MySQL-specific logic
}

// internal/proxy/protocol.go
case "mysql":
    return NewMySQLProxy(connConfig), nil
```

## Connection Management

### Lifecycle

1. **Create** - User requests connection
   - Generate unique UUID
   - Calculate expiry time
   - Create protocol handler
   - Store in memory map

2. **Active** - Connection in use
   - Track all requests
   - Validate on each use
   - Log all operations

3. **Expire** - Timeout reached
   - Cleanup goroutine removes connection
   - Close protocol handler
   - Remove from map

### Concurrency

- Connection map protected by RWMutex
- Cleanup ticker runs every 30 seconds
- Safe for concurrent access

## Configuration

### Structure

```yaml
server:
  port: 8080                          # API listen port
  max_connection_duration: 2h         # Max lifetime per connection

auth:
  jwt_secret: "..."                   # Secret for JWT signing
  token_expiry: 24h                   # Token validity period
  users: [...]                        # User definitions

connections: [...]                    # Available endpoints

security:
  enable_llm_analysis: false          # Optional AI validation
  llm_provider: "openai"
  llm_api_key: "..."

logging:
  audit_log_path: "audit.log"
  log_level: "info"
```

## Future Enhancements

### Planned Features

1. **Database-backed User Management**
   - Store users in PostgreSQL/MySQL
   - Support for user groups/roles
   - Password hashing with bcrypt

2. **Rate Limiting**
   - Per-user request limits
   - Per-connection limits
   - Configurable windows

3. **Full PostgreSQL Wire Protocol**
   - Complete protocol parser
   - Transaction support
   - Prepared statements

4. **WebSocket Support**
   - Real-time bidirectional communication
   - Connection status updates

5. **Metrics & Monitoring**
   - Prometheus metrics
   - Grafana dashboards
   - Active connection counts
   - Request rates

6. **LLM Integration**
   - OpenAI GPT-4 integration
   - Anthropic Claude integration
   - Custom model support
   - Risk scoring

7. **TLS/SSL**
   - HTTPS for API server
   - Certificate management
   - mTLS for client auth

## Performance Considerations

- Connection pooling for protocol handlers
- HTTP client connection reuse
- Efficient audit log buffering
- Minimal memory footprint per connection

## Deployment

### Development
```bash
make build && ./bin/port-authorizing-api
```

### Production
- Run behind reverse proxy (nginx/Traefik)
- Enable TLS
- Use environment variables for secrets
- Set up log rotation
- Monitor with systemd/supervisord

### Docker (future)
```dockerfile
FROM golang:1.21-alpine
WORKDIR /app
COPY . .
RUN go build -o api ./cmd/api
CMD ["./api", "--config", "/config/config.yaml"]
```

## Testing Strategy

1. **Unit Tests** - Individual component testing
2. **Integration Tests** - API endpoint testing
3. **End-to-End Tests** - CLI to API to service
4. **Security Tests** - Whitelist validation, auth bypass attempts
5. **Load Tests** - Concurrent connection handling

## Dependencies

- `github.com/gorilla/mux` - HTTP routing
- `github.com/golang-jwt/jwt/v5` - JWT authentication
- `github.com/spf13/cobra` - CLI framework
- `gopkg.in/yaml.v3` - YAML parsing
- `github.com/google/uuid` - UUID generation

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for:
- Code style guide
- Development workflow
- Pull request process
- Testing requirements


