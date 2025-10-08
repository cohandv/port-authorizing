# SQL Parsing - Quick Start Guide

## 🚀 Quick Reference

### Basic Configuration

```yaml
policies:
  - name: my-policy
    roles: [developer]
    tags: [env:staging]

    # Table-level permissions (NEW!)
    database_permissions:
      - operations: [SELECT, EXPLAIN]
        tables: ["*"]  # All tables

      - operations: [INSERT]
        tables: [logs_*, audit_*]  # Pattern matching

      - operations: [UPDATE, DELETE]
        tables: [sessions, temp_*]
```

### Testing (macOS)

```bash
# SQL parser tests in Docker
make docker-test-security

# All tests in Docker
make docker-test

# Verbose output
make docker-test-verbose
```

### Defense in Depth

```
Query → ① SQL Parse → ② Regex → ③ Approval → ④ Audit
        ✓ Injection   ✓ Patterns ✓ Human   ✓ Log
        ✓ Tables
        ✓ Operations
```

### Pattern Matching

| Pattern | Matches |
|---------|---------|
| `*` | All tables |
| `logs_*` | `logs_2024`, `logs_errors`, `logs_access` |
| `*_temp` | `user_temp`, `session_temp`, `cache_temp` |
| `users` | Exact match: `users` only |

### Supported Operations

```yaml
operations: [
  SELECT,    # Read data
  INSERT,    # Add rows
  UPDATE,    # Modify rows
  DELETE,    # Remove rows
  TRUNCATE,  # Fast delete all
  DROP,      # Delete table
  ALTER,     # Modify structure
  CREATE,    # Create table
  EXPLAIN    # Query plan
]
```

### Security Features

✅ **SQL Injection Prevention**
```sql
-- ❌ Blocked automatically
SELECT * FROM users; DROP TABLE users;
-- Detected as multiple statements
```

✅ **Table Access Control**
```yaml
database_permissions:
  - operations: [SELECT]
    tables: [public_data]
```
```sql
SELECT * FROM public_data;  -- ✅ Allowed
SELECT * FROM secrets;       -- ❌ Blocked
```

✅ **Operation Control**
```yaml
database_permissions:
  - operations: [SELECT]  # Read-only
    tables: ["*"]
```
```sql
SELECT * FROM users;  -- ✅ Allowed
DELETE FROM users;    -- ❌ Blocked
```

### Audit Logs

**Successful Query:**
```json
{
  "action": "postgres_query",
  "sql_analysis": "passed",
  "operations": ["SELECT"],
  "tables": ["users"]
}
```

**Blocked Query:**
```json
{
  "action": "postgres_query_blocked",
  "reason": "table_permission_violation",
  "details": "operation DELETE not allowed on table 'users'"
}
```

### Common Patterns

**Read-Only Access:**
```yaml
database_permissions:
  - operations: [SELECT, EXPLAIN]
    tables: ["*"]
```

**Logging/Audit Only:**
```yaml
database_permissions:
  - operations: [INSERT]
    tables: [logs_*, events_*, audit_*]
```

**Limited Admin:**
```yaml
database_permissions:
  - operations: [SELECT, EXPLAIN]
    tables: ["*"]
  - operations: [INSERT, UPDATE]
    tables: [config, settings]
  - operations: [DELETE]
    tables: [sessions, temp_*]
  # DROP, TRUNCATE, ALTER not listed → denied
```

### Troubleshooting

**Issue:** Tests fail on macOS
```bash
# Solution: Use Docker
make docker-test-security
```

**Issue:** Queries blocked despite matching regex
```bash
# Check table permissions in audit log
grep "table_permission_violation" audit.log

# Both layers must pass: regex AND table permissions
```

**Issue:** JOINs always blocked
```yaml
# Solution: Include ALL joined tables
database_permissions:
  - operations: [SELECT]
    tables: [users, orders, products]  # All tables in JOIN
```

### Best Practices

1. **Start with read-only**
   ```yaml
   database_permissions:
     - operations: [SELECT]
       tables: ["*"]
   ```

2. **Use wildcards for related tables**
   ```yaml
   tables: [logs_*, events_*, audit_*]
   ```

3. **Combine with approval for dangerous ops**
   ```yaml
   database_permissions:
     - operations: [DELETE]
       tables: [temp_*]

   approval:
     patterns:
       - pattern: "(?i)^DELETE.*"
   ```

4. **Monitor audit logs**
   ```bash
   grep "postgres_query_blocked" audit.log | jq .
   ```

5. **Test in staging first**
   ```yaml
   tags: [env:staging]  # Test before production
   ```

## 📚 Full Documentation

- **`docs/features/SQL-TABLE-PERMISSIONS.md`** - Complete feature guide
- **`docs/SQL-PARSER-MACOS.md`** - macOS compilation issue
- **`docs/SQL-PARSING-IMPLEMENTATION.md`** - Implementation details

## 🎯 Example: Production Read-Only

```yaml
policies:
  - name: analyst-prod-readonly
    roles: [data_analyst]
    tags: [env:production]
    tag_match: any

    database_permissions:
      # Read access to analytics tables only
      - operations: [SELECT, EXPLAIN]
        tables: [analytics_*, reports_*, dashboards_*]
```

**Result:**
```sql
SELECT * FROM analytics_users;   -- ✅ Allowed
SELECT * FROM reports_daily;     -- ✅ Allowed
SELECT * FROM users;              -- ❌ Blocked (not in pattern)
INSERT INTO analytics_users ...  -- ❌ Blocked (INSERT not allowed)
DROP TABLE analytics_users;      -- ❌ Blocked (DROP not allowed)
```

---

**That's it!** SQL parsing provides fine-grained, semantic access control for PostgreSQL. 🚀

