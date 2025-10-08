# SQL Table-Level Permissions

## Overview

Port Authorizing provides **fine-grained, table-level access control** for PostgreSQL connections using **semantic SQL analysis**. This goes beyond simple regex pattern matching by actually **parsing SQL queries** to understand operations, tables, and structure.

## Why SQL Parsing?

### Problems with Regex-Only Approach

**Regex patterns are brittle:**
```sql
-- Both match "^DELETE FROM users.*"
DELETE FROM users WHERE id = 123;  -- Safe, specific
DELETE FROM users;                  -- DANGEROUS, deletes all!
```

**Regex can't detect SQL injection:**
```sql
-- Matches "^SELECT.*" pattern
SELECT * FROM users; DROP TABLE users; --  -- INJECTION!
```

**Regex can't understand multi-table queries:**
```sql
-- Which tables are accessed?
SELECT u.name, o.total
FROM users u
JOIN orders o ON u.id = o.user_id
WHERE o.status = 'pending';
```

### SQL Parsing Advantages

✅ **Semantic Understanding:** Parse SQL into AST, understand actual operations
✅ **Injection Prevention:** Detect multiple statements, invalid SQL
✅ **Table-Level Control:** Allow `SELECT` on `users` but not `INSERT`
✅ **Pattern Matching:** Support wildcards: `logs_*`, `*_temp`
✅ **Defense in Depth:** Works alongside regex whitelist & approval workflow

## Architecture

### Parsing Flow

```
┌─────────────────────┐
│  SQL Query          │
│  "SELECT * FROM..." │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│  pg_query_go        │ ← PostgreSQL's native parser
│  (libpg_query)      │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│  AST (Parse Tree)   │
│  - Operations       │
│  - Tables           │
│  - Columns          │
│  - Joins            │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│  Security Analyzer  │
│  Check permissions  │
└──────────┬──────────┘
           │
     ┌─────┴─────┐
     │           │
 ❌ BLOCK     ✅ ALLOW
```

### Defense in Depth (4 Layers)

When a SQL query is executed:

1. **SQL Semantic Analysis** (NEW)
   - Parse query into AST
   - Detect malformed SQL / injection
   - Check table-level permissions

2. **Regex Whitelist** (Legacy)
   - Broad pattern matching
   - Additional custom rules

3. **Approval Workflow**
   - Human oversight for dangerous operations
   - Pattern-based approval triggers

4. **Audit Logging**
   - Record all queries (allowed & blocked)
   - Full audit trail

## Configuration

### Basic Example

```yaml
policies:
  - name: developer-readonly
    roles:
      - developer
    tags:
      - env:production
    tag_match: any

    # Fine-grained table permissions
    database_permissions:
      - operations: [SELECT, EXPLAIN]
        tables: ["*"]  # Allow SELECT on all tables
```

### Advanced Example (Wildcard Patterns)

```yaml
policies:
  - name: developer-staging
    roles:
      - developer
    tags:
      - env:staging
    tag_match: any

    database_permissions:
      # Read access to all tables
      - operations: [SELECT, EXPLAIN]
        tables: ["*"]

      # Write access to logging tables only
      - operations: [INSERT]
        tables: [logs_*, audit_*, events_*]  # Pattern matching!

      # Update access to specific tables
      - operations: [UPDATE]
        tables: [users, sessions, config]

      # Delete only temporary tables
      - operations: [DELETE]
        tables: [temp_*, *_cache]

      # DROP, TRUNCATE, ALTER are implicitly denied
      # (not in any permission list)
```

### Production Admin (Strict Control)

```yaml
policies:
  - name: admin-prod-controlled
    roles:
      - admin
    tags:
      - env:production
    tag_match: any

    whitelist:
      - ".*"  # Regex allows everything

    # BUT table permissions provide fine-grained control
    database_permissions:
      - operations: [SELECT, EXPLAIN]
        tables: ["*"]  # Read everything

      - operations: [INSERT]
        tables: [logs_*, audit_*]  # Only logging tables

      - operations: [UPDATE]
        tables: [users, config]  # Limited updates

      - operations: [DELETE]
        tables: [sessions]  # Only session cleanup

      # NOTE: DROP, TRUNCATE, ALTER are NOT listed
      # → Automatically blocked, even though regex allows!
```

## Supported Operations

| Operation | Description |
|-----------|-------------|
| `SELECT` | Read data from tables |
| `INSERT` | Add new rows |
| `UPDATE` | Modify existing rows |
| `DELETE` | Remove rows |
| `TRUNCATE` | Remove all rows (fast delete) |
| `DROP` | Delete table/database entirely |
| `ALTER` | Modify table structure |
| `CREATE` | Create new table/database |
| `GRANT` | Grant permissions |
| `REVOKE` | Revoke permissions |
| `EXPLAIN` | Query execution plan (read-only) |

## Pattern Matching

### Exact Match

```yaml
tables: [users, orders, products]
```

### Wildcard (All Tables)

```yaml
tables: ["*"]
```

### Prefix Match

```yaml
tables: [logs_*]  # Matches: logs_2024, logs_errors, logs_access
```

### Suffix Match

```yaml
tables: [*_temp, *_cache]  # Matches: user_temp, session_cache
```

## Security Features

### 1. SQL Injection Prevention

**Malformed SQL is automatically blocked:**

```sql
-- Parse error → blocked
SELCT * FROM users;  -- Typo

-- Multiple statements → blocked
SELECT * FROM users; DROP TABLE users;
```

**Audit log:**
```json
{
  "action": "postgres_query_blocked",
  "reason": "sql_parse_error",
  "error": "syntax error at or near \"SELCT\""
}
```

### 2. Table Access Control

**Only allowed operations pass:**

```yaml
database_permissions:
  - operations: [SELECT]
    tables: [users]
```

```sql
SELECT * FROM users WHERE id = 1;  -- ✅ Allowed
DELETE FROM users WHERE id = 1;    -- ❌ Blocked
```

**Audit log:**
```json
{
  "action": "postgres_query_blocked",
  "reason": "table_permission_violation",
  "details": "operation DELETE not allowed on table 'users'",
  "operations": ["DELETE"],
  "tables": ["users"]
}
```

### 3. Multi-Table Queries

**All accessed tables must be allowed:**

```yaml
database_permissions:
  - operations: [SELECT]
    tables: [users]  # Only users, not orders!
```

```sql
-- ❌ Blocked - orders not in allowed list
SELECT u.name, o.total
FROM users u
JOIN orders o ON u.id = o.user_id;
```

**Audit log:**
```json
{
  "action": "postgres_query_blocked",
  "reason": "table_permission_violation",
  "details": "operation SELECT not allowed on table 'orders'",
  "operations": ["SELECT"],
  "tables": ["users", "orders"]
}
```

### 4. Defense in Depth

**Both systems work together:**

```yaml
whitelist:
  - "^SELECT.*"  # Regex layer

database_permissions:
  - operations: [SELECT]
    tables: [public_*]  # Table-level layer
```

```sql
SELECT * FROM users;        -- ✅ Passes regex, ❌ blocked by table permissions
SELECT * FROM public_data;  -- ✅ Passes both layers
```

## Audit Logging

### Successful Query

```json
{
  "timestamp": "2025-10-07T14:32:01Z",
  "username": "developer",
  "action": "postgres_query",
  "resource": "postgres-staging",
  "metadata": {
    "connection_id": "abc-123",
    "query": "SELECT * FROM logs_2024 WHERE level = 'error'",
    "database": "app_db",
    "sql_analysis": "passed",
    "operations": ["SELECT"],
    "tables": ["logs_2024"],
    "table_permissions": true
  }
}
```

### Blocked Query (Parse Error)

```json
{
  "timestamp": "2025-10-07T14:33:15Z",
  "username": "attacker",
  "action": "postgres_query_blocked",
  "resource": "postgres-prod",
  "metadata": {
    "connection_id": "def-456",
    "query": "SELECT * FROM users; DROP TABLE users;",
    "reason": "sql_parse_error",
    "error": "cannot insert multiple commands into a prepared statement"
  }
}
```

### Blocked Query (Table Permission)

```json
{
  "timestamp": "2025-10-07T14:35:42Z",
  "username": "developer",
  "action": "postgres_query_blocked",
  "resource": "postgres-prod",
  "metadata": {
    "connection_id": "ghi-789",
    "query": "DELETE FROM users WHERE id = 1",
    "reason": "table_permission_violation",
    "details": "operation DELETE not allowed on table 'users'",
    "operations": ["DELETE"],
    "tables": ["users"]
  }
}
```

## Usage Example

### Step 1: Configure Permissions

```yaml
# config.yaml
policies:
  - name: analyst-readonly
    roles:
      - data_analyst
    tags:
      - env:production
    database_permissions:
      - operations: [SELECT, EXPLAIN]
        tables: [analytics_*, reports_*]
```

### Step 2: Connect

```bash
# Login and get token
./bin/port-authorizing-cli login -u analyst -p password

# Connect to database
./bin/port-authorizing-cli connect postgres-prod -l 5433
```

### Step 3: Query Database

```bash
# In another terminal
psql -h localhost -p 5433 -U postgres app_db
```

```sql
-- ✅ Allowed
SELECT * FROM analytics_users;
SELECT * FROM reports_daily;
EXPLAIN SELECT * FROM analytics_sales;

-- ❌ Blocked (table not in pattern)
SELECT * FROM sensitive_data;
-- Error: operation SELECT not allowed on table 'sensitive_data'

-- ❌ Blocked (operation not allowed)
INSERT INTO analytics_users VALUES (...);
-- Error: operation INSERT not allowed on table 'analytics_users'

-- ❌ Blocked (SQL injection detected)
SELECT * FROM analytics_users; DROP TABLE analytics_users;
-- Error: sql_parse_error
```

## Best Practices

### 1. **Start with Read-Only**

```yaml
database_permissions:
  - operations: [SELECT, EXPLAIN]
    tables: ["*"]
```

### 2. **Use Wildcard Patterns**

Group similar tables:
```yaml
- operations: [INSERT]
  tables: [logs_*, events_*, audit_*]
```

### 3. **Combine with Approval Workflow**

Table permissions + approval = maximum security:

```yaml
database_permissions:
  - operations: [DELETE]
    tables: [temp_*, *_cache]

approval:
  enabled: true
  patterns:
    - pattern: "(?i)^DELETE.*"  # Still require approval!
      timeout_seconds: 30
```

### 4. **Monitor Audit Logs**

Track blocked queries to refine policies:

```bash
grep "postgres_query_blocked" audit.log | jq .
```

### 5. **Test in Staging First**

Don't apply strict table permissions to production without testing!

```yaml
# Test in staging
- name: test-strict-perms
  tags: [env:staging]
  database_permissions:
    - operations: [SELECT]
      tables: ["*"]
```

## Limitations

### 1. **PostgreSQL Only**

Table-level permissions currently only work for **PostgreSQL** connections. HTTP/TCP/MySQL connections still use regex whitelists.

### 2. **No Column-Level Permissions (Yet)**

You can control operations and tables, but not specific columns:

```yaml
# NOT YET SUPPORTED
columns: [id, name]  # Restrict to specific columns
```

### 3. **No WHERE Clause Analysis (Yet)**

Can't enforce "DELETE must have WHERE clause":

```sql
DELETE FROM users;  -- Both allowed if DELETE is permitted
DELETE FROM users WHERE id = 1;
```

**Workaround:** Use approval workflow:

```yaml
approval:
  patterns:
    - pattern: "(?i)^DELETE FROM \\w+ *$"  # DELETE without WHERE
      timeout_seconds: 60
```

## Troubleshooting

### Issue: "sql_parse_error" for Valid SQL

**Cause:** SQL syntax not supported by PostgreSQL parser.

**Solution:** Check PostgreSQL compatibility, or use regex whitelist as fallback.

### Issue: Queries Blocked Despite Matching Pattern

**Cause:** Table permissions are **more restrictive** than regex whitelist.

**Solution:** Both layers must pass. Check table permissions:

```bash
# Check audit logs
grep "table_permission_violation" audit.log
```

### Issue: JOINs Always Blocked

**Cause:** All tables in JOIN must be in `tables` list.

**Solution:** Add all joined tables:

```yaml
database_permissions:
  - operations: [SELECT]
    tables: [users, orders, products]  # Include all!
```

Or use wildcard:

```yaml
tables: ["*"]
```

## Comparison: Regex vs Table Permissions

| Feature | Regex Whitelist | Table Permissions |
|---------|----------------|-------------------|
| **Injection Prevention** | ❌ Limited | ✅ Strong (parser detects) |
| **Table-Level Control** | ❌ Hard to enforce | ✅ Built-in |
| **Operation Control** | ❌ Regex patterns only | ✅ Explicit (SELECT, INSERT, etc.) |
| **Wildcard Patterns** | ✅ Yes (regex) | ✅ Yes (prefix/suffix) |
| **Performance** | ⚡ Very Fast | ⚡ Fast (cached AST) |
| **Setup Complexity** | 🟢 Simple | 🟡 Moderate |
| **Accuracy** | 🟡 Pattern-based | 🟢 Semantic |

**Recommendation:** Use **both** for defense in depth!

## Summary

🛡️ **SQL table-level permissions provide fine-grained, semantic access control for PostgreSQL connections.**

✅ **Prevents SQL injection** by parsing queries
✅ **Controls operations** (SELECT, INSERT, UPDATE, DELETE, etc.)
✅ **Controls table access** with wildcard patterns
✅ **Works alongside** regex whitelist & approval workflow
✅ **Full audit trail** of allowed and blocked queries

**Security through layers:** SQL parsing + regex whitelist + approval + audit logging = **defense in depth**! 🚀

