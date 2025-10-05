# Authentication & Authorization Guide

This guide explains the authentication and authorization system in Port Authorizing.

## Overview

Port Authorizing supports multiple authentication providers and implements role-based access control (RBAC) with tag-based policies.

### Architecture

1. **Authentication** - Verifies user identity via multiple providers (local, OIDC, LDAP, SAML2)
2. **Authorization** - Controls access to connections based on:
   - User roles (from authentication provider)
   - Connection tags (environment, team, type, etc.)
   - Role policies (which roles can access which tagged connections)
   - Whitelists (what operations are allowed per policy)

## Authentication Providers

### Local Provider

Simple username/password authentication. Best for development and small deployments.

```yaml
auth:
  users:
    - username: admin
      password: admin123
      roles:
        - admin
    - username: developer
      password: dev123
      roles:
        - developer
```

### OpenID Connect (OIDC)

Supports any OIDC-compliant provider (Keycloak, Auth0, Okta, Google, Azure AD, etc.)

```yaml
auth:
  providers:
    - name: keycloak
      type: oidc
      enabled: true
      config:
        issuer: "http://localhost:8180/realms/portauth"
        client_id: "port-authorizing"
        client_secret: "your-client-secret"
        redirect_url: "http://localhost:8080/auth/callback/oidc"
        roles_claim: "roles"            # JWT claim containing roles
        username_claim: "preferred_username"
```

**Testing with Keycloak:**
1. Start Keycloak: `docker-compose up keycloak`
2. Access UI: http://localhost:8180
3. Login with admin/admin
4. Test users (password: password123):
   - alice (developer role)
   - bob (admin role)
   - charlie (qa role)

### LDAP

Authenticate against LDAP/Active Directory servers.

```yaml
auth:
  providers:
    - name: corporate-ldap
      type: ldap
      enabled: true
      config:
        url: "ldap.example.com:389"
        bind_dn: "cn=admin,dc=example,dc=com"
        bind_password: "adminpass"
        user_base_dn: "ou=users,dc=example,dc=com"
        user_filter: "(uid=%s)"
        group_base_dn: "ou=groups,dc=example,dc=com"
        group_filter: "(member=%s)"
        use_tls: "true"
        skip_tls_verify: "false"
```

**Testing with OpenLDAP:**
1. Start OpenLDAP: `docker-compose up openldap`
2. Access phpLDAPadmin: http://localhost:8181
3. Login: cn=admin,dc=portauth,dc=local / adminpass
4. Test users (password: password123):
   - alice (developer group)
   - bob (admin group)
   - charlie (qa group)

### SAML2

SAML 2.0 authentication (work in progress).

```yaml
auth:
  providers:
    - name: corporate-saml
      type: saml2
      enabled: true
      config:
        idp_metadata_url: "https://idp.example.com/metadata"
        sp_entity_id: "port-authorizing"
        sp_acs_url: "http://localhost:8080/auth/callback/saml2"
```

## Authorization System

### Connection Tags

Connections are tagged to describe their attributes:

```yaml
connections:
  - name: postgres-prod
    type: postgres
    host: prod-db.example.com
    port: 5432
    tags:
      - env:production        # Environment
      - type:database         # Resource type
      - team:backend          # Owning team
      - critical:true         # Criticality flag
    backend_username: "produser"
    backend_password: "prodpass"
```

Common tag patterns:
- `env:dev`, `env:staging`, `env:production`
- `type:database`, `type:api`, `type:cache`, `type:web`
- `team:backend`, `team:frontend`, `team:data`
- `critical:true`, `critical:false`
- `region:us-east`, `region:eu-west`

### Role Policies

Policies define which roles can access connections with specific tags:

```yaml
policies:
  # Admins have full access to all environments
  - name: admin-full-access
    roles:
      - admin
    tags:
      - env:production
      - env:staging
      - env:test
    tag_match: any          # Match if connection has ANY of these tags
    whitelist:
      - ".*"                # Allow all queries/operations

  # Developers have full access to test
  - name: developer-test-full
    roles:
      - developer
    tags:
      - env:test
    tag_match: any
    whitelist:
      - ".*"

  # Developers have read-only access to production
  - name: developer-prod-readonly
    roles:
      - developer
    tags:
      - env:production
    tag_match: any
    whitelist:
      - "^SELECT.*"         # SQL queries
      - "^EXPLAIN.*"
      - "^GET .*"           # HTTP requests

  # Backend team can access backend services
  - name: backend-team-access
    roles:
      - developer
      - admin
    tags:
      - team:backend
      - type:database
    tag_match: all          # Must have ALL these tags
    whitelist:
      - ".*"
```

### Tag Matching Modes

- **`any`** (default for single-tag policies): Connection must have at least ONE of the policy tags
- **`all`**: Connection must have ALL the policy tags

### Whitelists

Whitelists are regex patterns that define allowed operations:

**For SQL (Postgres):**
```yaml
whitelist:
  - "^SELECT.*"              # Allow all SELECT queries
  - "^INSERT INTO logs.*"    # Allow inserts to logs table
  - "^UPDATE users SET.*WHERE id.*"  # Allow updates with WHERE clause
  - "^EXPLAIN.*"             # Allow EXPLAIN
```

**For HTTP:**
```yaml
whitelist:
  - "^GET .*"                # Allow all GET requests
  - "^POST /api/logs.*"      # Allow POST to logs API
  - "^PUT /api/config.*"     # Allow PUT to config API
```

## Usage Examples

### Example 1: Environment-Based Access

```yaml
connections:
  - name: db-dev
    tags: [env:dev, type:database]
  - name: db-staging
    tags: [env:staging, type:database]
  - name: db-prod
    tags: [env:production, type:database]

policies:
  # Junior devs: full access to dev only
  - name: junior-dev
    roles: [junior-developer]
    tags: [env:dev]
    tag_match: any
    whitelist: [".*"]

  # Senior devs: full dev/staging, read-only prod
  - name: senior-dev-nonprod
    roles: [senior-developer]
    tags: [env:dev, env:staging]
    tag_match: any
    whitelist: [".*"]

  - name: senior-dev-prod
    roles: [senior-developer]
    tags: [env:production]
    tag_match: any
    whitelist: ["^SELECT.*", "^EXPLAIN.*"]
```

### Example 2: Team-Based Access

```yaml
connections:
  - name: backend-api-prod
    tags: [env:production, type:api, team:backend]
  - name: frontend-cdn-prod
    tags: [env:production, type:cdn, team:frontend]

policies:
  # Backend team can only access backend services
  - name: backend-team
    roles: [backend-developer]
    tags: [team:backend]
    tag_match: any
    whitelist: [".*"]

  # Frontend team can only access frontend services
  - name: frontend-team
    roles: [frontend-developer]
    tags: [team:frontend]
    tag_match: any
    whitelist: [".*"]
```

### Example 3: Multi-Tag Policies

```yaml
policies:
  # SRE team: full access to production databases only
  - name: sre-prod-databases
    roles: [sre]
    tags: [env:production, type:database]
    tag_match: all          # Must have BOTH tags
    whitelist: [".*"]

  # Data team: read-only access to all databases in all environments
  - name: data-team-all-db
    roles: [data-analyst]
    tags: [type:database]
    tag_match: any
    whitelist: ["^SELECT.*", "^EXPLAIN.*"]
```

## API Usage

### Login

```bash
curl -X POST http://localhost:8080/api/login \
  -H "Content-Type: application/json" \
  -d '{"username": "developer", "password": "dev123"}'
```

Response:
```json
{
  "token": "eyJhbGc...",
  "expires_at": "2024-01-02T15:04:05Z",
  "user": {
    "username": "developer",
    "email": "developer@example.com",
    "roles": ["developer"]
  }
}
```

### List Connections

Only shows connections the user has access to:

```bash
curl http://localhost:8080/api/connections \
  -H "Authorization: Bearer eyJhbGc..."
```

Response:
```json
[
  {
    "name": "postgres-test",
    "type": "postgres",
    "tags": ["env:test", "type:database"],
    "metadata": {
      "description": "Test database"
    }
  }
]
```

## Migration Guide

### From Old Config (Direct Whitelists)

**Old format:**
```yaml
connections:
  - name: postgres-prod
    type: postgres
    whitelist:
      - "^SELECT.*"
```

**New format:**
```yaml
connections:
  - name: postgres-prod
    type: postgres
    tags:
      - env:production
      - type:database

policies:
  - name: prod-db-access
    roles: [developer]
    tags: [env:production]
    tag_match: any
    whitelist:
      - "^SELECT.*"
```

### Backward Compatibility

The system supports both formats:
- Connections without tags will use legacy whitelist (if present)
- Local users in `auth.users` work alongside auth providers
- Existing configs will continue to work

## Security Best Practices

1. **Use External Auth Providers**: OIDC/LDAP instead of local users in production
2. **Least Privilege**: Give users minimum required access via role policies
3. **Tag Consistently**: Use standard tag naming (env:, type:, team:, etc.)
4. **Specific Whitelists**: Avoid `.*` wildcards except for dev environments
5. **Audit Regularly**: Review audit logs for access patterns
6. **Rotate Secrets**: Change JWT secrets and auth provider secrets regularly

## Troubleshooting

### User Can't See Connections

Check:
1. User has correct roles (check JWT token or auth provider)
2. Roles are spelled correctly in policies
3. Connection has tags that match policy tags
4. Tag match mode (any vs all) is correct

### User Can't Execute Query

Check:
1. User has access to connection (via `CanAccessConnection`)
2. Query matches whitelist regex patterns
3. Whitelist is properly configured in applicable policy
4. Check audit logs for denied operations

### Auth Provider Issues

**OIDC:**
- Verify issuer URL is accessible
- Check client ID and secret
- Ensure redirect URL is registered in provider

**LDAP:**
- Test connectivity: `ldapsearch -x -H ldap://server:389 -D "cn=admin,dc=example,dc=com" -W`
- Verify bind DN and password
- Check user/group filters

## Docker Testing Setup

Start all test services:

```bash
docker-compose up
```

This starts:
- **PostgreSQL** (localhost:5432) - Test database
- **Nginx** (localhost:8888) - Test web server
- **Keycloak** (localhost:8180) - OIDC/SAML2 provider
- **OpenLDAP** (localhost:389) - LDAP server
- **phpLDAPadmin** (localhost:8181) - LDAP UI

Test users (all passwords: `password123`):
- alice: developer role
- bob: admin role
- charlie: qa role

## Example Test Commands

```bash
# Test local auth
./bin/port-authorizing-cli login --username admin --password admin123

# Test OIDC auth (requires Keycloak)
# (Configure OIDC in config.yaml first)
./bin/port-authorizing-cli login --username alice --password password123

# List accessible connections
./bin/port-authorizing-cli list

# Connect to test database
./bin/port-authorizing-cli connect postgres-test
```

