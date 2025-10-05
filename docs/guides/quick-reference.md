# Quick Reference

## Installation & Setup

```bash
# Build
make build

# Copy config
cp config.example.yaml config.yaml
```

## Start Services

```bash
# Start Docker services (PostgreSQL + Nginx)
docker-compose up -d

# Start API server
./bin/port-authorizing-api --config config.yaml
```

## CLI Commands

### Login
```bash
./bin/port-authorizing-cli login -u admin -p admin123
```

### List Connections
```bash
./bin/port-authorizing-cli list
```

### Connect to Service
```bash
# HTTP (Nginx)
./bin/port-authorizing-cli connect nginx-server -l 9090 -d 1h

# PostgreSQL
./bin/port-authorizing-cli connect postgres-test -l 5433 -d 1h
```

### Options
- `-l, --local-port` - Local port to listen on (required)
- `-d, --duration` - Connection duration (e.g., 30m, 1h, 2h)
- `--api-url` - API server URL (default: http://localhost:8080)

## Using Through Proxy

### HTTP (Nginx)
```bash
# After connecting on port 9090
curl http://localhost:9090/
curl http://localhost:9090/api/
curl http://localhost:9090/health
```

### PostgreSQL
```bash
# After connecting on port 5433
psql -h localhost -p 5433 -U testuser -d testdb

# Or via curl
curl -X POST http://localhost:8080/api/proxy/CONNECTION_ID \
  -H "Authorization: Bearer TOKEN" \
  --data "SELECT * FROM users;"
```

## API Endpoints

### Public
- `POST /api/login` - Login and get JWT token
- `GET /api/health` - Health check

### Protected (require JWT)
- `GET /api/connections` - List available connections
- `POST /api/connect/{name}` - Create connection
- `POST /api/proxy/{connectionID}` - Proxy request

## Configuration

### Basic Setup
```yaml
server:
  port: 8080
  max_connection_duration: 2h

auth:
  jwt_secret: "your-secret-key"
  token_expiry: 24h
  users:
    - username: admin
      password: admin123

connections:
  - name: my-service
    type: http  # or postgres, tcp
    host: localhost
    port: 8888
    whitelist:
      - "^SELECT.*"  # for postgres
```

## Testing

### Run All Tests
```bash
./test.sh
```

### Start Docker Only
```bash
make docker-up
```

### Stop Docker
```bash
make docker-down
```

### View Logs
```bash
# Audit log
cat audit.log | jq

# API log
tail -f api.log

# Docker logs
docker-compose logs -f
```

## Audit Log

### View All Events
```bash
cat audit.log | jq
```

### Filter Events
```bash
# By action
cat audit.log | jq 'select(.action == "login")'
cat audit.log | jq 'select(.action == "connect")'
cat audit.log | jq 'select(.action == "proxy_request")'

# By resource
cat audit.log | jq 'select(.resource == "nginx-server")'
cat audit.log | jq 'select(.resource == "postgres-test")'

# By user
cat audit.log | jq 'select(.username == "admin")'
```

### Statistics
```bash
# Count by action
cat audit.log | jq -r '.action' | sort | uniq -c

# Count by user
cat audit.log | jq -r '.username' | sort | uniq -c
```

## Whitelist Patterns

### PostgreSQL
```yaml
whitelist:
  - "^SELECT.*"              # All SELECT queries
  - "^SELECT.*FROM users.*"  # SELECT from users table
  - "^INSERT INTO logs.*"    # INSERT to logs table only
  - "^UPDATE users SET.*"    # UPDATE users table
```

### Pattern Syntax
- `^` - Start of string
- `.*` - Any characters
- `\d+` - One or more digits
- `[a-z]+` - One or more lowercase letters

## Troubleshooting

### API won't start
```bash
# Check port
lsof -i :8080

# Check config
./bin/port-authorizing-api --config config.yaml

# View logs
cat api.log
```

### Can't login
```bash
# Check credentials in config.yaml
cat config.yaml | grep -A 5 users

# Try direct API call
curl -X POST http://localhost:8080/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}'
```

### Connection not working
```bash
# List available connections
./bin/port-authorizing-cli list

# Check if service is running
curl http://localhost:8888/health  # Nginx
docker exec port-auth-postgres pg_isready  # PostgreSQL

# Check audit log
cat audit.log | tail -5
```

### Docker issues
```bash
# Check status
docker-compose ps

# Restart
docker-compose restart

# View logs
docker-compose logs

# Full reset
docker-compose down -v
docker-compose up -d
```

## Security Best Practices

1. **Change JWT Secret**
   ```yaml
   auth:
     jwt_secret: "use-a-long-random-string-here"
   ```

2. **Use Strong Passwords**
   - Hash passwords in production (bcrypt)
   - Don't commit passwords to git

3. **Configure Whitelists**
   ```yaml
   whitelist:
     - "^SELECT.*"  # Read-only access
   ```

4. **Set Short Timeouts**
   ```yaml
   server:
     max_connection_duration: 1h
   ```

5. **Monitor Audit Logs**
   ```bash
   tail -f audit.log | jq
   ```

## Common Workflows

### Give Developer Database Access
```bash
# 1. Add connection to config.yaml
# 2. Developer logs in
./bin/port-authorizing-cli login -u developer -p devpass

# 3. Developer connects
./bin/port-authorizing-cli connect postgres-prod -l 5433 -d 2h

# 4. Developer uses psql
psql -h localhost -p 5433 -U dbuser -d proddb

# 5. All queries logged with developer's username
```

### Proxy Internal API
```bash
# 1. Connect to internal API
./bin/port-authorizing-cli connect internal-api -l 8080 -d 1h

# 2. Use as normal
curl http://localhost:8080/api/endpoint
```

### Temporary Redis Access
```bash
# 1. Connect to Redis
./bin/port-authorizing-cli connect redis-cache -l 6379 -d 30m

# 2. Use redis-cli
redis-cli -p 6379
```

## Makefile Targets

```bash
make build      # Build binaries
make test       # Run unit tests
make test-e2e   # Run end-to-end tests
make clean      # Clean build artifacts
make docker-up  # Start Docker services
make docker-down # Stop Docker services
make docker-logs # View Docker logs
make run-api    # Run API server
make install    # Install to /usr/local/bin
make fmt        # Format code
make lint       # Run linter
make help       # Show help
```

## Environment Variables

```bash
# API server
export PORT=8080
export JWT_SECRET=your-secret
export MAX_CONNECTION_DURATION=2h

# CLI
export API_URL=http://localhost:8080
```

## Docker Services

### PostgreSQL
- **Port:** 5432
- **User:** testuser
- **Password:** testpass
- **Database:** testdb
- **Tables:** users, logs

### Nginx
- **Port:** 8888
- **Endpoints:**
  - GET / - Main page
  - GET /api/ - JSON response
  - GET /health - Health check

## Files & Directories

```
port-authorizing/
├── bin/                          # Compiled binaries
├── cmd/                          # Entry points
├── internal/                     # Internal packages
├── docker/                       # Docker configs
├── config.yaml                   # Configuration
├── audit.log                     # Audit trail
├── api.log                       # API logs
├── README.md                     # Overview
├── GETTING_STARTED.md           # Tutorial
├── ARCHITECTURE.md              # Technical details
├── DOCKER_TESTING.md            # Docker testing guide
└── QUICK_REFERENCE.md           # This file
```

## Support

- 📖 Documentation: See README.md, GETTING_STARTED.md
- 🏗️ Architecture: See ARCHITECTURE.md
- 🐳 Docker Testing: See DOCKER_TESTING.md
- 🐛 Issues: Check audit.log and api.log
- 💬 Questions: Review configuration examples

## Quick Links

- [README](README.md) - Project overview
- [Getting Started](GETTING_STARTED.md) - Step-by-step tutorial
- [Architecture](ARCHITECTURE.md) - Technical architecture
- [Docker Testing](DOCKER_TESTING.md) - Testing with Docker
- [TODO](TODO.md) - Future enhancements

