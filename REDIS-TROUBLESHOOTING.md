# Redis Connection Troubleshooting

## Quick Fixes

### 1. **Admin UI JavaScript Error Fixed** ✅
The `Missing catch or finally after try` error has been fixed. Refresh your browser to load the updated JavaScript.

### 2. **CLI Connection Issues**

If you're getting "failed to connect" errors, follow these steps:

#### Step 1: Verify Docker Services are Running

```bash
docker-compose ps
```

You should see:
- ✅ `port-auth-redis-noauth` (port 6379) - healthy
- ✅ `port-auth-redis-auth` (port 6380) - healthy  
- ✅ `port-auth-api` (port 8080) - healthy

If not, start them:
```bash
docker-compose up -d redis-noauth redis-auth api
```

#### Step 2: Rebuild the Application

The Redis support was just added, so you need to rebuild:

```bash
# Rebuild the binary
make build

# Rebuild Docker images
docker-compose build api

# Restart the API
docker-compose restart api
```

#### Step 3: Verify Configuration

Check that Redis connections exist in `docker/config.yaml`:

```bash
grep -A 10 "redis-test" docker/config.yaml
```

You should see both `redis-test-noauth` and `redis-test-auth`.

#### Step 4: Test the API Directly

```bash
# Login
curl -X POST http://localhost:8080/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}' | jq

# Save the token
export TOKEN="your-token-here"

# List connections (should include Redis)
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/connections | jq

# Request a Redis connection
curl -X POST -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/connect/redis-test-noauth | jq
```

#### Step 5: Test with CLI

```bash
# Rebuild CLI if needed
make build

# Login
./bin/port-authorizing login

# List connections
./bin/port-authorizing list

# Connect to Redis without auth
./bin/port-authorizing connect redis-test-noauth -l 6379

# In another terminal, test with redis-cli
redis-cli -h localhost -p 6379
> PING
> SET test "hello"
> GET test
```

## Common Issues

### Issue: "Connection redis-test-noauth not found"

**Solution**: The configuration may not be loaded. Check:
1. `docker/config.yaml` has the Redis connections
2. API container is using the correct config file
3. Restart API: `docker-compose restart api`

### Issue: "Failed to connect to backend"

**Solution**: Redis container may not be running:
```bash
# Check Redis containers
docker ps | grep redis

# Start them if missing
docker-compose up -d redis-noauth redis-auth

# Check logs
docker-compose logs redis-noauth
docker-compose logs redis-auth
```

### Issue: "Connection refused on localhost:6379"

**Solution**: Port may be in use:
```bash
# Check what's using the port
lsof -i :6379

# Use a different local port
./bin/port-authorizing connect redis-test-noauth -l 16379
redis-cli -h localhost -p 16379
```

### Issue: "Command blocked by whitelist"

**Solution**: Your role doesn't have permission for that Redis command. Check policies in `docker/config.yaml`:

```yaml
policies:
  - name: dev-test
    roles:
      - developer
    tags:
      - env:test
    whitelist:
      - "^GET .*"   # Allowed
      - "^SET .*"   # Allowed
      - "^DEL .*"   # Allowed
      # FLUSHDB not in list = blocked!
```

### Issue: Admin UI shows old code (JavaScript error)

**Solution**: Hard refresh your browser:
- Chrome/Firefox: `Ctrl+Shift+R` (Windows/Linux) or `Cmd+Shift+R` (Mac)
- Or open Developer Tools and right-click the refresh button → "Empty Cache and Hard Reload"

## Testing Redis Directly

### Test Redis Without Auth (port 6379)
```bash
docker exec -it port-auth-redis-noauth redis-cli
> PING
> SET mykey "test value"
> GET mykey
> DEL mykey
```

### Test Redis With Auth (port 6380)
```bash
docker exec -it port-auth-redis-auth redis-cli -a redispass123
> PING
> SET authkey "secure value"
> GET authkey
> DEL authkey
```

## Full Stack Test

Run the automated test script:
```bash
./test_redis.sh
```

This will:
1. ✅ Check all services are running
2. ✅ Login to the API
3. ✅ Create Redis connections
4. ✅ Test Redis commands
5. ✅ Show audit logs

## Still Having Issues?

1. **Check API logs**:
   ```bash
   docker-compose logs -f api
   ```

2. **Check audit logs**:
   ```bash
   tail -f data/audit.log | grep redis
   ```

3. **Verify Redis is accessible from API container**:
   ```bash
   docker exec port-auth-api sh -c "nc -zv redis-noauth 6379"
   docker exec port-auth-api sh -c "nc -zv redis-auth 6379"
   ```

4. **Clean slate**:
   ```bash
   docker-compose down
   docker-compose up -d
   ```

## Success Indicators

When everything is working, you should see:

1. **Admin UI**: No JavaScript errors in console
2. **CLI list**: Shows `redis-test-noauth` and `redis-test-auth`
3. **Redis connection**: 
   ```
   ✓ Connection established: redis-test-noauth
   ✓ Proxy server listening on localhost:6379
   
   📝 Redis Connection Info:
     Connect using redis-cli:
     redis-cli -h localhost -p 6379
   ```
4. **Redis commands work**: `PING` returns `PONG`
5. **Audit logs**: Show Redis commands being logged

---

**Need more help?** Check the logs and error messages for specific issues.

