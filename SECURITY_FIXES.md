# Critical Security Fixes

## Overview

Two critical security vulnerabilities and two additional issues were discovered and fixed in the PostgreSQL proxy authentication and authorization system.

**All issues are now resolved and tested.** ✅

## Fixed Vulnerabilities

### 1. Username Impersonation (HIGH SEVERITY)

**Status:** ✅ FIXED

**Issue:** Users could connect to PostgreSQL using ANY username/password from the API user list, not just their own authenticated credentials.

**Impact:** A user authenticated as "developer" could connect to PostgreSQL as "admin" or any other user, bypassing role-based access controls.

**Example Attack:**
```bash
# User logs in as "developer"
./bin/port-authorizing-cli login -u developer -p dev123

# But then connects to PostgreSQL as "admin"
psql -h localhost -p 5433 -U admin -d testdb
# This would succeed! ❌
```

**Fix:** Added strict username validation in `internal/proxy/postgres_auth.go`:

```go
// SECURITY: Enforce that psql username matches authenticated API username
if clientUser != p.username {
    p.sendAuthError(clientConn, "Username mismatch: you must connect as your authenticated user")
    audit.Log(p.auditLogPath, p.username, "postgres_auth_failed", p.config.Name, map[string]interface{}{
        "connection_id": p.connectionID,
        "client_user":   clientUser,
        "expected_user": p.username,
        "reason":        "username_mismatch",
    })
    return fmt.Errorf("username mismatch: client=%s, authenticated=%s", clientUser, p.username)
}
```

**After Fix:**
```bash
# User logs in as "developer"
./bin/port-authorizing-cli login -u developer -p dev123

# Can ONLY connect as "developer"
psql -h localhost -p 5433 -U developer -d testdb  # ✅ Works
psql -h localhost -p 5433 -U admin -d testdb      # ❌ Blocked with error
```

---

### 2. Whitelist Bypass (CRITICAL SEVERITY)

**Status:** ✅ FIXED

**Issue:** Query whitelists were completely ignored - queries were logged but NOT validated. Users could execute ANY SQL command regardless of configured whitelist policies.

**Impact:** Developers with read-only access could execute DELETE, DROP, UPDATE, and any other SQL commands, completely bypassing access controls.

**Example Attack:**
```yaml
# Config policy: developer can only SELECT
policies:
  - name: dev-test
    roles: [developer]
    tags: [env:test]
    whitelist:
      - "^SELECT.*"
```

```bash
# User logs in as developer
./bin/port-authorizing-cli login -u developer -p dev123

# Despite whitelist, ALL queries work
psql -h localhost -p 5433 -U developer -d testdb
> DELETE FROM users;  # This would succeed! ❌
> DROP TABLE logs;    # This would succeed! ❌
```

**Fix:**

1. **Pass whitelist from authorizer to proxy** (`internal/api/proxy_postgres.go`):
```go
// Get whitelist for this user's roles and connection
whitelist := s.authz.GetWhitelistForConnection(roles, conn.Config.Name)

// Pass to proxy
pgProxy := proxy.NewPostgresAuthProxy(
    conn.Config,
    s.config.Logging.AuditLogPath,
    username,
    connectionID,
    s.config,
    whitelist,  // NEW: Pass whitelist
)
```

2. **Validate queries before forwarding** (`internal/proxy/postgres_auth.go`):
```go
func (p *PostgresAuthProxy) forwardWithLogging(src, dst net.Conn, logQueries bool) {
    // ...
    if logQueries {
        // Validate queries against whitelist before forwarding
        if blocked, query := p.validateAndLogQuery(data); blocked {
            // Send error to client and don't forward to backend
            p.sendQueryBlockedError(src, query)
            continue  // Don't forward blocked query
        }
    }
    // ...
}
```

3. **Implement whitelist validation**:
```go
func (p *PostgresAuthProxy) isQueryAllowed(query string) bool {
    // If no whitelist, allow everything (backward compatibility)
    if len(p.whitelist) == 0 {
        return true
    }

    // Check each whitelist pattern
    for _, pattern := range p.whitelist {
        matched, err := regexp.MatchString(pattern, query)
        if err != nil {
            // Log bad pattern but don't block
            continue
        }
        if matched {
            return true
        }
    }

    return false
}
```

**After Fix:**
```bash
# User logs in as developer (only SELECT allowed)
./bin/port-authorizing-cli login -u developer -p dev123

psql -h localhost -p 5433 -U developer -d testdb
> SELECT * FROM users;    # ✅ Works (matches whitelist)
> DELETE FROM users;      # ❌ Blocked: "Query blocked by whitelist policy"
> DROP TABLE logs;        # ❌ Blocked: "Query blocked by whitelist policy"
> UPDATE users SET ...;   # ❌ Blocked: "Query blocked by whitelist policy"
```

---

### 3. Additional Improvements

#### 3.1 Case-Insensitive Whitelist Matching

**Issue:** Whitelist patterns were case-sensitive, making them fragile and easy to bypass.

**Example:**
```yaml
whitelist:
  - "^SELECT.*"  # Would NOT match: "select * from users"
```

**Fix:** All whitelist patterns now use case-insensitive matching with `(?i)` flag:

```go
// Compile with case-insensitive flag
re, err := regexp.Compile("(?i)" + pattern)
```

**Result:**
- `^SELECT.*` matches: `SELECT`, `select`, `SeLeCt` ✅
- `^DELETE.*` blocks: `DELETE`, `delete`, `DeLeTe` ✅

#### 3.2 Client Hanging on Blocked Queries

**Issue:** When a query was blocked, the client would hang waiting for a response because we only sent the error but not the `ReadyForQuery` message.

**Fix:** Send proper PostgreSQL protocol response:

```go
// 1. Send error message
conn.Write(errorMessage)

// 2. Send ReadyForQuery to prevent hanging
var readyBuf bytes.Buffer
readyBuf.WriteByte('Z') // ReadyForQuery message type
binary.Write(&readyBuf, binary.BigEndian, int32(5)) // Length
readyBuf.WriteByte('I') // Transaction status: Idle
conn.Write(readyBuf.Bytes())
```

**Result:**
- Client receives proper error immediately
- Connection stays open for next query
- No hanging or timeout ✅

---

## Audit Logging Enhancements

Both fixes include comprehensive audit logging:

### Username Mismatch
```json
{
  "timestamp": "2025-10-04T11:45:00Z",
  "username": "developer",
  "action": "postgres_auth_failed",
  "connection": "postgres-test",
  "client_user": "admin",
  "expected_user": "developer",
  "reason": "username_mismatch"
}
```

### Query Blocked
```json
{
  "timestamp": "2025-10-04T11:45:30Z",
  "username": "developer",
  "action": "postgres_query_blocked",
  "connection": "postgres-test",
  "query": "DELETE FROM users",
  "reason": "whitelist_violation"
}
```

## Files Modified

- `internal/proxy/postgres_auth.go` - Added username validation and whitelist enforcement
- `internal/api/proxy_postgres.go` - Pass roles and whitelist to proxy
- `internal/cli/connect.go` - Updated connection info to warn about username requirement
- `config.yaml` - Fixed whitelist patterns (added `^` anchors)

## Testing

To verify the fixes:

```bash
# 1. Start API
./bin/port-authorizing-api --config config.yaml

# 2. Login as developer
./bin/port-authorizing-cli login -u developer -p dev123

# 3. Connect to PostgreSQL
./bin/port-authorizing-cli connect postgres-test -l 5433

# 4. Try various queries
psql -h localhost -p 5433 -U developer -d testdb

# Test username validation
psql -h localhost -p 5433 -U admin -d testdb
# Expected: Error: "Username mismatch"

# Test whitelist (if developer only has SELECT)
> SELECT * FROM users;  # Should work
> DELETE FROM users;    # Should be blocked
> UPDATE users SET ...  # Should be blocked

# 5. Check audit log
tail -f audit.log | jq
```

## Backward Compatibility

✅ **Fully backward compatible:**
- Empty whitelist = allow all (backward compatible)
- Legacy whitelist in connections still works
- No breaking changes to API or CLI

## Security Best Practices

1. **Always use role-based policies** with specific whitelists
2. **Use `^` anchors** in regex patterns: `^SELECT.*` not `SELECT.*`
3. **Monitor audit logs** for:
   - `postgres_auth_failed` with `username_mismatch`
   - `postgres_query_blocked` events
4. **Test whitelists** before deploying to production
5. **Use least privilege** - grant minimum required SQL operations

## Configuration Examples

### Correct Whitelist Patterns

```yaml
policies:
  # Read-only access
  - name: readonly
    roles: [analyst]
    tags: [env:production]
    whitelist:
      - "^SELECT.*"           # ✅ Anchored at start
      - "^EXPLAIN.*"          # ✅ Specific command
      - "^SHOW.*"             # ✅ Show commands

  # Limited write access
  - name: app-writer
    roles: [application]
    tags: [env:production, type:database]
    whitelist:
      - "^SELECT.*"
      - "^INSERT INTO (logs|events|metrics).*"  # ✅ Specific tables
      - "^UPDATE users SET last_login.*WHERE id.*"  # ✅ Specific updates
```

### Incorrect Patterns (DO NOT USE)

```yaml
policies:
  - name: bad-policy
    whitelist:
      - "SELECT.*"          # ❌ No anchor - matches anywhere in query
      - ".*DELETE.*"        # ❌ Would allow: "-- comment DELETE; DROP TABLE"
      - ".*"                # ❌ Allows everything
```

## Impact Assessment

**Before Fix:**
- ❌ No username enforcement
- ❌ No whitelist enforcement
- ❌ Any authenticated user could impersonate others
- ❌ Any authenticated user could execute any SQL

**After Fix:**
- ✅ Strict username enforcement
- ✅ Whitelist validation before query execution
- ✅ Comprehensive audit logging
- ✅ Blocked queries don't reach the database
- ✅ Clear error messages to users

## Credits

Vulnerabilities discovered during security review.
Fixes implemented: 2025-10-04

## Related Documentation

- `AUTHENTICATION_GUIDE.md` - Complete authentication and authorization guide
- `AUTH_UPGRADE_SUMMARY.md` - Migration and upgrade guide
- `ARCHITECTURE.md` - System architecture

