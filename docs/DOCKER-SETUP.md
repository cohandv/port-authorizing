# Docker Compose Setup Guide

## Overview

This guide explains how to run the complete port-authorizing stack using Docker Compose, including:
- Port-Authorizing API Server
- Mock Approval Server
- Nginx Reverse Proxy (Load Balancer)
- PostgreSQL Database
- Nginx Web Server (test backend)
- Keycloak (OIDC provider)
- OpenLDAP (LDAP provider)

## Architecture

```
┌─────────────┐
│   Client    │
└──────┬──────┘
       │
       ▼
┌──────────────────┐      ┌─────────────────┐
│  Nginx Proxy     │─────▶│  API Server     │
│  :8090           │      │  :8080          │
└──────────────────┘      └────────┬────────┘
                                   │
                    ┌──────────────┼──────────────┐
                    │              │              │
                    ▼              ▼              ▼
            ┌──────────┐   ┌────────────┐  ┌───────────┐
            │PostgreSQL│   │   Nginx    │  │ Keycloak  │
            │  :5432   │   │   :8888    │  │  :8180    │
            └──────────┘   └────────────┘  └───────────┘

                    ┌──────────────────┐
                    │ Mock Approval    │
                    │     :9000        │
                    └──────────────────┘
```

## Quick Start

### 1. Build and Start All Services

```bash
# Build and start everything
docker compose up -d --build

# Watch logs
docker compose logs -f

# Check status
docker compose ps
```

### 2. Wait for Services to be Healthy

```bash
# Check health status
docker compose ps

# All services should show "(healthy)" status
# This may take 30-60 seconds on first start
```

### 3. Test the Setup

```bash
# Test API health
curl http://localhost:8080/api/health

# Test through nginx proxy
curl http://localhost:8090/api/health

# Test mock approval server
curl http://localhost:9000/health
```

## Service Details

### API Server (port-authorizing)
- **Port**: 8080
- **Config**: `config.docker.yaml` (uses Docker service names)
- **Health**: http://localhost:8080/api/health
- **Logs**: `docker compose logs -f api`

### Mock Approval Server
- **Port**: 9000
- **Auto-approve**: Enabled by default
- **Health**: http://localhost:9000/health
- **Logs**: `docker compose logs -f mock-approval`

### Nginx Reverse Proxy
- **Port**: 8090 (external) → 8080 (API)
- **Purpose**: Load balancer / reverse proxy for testing WebSocket
- **Config**: `docker/nginx-proxy.conf`
- **Health**: http://localhost:8090/health

### PostgreSQL Database
- **Port**: 5432
- **Database**: testdb
- **User**: testuser / testpass
- **Health**: `docker compose exec postgres pg_isready`

### Nginx Web Server
- **Port**: 8888
- **Purpose**: Test backend for HTTP proxy
- **Health**: http://localhost:8888/health

### Keycloak (OIDC)
- **Port**: 8180
- **Admin**: admin / admin
- **Console**: http://localhost:8180
- **Realm**: portauth

## Configuration Files

### docker/config.yaml

This is a Docker-specific configuration that uses Docker service names instead of localhost:

```yaml
connections:
  - name: postgres-test
    host: postgres  # Docker service name, not localhost!
    port: 5432

  - name: nginx-server
    host: nginx     # Docker service name
    port: 80

approval:
  webhook:
    url: "http://mock-approval:9000/webhook"  # Docker service name

auth:
  providers:
    - name: keycloak
      config:
        issuer: "http://keycloak:8180/realms/portauth"  # Docker service name
```

## Testing Through Docker

### Test 1: Direct API Access

```bash
# Login (local auth)
curl -X POST http://localhost:8080/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"developer","password":"dev123"}'

# Save the token
export TOKEN="<token from response>"

# List connections
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/connections
```

### Test 2: Through Nginx Proxy

```bash
# Same commands but use port 8090
curl -X POST http://localhost:8090/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"developer","password":"dev123"}'

export TOKEN="<token from response>"

curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8090/api/connections
```

### Test 3: PostgreSQL Connection

```bash
# Get a connection token
curl -X POST -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/connect/postgres-test

# Extract connection_id and proxy_url from response
# Then use psql through the proxy
psql -h localhost -p [proxy_port] -U developer -d testdb
```

### Test 4: WebSocket Through Nginx

```bash
# Connect through nginx proxy (port 8090)
curl -X POST -H "Authorization: Bearer $TOKEN" \
  http://localhost:8090/api/connect/postgres-test

# The WebSocket upgrade will go through nginx → api
```

## Viewing Logs

### All Services
```bash
docker compose logs -f
```

### Specific Service
```bash
docker compose logs -f api
docker compose logs -f nginx-proxy
docker compose logs -f mock-approval
docker compose logs -f postgres
```

### API Audit Logs
```bash
# API audit logs are in the volume
docker compose exec api cat logs/audit.log

# Or use jq for pretty printing
docker compose exec api cat logs/audit.log | jq
```

### Nginx Access Logs
```bash
docker compose exec nginx-proxy cat /var/log/nginx/websocket.log
docker compose exec nginx-proxy cat /var/log/nginx/access.log
```

## Development Workflow

### Rebuilding After Code Changes

```bash
# Rebuild just the API server
docker compose up -d --build api

# Rebuild just the mock server
docker compose up -d --build mock-approval

# Rebuild both
docker compose up -d --build api mock-approval
```

### Live Development

For faster iteration during development:

```bash
# Stop the containerized API
docker compose stop api

# Run locally with docker config (uses docker service names)
./bin/port-authorizing server --config docker/config.yaml

# Services in docker can still reach your local API via host.docker.internal
# (may need to adjust config.docker.yaml)
```

## Stopping and Cleaning Up

### Stop Services
```bash
# Stop all services
docker compose stop

# Stop specific service
docker compose stop api
```

### Remove Containers
```bash
# Remove all containers (keeps volumes/data)
docker compose down

# Remove containers and volumes (clean slate)
docker compose down -v
```

### Rebuild from Scratch
```bash
# Clean everything and rebuild
docker compose down -v
docker compose build --no-cache
docker compose up -d
```

## Troubleshooting

### Issue 1: API Can't Connect to Postgres

**Symptom**: `connection refused` errors

**Solution**: Check postgres is healthy
```bash
docker compose ps postgres
docker compose logs postgres
docker compose exec postgres pg_isready -U testuser -d testdb
```

### Issue 2: Nginx Proxy Returns 502

**Symptom**: `502 Bad Gateway` from nginx-proxy

**Solution**: Check API is running and healthy
```bash
docker compose ps api
curl http://localhost:8080/api/health
docker compose logs api
```

### Issue 3: WebSocket Upgrade Fails

**Symptom**: Connection drops immediately after WebSocket upgrade

**Solution**: Check nginx-proxy configuration
```bash
# View nginx config
docker compose exec nginx-proxy cat /etc/nginx/nginx.conf | grep -A5 upgrade

# Check nginx error logs
docker compose logs nginx-proxy | grep error
```

### Issue 4: Mock Approval Not Responding

**Symptom**: Approval requests timeout

**Solution**: Check mock-approval service
```bash
docker compose ps mock-approval
curl http://localhost:9000/health
docker compose logs mock-approval
```

### Issue 5: Permission Denied

**Symptom**: `permission denied` when accessing files

**Solution**: Check volume permissions
```bash
# API logs directory
ls -la data/ logs/

# Fix permissions if needed
sudo chown -R 1000:1000 data/ logs/
```

## Environment Variables

You can override settings via environment variables:

```bash
# docker-compose.override.yml
version: '3.8'

services:
  api:
    environment:
      - LOG_LEVEL=debug
      - PORT=8080

  mock-approval:
    environment:
      - AUTO_APPROVE=false  # Disable auto-approve
      - INTERACTIVE=true    # Enable interactive mode
```

Then run:
```bash
docker compose up -d
```

## Health Checks

All services have health checks configured:

```bash
# Check health status
docker compose ps

# Services should show:
# - Up (healthy)      - Service is ready
# - Up (health: starting) - Warming up
# - Up (unhealthy)    - Check logs

# Manual health checks
curl http://localhost:8080/api/health  # API
curl http://localhost:9000/health       # Mock Approval
curl http://localhost:8090/health       # Nginx Proxy
```

## Production Considerations

This Docker Compose setup is designed for **development and testing**. For production:

1. **Remove mock-approval service** - Use real approval system (Slack, webhooks, etc.)
2. **Use external databases** - Don't use the containerized postgres for production data
3. **Configure TLS** - Enable HTTPS in nginx-proxy
4. **Use secrets** - Don't commit secrets to config files
5. **Resource limits** - Add CPU/memory limits to services
6. **Monitoring** - Add Prometheus/Grafana for monitoring
7. **Logging** - Configure centralized logging (ELK, etc.)

## See Also

- [Testing Through Nginx](TESTING-THROUGH-NGINX.md)
- [WebSocket Architecture](WEBSOCKET-SUMMARY.md)
- [Configuration Guide](guides/configuration.md)

