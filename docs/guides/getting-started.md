# Getting Started with Port Authorizing

This guide will help you get up and running with the port-authorizing proxy system.

## Quick Start

### 1. Build the Binaries

```bash
# Build API server
go build -o bin/port-authorizing-api ./cmd/api

# Build CLI client
go build -o bin/port-authorizing-cli ./cmd/cli
```

### 2. Configure the API Server

Copy the example configuration:

```bash
cp config.example.yaml config.yaml
```

Edit `config.yaml` and update:
- `auth.jwt_secret` - Use a strong secret key in production
- `auth.users` - Add your users (use hashed passwords in production)
- `connections` - Configure your target services

### 3. Start the API Server

```bash
./bin/port-authorizing-api --config config.yaml
```

You should see:
```
Starting API server on port 8080
```

### 4. Login with CLI

```bash
./bin/port-authorizing-cli login -u admin -p admin123
```

Output:
```
✓ Successfully logged in as admin
Token expires at: 2025-10-02T10:00:00Z
```

### 5. List Available Connections

```bash
./bin/port-authorizing-cli list
```

Output:
```
Available Connections:
----------------------
  • postgres-prod [postgres]
    description: Production PostgreSQL database
    environment: production
  • internal-api [http]
    description: Internal REST API
  • redis-cache [tcp]
    description: Redis cache server
```

### 6. Connect to a Service

```bash
# Connect to PostgreSQL on local port 5433
./bin/port-authorizing-cli connect postgres-prod --local-port 5433 --duration 1h
```

Output:
```
✓ Connection established: postgres-prod
  Connection ID: 550e8400-e29b-41d4-a716-446655440000
  Expires at: 2025-10-01T11:00:00Z
  Local port: 5433

Starting local proxy server...
✓ Proxy server listening on localhost:5433
Press Ctrl+C to stop
```

### 7. Use Your Favorite Client

Now you can connect using standard tools:

**PostgreSQL:**
```bash
psql -h localhost -p 5433 -U dbuser -d mydb
```

**HTTP API:**
```bash
curl http://localhost:8080
```

**Redis:**
```bash
redis-cli -h localhost -p 6379
```

## Example Workflow

### Setting Up a PostgreSQL Connection

1. **Add connection to config.yaml:**

```yaml
connections:
  - name: my-database
    type: postgres
    host: db.example.com
    port: 5432
    whitelist:
      - "^SELECT.*"
      - "^INSERT INTO logs.*"
    metadata:
      description: "My production database"
```

2. **Restart API server**

3. **Connect from CLI:**

```bash
./bin/port-authorizing-cli connect my-database -l 5433 -d 2h
```

4. **Use psql:**

```bash
psql -h localhost -p 5433 -U myuser -d mydb
```

All queries will be:
- ✅ Authenticated with your user token
- ✅ Validated against the whitelist
- ✅ Logged with your username in `audit.log`
- ✅ Automatically disconnected after 2 hours

## Configuration Examples

### HTTP Proxy with HTTPS

```yaml
connections:
  - name: secure-api
    type: http
    host: api.example.com
    port: 443
    scheme: https
    metadata:
      description: "Secure API endpoint"
```

### TCP Proxy (Redis, MongoDB, etc.)

```yaml
connections:
  - name: mongodb
    type: tcp
    host: mongo.example.com
    port: 27017
    metadata:
      description: "MongoDB cluster"
```

### PostgreSQL with Strict Whitelist

```yaml
connections:
  - name: read-only-db
    type: postgres
    host: readonly.db.com
    port: 5432
    whitelist:
      - "^SELECT.*FROM users WHERE id = \\d+$"
      - "^SELECT.*FROM orders WHERE.*"
    metadata:
      description: "Read-only database access"
```

## Security Best Practices

1. **Use Strong JWT Secrets**
   ```yaml
   auth:
     jwt_secret: "use-a-long-random-string-here-not-this"
   ```

2. **Hash Passwords** - Don't store plain text passwords in production. Use bcrypt or similar.

3. **Configure Whitelists** - Always use whitelist patterns for production databases:
   ```yaml
   whitelist:
     - "^SELECT.*"  # Only allow SELECT queries
   ```

4. **Set Appropriate Timeouts**
   ```yaml
   server:
     max_connection_duration: 1h  # Auto-disconnect after 1 hour
   ```

5. **Monitor Audit Logs**
   ```bash
   tail -f audit.log
   ```

## Troubleshooting

### "not logged in" Error

Run login command:
```bash
./bin/port-authorizing-cli login -u username -p password
```

### "Connection not found" Error

Check if the connection exists:
```bash
./bin/port-authorizing-cli list
```

### API Server Not Starting

Check if port 8080 is already in use:
```bash
lsof -i :8080
```

Use a different port:
```yaml
server:
  port: 9090
```

### Whitelist Blocking Queries

Check the audit log to see what query was blocked:
```bash
cat audit.log | jq '.action == "proxy_request"'
```

Update whitelist pattern in `config.yaml`.

## Advanced Usage

### Custom API URL

```bash
./bin/port-authorizing-cli --api-url https://my-proxy.example.com login -u user -p pass
```

### Different Config Path

```bash
./bin/port-authorizing-api --config /etc/port-auth/config.yaml
```

### Environment-Specific Configs

```bash
# Development
./bin/port-authorizing-api --config config.dev.yaml

# Production
./bin/port-authorizing-api --config config.prod.yaml
```

## Next Steps

- [ ] Set up proper user authentication with hashed passwords
- [ ] Configure SSL/TLS for the API server
- [ ] Set up monitoring and alerting
- [ ] Enable LLM risk analysis (coming soon)
- [ ] Add rate limiting
- [ ] Deploy to production

## Support

For issues or questions:
- Check the [README.md](README.md) for architecture details
- Review audit logs for debugging
- Check [GitHub Issues](https://github.com/cohandv/port-authorizing/issues)

## Example: Complete Setup

Here's a complete example from scratch:

```bash
# 1. Clone and build
git clone https://github.com/cohandv/port-authorizing
cd port-authorizing
go build -o bin/port-authorizing-api ./cmd/api
go build -o bin/port-authorizing-cli ./cmd/cli

# 2. Configure
cp config.example.yaml config.yaml
# Edit config.yaml with your settings

# 3. Start API
./bin/port-authorizing-api &

# 4. Login
./bin/port-authorizing-cli login -u admin -p admin123

# 5. List connections
./bin/port-authorizing-cli list

# 6. Connect
./bin/port-authorizing-cli connect postgres-prod -l 5433 -d 1h

# 7. Use your client
psql -h localhost -p 5433 -U myuser -d mydb
```

That's it! You now have a secure, audited proxy to your protected services. 🎉


