# Testing WebSocket Through Nginx Load Balancer

## Overview

This guide explains how to test port-authorizing WebSocket connections through an nginx reverse proxy/load balancer. This simulates real-world deployment scenarios where the API server sits behind a load balancer.

## Architecture

```
┌─────────────┐       ┌──────────────┐       ┌─────────────────────┐
│   Client    │──────▶│    Nginx     │──────▶│ Port-Authorizing    │
│  (psql/CLI) │       │ Load Balancer│       │    API Server       │
│             │       │  :8090       │       │     :8080           │
└─────────────┘       └──────────────┘       └─────────────────────┘
                            │                          │
                            │                          ▼
                      WebSocket                   ┌─────────┐
                      Upgrade                     │ Backend │
                      Headers                     │ (Postgres)
                                                  └─────────┘
```

## Why Test Through Nginx?

1. **Real-world scenario**: Most production deployments have a load balancer in front of the API
2. **WebSocket upgrade handling**: Nginx needs proper configuration for WebSocket to work
3. **Header forwarding**: Ensure client IP and headers are preserved
4. **Performance testing**: Test WebSocket through a proxy layer
5. **Load balancing**: Test multiple API server instances (future)

## Setup

### 1. Start Docker Services

```bash
# Start all services including nginx-proxy
docker-compose up -d

# Or just start nginx-proxy
docker-compose up -d nginx-proxy

# Check nginx-proxy is running
docker-compose ps nginx-proxy
```

### 2. Start Port-Authorizing API Server

```bash
# Terminal 1: Start API server
./bin/port-authorizing server --config config.yaml

# API will be listening on localhost:8080
# Nginx proxy will forward from localhost:8090 → localhost:8080
```

### 3. Start Mock Approval Server (Optional)

```bash
# Terminal 2: For approval testing
./bin/mock-approval-server -interactive=true
```

## Testing

### Test 1: Direct Connection (Baseline)

First, test without nginx to ensure everything works:

```bash
# Connect directly to API server
port-authorizing login developer

# Connect to postgres (directly to API on port 8080)
port-authorizing connect postgres-test

# Should show: http://localhost:8080/api/proxy/{connection-id}
# Try a query
psql -h localhost -p [PORT] -U developer -d testdb
SELECT 1;
```

### Test 2: Through Nginx Proxy

Now test through nginx load balancer:

```bash
# Update CLI to use nginx proxy
export PORT_AUTHORIZING_URL="http://localhost:8090"

# Or modify config.yaml temporarily:
# server:
#   port: 8090  # Point to nginx-proxy

# Login through nginx
port-authorizing login developer

# Connect through nginx
port-authorizing connect postgres-test

# WebSocket will be upgraded through nginx
psql -h localhost -p [PORT] -U developer -d testdb
SELECT 1;
```

### Test 3: WebSocket Specific Tests

Test WebSocket-specific functionality:

```bash
# Terminal 1: Watch nginx logs
docker logs -f port-auth-nginx-proxy

# Terminal 2: Connect and run queries
psql -h localhost -p [PORT] -U developer -d testdb

# Try various queries to test whitelist/approval
SELECT * FROM test_table;          # Should work
DELETE FROM test_table WHERE id=1; # Should request approval
INSERT INTO test_table VALUES (1); # May be blocked by whitelist
```

## Verifying WebSocket Through Nginx

### Check Nginx Logs

```bash
# Access log (shows WebSocket upgrade)
docker exec port-auth-nginx-proxy cat /var/log/nginx/websocket.log

# Error log (should be empty)
docker exec port-auth-nginx-proxy cat /var/log/nginx/error.log

# Look for WebSocket upgrade in logs
docker logs port-auth-nginx-proxy 2>&1 | grep -i upgrade
```

### Check Audit Logs

Audit logs should show connections through the proxy:

```bash
tail -f audit.log | jq 'select(.action | contains("postgres") or contains("http"))'
```

Look for:
- `X-Forwarded-For` headers with client IP
- WebSocket connections working correctly
- No connection drops or errors

### Test Long-Running Connections

WebSocket connections should stay alive:

```bash
# Start psql and leave it open
psql -h localhost -p [PORT] -U developer -d testdb

# Run query, wait a few minutes, run another
SELECT 1;
-- wait 5 minutes --
SELECT 2;

# Connection should remain alive due to nginx keepalive settings
```

## Nginx Configuration Explained

Key configuration for WebSocket support in `docker/nginx-proxy.conf`:

```nginx
# WebSocket upgrade headers (CRITICAL!)
proxy_http_version 1.1;
proxy_set_header Upgrade $http_upgrade;
proxy_set_header Connection $connection_upgrade;

# Long timeouts for persistent connections
proxy_connect_timeout 7d;
proxy_send_timeout 7d;
proxy_read_timeout 7d;

# Disable buffering (important for WebSocket)
proxy_buffering off;
proxy_request_buffering off;

# Map for upgrade header
map $http_upgrade $connection_upgrade {
    default upgrade;
    '' close;
}
```

## Common Issues

### Issue 1: Connection Immediately Closes

**Symptoms**: Connection drops right after WebSocket upgrade

**Cause**: Nginx buffering enabled

**Fix**: Already configured in `nginx-proxy.conf`:
```nginx
proxy_buffering off;
proxy_request_buffering off;
```

### Issue 2: Connection Times Out After 60 Seconds

**Symptoms**: Connection works but drops after ~60 seconds

**Cause**: Default nginx timeout

**Fix**: Already configured with long timeouts:
```nginx
proxy_read_timeout 7d;
```

### Issue 3: 400 Bad Request on WebSocket Upgrade

**Symptoms**: `400 Bad Request` when trying to establish WebSocket

**Cause**: Missing upgrade headers

**Fix**: Already configured:
```nginx
proxy_set_header Upgrade $http_upgrade;
proxy_set_header Connection $connection_upgrade;
```

### Issue 4: Can't Connect to API Server

**Symptoms**: `502 Bad Gateway` from nginx

**Causes**:
1. API server not running
2. Wrong port in nginx config
3. Network mode issue

**Debug**:
```bash
# Check API server is running
curl http://localhost:8080/api/health

# Check nginx can reach API server
docker exec port-auth-nginx-proxy wget -qO- http://localhost:8080/api/health

# Check nginx config
docker exec port-auth-nginx-proxy nginx -t
```

## Load Balancing (Future)

To test with multiple API server instances:

### 1. Start Multiple API Servers

```bash
# Terminal 1: Instance 1
PORT=8080 ./bin/port-authorizing server --config config.yaml

# Terminal 2: Instance 2
PORT=8081 ./bin/port-authorizing server --config config2.yaml

# Terminal 3: Instance 3
PORT=8082 ./bin/port-authorizing server --config config3.yaml
```

### 2. Update nginx-proxy.conf

```nginx
upstream port_authorizing_api {
    server localhost:8080;
    server localhost:8081;
    server localhost:8082;

    # Load balancing method
    least_conn;  # Route to instance with fewest connections
    keepalive 32;
}
```

### 3. Restart Nginx

```bash
docker-compose restart nginx-proxy
```

### 4. Test Load Distribution

```bash
# Connect multiple times
for i in {1..10}; do
    port-authorizing connect postgres-test &
done

# Check which instances handled connections in audit logs
tail -100 audit.log | jq -r '.metadata.connection_id' | sort | uniq -c
```

## Performance Testing

### Test 1: Single Connection Throughput

```bash
# Time a large query through nginx
time psql -h localhost -p [PORT] -U developer -d testdb -c "SELECT * FROM large_table"
```

### Test 2: Multiple Concurrent Connections

```bash
# Start 10 concurrent psql sessions
for i in {1..10}; do
    psql -h localhost -p [PORT] -U developer -d testdb &
done

# Monitor nginx
docker stats port-auth-nginx-proxy
```

### Test 3: WebSocket Frame Size

```bash
# Test large data transfers
psql -h localhost -p [PORT] -U developer -d testdb
COPY large_table TO STDOUT;
```

## Monitoring

### Watch Real-Time Traffic

```bash
# Terminal 1: Nginx access log
docker logs -f port-auth-nginx-proxy | grep -v health

# Terminal 2: API server audit log
tail -f audit.log | jq -C

# Terminal 3: Run tests
psql -h localhost -p [PORT] -U developer -d testdb
```

### Check Connection Statistics

```bash
# Active connections
docker exec port-auth-nginx-proxy cat /proc/net/tcp | grep 1F90 | wc -l

# Nginx worker processes
docker exec port-auth-nginx-proxy ps aux | grep nginx
```

## Best Practices

1. **Always test through load balancer** before production deployment
2. **Monitor nginx error logs** for WebSocket issues
3. **Test long-running connections** (> 1 hour)
4. **Verify audit logs** show correct client IPs via X-Forwarded-For
5. **Test approval workflow** through nginx
6. **Measure latency** added by proxy layer

## See Also

- [WebSocket Architecture](WEBSOCKET-SUMMARY.md)
- [WebSocket Audit & Permissions](WEBSOCKET-AUDIT-PERMISSIONS-ANALYSIS.md)
- [Approval Workflow](../features/APPROVAL-WORKFLOW.md)

