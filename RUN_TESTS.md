# 🧪 How to Run Tests

## One-Command Test

The easiest way to test everything:

```bash
./test.sh
```

This will:
1. ✅ Start Docker services (PostgreSQL + Nginx)
2. ✅ Start API server
3. ✅ Test authentication
4. ✅ Test HTTP proxy through Nginx
5. ✅ Test PostgreSQL proxy
6. ✅ Verify audit logging
7. ✅ Clean up everything

## Expected Output

```
🚀 Port Authorizing End-to-End Test Suite
==========================================

Checking dependencies...
✓ Dependencies OK

Building binaries...
✓ Build complete!

Step 1: Starting Docker services (PostgreSQL + Nginx)...
✓ Docker services are healthy
✓ Nginx is accessible on port 8888
✓ PostgreSQL is accessible

Step 2: Starting API server...
✓ API server started (PID: 12345)
✓ API server is ready

Step 3: Testing API health endpoint...
✓ API health check passed

Step 4: Testing CLI login...
✓ CLI login successful

Step 5: Listing available connections...
Available Connections:
----------------------
  • postgres-test [postgres]
    description: Test PostgreSQL database (Docker)
  • nginx-server [http]
    description: Test Nginx web server (Docker)

✓ Connections listed successfully

Step 6: Testing HTTP proxy (CLI → API → Nginx)...
✓ Connection created: 550e8400-e29b-41d4-a716-446655440000
✓ HTTP proxy successful! Got response from Nginx
✓ HTTP API proxy successful!

Step 7: Testing PostgreSQL proxy (CLI → API → PostgreSQL)...
✓ PostgreSQL connection created: 660f9511-f30c-52e5-b827-557766551111
✓ PostgreSQL proxy query sent
✓ PostgreSQL INSERT query sent

Step 8: Verifying audit logs...
✓ Audit log contains 8 entries
  • Login events: 2
  • List connections events: 1
  • Connection establishment events: 2
  • Proxy request events: 3
  • Nginx proxy activity: 1
  • PostgreSQL proxy activity: 1

Sample audit log entries:
[JSON output showing login, connection, and proxy events]

Step 9: Testing whitelist validation...
✓ Whitelist validation working - DELETE query blocked

========================================
✅ All End-to-End Tests Passed!
========================================

Test Summary:
  ✓ Docker services (Nginx + PostgreSQL) running
  ✓ API server operational
  ✓ CLI authentication working
  ✓ HTTP proxy through Nginx successful
  ✓ PostgreSQL proxy functional
  ✓ Audit logging captured all activity
  ✓ Whitelist validation active

Audit Log Statistics:
  • Total events: 8
  • Login events: 2
  • Connection events: 2
  • Proxy requests: 3
  • Nginx activity: 1
  • PostgreSQL activity: 1

Files:
  • API log: api.log
  • Audit log: audit.log (full activity trail)

Next steps:
  1. Review audit.log for complete activity trail
  2. Try interactive mode:
     ./bin/port-authorizing-cli connect nginx-server -l 9090 -d 1h
     curl http://localhost:9090/
  3. View Docker logs: docker-compose logs
  4. Stop services: docker-compose down
```

## Manual Testing

### Step 1: Start Services

```bash
# Start Docker
docker-compose up -d

# Wait for services to be healthy (5-10 seconds)
docker-compose ps

# Start API
./bin/port-authorizing-api --config config.yaml &
```

### Step 2: Login

```bash
./bin/port-authorizing-cli login -u admin -p admin123
```

### Step 3: Test HTTP Proxy (Nginx)

```bash
# Connect to Nginx
./bin/port-authorizing-cli connect nginx-server -l 9090 -d 1h

# In another terminal, test it
curl http://localhost:9090/
curl http://localhost:9090/api/
curl http://localhost:9090/health
```

### Step 4: Test PostgreSQL Proxy

```bash
# Connect to PostgreSQL
./bin/port-authorizing-cli connect postgres-test -l 5433 -d 1h

# In another terminal, test it
# (Note: Full wire protocol pending, use API for now)
TOKEN=$(cat ~/.port-auth/config.json | jq -r .token)
CONN_ID="your-connection-id"

curl -X POST http://localhost:8080/api/proxy/$CONN_ID \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/octet-stream" \
  -d "SELECT * FROM users;"
```

### Step 5: View Audit Logs

```bash
# View all events
cat audit.log | jq

# Filter by action
cat audit.log | jq 'select(.action == "proxy_request")'

# Count by resource
cat audit.log | jq -r '.resource' | sort | uniq -c
```

### Step 6: Cleanup

```bash
# Stop Docker
docker-compose down

# Stop API (if running in background)
pkill port-authorizing-api
```

## Quick Tests

### Just Test HTTP Proxy

```bash
# 1. Start services
docker-compose up -d
./bin/port-authorizing-api --config config.yaml &

# 2. Test
./bin/port-authorizing-cli login -u admin -p admin123
./bin/port-authorizing-cli connect nginx-server -l 9090 -d 5m
curl http://localhost:9090/

# 3. Check audit log
cat audit.log | jq 'select(.resource == "nginx-server")'
```

### Just Test Authentication

```bash
# Start API
./bin/port-authorizing-api --config config.yaml &

# Test login
./bin/port-authorizing-cli login -u admin -p admin123
./bin/port-authorizing-cli list

# Check audit log
cat audit.log | jq 'select(.action == "login")'
```

## Using Make Commands

```bash
# Build everything
make build

# Run all tests (unit + e2e)
make test-e2e

# Start Docker only
make docker-up

# Stop Docker
make docker-down

# View Docker logs
make docker-logs

# See all commands
make help
```

## Troubleshooting Tests

### Test fails at Docker startup

```bash
# Check Docker is running
docker ps

# Restart Docker services
docker-compose down -v
docker-compose up -d

# Check logs
docker-compose logs
```

### Test fails at API startup

```bash
# Check if port 8080 is in use
lsof -i :8080

# View API logs
cat api.log

# Try running API manually
./bin/port-authorizing-api --config config.yaml
```

### Test fails at authentication

```bash
# Check config file
cat config.yaml | grep -A 5 users

# Try login manually
./bin/port-authorizing-cli login -u admin -p admin123

# Check API health
curl http://localhost:8080/api/health
```

### Audit log is empty

```bash
# Check if file exists
ls -la audit.log

# Check permissions
chmod 644 audit.log

# Check API can write
cat api.log | grep -i audit
```

## Test Requirements

### System Requirements
- Docker & docker-compose
- Go 1.21+
- curl
- jq (optional, for pretty JSON)
- psql (optional, for PostgreSQL testing)

### Check Requirements

```bash
# Check Docker
docker --version
docker-compose --version

# Check Go
go version

# Check other tools
curl --version
jq --version    # Optional
psql --version  # Optional
```

## CI/CD Integration

### GitHub Actions

```yaml
name: Test
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      - name: Run tests
        run: ./test.sh
```

### GitLab CI

```yaml
test:
  image: golang:1.21
  services:
    - docker:dind
  script:
    - ./test.sh
```

## What Gets Tested

### ✅ Authentication
- JWT token generation
- Token validation
- Login endpoint
- Authorization headers

### ✅ Connections
- List available connections
- Create connection with timeout
- Connection expiration
- Connection ownership

### ✅ HTTP Proxy
- Request forwarding
- Header copying
- Response handling
- Nginx integration

### ✅ PostgreSQL Proxy
- Query forwarding
- Whitelist validation
- Blocked query handling
- PostgreSQL integration

### ✅ Audit Logging
- Login events
- Connection events
- Proxy requests
- User attribution
- Timestamps
- Metadata capture

### ✅ Security
- JWT authentication required
- Connection authorization
- Whitelist enforcement
- Query blocking

## Performance Testing

### Load Test HTTP Proxy

```bash
# Start services
docker-compose up -d
./bin/port-authorizing-api --config config.yaml &
./bin/port-authorizing-cli login -u admin -p admin123
./bin/port-authorizing-cli connect nginx-server -l 9090 -d 1h &

# Load test (requires Apache Bench)
ab -n 1000 -c 10 http://localhost:9090/

# Or use hey
hey -n 1000 -c 10 http://localhost:9090/
```

### Monitor Resources

```bash
# Docker stats
docker stats

# API resource usage
top -p $(pgrep port-authorizing-api)

# Connection count
cat audit.log | jq 'select(.action == "connect")' | wc -l
```

## Next Steps

After tests pass:

1. **Review Audit Logs**
   ```bash
   cat audit.log | jq
   ```

2. **Try Interactive Mode**
   ```bash
   ./bin/port-authorizing-cli connect nginx-server -l 9090 -d 1h
   curl http://localhost:9090/
   ```

3. **Customize Configuration**
   - Edit `config.yaml`
   - Add your own services
   - Configure whitelists

4. **Deploy to Production**
   - See deployment guides
   - Set up TLS/HTTPS
   - Configure monitoring

## Getting Help

- **Documentation**: See [README.md](README.md)
- **Testing Guide**: See [DOCKER_TESTING.md](DOCKER_TESTING.md)
- **Quick Reference**: See [QUICK_REFERENCE.md](QUICK_REFERENCE.md)
- **Architecture**: See [ARCHITECTURE.md](ARCHITECTURE.md)

## Success!

If all tests pass, you have:
- ✅ Working Docker environment
- ✅ Functional API server
- ✅ Working CLI client
- ✅ HTTP proxy through Nginx
- ✅ PostgreSQL proxy
- ✅ Complete audit trail
- ✅ Security validation

**You're ready to use port-authorizing! 🎉**

