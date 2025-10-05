# OIDC Authentication Setup Guide

## Overview

The system now supports **browser-based OIDC authentication** using Authorization Code Flow. This allows users to authenticate via Keycloak (or other OIDC providers) without needing local passwords.

## How It Works

### 1. Authentication Flow

```
┌─────────┐           ┌─────────┐           ┌──────────┐           ┌──────────┐
│   CLI   │           │   API   │           │ Keycloak │           │ Browser  │
└────┬────┘           └────┬────┘           └────┬─────┘           └────┬─────┘
     │                     │                     │                      │
     │ 1. Start login      │                     │                      │
     ├────────────────────>│                     │                      │
     │                     │                     │                      │
     │ 2. Open browser     │                     │                      │
     │ with auth URL       │                     │                      │
     ├────────────────────────────────────────────────────────────────>│
     │                     │                     │                      │
     │                     │  3. User logs in    │                      │
     │                     │     (alice/password123)                    │
     │                     │<────────────────────┼──────────────────────┤
     │                     │                     │                      │
     │                     │  4. Authorization   │                      │
     │                     │     code callback   │                      │
     │                     │<────────────────────┤                      │
     │                     │                     │                      │
     │                     │  5. Exchange code   │                      │
     │                     │     for tokens      │                      │
     │                     ├────────────────────>│                      │
     │                     │<────────────────────┤                      │
     │                     │                     │                      │
     │  6. JWT token       │                     │                      │
     │<────────────────────┤                     │                      │
     │                     │                     │                      │
```

### 2. PostgreSQL Connection Flow

```
┌─────────┐           ┌─────────┐           ┌──────────┐           ┌──────────┐
│  psql   │           │   CLI   │           │   API    │           │ Backend  │
│ Client  │           │  Proxy  │           │  Server  │           │ Postgres │
└────┬────┘           └────┬────┘           └────┬─────┘           └────┬─────┘
     │                     │                     │                      │
     │ 1. connect request  │                     │                      │
     │    postgres-test    │                     │                      │
     ├────────────────────>│                     │                      │
     │                     │                     │                      │
     │                     │ 2. Establish proxy  │                      │
     │                     │    (with JWT token) │                      │
     │                     ├────────────────────>│                      │
     │                     │  ✅ JWT validated   │                      │
     │                     │<────────────────────┤                      │
     │                     │                     │                      │
     │ 3. psql connect     │                     │                      │
     │    -U alice         │                     │                      │
     ├────────────────────>│                     │                      │
     │                     │                     │                      │
     │ 4. Password prompt  │                     │                      │
     │<────────────────────┤                     │                      │
     │                     │                     │                      │
     │ 5. Password (any)   │                     │                      │
     ├────────────────────>│                     │                      │
     │     ✅ Accepted     │                     │                      │
     │     (no validation) │                     │                      │
     │                     │                     │                      │
     │                     │ 6. Connect to backend                      │
     │                     │    (backend creds)  │                      │
     │                     ├────────────────────────────────────────────>│
     │                     │                     │                      │
     │ 7. Ready            │                     │                      │
     │<────────────────────┤                     │                      │
     │                     │                     │                      │
     │ 8. SQL queries ──────────────────────> Whitelist validated      │
     │                     │                     │                      │
```

## Key Security Features

### 1. **JWT-Based Authentication**
- Users authenticate once via OIDC (browser)
- Receive a signed JWT token with roles
- Token used for all subsequent API calls
- No passwords stored or transmitted repeatedly

### 2. **Username Enforcement**
- PostgreSQL clients MUST use their authenticated username
- Username from JWT is enforced at the proxy level
- Prevents user impersonation

### 3. **Password-Less PostgreSQL**
- PostgreSQL password prompt is protocol ceremony
- Proxy accepts ANY password because JWT already authenticated
- Enables OIDC/SAML users (who have no local passwords) to connect

### 4. **Role-Based Authorization**
- Roles from OIDC provider included in JWT
- Tag-based policies determine connection access
- Different whitelists per role per connection

### 5. **Query Whitelisting**
- All queries validated against regex patterns
- Case-insensitive matching
- Blocked queries return proper PostgreSQL errors

## Setup Instructions

### 1. Configure Keycloak

Run the comprehensive setup script:

```bash
./docker/setup-keycloak.sh setup
```

This will:
- Import the realm configuration
- Configure client scopes (profile, email, roles)
- Set up redirect URIs
- Verify the configuration
- Test authentication

### 2. Update config.yaml

Ensure OIDC is enabled:

```yaml
auth:
  providers:
    - name: keycloak
      type: oidc
      enabled: true
      config:
        issuer: "http://localhost:8180/realms/portauth"
        client_id: "port-authorizing"
        client_secret: "your-client-secret-change-in-production"
        redirect_url: "http://localhost:8080/api/auth/oidc/callback"
        roles_claim: "roles"
        username_claim: "preferred_username"
```

### 3. Restart API Server

```bash
pkill port-authorizing-api
./bin/port-authorizing-api --config config.yaml > api.log 2>&1 &
```

## Usage

### CLI Login (Browser-Based)

```bash
# Login via browser (OIDC)
./bin/port-authorizing-cli login

# This will:
# 1. Open browser to Keycloak
# 2. User logs in (alice/bob/charlie with password: password123)
# 3. Browser redirects back automatically
# 4. CLI receives JWT token
# 5. Token saved for future commands
```

### List Available Connections

```bash
./bin/port-authorizing-cli list
```

### Connect to PostgreSQL

```bash
# Establish proxy connection
./bin/port-authorizing-cli connect postgres-test -l 5433

# In another terminal, connect with psql
psql -h localhost -p 5433 -U alice -d testdb
# Password: (type anything - it's not validated)
```

**Important:** The username in psql MUST match your authenticated username (from JWT).

### Example Session

```bash
# 1. Login via browser
$ ./bin/port-authorizing-cli login
🔐 Starting browser-based OIDC authentication...
Opening browser for authentication...
⏳ Waiting for authentication in browser...
✓ Authentication successful!
  User: alice (alice@portauth.local)
  Roles: [developer user]
  Token expires at: 2025-10-05 12:00:00

# 2. Start proxy
$ ./bin/port-authorizing-cli connect postgres-test -l 5433
✓ Connection established: postgres-test
📝 PostgreSQL Connection Info:
  • Username: alice (required - no other username will work)
  • Database: testdb
  Connection string:
  psql -h localhost -p 5433 -U alice -d testdb

# 3. Connect with psql (in another terminal)
$ psql -h localhost -p 5433 -U alice -d testdb
Password for user alice: (type anything)
testdb=> SELECT * FROM users;
 id | name  | email
----+-------+-------
  1 | Alice | alice@example.com
(1 row)

testdb=> DELETE FROM users;  -- Blocked by whitelist
ERROR:  42501: Query blocked by whitelist policy
HINT:  Your role only allows: SELECT.*, EXPLAIN.*
```

## Test Users

Configured in Keycloak:

| Username | Password     | Roles          | Access                    |
|----------|--------------|----------------|---------------------------|
| alice    | password123  | developer,user | Test env (SELECT, EXPLAIN)|
| bob      | password123  | admin,user     | All environments (full)   |
| charlie  | password123  | qa,user        | Test env (SELECT, EXPLAIN)|

## Troubleshooting

### 1. Browser doesn't open

Manually visit the URL shown in the terminal.

### 2. "Missing code or state parameter"

- Keycloak redirect URI not configured correctly
- Run: `./docker/setup-keycloak.sh setup`

### 3. "invalid_scope" error

- Client scopes not configured in Keycloak
- Run: `./docker/setup-keycloak.sh setup`

### 4. "Username mismatch" in PostgreSQL

You must connect with your authenticated username:
```bash
# ✅ Correct
psql -h localhost -p 5433 -U alice -d testdb

# ❌ Wrong
psql -h localhost -p 5433 -U bob -d testdb
```

### 5. Token expired

Login again:
```bash
./bin/port-authorizing-cli login
```

### 6. Check audit logs

```bash
tail -f audit.log | jq
```

## Keycloak Administration

### Access Admin Console

```
URL: http://localhost:8180
Username: admin
Password: admin
Realm: portauth
```

### View User Roles

1. Login to Keycloak admin
2. Select "portauth" realm
3. Users → Select user → Role mapping

### Get Client Secret

1. Clients → port-authorizing
2. Credentials tab
3. Copy "Client Secret"
4. Update in `config.yaml`

### Add New User

1. Users → Add user
2. Set username, email
3. Credentials → Set password
4. Role mapping → Assign roles

## Configuration Files

- **Main Config**: `config.yaml`
- **Keycloak Realm**: `docker/keycloak-realm.json`
- **Setup Script**: `docker/setup-keycloak.sh`
- **Docker Compose**: `docker-compose.yml`

## Security Considerations

### Production Deployment

1. **Use HTTPS**
   ```yaml
   issuer: "https://keycloak.production.com/realms/portauth"
   redirect_url: "https://api.production.com/api/auth/oidc/callback"
   ```

2. **Update Secrets**
   ```yaml
   jwt_secret: "use-strong-random-secret-here"
   client_secret: "get-from-keycloak-admin"
   ```

3. **Restrict Redirect URIs**
   In Keycloak: Only allow production URLs

4. **Enable TLS for PostgreSQL**
   Configure backend connections to use SSL

5. **Audit Everything**
   ```yaml
   logging:
     audit_log_path: "/secure/path/audit.log"
   ```

6. **Token Expiry**
   ```yaml
   token_expiry: 1h  # Shorter for production
   ```

## Architecture

### Components

1. **API Server** (`internal/api/`)
   - OIDC endpoints (`/api/auth/oidc/login`, `/api/auth/oidc/callback`)
   - JWT generation and validation
   - Connection management

2. **CLI Client** (`internal/cli/`)
   - Browser-based login flow
   - Local callback server (port 8089)
   - Token storage

3. **PostgreSQL Proxy** (`internal/proxy/`)
   - Username enforcement
   - Password acceptance (no validation)
   - Backend authentication
   - Query validation

4. **Authorization** (`internal/authorization/`)
   - Role-based access control
   - Tag matching
   - Whitelist enforcement

## API Endpoints

### Public Endpoints

- `POST /api/login` - Local user login
- `GET /api/health` - Health check
- `GET /api/auth/oidc/login` - Initiate OIDC flow
- `GET /api/auth/oidc/callback` - OIDC callback handler

### Protected Endpoints (Require JWT)

- `GET /api/connections` - List available connections
- `POST /api/connect/{name}` - Establish proxy connection
- `ALL /api/proxy/{connectionID}` - Proxy requests

## References

- [AUTHENTICATION_GUIDE.md](./AUTHENTICATION_GUIDE.md) - Full auth/authz documentation
- [CONFIGURATION_GUIDE.md](./CONFIGURATION_GUIDE.md) - Configuration reference
- [SECURITY_FIXES.md](./SECURITY_FIXES.md) - Security improvements
- [README.md](./README.md) - Project overview

