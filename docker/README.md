# Docker Configuration Files

This directory contains all Docker-related configuration files for port-authorizing.

## Files

### config.yaml
Docker-specific configuration that uses Docker service names (e.g., `postgres`, `keycloak`, `nginx`, `redis-noauth`, `redis-auth`) instead of localhost. This is mounted into the API container at runtime.

**Key differences from root `config.yaml`:**
- Uses Docker service names: `host: postgres`, `host: redis-noauth`, `host: redis-auth` instead of `host: localhost`
- Approval webhook URL: `http://mock-approval:9000/webhook`
- Keycloak issuer: `http://keycloak:8180/realms/portauth`
- Includes Redis test connections (with and without authentication)

### nginx.conf
Configuration for the test nginx web server (port 8888) used as a backend for HTTP proxy testing.

### nginx-proxy.conf
Configuration for the nginx reverse proxy/load balancer (port 8090) used to test WebSocket connections through an intermediary.

**Key features:**
- WebSocket upgrade header support
- Long timeout for persistent connections (7 days)
- Proxy buffering disabled for streaming
- Upstream to `api:8080` Docker service

### postgres-init.sql
Initialization SQL script for the PostgreSQL test database. Creates test tables and sample data.

### keycloak-realm.json
Pre-configured Keycloak realm for OIDC authentication testing. Includes:
- Realm: `portauth`
- Client: `port-authorizing`
- Test users with roles

### ldap-init.ldif
LDAP initialization file for OpenLDAP. Creates test users and groups for LDAP authentication testing.

### html/
Static HTML files served by the test nginx server for HTTP proxy testing.

## Docker Services

The `docker-compose.yml` includes the following backend services for testing:

### Databases
- **postgres** (port 5432) - PostgreSQL 15 test database
  - Username: `testuser`
  - Password: `testpass`
  - Database: `testdb`

### Caches
- **redis-noauth** (port 6379) - Redis 7 without authentication
  - No password required
  - For testing basic Redis proxying

- **redis-auth** (port 6380) - Redis 7 with authentication
  - Password: `redispass123`
  - For testing Redis AUTH command and authenticated proxying

### Web Servers
- **nginx** (port 8888) - Test backend web server
- **nginx-proxy** (port 8090) - Reverse proxy/load balancer for WebSocket testing

### Authentication
- **keycloak** (port 8180) - OIDC authentication provider
- **openldap** (port 389/636) - LDAP authentication provider
- **phpldapadmin** (port 8181) - LDAP admin UI

### Testing
- **mock-approval** (port 9000) - Mock approval webhook server

## Testing Redis

Run the included test script to verify Redis connections:

```bash
./test_redis.sh
```

Or test manually:

```bash
# Start services
docker-compose up -d

# Login
port-authorizing login

# Connect to Redis without auth
port-authorizing connect redis-test-noauth -l 6379
redis-cli -h localhost -p 6379

# Connect to Redis with auth
port-authorizing connect redis-test-auth -l 6380
redis-cli -h localhost -p 6380
```

## Usage

All these files are referenced in `docker-compose.yml` and automatically mounted into the appropriate containers.

See [Docker Setup Guide](../docs/DOCKER-SETUP.md) for complete usage instructions.

