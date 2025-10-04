# Authentication & Authorization System Upgrade

## Summary

This upgrade adds enterprise-grade authentication and authorization to Port Authorizing with support for multiple authentication providers, role-based access control (RBAC), and tag-based connection policies.

## What Changed

### 1. Authentication System

**Before:** Only local username/password authentication

**After:** Multi-provider authentication framework supporting:
- ✅ **Local** - username/password (backward compatible)
- ✅ **OIDC** - OpenID Connect (Keycloak, Auth0, Okta, Google, Azure AD, etc.)
- ✅ **LDAP** - LDAP/Active Directory integration
- ✅ **SAML2** - SAML 2.0 (framework ready, implementation in progress)

### 2. Authorization System

**Before:**
- Whitelists defined directly on each connection
- No role-based access control
- Users couldn't be restricted to specific environments

**After:**
- ✅ Role-based access control (RBAC)
- ✅ Tag-based connection categorization (env, team, type, etc.)
- ✅ Flexible policies that combine roles, tags, and whitelists
- ✅ Multi-tag matching (any/all modes)
- ✅ Centralized policy management

### 3. Configuration Changes

**Connections now support tags:**
```yaml
connections:
  - name: postgres-prod
    type: postgres
    tags:                    # NEW
      - env:production
      - type:database
      - team:backend
    # whitelist: []          # DEPRECATED (use policies instead)
```

**New policies section:**
```yaml
policies:                    # NEW
  - name: dev-prod-readonly
    roles:
      - developer
    tags:
      - env:production
    tag_match: any
    whitelist:
      - "^SELECT.*"
```

**New auth providers:**
```yaml
auth:
  providers:                 # NEW
    - name: keycloak
      type: oidc
      enabled: true
      config:
        issuer: "http://localhost:8180/realms/portauth"
        client_id: "port-authorizing"
        # ...
  users:                     # LEGACY (still supported)
    - username: admin
      password: admin123
```

## New Files

### Authentication (`internal/auth/`)
- `provider.go` - Provider interface and manager
- `local.go` - Local username/password provider
- `oidc.go` - OpenID Connect provider
- `ldap.go` - LDAP provider
- `saml2.go` - SAML2 provider (stub)

### Authorization (`internal/authorization/`)
- `authz.go` - Role-based authorization with tag matching

### Docker Testing
- `docker/keycloak-realm.json` - Pre-configured Keycloak realm with test users
- `docker/ldap-init.ldif` - OpenLDAP initialization with test users
- Updated `docker-compose.yml` with Keycloak, OpenLDAP, and phpLDAPadmin

### Documentation
- `AUTHENTICATION_GUIDE.md` - Complete authentication and authorization guide
- `AUTH_UPGRADE_SUMMARY.md` - This file

## Updated Files

### Configuration
- `internal/config/config.go` - Added `AuthProviderConfig`, `RolePolicy`, tags to connections
- `config.yaml` - Updated with new structure and examples
- `config.example.yaml` - Comprehensive example with all auth providers

### API
- `internal/api/auth.go` - Integrated auth manager, added roles to JWT claims
- `internal/api/server.go` - Added authorizer
- `internal/api/handlers.go` - Role-based connection filtering and access checks

### Dependencies
- `go.mod` / `go.sum` - Added OIDC, OAuth2, and LDAP libraries

## Backward Compatibility

✅ **Fully backward compatible:**
- Existing configs with local users still work
- Connections without tags work (using legacy whitelist if present)
- No breaking API changes

## Docker Test Environment

Start all services:
```bash
docker-compose up
```

This starts:
- **PostgreSQL** (5432) - Test database
- **Nginx** (8888) - Test web server
- **Keycloak** (8180) - OIDC/SAML2 provider
  - Admin UI: http://localhost:8180 (admin/admin)
  - Pre-configured realm: `portauth`
  - Test users: alice, bob, charlie (all password: `password123`)
- **OpenLDAP** (389) - LDAP server
  - Admin: cn=admin,dc=portauth,dc=local / adminpass
- **phpLDAPadmin** (8181) - LDAP management UI
  - UI: http://localhost:8181

## Testing

### Test Local Authentication
```bash
# Start API
./bin/port-authorizing-api

# Login
curl -X POST http://localhost:8080/api/login \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": "admin123"}'
```

### Test OIDC (Keycloak)
1. Start Keycloak: `docker-compose up keycloak`
2. Enable OIDC in `config.yaml` (uncomment the keycloak provider)
3. Restart API
4. Test with user alice (developer role):
```bash
curl -X POST http://localhost:8080/api/login \
  -H "Content-Type: application/json" \
  -d '{"username": "alice", "password": "password123"}'
```

### Test LDAP
1. Start OpenLDAP: `docker-compose up openldap`
2. Enable LDAP in `config.yaml` (uncomment the ldap provider)
3. Restart API
4. Test with LDAP user bob (admin group)

### Test Role-Based Access

**Admin user** (has admin role):
```bash
# Can see all connections (test + production)
curl http://localhost:8080/api/connections \
  -H "Authorization: Bearer <admin-token>"

# Can connect to production database
curl -X POST http://localhost:8080/api/connect/postgres-prod \
  -H "Authorization: Bearer <admin-token>"
```

**Developer user** (has developer role):
```bash
# Only sees test connections
curl http://localhost:8080/api/connections \
  -H "Authorization: Bearer <dev-token>"

# Can connect to test database
curl -X POST http://localhost:8080/api/connect/postgres-test \
  -H "Authorization: Bearer <dev-token>"

# Gets 403 Forbidden for production database
curl -X POST http://localhost:8080/api/connect/postgres-prod \
  -H "Authorization: Bearer <dev-token>"
```

## Migration Guide

### Minimal Changes (Keep Using Local Auth)

Your existing config will work as-is. Optionally add tags:

```yaml
connections:
  - name: postgres-test
    # ... existing config ...
    tags:              # Add tags (optional)
      - env:test
```

### Migrate to Policies

**Old:**
```yaml
connections:
  - name: postgres-prod
    whitelist:
      - "^SELECT.*"
```

**New:**
```yaml
connections:
  - name: postgres-prod
    tags:
      - env:production

policies:
  - name: prod-readonly
    roles: [developer]
    tags: [env:production]
    tag_match: any
    whitelist:
      - "^SELECT.*"
```

### Enable OIDC

1. Set up OIDC provider (Keycloak, Auth0, etc.)
2. Add provider config to `config.yaml`
3. Ensure roles are included in JWT (configure in OIDC provider)
4. Define policies for those roles

## Use Cases

### 1. Environment Separation
```yaml
# Connections tagged by environment
connections:
  - name: db-dev
    tags: [env:dev]
  - name: db-prod
    tags: [env:production]

# Policies restrict access
policies:
  - name: dev-full
    roles: [developer]
    tags: [env:dev]
    whitelist: [".*"]

  - name: dev-prod-readonly
    roles: [developer]
    tags: [env:production]
    whitelist: ["^SELECT.*"]
```

### 2. Team Isolation
```yaml
# Team-specific connections
connections:
  - name: backend-api
    tags: [team:backend]
  - name: frontend-cdn
    tags: [team:frontend]

# Team-specific policies
policies:
  - name: backend-team-access
    roles: [backend-developer]
    tags: [team:backend]
    whitelist: [".*"]
```

### 3. Multi-Tenant
```yaml
# Customer-specific connections
connections:
  - name: customer-a-db
    tags: [customer:a, type:database]
  - name: customer-b-db
    tags: [customer:b, type:database]

# Customer-specific access
policies:
  - name: customer-a-support
    roles: [customer-a-support]
    tags: [customer:a]
    whitelist: ["^SELECT.*"]
```

## Security Improvements

1. ✅ **Enterprise Auth** - OIDC/LDAP integration
2. ✅ **Least Privilege** - Granular role-based policies
3. ✅ **Environment Isolation** - Tag-based separation
4. ✅ **Audit Trail** - Roles logged in audit events
5. ✅ **Centralized Control** - Policies in config, not per-connection

## Next Steps

1. Review `AUTHENTICATION_GUIDE.md` for detailed documentation
2. Update your `config.yaml` with tags and policies
3. Test with docker-compose test environment
4. Configure OIDC or LDAP for production
5. Define role policies for your organization

## Support

- Full documentation: `AUTHENTICATION_GUIDE.md`
- Example config: `config.example.yaml`
- Test environment: `docker-compose.yml`
- Architecture: `ARCHITECTURE.md`

## Breaking Changes

**None** - This is a fully backward-compatible upgrade.

Existing configurations will continue to work without modification. New features are opt-in.

