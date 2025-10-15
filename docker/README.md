# Docker Configuration Files

This directory contains all Docker-related configuration files for port-authorizing.

## Files

### config.yaml
Docker-specific configuration that uses Docker service names (e.g., `postgres`, `keycloak`, `nginx`) instead of localhost. This is mounted into the API container at runtime.

**Key differences from root `config.yaml`:**
- Uses Docker service names: `host: postgres` instead of `host: localhost`
- Approval webhook URL: `http://mock-approval:9000/webhook`
- Keycloak issuer: `http://keycloak:8180/realms/portauth`

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

## Usage

All these files are referenced in `docker-compose.yml` and automatically mounted into the appropriate containers.

See [Docker Setup Guide](../docs/DOCKER-SETUP.md) for complete usage instructions.

