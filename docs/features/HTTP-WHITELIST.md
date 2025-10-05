# HTTP Request Whitelisting

This document describes how to use HTTP request whitelisting to control which HTTP requests users can make through the port-authorizing proxy.

## Overview

HTTP whitelisting works similarly to PostgreSQL query whitelisting but applies to HTTP/HTTPS connections. It allows you to restrict which HTTP methods and endpoints users can access based on their roles.

## Whitelist Format

HTTP whitelist patterns follow this format:

```
METHOD /path/pattern
```

The pattern is a regular expression that matches against the full HTTP request line.

### Pattern Components

- **METHOD**: HTTP verb (GET, POST, PUT, DELETE, PATCH, HEAD, OPTIONS, etc.)
  - Case-insensitive matching (GET, get, Get all match)
- **PATH**: URL path with optional regex patterns
  - Can include query parameters
  - Supports regex patterns for dynamic segments

## Examples

### Basic Patterns

```yaml
whitelist:
  # Allow all GET requests
  - "^GET .*"

  # Allow specific endpoint
  - "^POST /api/users$"

  # Allow endpoint with ID parameter
  - "^GET /api/users/[0-9]+$"

  # Allow DELETE with specific pattern
  - "^DELETE /api/sessions/[a-f0-9\\-]+$"
```

### Multiple Methods

```yaml
whitelist:
  # Allow GET and POST to same endpoint
  - "^(GET|POST) /api/users.*"

  # Allow all safe methods
  - "^(GET|HEAD|OPTIONS) .*"
```

### Complex Patterns

```yaml
whitelist:
  # Allow versioned API endpoints
  - "^GET /api/v[0-9]+/.*"

  # Allow query parameters
  - "^GET /api/users\\?page=[0-9]+&limit=[0-9]+$"

  # Allow nested resources
  - "^GET /api/users/[0-9]+/(posts|comments).*"

  # Allow specific file types
  - "^GET /files/.*\\.(jpg|png|pdf)$"
```

### RESTful CRUD Operations

```yaml
whitelist:
  # Read operations
  - "^GET /api/.*"  # List and read all resources
  - "^HEAD /api/.*"  # Check existence

  # Create operations
  - "^POST /api/users$"  # Create users
  - "^POST /api/items$"  # Create items

  # Update operations
  - "^PUT /api/users/[0-9]+$"  # Full update
  - "^PATCH /api/users/[0-9]+$"  # Partial update

  # Delete operations
  - "^DELETE /api/users/[0-9]+$"
```

## Configuration Examples

### Developer Policy (Staging)

```yaml
- name: developer-staging-api
  roles:
    - developer
  tags:
    - env:staging
    - type:api
  whitelist:
    # Read operations
    - "^GET /api/.*"
    - "^HEAD /api/.*"
    - "^OPTIONS /api/.*"  # CORS preflight

    # Write operations (limited)
    - "^POST /api/logs.*"  # Create logs
    - "^POST /api/analytics.*"  # Send analytics
    - "^PUT /api/users/[0-9]+/profile$"  # Update own profile
    - "^PATCH /api/users/[0-9]+/settings$"  # Update settings
```

### Read-Only Policy (Production)

```yaml
- name: developer-prod-api-readonly
  roles:
    - developer
  tags:
    - env:production
    - type:api
  whitelist:
    - "^GET /api/.*"  # All GET requests
    - "^HEAD /api/.*"  # HEAD requests
    - "^OPTIONS /api/.*"  # OPTIONS for CORS
```

### QA Testing Policy

```yaml
- name: qa-test-api
  roles:
    - qa
  tags:
    - env:test
    - type:api
  whitelist:
    # Allow all CRUD operations for testing
    - "^(GET|POST|PUT|PATCH|DELETE) /api/.*"
    - "^HEAD /api/.*"
    - "^OPTIONS /api/.*"
```

### Admin Policy (Full Access)

```yaml
- name: admin-api-full
  roles:
    - admin
  tags:
    - type:api
  tag_match: any
  whitelist:
    - ".*"  # Allow everything
```

## How It Works

1. **Connection Establishment**: When a user connects to an HTTP/HTTPS service, the whitelist for their roles is retrieved

2. **Request Interception**: Each HTTP request is intercepted and parsed to extract:
   - HTTP method (GET, POST, etc.)
   - Request path (/api/users/123)

3. **Pattern Matching**: The request is matched against whitelist patterns:
   ```
   Request: "GET /api/users/123"
   Pattern: "^GET /api/users/[0-9]+$"
   Result: ALLOWED
   ```

4. **Action**:
   - **Allowed**: Request is forwarded to the backend
   - **Blocked**: Returns 403 Forbidden with JSON error

## Audit Logging

All HTTP requests are logged to the audit log:

### Allowed Request

```json
{
  "timestamp": "2025-01-05T10:30:00Z",
  "username": "developer",
  "action": "http_request",
  "resource": "api-staging",
  "metadata": {
    "connection_id": "uuid",
    "method": "GET",
    "path": "/api/users/123",
    "allowed": true
  }
}
```

### Blocked Request

```json
{
  "timestamp": "2025-01-05T10:30:00Z",
  "username": "developer",
  "action": "http_request_blocked",
  "resource": "api-staging",
  "metadata": {
    "connection_id": "uuid",
    "method": "DELETE",
    "path": "/api/users/123",
    "reason": "does not match whitelist"
  }
}
```

## CORS Support

Port-authorizing includes full CORS support (non-configurable):

- **Access-Control-Allow-Origin**: `*` (all origins)
- **Access-Control-Allow-Methods**: All common HTTP methods
- **Access-Control-Allow-Headers**: Content-Type, Authorization, etc.
- **Preflight Caching**: 24 hours

This allows browser-based applications to connect through the proxy from any origin.

## Combining PostgreSQL and HTTP Whitelists

Whitelists apply to all connection types. You can mix PostgreSQL and HTTP patterns in the same policy:

```yaml
policies:
  - name: developer-mixed
    roles:
      - developer
    tags:
      - env:staging
    whitelist:
      # PostgreSQL patterns
      - "^SELECT.*"  # SQL SELECT
      - "^EXPLAIN.*"  # SQL EXPLAIN

      # HTTP patterns
      - "^GET /api/.*"  # HTTP GET
      - "^POST /api/users$"  # HTTP POST
```

The appropriate patterns are applied based on the connection type:
- **postgres** connections: Only PostgreSQL patterns are checked
- **http/https** connections: Only HTTP patterns are checked

## Testing Whitelist Patterns

You can test your regex patterns using online regex testers like [regex101.com](https://regex101.com/).

### Example Test Cases

| Pattern | Request | Match |
|---------|---------|-------|
| `^GET /api/.*` | `GET /api/users` | ✅ Yes |
| `^GET /api/.*` | `POST /api/users` | ❌ No |
| `^GET /api/users/[0-9]+$` | `GET /api/users/123` | ✅ Yes |
| `^GET /api/users/[0-9]+$` | `GET /api/users/abc` | ❌ No |
| `^(GET\|POST) /api/.*` | `PUT /api/users` | ❌ No |

## Best Practices

1. **Start Restrictive**: Begin with minimal permissions and add as needed
2. **Use Anchors**: Start patterns with `^` to match from the beginning
3. **Test Patterns**: Test regex patterns before deploying to production
4. **Specific Over General**: Use specific endpoints rather than wildcards
5. **Document Patterns**: Add comments explaining each pattern's purpose
6. **Review Regularly**: Audit logs help identify needed permissions
7. **Least Privilege**: Grant only the permissions required for the role

## Common Mistakes

### ❌ Incorrect: Missing anchor

```yaml
whitelist:
  - "GET /api/users"  # Matches anywhere in string
```

### ✅ Correct: With anchor

```yaml
whitelist:
  - "^GET /api/users"  # Matches from start
```

### ❌ Incorrect: Unescaped special characters

```yaml
whitelist:
  - "^GET /api/users?page=1"  # ? is regex special char
```

### ✅ Correct: Escaped special characters

```yaml
whitelist:
  - "^GET /api/users\\?page=[0-9]+"
```

### ❌ Incorrect: Too permissive

```yaml
whitelist:
  - ".*"  # Allows everything
```

### ✅ Correct: Specific permissions

```yaml
whitelist:
  - "^GET /api/.*"  # Only GET requests
  - "^POST /api/users$"  # Specific POST endpoint
```

## Security Considerations

1. **Case Sensitivity**: HTTP methods are case-insensitive in the matching
2. **Query Parameters**: Include query parameter patterns if needed
3. **Path Traversal**: Patterns should prevent path traversal attacks
4. **Method Override**: Be aware of X-HTTP-Method-Override headers
5. **Audit Everything**: All requests (allowed and blocked) are audited

## Related Documentation

- [PostgreSQL Whitelisting](./POSTGRES-WHITELIST.md)
- [Role-Based Access Control](./RBAC.md)
- [Audit Logging](./AUDIT-LOGGING.md)
- [Configuration Guide](../guides/CONFIGURATION.md)

