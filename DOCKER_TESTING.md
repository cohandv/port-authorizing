# Docker-Based Testing Guide

## Overview

This guide explains how to test the port-authorizing system with real services (PostgreSQL and Nginx) running in Docker containers.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      Test Environment                        │
│                                                               │
│  ┌──────────┐    ┌─────────┐    ┌─────────┐                │
│  │   User   │───▶│   CLI   │───▶│   API   │                │
│  │  (curl)  │    │ (proxy) │    │ (8080)  │                │
│  └──────────┘    └─────────┘    └────┬────┘                │
│                                       │                      │
│                       ┌───────────────┴──────────────┐      │
│                       │                              │      │
│              ┌────────▼────────┐          ┌─────────▼─────┐│
│              │  Nginx (Docker) │          │ PostgreSQL    ││
│              │   Port: 8888    │          │   (Docker)    ││
│              │                 │          │   Port: 5432  ││
│              └─────────────────┘          └───────────────┘│
│                                                               │
│              All activity logged to audit.log                │
└─────────────────────────────────────────────────────────────┘
```

## Quick Start

### 1. Start Docker Services

```bash
docker-compose up -d
```

This starts:
- **PostgreSQL** on port 5432 with test database and sample data
- **Nginx** on port 8888 with a test web page

### 2. Run Comprehensive Tests

```bash
./test.sh
```

This automated test suite will:
1. Start Docker services
2. Wait for services to be healthy
3. Start the API server
4. Test authentication
5. Test HTTP proxy through Nginx
6. Test PostgreSQL proxy
7. Verify all activity is logged
8. Clean up everything

### 3. View Results

```bash
# Check audit log
cat audit.log | jq

# View API logs
cat api.log

# View Docker logs
docker-compose logs
```

## Manual Testing

### Test HTTP Proxy (Nginx)

```bash
# 1. Start services
docker-compose up -d
./bin/port-authorizing-api --config config.yaml &

# 2. Login
./bin/port-authorizing-cli login -u admin -p admin123

# 3. Connect to Nginx
./bin/port-authorizing-cli connect nginx-server -l 9090 -d 1h

# 4. In another terminal, access Nginx through proxy
curl http://localhost:9090/
curl http://localhost:9090/api/
curl http://localhost:9090/health

# 5. Check audit log
cat audit.log | jq '.action' -r
```

### Test PostgreSQL Proxy

```bash
# 1. Start services (if not already running)
docker-compose up -d
./bin/port-authorizing-api --config config.yaml &

# 2. Login
./bin/port-authorizing-cli login -u admin -p admin123

# 3. Connect to PostgreSQL
./bin/port-authorizing-cli connect postgres-test -l 5433 -d 1h

# 4. In another terminal, use psql through proxy
# Note: Full wire protocol support is pending, but basic queries work
psql -h localhost -p 5433 -U testuser -d testdb

# Or test via API directly
TOKEN=$(curl -s -X POST http://localhost:8080/api/login \
    -H "Content-Type: application/json" \
    -d '{"username":"admin","password":"admin123"}' | \
    jq -r '.token')

# Create connection
CONN=$(curl -s -X POST http://localhost:8080/api/connect/postgres-test \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"duration":"3600000000000"}' | jq -r '.connection_id')

# Send query
curl -X POST http://localhost:8080/api/proxy/$CONN \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/octet-stream" \
    -d "SELECT * FROM users;"

# 5. Check audit log for PostgreSQL activity
cat audit.log | jq 'select(.resource == "postgres-test")'
```

## Docker Services

### PostgreSQL

**Connection Details:**
- Host: localhost
- Port: 5432
- Database: testdb
- User: testuser
- Password: testpass

**Sample Data:**
```sql
-- Users table with 3 sample users
SELECT * FROM users;

-- Logs table
SELECT * FROM logs;
```

**Test Queries:**
```sql
-- Allowed (SELECT)
SELECT * FROM users WHERE id = 1;

-- Allowed (INSERT INTO logs)
INSERT INTO logs (log_level, message) VALUES ('INFO', 'Test message');

-- Blocked (DELETE not in whitelist)
DELETE FROM users WHERE id = 1;

-- Blocked (DROP not in whitelist)
DROP TABLE users;
```

### Nginx

**URLs:**
- http://localhost:8888/ - Main page (HTML)
- http://localhost:8888/api/ - JSON API response
- http://localhost:8888/health - Health check

**Test Requests:**
```bash
# Direct access (without proxy)
curl http://localhost:8888/

# Through proxy (after connecting)
curl http://localhost:9090/
```

## Audit Log Analysis

### View All Events

```bash
cat audit.log | jq
```

### Filter by Action

```bash
# Login events
cat audit.log | jq 'select(.action == "login")'

# Connection events
cat audit.log | jq 'select(.action == "connect")'

# Proxy requests
cat audit.log | jq 'select(.action == "proxy_request")'
```

### Filter by Resource

```bash
# Nginx activity
cat audit.log | jq 'select(.resource == "nginx-server")'

# PostgreSQL activity
cat audit.log | jq 'select(.resource == "postgres-test")'
```

### Statistics

```bash
# Count events by action
cat audit.log | jq -r '.action' | sort | uniq -c

# Count events by user
cat audit.log | jq -r '.username' | sort | uniq -c

# Count events by resource
cat audit.log | jq -r '.resource' | sort | uniq -c
```

## Whitelist Testing

The PostgreSQL connection has these whitelist rules:

```yaml
whitelist:
  - "^SELECT.*"       # Allow all SELECT queries
  - "^INSERT INTO logs.*"  # Allow INSERT only to logs table
  - "^UPDATE users.*"      # Allow UPDATE only to users table
```

### Test Allowed Queries

```bash
# These should succeed
SELECT * FROM users;
SELECT id, username FROM users WHERE id = 1;
INSERT INTO logs (log_level, message) VALUES ('INFO', 'Test');
UPDATE users SET email = 'new@example.com' WHERE id = 1;
```

### Test Blocked Queries

```bash
# These should be blocked
DELETE FROM users;
DROP TABLE users;
INSERT INTO users (username, email) VALUES ('hacker', 'hack@evil.com');
UPDATE logs SET message = 'modified';
```

## Troubleshooting

### Docker Services Not Starting

```bash
# Check Docker status
docker-compose ps

# View logs
docker-compose logs postgres
docker-compose logs nginx

# Restart services
docker-compose restart
```

### PostgreSQL Connection Issues

```bash
# Test direct connection
docker exec port-auth-postgres psql -U testuser -d testdb -c "SELECT 1"

# Check if port is open
nc -zv localhost 5432
```

### Nginx Not Accessible

```bash
# Test direct access
curl http://localhost:8888/health

# Check if port is open
nc -zv localhost 8888

# View Nginx logs
docker-compose logs nginx
```

### API Server Issues

```bash
# Check if API is running
curl http://localhost:8080/api/health

# View API logs
cat api.log

# Check if port is in use
lsof -i :8080
```

### Audit Log Not Created

```bash
# Check permissions
ls -la audit.log

# Check API log for errors
cat api.log | grep -i error

# Verify logging configuration
cat config.yaml | grep -A 3 logging
```

## Cleanup

### Stop Everything

```bash
# Stop Docker services
docker-compose down

# Remove volumes (deletes database data)
docker-compose down -v

# Clean up logs
rm -f audit.log api.log
```

### Full Reset

```bash
# Remove all traces
docker-compose down -v
rm -f audit.log api.log
rm -rf ~/.port-auth/
```

## Performance Testing

### Load Test HTTP Proxy

```bash
# Using Apache Bench
ab -n 1000 -c 10 http://localhost:9090/

# Using hey
hey -n 1000 -c 10 http://localhost:9090/
```

### Monitor Resources

```bash
# Docker stats
docker stats

# API resource usage
ps aux | grep port-authorizing-api

# Connection count
cat audit.log | jq 'select(.action == "connect")' | wc -l
```

## Security Validation

### Test Authentication

```bash
# Try without token (should fail)
curl http://localhost:8080/api/connections

# Try with invalid token (should fail)
curl http://localhost:8080/api/connections \
    -H "Authorization: Bearer invalid-token"

# Try with valid token (should succeed)
curl http://localhost:8080/api/connections \
    -H "Authorization: Bearer $TOKEN"
```

### Test Authorization

```bash
# Create connection as admin
ADMIN_TOKEN=...
CONN_ID=$(curl -X POST http://localhost:8080/api/connect/nginx-server \
    -H "Authorization: Bearer $ADMIN_TOKEN" -d '{}' | jq -r '.connection_id')

# Try to use connection as different user (should fail)
DEV_TOKEN=...
curl -X POST http://localhost:8080/api/proxy/$CONN_ID \
    -H "Authorization: Bearer $DEV_TOKEN"
```

### Test Whitelist

```bash
# Test allowed query
echo "SELECT * FROM users;" | \
    curl -X POST http://localhost:8080/api/proxy/$PG_CONN \
    -H "Authorization: Bearer $TOKEN" \
    --data-binary @-

# Test blocked query
echo "DELETE FROM users;" | \
    curl -X POST http://localhost:8080/api/proxy/$PG_CONN \
    -H "Authorization: Bearer $TOKEN" \
    --data-binary @-
```

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Test
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      - uses: actions/setup-go@v2
      - run: make build
      - run: ./test.sh
```

### GitLab CI Example

```yaml
test:
  image: golang:1.21
  services:
    - postgres:15
    - nginx:alpine
  script:
    - make build
    - ./test.sh
```

## Next Steps

1. **Extend Tests**: Add more test cases for edge cases
2. **Performance**: Run load tests to identify bottlenecks
3. **Security**: Conduct security audit of proxy implementation
4. **Monitoring**: Add Prometheus metrics for production monitoring
5. **Documentation**: Document production deployment strategies

## Resources

- [Docker Compose Documentation](https://docs.docker.com/compose/)
- [PostgreSQL Documentation](https://www.postgresql.org/docs/)
- [Nginx Documentation](https://nginx.org/en/docs/)

