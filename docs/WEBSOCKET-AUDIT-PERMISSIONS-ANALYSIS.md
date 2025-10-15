# WebSocket Audit & Permissions Flow Analysis

## Summary

**GOOD NEWS**: The audit and permissions workflow is **PRESERVED** in the WebSocket implementation! Both whitelist checking and approval workflow are fully functional for both PostgreSQL and HTTP connections over WebSocket.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Client Connection                            │
├─────────────────────────────────────────────────────────────────────┤
│                                                                       │
│  ┌──────────────┐                              ┌──────────────┐     │
│  │  TCP Stream  │                              │  WebSocket   │     │
│  │  (hijacked)  │                              │  (upgraded)  │     │
│  └──────┬───────┘                              └──────┬───────┘     │
│         │                                             │             │
│         v                                             v             │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │          Protocol-Aware Proxy Layer                          │   │
│  │  (PostgresAuthProxy / HTTPProxy)                            │   │
│  │                                                               │   │
│  │  ✓ Extracts queries/requests                                 │   │
│  │  ✓ Checks whitelist (regex patterns)                         │   │
│  │  ✓ Requests approval (if pattern matches)                    │   │
│  │  ✓ Logs audit events                                         │   │
│  │  ✓ Forwards to backend (if allowed)                          │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                       │
└─────────────────────────────────────────────────────────────────────┘
```

## PostgreSQL Flow Comparison

### Non-WebSocket Flow (`handlePostgresProxy`)

**File**: `internal/api/proxy_postgres.go`

```
1. Client connects via TCP stream (HTTP hijacked)
2. Get whitelist from authorization system
3. Create PostgresAuthProxy with whitelist + approval manager
4. Call pgProxy.HandleConnection(clientConn)
   └─> PostgresAuthProxy.forwardWithLogging()
       └─> PostgresAuthProxy.validateAndLogQuery()
           ├─> Check whitelist (regex match)
           ├─> Check approval (if pattern matches)
           ├─> Log audit events
           └─> Forward to backend OR block
```

**Code**: Lines 39-88 in `proxy_postgres.go`
```go
// Get whitelist for this user's roles and connection
whitelist := s.authz.GetWhitelistForConnection(roles, conn.Config.Name)

// Create Postgres proxy with credential substitution and whitelist
pgProxy := proxy.NewPostgresAuthProxy(
    conn.Config,
    s.config.Logging.AuditLogPath,
    username,
    connectionID,
    s.config,
    whitelist,
)

// Set approval manager if enabled
if s.approvalMgr != nil {
    pgProxy.SetApprovalManager(s.approvalMgr)
}

// Handle the Postgres protocol connection
if err := pgProxy.HandleConnection(clientConn); err != nil {
```

### WebSocket Flow (`handlePostgresWebSocket`)

**File**: `internal/api/proxy_stream.go`

```
1. Client connects via WebSocket (HTTP upgraded)
2. Get whitelist from authorization system
3. Create PostgresAuthProxy with whitelist + approval manager
4. Wrap WebSocket in net.Conn adapter (websocketConn)
5. Call pgProxy.HandleConnection(wsNetConn)
   └─> PostgresAuthProxy.forwardWithLogging()
       └─> PostgresAuthProxy.validateAndLogQuery()
           ├─> Check whitelist (regex match)
           ├─> Check approval (if pattern matches)
           ├─> Log audit events
           └─> Forward to backend OR block
```

**Code**: Lines 240-282 in `proxy_stream.go`
```go
// Get whitelist for this user's roles
whitelist := s.authz.GetWhitelistForConnection(roles, conn.Config.Name)

// Create Postgres proxy with protocol-aware query logging and security
pgProxy := proxy.NewPostgresAuthProxy(
    conn.Config,
    s.config.Logging.AuditLogPath,
    username,
    connectionID,
    s.config,
    whitelist,
)

// Set approval manager if enabled
if s.approvalMgr != nil {
    pgProxy.SetApprovalManager(s.approvalMgr)
}

// Create a virtual connection that wraps WebSocket
wsNetConn := &websocketConn{
    ws:   wsConn,
    done: make(chan struct{}),
}

// Handle the Postgres protocol connection through WebSocket
if err := pgProxy.HandleConnection(wsNetConn); err != nil {
```

**Key Insight**: Both flows call the **SAME** `PostgresAuthProxy.HandleConnection()` method, which contains all the whitelist and approval logic!

## HTTP Flow Comparison

### Non-WebSocket Flow (`handleHTTPProxyStream`)

**File**: `internal/api/proxy_http_stream.go`

```
1. Client connects via TCP stream (HTTP hijacked)
2. Get HTTPProxy from conn.Proxy (already has whitelist configured)
3. Loop: read HTTP requests from stream
4. For each request:
   └─> Call httpProxy.HandleRequest()
       ├─> Parse HTTP method and path
       ├─> Check whitelist (regex match)
       ├─> Check approval (if pattern matches)
       ├─> Log audit events
       └─> Forward to backend OR block
```

**Code**: Lines 77-135 in `proxy_http_stream.go`
```go
// Use the HTTP proxy instance from connection (which has approval support)
httpProxy := conn.Proxy

// Process HTTP requests in a loop
for {
    // Read HTTP request from client
    requestBytes, err := readHTTPRequest(reader)

    // Parse HTTP request to get method and path for logging
    httpReq, err := http.ReadRequest(reqReader)

    // Call the HTTP proxy's HandleRequest
    // This will check whitelist, approval, and forward to backend!
    err = httpProxy.HandleRequest(respWriter, proxyReq)
```

### WebSocket Flow (`handleHTTPWebSocket`)

**File**: `internal/api/proxy_stream.go`

```
1. Client connects via WebSocket (HTTP upgraded)
2. Get HTTPProxy from conn.Proxy (already has whitelist configured)
3. Wrap WebSocket in net.Conn adapter (websocketConn)
4. Call handleHTTPOverWebSocket()
   └─> Loop: read HTTP requests from WebSocket stream
       └─> For each request:
           └─> Call httpProxy.HandleRequest()
               ├─> Parse HTTP method and path
               ├─> Check whitelist (regex match)
               ├─> Check approval (if pattern matches)
               ├─> Log audit events
               └─> Forward to backend OR block
```

**Code**: Lines 347-368 in `proxy_stream.go`
```go
// Create HTTP proxy with whitelist and approval support
httpProxy := conn.Proxy

// Create a virtual connection that wraps WebSocket
wsNetConn := &websocketConn{
    ws:     wsConn,
    done:   make(chan struct{}),
    buffer: nil,
}

// Process HTTP requests from WebSocket stream
// Similar to handleHTTPProxyStream but over WebSocket
if err := s.handleHTTPOverWebSocket(wsNetConn, httpProxy, username, conn, connectionID); err != nil {
```

**Key Insight**: Both flows call the **SAME** `HTTPProxy.HandleRequest()` method, which contains all the whitelist and approval logic!

## Whitelist Implementation

### PostgreSQL Whitelist

**File**: `internal/proxy/postgres_auth.go`

**Location**: Lines 639-771

**How it works**:
1. Extracts SQL queries from PostgreSQL protocol messages (message type 'Q')
2. For each query, calls `isQueryAllowed(query)` which:
   - Checks if query matches any whitelist regex pattern (case-insensitive)
   - Returns `true` if matched, `false` if not matched
3. If blocked:
   - Logs audit event: `postgres_query_blocked`
   - Sends PostgreSQL error response to client
   - Does NOT forward to backend

**Code snippet**:
```go
// Check whitelist first
allowed := p.isQueryAllowed(query)

// Log the query with whitelist result
audit.Log(p.auditLogPath, p.username, "postgres_query", p.config.Name, map[string]interface{}{
    "connection_id": p.connectionID,
    "query":         query,
    "database":      p.config.BackendDatabase,
    "allowed":       allowed,
    "whitelist":     len(p.whitelist) > 0,
})

if !allowed {
    // Log blocked query
    audit.Log(p.auditLogPath, p.username, "postgres_query_blocked", ...)
    return true, query  // BLOCKED!
}
```

### HTTP Whitelist

**File**: `internal/proxy/http.go`

**Location**: Lines 104-137

**How it works**:
1. Constructs request pattern: `"{METHOD} {PATH}"` (e.g., "GET /api/users")
2. Calls `isRequestAllowed(requestPattern)` which:
   - Checks if pattern matches any whitelist regex (case-insensitive)
   - Returns `true` if matched, `false` if not matched
3. If blocked:
   - Logs audit event: `http_request_blocked`
   - Sends HTTP 403 Forbidden response
   - Does NOT forward to backend

**Code snippet**:
```go
// Validate request against whitelist if configured
if len(p.whitelist) > 0 {
    requestPattern := fmt.Sprintf("%s %s", method, path)
    if !p.isRequestAllowed(requestPattern) {
        // Log blocked request
        audit.Log(p.auditLogPath, p.username, "http_request_blocked", ...)

        // Return 403 Forbidden
        w.WriteHeader(http.StatusForbidden)
        w.Write([]byte(`{"error":"Request blocked by security policy"}`))
        return fmt.Errorf("request blocked by whitelist: %s %s", method, path)
    }
}
```

## Approval Workflow Implementation

### PostgreSQL Approval

**File**: `internal/proxy/postgres_auth.go`

**Location**: Lines 674-734

**How it works**:
1. After whitelist check passes, checks if query matches approval pattern
2. If approval required:
   - Logs audit event: `postgres_approval_requested`
   - Calls `approvalMgr.RequestApproval()` with timeout
   - Waits for approval decision (approved/rejected/timeout)
3. If NOT approved:
   - Logs audit event: `postgres_approval_rejected`
   - Blocks the query (same as whitelist block)
4. If approved:
   - Logs audit event: `postgres_approval_granted`
   - Forwards to backend

**Code snippet**:
```go
// Check if approval is required for this query
if p.approvalMgr != nil {
    requiresApproval, timeout := p.approvalMgr.RequiresApproval(normalizedQuery, "", p.config.Tags)
    if requiresApproval {
        // Request approval
        approvalReq := &approval.Request{
            Username:     p.username,
            ConnectionID: p.connectionID,
            Method:       normalizedQuery,
            // ...
        }

        // Wait for approval with timeout
        ctx, cancel := context.WithTimeout(context.Background(), timeout)
        defer cancel()

        approvalResp, err := p.approvalMgr.RequestApproval(ctx, approvalReq, timeout)

        // Check approval decision
        if approvalResp.Decision != approval.DecisionApproved {
            audit.Log(p.auditLogPath, p.username, "postgres_approval_rejected", ...)
            return true, query  // BLOCKED!
        }

        audit.Log(p.auditLogPath, p.username, "postgres_approval_granted", ...)
    }
}
```

### HTTP Approval

**File**: `internal/proxy/http.go`

**Location**: Lines 140-214

**How it works**:
1. After whitelist check passes, checks if request matches approval pattern
2. If approval required:
   - Logs audit event: `http_approval_requested`
   - Calls `approvalMgr.RequestApproval()` with timeout
   - Waits for approval decision (approved/rejected/timeout)
3. If NOT approved:
   - Logs audit event: `http_approval_rejected`
   - Sends HTTP 403 Forbidden response
   - Does NOT forward to backend
4. If approved:
   - Logs audit event: `http_approval_granted`
   - Forwards to backend

**Code snippet**:
```go
// Check if approval is required for this request
if p.approvalMgr != nil {
    requiresApproval, timeout := p.approvalMgr.RequiresApproval(method, path, p.config.Tags)
    if requiresApproval {
        // Request approval
        approvalReq := &approval.Request{
            Username:     p.username,
            ConnectionID: p.connectionID,
            Method:       method,
            Path:         path,
            // ...
        }

        // Wait for approval with timeout
        ctx, cancel := context.WithTimeout(r.Context(), timeout)
        defer cancel()

        approvalResp, err := p.approvalMgr.RequestApproval(ctx, approvalReq, timeout)

        // Check approval decision
        if approvalResp.Decision != approval.DecisionApproved {
            audit.Log(p.auditLogPath, p.username, "http_approval_rejected", ...)

            w.WriteHeader(http.StatusForbidden)
            w.Write([]byte(fmt.Sprintf(`{"error":"Request not approved"}`)))
            return fmt.Errorf("request not approved: %s", approvalResp.Decision)
        }

        audit.Log(p.auditLogPath, p.username, "http_approval_granted", ...)
    }
}
```

## Audit Events Logged

### PostgreSQL Audit Events

**Connection Events:**
- `postgres_connect` / `postgres_connect_websocket` - When connection starts
- `postgres_disconnect` / `postgres_disconnect_websocket` - When connection ends
- `postgres_auth` - When backend authentication succeeds
- `postgres_error` - When errors occur

**Query Events:**
- `postgres_query` - Every query (with `allowed: true/false` field)
- `postgres_query_blocked` - When query is blocked by whitelist

**Approval Events:**
- `postgres_approval_requested` - When approval is requested
- `postgres_approval_granted` - When approval is granted
- `postgres_approval_rejected` - When approval is rejected/timeout

### HTTP Audit Events

**Connection Events:**
- `http_connect` / `http_connect_websocket` - When connection starts
- `http_disconnect` / `http_disconnect_websocket` - When connection ends
- `http_preflight` - OPTIONS preflight requests

**Request Events:**
- `http_request` - Allowed requests (with `allowed: true` field)
- `http_request_blocked` - When request is blocked by whitelist

**Approval Events:**
- `http_approval_requested` - When approval is requested
- `http_approval_granted` - When approval is granted
- `http_approval_rejected` - When approval is rejected/timeout

## Configuration Example

Based on your `config.yaml`:

### Whitelist Patterns

```yaml
policies:
  - name: dev-test
    roles:
      - developer
    tags:
      - env:test
    whitelist:
      # PostgreSQL patterns
      - "^SELECT.*"
      - "^EXPLAIN.*"
      # HTTP patterns
      - "^GET /.*"
      - "^POST /api/.*"
      - "^PUT /api/.*"
```

### Approval Patterns

```yaml
approval:
  enabled: true
  patterns:
    # HTTP DELETE requests
    - pattern: "^DELETE /.*"
      tags: ["env:test"]
      timeout_seconds: 10

    # PostgreSQL dangerous queries
    - pattern: "^DELETE FROM.*"
      tags: ["env:test"]
      timeout_seconds: 10
    - pattern: "^DROP TABLE.*"
      tags: ["env:test"]
      timeout_seconds: 10
    - pattern: "^UPDATE.*"
      tags: ["env:test"]
      timeout_seconds: 10
```

## Flow Diagram: Example PostgreSQL Query

```
┌──────────────────────────────────────────────────────────────────────┐
│ Client: psql -h localhost -p 8080 -U developer                       │
└────────────────────────────┬─────────────────────────────────────────┘
                             │
                             v
                    ┌────────────────┐
                    │  WebSocket or  │
                    │   TCP Stream   │
                    └────────┬───────┘
                             │
                             v
        ┌────────────────────────────────────────┐
        │  PostgresAuthProxy.HandleConnection()   │
        └────────────────────┬───────────────────┘
                             │
                             v
                  Client sends: "SELECT * FROM users"
                             │
                             v
        ┌────────────────────────────────────────┐
        │  PostgresAuthProxy.validateAndLogQuery │
        └────────────────────┬───────────────────┘
                             │
                             v
                    ┌────────────────┐
                    │ Check Whitelist│
                    │ Pattern: "^SELECT.*" │
                    │ ✓ MATCH        │
                    └────────┬───────┘
                             │
                             v
                    ┌────────────────┐
                    │ Check Approval │
                    │ Pattern: None  │
                    │ ✓ No approval  │
                    │   required     │
                    └────────┬───────┘
                             │
                             v
                    ┌────────────────┐
                    │ Log Audit Event│
                    │ "postgres_query"│
                    │ allowed=true   │
                    └────────┬───────┘
                             │
                             v
                    ┌────────────────┐
                    │ Forward to     │
                    │ Backend DB     │
                    └────────┬───────┘
                             │
                             v
                    ┌────────────────┐
                    │ Return Results │
                    └────────────────┘

────────────────────────────────────────────────────────────────────────

Now client sends: "DELETE FROM users WHERE id=1"
                             │
                             v
        ┌────────────────────────────────────────┐
        │  PostgresAuthProxy.validateAndLogQuery │
        └────────────────────┬───────────────────┘
                             │
                             v
                    ┌────────────────┐
                    │ Check Whitelist│
                    │ Pattern: None  │
                    │ ✗ NO MATCH     │
                    └────────┬───────┘
                             │
                             v
                    ┌────────────────┐
                    │ Log Audit Event│
                    │ "postgres_query_blocked" │
                    └────────┬───────┘
                             │
                             v
                    ┌────────────────┐
                    │ Send ERROR to  │
                    │ Client         │
                    │ "Query blocked │
                    │ by whitelist"  │
                    └────────────────┘
```

## Conclusion

✅ **Audit logging is FULLY functional** in WebSocket implementation
✅ **Whitelist checking is FULLY functional** in WebSocket implementation
✅ **Approval workflow is FULLY functional** in WebSocket implementation

**Why it works**: Both WebSocket and non-WebSocket implementations funnel through the **SAME** protocol-aware proxy classes (`PostgresAuthProxy` and `HTTPProxy`), which contain all the security logic. The WebSocket adapter (`websocketConn`) simply wraps the WebSocket in a `net.Conn` interface, making it transparent to the proxy layer.

**The only difference** between WebSocket and non-WebSocket is the **transport layer**:
- Non-WebSocket: Raw TCP stream (HTTP hijacked)
- WebSocket: WebSocket frames wrapped in net.Conn adapter

Everything else (authentication, authorization, whitelist, approval, audit) is **identical**.

