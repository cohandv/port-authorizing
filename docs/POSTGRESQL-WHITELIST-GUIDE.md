# PostgreSQL Whitelist Configuration Guide

## Overview

When using PostgreSQL connections through port-authorizing, you need to configure whitelist patterns for SQL queries. This guide explains common patterns and how to troubleshoot connection issues.

## Why Whitelist is Needed

Port-authorizing intercepts and validates ALL PostgreSQL queries (including connection initialization commands) before forwarding them to the database. This includes:
- User queries (SELECT, UPDATE, DELETE, etc.)
- Client initialization commands (SET, SHOW, etc.)
- Transaction control (BEGIN, COMMIT, ROLLBACK)

## Common Whitelist Patterns

### Minimal Read-Only Access
```yaml
whitelist:
  - "^SELECT.*"
  - "^EXPLAIN.*"
  - "^SET.*"        # Required for connection
  - "^SHOW.*"       # Required for connection
  - "^BEGIN.*"      # Transaction control
  - "^COMMIT.*"
  - "^ROLLBACK.*"
```

### Developer Access (Read + Write Test Data)
```yaml
whitelist:
  - "^SELECT.*"
  - "^INSERT.*"
  - "^UPDATE.*"
  - "^EXPLAIN.*"
  - "^SET.*"
  - "^SHOW.*"
  - "^BEGIN.*"
  - "^COMMIT.*"
  - "^ROLLBACK.*"
```

### Admin Access (Everything)
```yaml
whitelist:
  - ".*"  # Allow all queries
```

## Client-Specific Initialization Commands

Different PostgreSQL clients and drivers send different SET commands during connection. Here are common examples:

### psql (PostgreSQL CLI)
```sql
SET extra_float_digits = 3
SET client_encoding = 'UTF8'
SET DateStyle = 'ISO, MDY'
SET TimeZone = 'UTC'
```

### Python psycopg2
```sql
SET client_encoding = 'UTF8'
SET DateStyle = 'ISO'
SET TimeZone = 'UTC'
```

### Go lib/pq
```sql
SET application_name = 'myapp'
SET client_encoding = 'UTF8'
```

### Node.js pg
```sql
SET client_encoding = 'UTF8'
SET DateStyle = 'ISO'
```

## Troubleshooting Connection Issues

### Error: "Query blocked by whitelist policy: SET ..."

**Problem**: Client is sending initialization commands that aren't in your whitelist.

**Solution**:
1. Check audit.log for the blocked query:
   ```bash
   tail -100 audit.log | grep postgres_query_blocked
   ```

2. Identify the blocked command (e.g., `SET extra_float_digits = 3`)

3. Add a pattern to your whitelist:
   ```yaml
   whitelist:
     - "^SET.*"  # Allow all SET commands (recommended)
   ```

4. Or be more specific:
   ```yaml
   whitelist:
     - "^SET extra_float_digits.*"
     - "^SET client_encoding.*"
     - "^SET DateStyle.*"
   ```

### Error: "Connection immediately closes"

**Problem**: Multiple initialization commands are being blocked.

**Solution**: Add the complete set of connection patterns:
```yaml
whitelist:
  - "^SET.*"
  - "^SHOW.*"
  - "^BEGIN.*"
  - "^COMMIT.*"
  - "^ROLLBACK.*"
```

## Security Considerations

### Safe Connection Commands

These commands are generally safe to allow because they only affect the client session:

- ✅ `^SET.*` - Session settings (doesn't modify data)
- ✅ `^SHOW.*` - Display settings (read-only)
- ✅ `^BEGIN.*`, `^COMMIT.*`, `^ROLLBACK.*` - Transaction control

### Dangerous Commands

These should be restricted or require approval:

- ⚠️ `^DELETE.*` - Deletes data (consider approval workflow)
- ⚠️ `^DROP.*` - Drops tables/databases (high risk)
- ⚠️ `^TRUNCATE.*` - Removes all data (high risk)
- ⚠️ `^ALTER.*` - Modifies schema (high risk)
- ⚠️ `^CREATE.*` - Creates objects (moderate risk)
- ⚠️ `^GRANT.*` - Modifies permissions (high risk)

## Extended Query Protocol

Modern PostgreSQL clients use the Extended Query Protocol, which sends queries in Parse ('P') messages. Port-authorizing correctly intercepts both:
- Simple Query Protocol ('Q' messages)
- Extended Query Protocol ('P' messages)

This means parameterized queries like `SELECT * FROM users WHERE id = $1` are properly validated.

## Example Configuration

### Development Environment
```yaml
policies:
  - name: dev-test
    roles:
      - developer
    tags:
      - env:test
    whitelist:
      # Read queries
      - "^SELECT.*"
      - "^EXPLAIN.*"

      # Write queries (test env only)
      - "^INSERT.*"
      - "^UPDATE.*"

      # Connection commands (required)
      - "^SET.*"
      - "^SHOW.*"
      - "^BEGIN.*"
      - "^COMMIT.*"
      - "^ROLLBACK.*"

approval:
  enabled: true
  patterns:
    # Require approval for dangerous operations
    - pattern: "^DELETE.*"
      tags: ["env:test"]
      timeout_seconds: 30
    - pattern: "^DROP.*"
      tags: ["env:test"]
      timeout_seconds: 60
```

### Production Environment
```yaml
policies:
  - name: dev-prod-readonly
    roles:
      - developer
    tags:
      - env:production
    whitelist:
      # Read-only queries
      - "^SELECT.*"
      - "^EXPLAIN.*"

      # Connection commands (required)
      - "^SET.*"
      - "^SHOW.*"
      - "^BEGIN.*"
      - "^COMMIT.*"
      - "^ROLLBACK.*"
```

## Testing Your Configuration

1. **Connect to database**:
   ```bash
   port-authorizing connect postgres-test
   psql -h localhost -p [PORT] -U developer -d testdb
   ```

2. **Check audit log**:
   ```bash
   tail -f audit.log | jq 'select(.action | contains("postgres"))'
   ```

3. **Look for**:
   - `postgres_query` - Allowed queries
   - `postgres_query_blocked` - Blocked queries (add to whitelist if needed)
   - `postgres_approval_requested` - Queries requiring approval

## Common Patterns Reference

```yaml
# Case-insensitive regex patterns

# Data queries
"^SELECT.*"           # Any SELECT
"^SELECT .* FROM users.*"  # SELECT from users table only
"^INSERT INTO logs.*" # INSERT into logs table only
"^UPDATE users SET.*" # UPDATE users table

# Schema operations
"^CREATE TABLE.*"     # Create tables
"^ALTER TABLE.*"      # Modify tables
"^DROP TABLE.*"       # Delete tables

# Connection/Session
"^SET.*"              # All SET commands
"^SET (DateStyle|TimeZone|client_encoding).*"  # Specific settings
"^SHOW.*"             # All SHOW commands

# Utility
"^VACUUM.*"           # Vacuum (maintenance)
"^ANALYZE.*"          # Analyze (statistics)
"^COPY.*"             # COPY data
```

## Best Practices

1. **Start restrictive, add as needed**: Begin with minimal patterns and add more based on audit logs

2. **Use specific patterns when possible**: `^SELECT .* FROM public\..*` is better than `^SELECT.*` if you want to restrict schemas

3. **Monitor audit logs**: Regularly check for blocked queries that should be allowed

4. **Use approval workflow for dangerous operations**: Don't just whitelist DELETE/DROP - require approval!

5. **Different patterns for different environments**: Production should be more restrictive than development

6. **Document your patterns**: Add comments explaining why each pattern is needed

## See Also

- [Configuration Guide](configuration.md)
- [Approval Workflow](../features/APPROVAL-WORKFLOW.md)
- [Testing Guide](../development/TESTING.md)

