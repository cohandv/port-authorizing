# CLI Connect Command - Usage Examples

## Overview
The `connect` command now supports three modes:
1. **Single connection** - Connect to one service
2. **Multiple connections** - Connect to several services at once
3. **Tag-based connections** - Connect to all services with a specific tag

---

## Mode 1: Single Connection

```bash
# Syntax: connect <connection-name> <local-port>
port-authorizing connect mydb 5432
port-authorizing connect redis-prod 6379
port-authorizing connect api-dev 8080
```

---

## Mode 2: Multiple Connections

Connect to multiple services simultaneously with name/port pairs:

```bash
# Syntax: connect <name1> <port1> <name2> <port2> ...
port-authorizing connect \
  postgres-dev 5432 \
  redis-dev 6379 \
  api-dev 8080
```

**Example Output:**
```
🚀 Starting 3 connection(s):
  • postgres-dev → localhost:5432
  • redis-dev → localhost:6379
  • api-dev → localhost:8080

✓ postgres-dev connected (expires: 2025-11-20T12:00:00Z)
  └─ psql -h localhost -p 5432 -U admin -d myapp
✓ redis-dev connected (expires: 2025-11-20T12:00:00Z)
✓ api-dev connected (expires: 2025-11-20T12:00:00Z)
```

**Use Case:** Development environment where you need multiple services running:
```bash
# Terminal 1: Start all dev services
pa connect postgres-dev 5432 redis-dev 6379 http-api-dev 8080

# Terminal 2: Use them
psql -h localhost -p 5432 -U admin -d myapp
redis-cli -h localhost -p 6379
curl http://localhost:8080/health
```

---

## Mode 3: Tag-Based Connections

Connect to all services that have a specific tag:

```bash
# Syntax: connect --tag <tag> --base-port <starting-port>
port-authorizing connect --tag env:dev --base-port 8000
port-authorizing connect --tag type:database --base-port 5432
port-authorizing connect --tag team:backend --base-port 9000
```

**How it works:**
1. CLI queries the API for all connections
2. Filters connections by the specified tag
3. Assigns ports sequentially starting from `--base-port`
4. Starts all connections concurrently

**Example with `env:dev` tag:**
```bash
$ pa connect --tag env:dev --base-port 8000

🔍 Finding connections with tag: env:dev
✓ Found 4 connection(s)

🚀 Starting 4 connection(s):
  • postgres-dev → localhost:8000
  • redis-dev → localhost:8001
  • api-dev → localhost:8002
  • queue-dev → localhost:8003

✓ postgres-dev connected (expires: 2025-11-20T12:00:00Z)
✓ redis-dev connected (expires: 2025-11-20T12:00:00Z)
✓ api-dev connected (expires: 2025-11-20T12:00:00Z)
✓ queue-dev connected (expires: 2025-11-20T12:00:00Z)
```

**Use Cases:**

### 1. Environment-based connections
```bash
# Connect to all dev services
pa connect --tag env:dev --base-port 8000

# Connect to all staging services
pa connect --tag env:stage --base-port 9000

# Connect to all production services (read-only)
pa connect --tag env:prod --base-port 10000
```

### 2. Service type grouping
```bash
# All databases
pa connect --tag type:database --base-port 5432

# All caches
pa connect --tag type:cache --base-port 6379

# All APIs
pa connect --tag type:api --base-port 8080
```

### 3. Team-based access
```bash
# Frontend team services
pa connect --tag team:frontend --base-port 3000

# Backend team services
pa connect --tag team:backend --base-port 8000
```

### 4. Project-based
```bash
# All services for project X
pa connect --tag project:mobile-app --base-port 7000

# All services for project Y
pa connect --tag project:analytics --base-port 8000
```

---

## Shutdown

All modes support graceful shutdown:

```bash
# Press Ctrl+C to stop all connections
^C

🛑 Interrupt received, shutting down all connections...
```

All active client connections are closed cleanly.

---

## Port Auto-Assignment Example

When using `--tag`, ports are assigned sequentially:

```bash
$ pa connect --tag env:dev --base-port 5000

# Results in:
# Connection 1 → localhost:5000
# Connection 2 → localhost:5001
# Connection 3 → localhost:5002
# Connection 4 → localhost:5003
# ...
```

This makes it predictable and easy to document:
- Dev environment always starts at 5000
- Staging at 6000
- Production at 7000

---

## Configuration Requirements

For tag-based connections to work, ensure your connections have tags in `config.yaml`:

```yaml
connections:
  - name: postgres-dev
    type: postgres
    host: db.dev.internal
    port: 5432
    tags:
      - env:dev
      - type:database
      - team:backend
    
  - name: redis-dev
    type: redis
    host: cache.dev.internal
    port: 6379
    tags:
      - env:dev
      - type:cache
      - team:backend
    
  - name: api-dev
    type: http
    host: api.dev.internal
    port: 8080
    tags:
      - env:dev
      - type:api
      - team:backend
```

Now you can connect to all dev services:
```bash
pa connect --tag env:dev --base-port 8000
```

Or all backend team services:
```bash
pa connect --tag team:backend --base-port 9000
```

---

## Tips

1. **Use meaningful base ports** that match service types:
   - PostgreSQL: 5432
   - Redis: 6379
   - HTTP: 8000+
   
2. **Tag consistently** across environments:
   ```
   env:dev, env:stage, env:prod
   type:database, type:cache, type:api
   team:frontend, team:backend, team:data
   ```

3. **Document your port ranges** for your team:
   ```
   Dev:   5000-5999
   Stage: 6000-6999
   Prod:  7000-7999
   ```

4. **Use scripts** for common scenarios:
   ```bash
   #!/bin/bash
   # start-dev.sh
   port-authorizing connect --tag env:dev --base-port 5000
   ```

---

## Comparison with Old `-l` Flag

**Old way (deprecated):**
```bash
pa connect mydb -l 5432
```

**New way:**
```bash
pa connect mydb 5432
```

The `-l` flag still works for backwards compatibility but will show a deprecation warning.

