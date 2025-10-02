# Port Authorizing

A secure proxy system for controlled access to protected services with authentication, authorization, and audit logging.

## Architecture

```
┌─────────┐      ┌─────────┐      ┌──────────────┐
│  User   │──────│   CLI   │──────│     API      │
│ (psql)  │      │ (proxy) │      │   (proxy)    │
└─────────┘      └─────────┘      └──────────────┘
                                          │
                                          ├──── PostgreSQL
                                          ├──── HTTP Services
                                          └──── TCP Services
```

## Components

### API Server
- **Authentication**: JWT-based user authentication
- **Authorization**: Connection-based authorization from config
- **Proxy Hub**: Forwards connections to real endpoints (HTTP, PostgreSQL, TCP)
- **Security**: Whitelist validation and optional LLM risk analysis
- **Audit Logging**: All requests logged with user information
- **Connection Management**: Timeout control for active connections

### CLI Client
- **Login**: Authenticate with API server
- **List Connections**: View available proxy endpoints
- **Connect**: Open local port that proxies through API to target service
- **Duration Control**: Specify connection duration (respects API max timeout)

## Usage

### API Server

```bash
# Start the API server
./port-authorizing-api

# With custom config
./port-authorizing-api --config /path/to/config.yaml
```

### CLI Client

```bash
# Login
./port-authorizing-cli login --username user --password pass

# List available connections
./port-authorizing-cli list

# Connect to a PostgreSQL database
./port-authorizing-cli connect postgres-prod --local-port 5433 --duration 1h

# Now use your favorite client
psql -h localhost -p 5433 -U dbuser
```

## Configuration

### API Configuration (`config.yaml`)

```yaml
server:
  port: 8080
  max_connection_duration: 2h

auth:
  jwt_secret: your-secret-key
  token_expiry: 24h

connections:
  - name: postgres-prod
    type: postgres
    host: prod-db.example.com
    port: 5432
    whitelist:
      - "SELECT.*"
      - "INSERT INTO logs.*"

  - name: api-gateway
    type: http
    host: internal-api.example.com
    port: 443
    scheme: https

  - name: redis-cache
    type: tcp
    host: redis.example.com
    port: 6379

security:
  enable_llm_analysis: false
  llm_provider: openai
  llm_api_key: sk-...

logging:
  audit_log_path: /var/log/port-auth/audit.log
  log_level: info
```

## Features

- ✅ Multi-protocol support (HTTP, PostgreSQL, TCP)
- ✅ JWT authentication
- ✅ Connection whitelisting
- ✅ Audit logging
- ✅ Connection timeout management
- ✅ Extensible protocol interface
- ✅ Docker-based testing environment
- ✅ End-to-end test suite
- ⏳ Optional LLM risk analysis
- ⏳ User management
- ⏳ Rate limiting

## Testing

### Quick Test

```bash
# Run comprehensive end-to-end test
./test.sh
```

This will:
1. Start PostgreSQL and Nginx in Docker
2. Start the API server
3. Test authentication and authorization
4. Test HTTP proxy through Nginx
5. Test PostgreSQL proxy
6. Verify all activity is logged to audit.log
7. Clean up everything

### Manual Testing

```bash
# Start Docker services
docker-compose up -d

# Start API server
./bin/port-authorizing-api --config config.yaml &

# Login and test
./bin/port-authorizing-cli login -u admin -p admin123
./bin/port-authorizing-cli list
./bin/port-authorizing-cli connect nginx-server -l 9090 -d 1h

# In another terminal
curl http://localhost:9090/

# View audit log
cat audit.log | jq
```

See [DOCKER_TESTING.md](DOCKER_TESTING.md) for detailed testing guide.

## Development

```bash
# Build both API and CLI
make build

# Run unit tests
make test

# Run end-to-end tests with Docker
make test-e2e

# Start Docker services (PostgreSQL + Nginx)
make docker-up

# Stop Docker services
make docker-down
```

## Security Considerations

1. **Authentication**: Uses JWT tokens with configurable expiry
2. **Authorization**: Per-connection access control
3. **Whitelist**: Pattern-based query validation
4. **Audit Trail**: Complete logging of all operations
5. **Timeouts**: Automatic connection termination
6. **Optional LLM**: AI-based risk analysis for suspicious patterns

## License

MIT

