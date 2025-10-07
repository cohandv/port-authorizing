# Resilient Authentication Provider Initialization

## Overview

The Port Authorizing server is designed to be resilient to authentication provider failures during startup. If a configured authentication provider (OIDC, LDAP, SAML2) is temporarily unavailable or misconfigured, the server will still start successfully, logging warnings instead of failing completely.

## Behavior

### What Happens When a Provider Fails?

When an authentication provider fails to initialize (e.g., OIDC well-known endpoint unreachable, LDAP server down):

1. **⚠️ Warning Logged**: The server logs a clear warning message indicating which provider failed and why
2. **🚀 Server Continues**: The server continues starting up with the remaining working providers
3. **✅ Other Providers Work**: Any successfully initialized providers remain fully functional
4. **🔄 Restart Required**: The failed provider will be unavailable until the server is restarted

### Example Startup Output

```
✅ Initialized local provider: local-users
⚠️  Warning: failed to initialize oidc provider 'keycloak': failed to create OIDC provider: Get "http://keycloak:8180/realms/myrealm/.well-known/openid-configuration": dial tcp: lookup keycloak: no such host - skipping
   The server will start without this provider. It will be unavailable until the server is restarted.
✅ Initialized ldap provider: corporate-ldap
```

## Common Scenarios

### 1. OIDC Provider Temporarily Down

**Scenario**: Keycloak/Auth0/Okta server is temporarily unavailable during startup

**Result**:
- Server starts successfully
- Local users and other providers continue to work
- OIDC authentication will fail with "provider not found" errors
- Once OIDC server is back online, restart the Port Authorizing server

### 2. OIDC Well-Known Endpoint Unreachable

**Scenario**: Network issues prevent fetching `.well-known/openid-configuration`

**Error Message**:
```
⚠️  Warning: failed to initialize oidc provider 'myoidc': failed to create OIDC provider: 
Get "https://auth.example.com/.well-known/openid-configuration": dial tcp x.x.x.x:443: i/o timeout - skipping
```

**Result**:
- Server starts successfully
- OIDC authentication unavailable until connectivity restored and server restarted

### 3. LDAP Server Down

**Scenario**: LDAP server is unreachable

**Result**:
- Server starts successfully
- LDAP authentication fails with connection errors
- Other providers (OIDC, local) continue working

### 4. Misconfigured Provider

**Scenario**: Provider configuration has invalid URLs or credentials

**Result**:
- Server starts successfully
- Provider is skipped with detailed error message
- Fix configuration and restart server

### 5. No Providers Available

**Scenario**: All configured providers fail to initialize

**Warning Message**:
```
⚠️  Warning: no authentication providers successfully initialized!
   This means authentication will NOT work until you:
   1. Fix provider configurations (check OIDC/LDAP/SAML2 connectivity)
   2. Or add local users to config.yaml
   3. Then restart the server

   Server will continue to start, but API authentication endpoints will fail.
```

**Result**:
- Server starts successfully (still serves health endpoints, proxies for existing connections)
- **All authentication will fail** until providers are fixed
- No new connections can be authenticated
- Existing proxy connections continue to work

## Benefits

### 1. **High Availability**
The server can start even if external authentication services are temporarily down.

### 2. **Graceful Degradation**
With multiple providers configured, one provider's failure doesn't affect others.

### 3. **Clear Diagnostics**
Detailed warning messages help operators quickly identify and fix configuration issues.

### 4. **Production Stability**
The server doesn't crash due to transient network issues or external service outages.

## Best Practices

### 1. Configure Multiple Providers

For high availability, configure at least two authentication providers:

```yaml
auth:
  # Local fallback
  users:
    - username: admin
      password: "$2a$10$..."
      roles: ["admin"]
  
  # Primary provider (OIDC)
  providers:
    - name: keycloak
      type: oidc
      enabled: true
      config:
        issuer: "https://auth.example.com/realms/myrealm"
        # ... other config
    
    # Backup provider (LDAP)
    - name: corporate-ldap
      type: ldap
      enabled: true
      config:
        url: "ldaps://ldap.example.com:636"
        # ... other config
```

### 2. Monitor Startup Logs

Always check startup logs for warnings about failed providers:

```bash
docker-compose logs port-auth-api | grep "Warning"
```

### 3. Set Up Health Checks

Monitor provider health after startup:

```bash
# Check which providers are available
curl http://localhost:8080/api/providers
```

### 4. Test Provider Connectivity

Before deploying, test that all providers are reachable:

```bash
# Test OIDC well-known endpoint
curl https://auth.example.com/realms/myrealm/.well-known/openid-configuration

# Test LDAP connectivity
ldapsearch -H ldaps://ldap.example.com:636 -x
```

### 5. Plan for Restarts

Remember that failed providers **require a server restart** to retry initialization:

```bash
# After fixing provider issues
docker-compose restart port-auth-api

# Or in Kubernetes
kubectl rollout restart deployment/port-auth-api
```

## Recovery Steps

### When a Provider Fails to Initialize

1. **Check the Error Message**
   ```
   ⚠️  Warning: failed to initialize oidc provider 'keycloak': 
   failed to create OIDC provider: Get "http://keycloak:8180...": 
   dial tcp: lookup keycloak: no such host - skipping
   ```

2. **Identify the Root Cause**
   - Network connectivity issue?
   - DNS resolution problem?
   - Service down?
   - Configuration error?

3. **Fix the Issue**
   - Verify service is running
   - Check network connectivity
   - Validate configuration (URLs, credentials)
   - Test with curl/ldapsearch

4. **Restart the Server**
   ```bash
   docker-compose restart port-auth-api
   ```

5. **Verify Success**
   ```
   ✅ Initialized oidc provider: keycloak
   ```

## Troubleshooting

### "No authentication providers successfully initialized"

**Causes**:
- All configured providers are down/misconfigured
- No providers configured at all
- Network connectivity issues preventing initialization

**Solutions**:
1. Add local users as fallback
2. Check network connectivity to external services
3. Verify provider configurations
4. Check firewall rules
5. Review DNS resolution

### Provider Authentication Fails After Startup

Even if a provider initializes successfully, it may still fail during authentication if:
- Credentials are invalid
- Service becomes unavailable after startup
- Network issues develop

**Solution**: Configure multiple providers for redundancy.

## Implementation Details

### Initialization Flow

1. Load configuration from `config.yaml`
2. Initialize local users (if configured)
3. For each enabled provider:
   - Attempt initialization
   - On success: Add to active providers list
   - On failure: Log warning, continue with next provider
4. If no providers initialized: Log comprehensive warning
5. Start server (even with 0 providers)

### Code Reference

See `internal/auth/provider.go` - `NewManager()` function for implementation details.

## See Also

- [Authentication Guide](../AUTHENTICATION_GUIDE.md)
- [OIDC Setup](../OIDC_SETUP.md)
- [Configuration Guide](../CONFIGURATION_GUIDE.md)

