# Configuration Guide

## Current Configuration Status

Your `config.yaml` is configured with:

### ✅ Enabled Features

1. **OIDC Authentication (Keycloak)**
   - Provider: Keycloak at `http://localhost:8180/realms/portauth`
   - Client ID: `port-authorizing`
   - Browser-based login flow enabled

2. **Local Authentication**
   - Users: admin, developer
   - Kept for backward compatibility

3. **Role-Based Access Control**
   - Policies: admin-all, dev-test, dev-prod-readonly
   - Tag-based connection filtering

4. **Security Enhancements**
   - Username enforcement (users can only connect as themselves)
   - Case-insensitive whitelist matching
   - Query validation before execution

## Configuration File Location

```
/Users/davidcohan/freelos/port-authorizing/config.yaml
```

## Key Configuration Sections

### Authentication Providers

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
        redirect_url: "http://localhost:8080/auth/callback/oidc"
        roles_claim: "roles"
        username_claim: "preferred_username"
```

### Connections with Tags

```yaml
connections:
  - name: postgres-test
    tags:
      - env:test
      - type:database
    backend_username: "testuser"
    backend_password: "testpass"
```

### Role Policies

```yaml
policies:
  - name: dev-test
    roles:
      - developer
    tags:
      - env:test
    tag_match: any
    whitelist:
      - "^SELECT.*"
      - "^EXPLAIN.*"
```

## Login Methods

### Method 1: Browser-Based OIDC (Default)

```bash
# Just run login without credentials
./bin/port-authorizing-cli login

# Or explicitly use OIDC
./bin/port-authorizing-cli login --provider oidc
```

This will:
1. Open browser to Keycloak
2. User logs in via web UI
3. CLI receives token automatically
4. Roles from Keycloak are used for authorization

### Method 2: Local Username/Password

```bash
# Provide credentials for local auth
./bin/port-authorizing-cli login -u admin -p admin123
```

## Configuration Changes Checklist

When you update `config.yaml`:

- [ ] Restart API server for changes to take effect
- [ ] Verify Keycloak realm is imported (if using OIDC)
- [ ] Update client_secret from Keycloak admin
- [ ] Test authentication flows
- [ ] Verify role mappings in policies
- [ ] Check connection tags match policies

## Common Configuration Tasks

### 1. Get Keycloak Client Secret

```bash
# Access Keycloak Admin
open http://localhost:8180

# Navigate to:
# Clients → port-authorizing → Credentials tab
# Copy the Client Secret
```

Then update in `config.yaml`:
```yaml
client_secret: "<paste-secret-here>"
```

### 2. Add New User Role

In `config.yaml`:
```yaml
policies:
  - name: new-role-policy
    roles:
      - new-role-name
    tags:
      - env:staging
    whitelist:
      - "^SELECT.*"
```

### 3. Add New Connection

```yaml
connections:
  - name: new-service
    type: postgres  # or http, tcp
    host: service.example.com
    port: 5432
    tags:
      - env:production
      - team:platform
    backend_username: "user"
    backend_password: "pass"
```

## Restart API After Config Changes

```bash
# Stop current API
pkill port-authorizing-api

# Start with updated config
./bin/port-authorizing-api --config config.yaml > api.log 2>&1 &
```

## Configuration Validation

To verify your configuration:

```bash
# Check API starts without errors
tail -f api.log

# Test local authentication
./bin/port-authorizing-cli login -u admin -p admin123

# Test OIDC authentication
./bin/port-authorizing-cli login --provider oidc

# List connections (should show only allowed ones)
./bin/port-authorizing-cli list
```

## Security Notes

🔒 **Production Checklist:**

1. ✅ Change `jwt_secret` to a strong random value
2. ✅ Update `client_secret` from Keycloak
3. ✅ Use HTTPS for `issuer` and `redirect_url`
4. ✅ Remove default passwords from local users
5. ✅ Enable `enable_llm_analysis` if desired
6. ✅ Set appropriate `max_connection_duration`
7. ✅ Review and restrict whitelist patterns
8. ✅ Enable audit logging and monitor it
9. ✅ Use environment variables for sensitive values
10. ✅ Disable local users in production (use OIDC/LDAP only)

## Environment Variables (Production)

Instead of hardcoding secrets in config.yaml:

```yaml
auth:
  jwt_secret: "${JWT_SECRET}"
  providers:
    - name: keycloak
      config:
        client_secret: "${KEYCLOAK_CLIENT_SECRET}"
```

Then set in environment:
```bash
export JWT_SECRET="your-strong-secret"
export KEYCLOAK_CLIENT_SECRET="keycloak-secret"
```

## Troubleshooting

### Config Not Loading

```bash
# Verify config file exists
ls -la config.yaml

# Check syntax
cat config.yaml | grep -v "^#" | grep -v "^$"
```

### OIDC Not Working

1. Verify Keycloak is running: `docker ps | grep keycloak`
2. Check realm exists: `curl http://localhost:8180/realms/portauth`
3. Verify client configuration in Keycloak
4. Check client secret matches
5. Ensure "Direct access grants" is enabled in Keycloak client

### Roles Not Working

1. Check user has roles in Keycloak
2. Verify roles mapper is configured in Keycloak client
3. Test token: decode JWT at jwt.io and check for "roles" claim
4. Verify policy roles match user roles (case-sensitive)

## Files Reference

- `config.yaml` - Main configuration
- `config.example.yaml` - Full example with all options
- `AUTHENTICATION_GUIDE.md` - Auth/authz documentation
- `SECURITY_FIXES.md` - Security enhancements
- `docker/keycloak-setup.md` - Keycloak setup guide

## Quick Reference Commands

```bash
# View current config
cat config.yaml

# Edit config
vim config.yaml  # or your editor

# Restart API
pkill port-authorizing-api && ./bin/port-authorizing-api --config config.yaml &

# Test configuration
./bin/port-authorizing-cli login
./bin/port-authorizing-cli list

# View audit log
tail -f audit.log | jq

# Check API health
curl http://localhost:8080/api/health
```

