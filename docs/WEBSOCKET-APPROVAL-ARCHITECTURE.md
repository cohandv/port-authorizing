# WebSocket + Approval Workflow Architecture

## Problem Statement

When implementing WebSocket reverse tunneling (for ALB compatibility), the approval workflow broke because:

1. **Generic WebSocket tunnel** forwards raw bytes without parsing HTTP/PostgreSQL protocols
2. **Approval logic** needs to inspect HTTP methods/paths or SQL queries to decide if approval is needed
3. **Raw byte forwarding** = no visibility into what's being requested

---

## Solution: Protocol-Aware WebSocket Handlers

### Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                     CLI (Local)                             │
│  • Opens local port (e.g., localhost:8081)                  │
│  • Establishes WebSocket connection to API                  │
│  • Sends HTTP/PostgreSQL traffic as binary WebSocket msgs  │
└────────────┬────────────────────────────────────────────────┘
             │ WebSocket (ws:// or wss://)
             │ Binary frames containing HTTP or PostgreSQL data
             ↓
┌─────────────────────────────────────────────────────────────┐
│                  API Server (EKS/ALB)                       │
│                                                              │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ handleProxyStream (Router)                          │   │
│  │  • Detects: Upgrade: websocket header               │   │
│  │  • Routes based on connection type:                 │   │
│  │    - postgres → handlePostgresWebSocket()           │   │
│  │    - http/https → handleHTTPWebSocket()             │   │
│  │    - tcp → generic WebSocket tunnel                 │   │
│  └──────────┬──────────────────────────────────────────┘   │
│             │                                                │
│  ┌──────────▼────────────────────────┐  ┌──────────────────▼─┐
│  │ handleHTTPWebSocket               │  │ handlePostgresWeb  │
│  │                                   │  │ Socket             │
│  │ 1. Upgrade → WebSocket            │  │                    │
│  │ 2. Wrap as websocketConn          │  │ 1. Upgrade → WS    │
│  │ 3. Parse HTTP from WebSocket      │  │ 2. Wrap as ws Conn │
│  │ 4. Check approval patterns        │  │ 3. Parse Postgres  │
│  │ 5. Forward to backend             │  │ 4. Check approvals │
│  │ 6. Return via WebSocket           │  │ 5. Forward to DB   │
│  └───────────────────────────────────┘  └────────────────────┘
└─────────────────────────────────────────────────────────────┘
             │
             ↓
┌─────────────────────────────────────────────────────────────┐
│                    Backend Services                         │
│  • Nginx HTTP server                                        │
│  • PostgreSQL database                                      │
│  • Any TCP service                                          │
└─────────────────────────────────────────────────────────────┘
```

---

## Component Breakdown

### 1. `websocketConn` - The Magic Adapter

**Purpose**: Converts WebSocket (message-based) to `net.Conn` (stream-based)

```go
type websocketConn struct {
    ws     *websocket.Conn
    done   chan struct{}
    buffer []byte  // Buffers partial WebSocket messages
}

// Implements net.Conn interface
func (c *websocketConn) Read(b []byte) (n int, err error) {
    // 1. Check buffer for leftover data from previous message
    if len(c.buffer) > 0 {
        n = copy(b, c.buffer)
        c.buffer = c.buffer[n:]
        return n, nil
    }

    // 2. Read next WebSocket message (binary frame)
    messageType, data, err := c.ws.ReadMessage()

    // 3. Copy what fits, buffer the rest
    n = copy(b, data)
    if n < len(data) {
        c.buffer = data[n:]  // Save for next Read() call
    }

    return n, nil
}

func (c *websocketConn) Write(b []byte) (n int, err error) {
    // Send entire buffer as single WebSocket binary message
    err = c.ws.WriteMessage(websocket.BinaryMessage, b)
    return len(b), err
}
```

**Why it's needed**:
- PostgreSQL protocol parser expects to `Read()` small chunks (e.g., 8 bytes for message header)
- WebSocket delivers entire messages (could be 32KB)
- Adapter buffers excess data for next `Read()` call

---

### 2. HTTP Approval Flow (handleHTTPWebSocket)

**Step-by-Step**:

```go
func (s *Server) handleHTTPWebSocket(w http.ResponseWriter, r *http.Request) {
    // 1. Upgrade HTTP → WebSocket
    wsConn, _ := upgrader.Upgrade(w, r, nil)

    // 2. Setup ping/pong keepalive (every 30s, prevents ALB timeout)
    wsConn.SetPongHandler(...)

    // 3. Wrap WebSocket as net.Conn
    wsNetConn := &websocketConn{ws: wsConn}

    // 4. Process HTTP requests in a loop
    handleHTTPOverWebSocket(wsNetConn, httpProxy, ...)
}

func handleHTTPOverWebSocket(wsNetConn, httpProxy, ...) {
    reader := bufio.NewReader(wsNetConn)
    writer := bufio.NewWriter(wsNetConn)

    for {
        // A. Read HTTP request from WebSocket (as stream)
        requestBytes := readHTTPRequestFromStream(reader)

        // B. Parse HTTP request
        httpReq, _ := http.ReadRequest(bytes.NewReader(requestBytes))
        // Now we have: httpReq.Method, httpReq.URL.Path

        // C. Create synthetic request for proxy
        proxyReq := httptest.NewRequest("POST", "/", bytes.NewReader(requestBytes))

        // D. Call HTTPProxy.HandleRequest()
        //    This function checks:
        //    - Whitelist patterns (e.g., allow GET, block DELETE)
        //    - Approval patterns (e.g., DELETE needs approval)
        //    - Forwards to backend if allowed
        err := httpProxy.HandleRequest(respWriter, proxyReq)

        // E. Response is written back to WebSocket automatically
    }
}
```

**Approval Check Inside HTTPProxy.HandleRequest()**:

```go
// In internal/proxy/http.go
func (p *HTTPProxy) HandleRequest(w http.ResponseWriter, r *http.Request) error {
    // Parse HTTP request
    method, path := parseHTTPRequest(...)

    // Check if approval required
    if p.approvalMgr != nil {
        normalizedRequest := fmt.Sprintf("%s %s", method, path)

        if p.approvalMgr.RequiresApproval(normalizedRequest, tags) {
            // Request approval
            approved := p.approvalMgr.RequestApproval(...)

            if !approved {
                // Blocked! Send 403 response
                w.WriteHeader(http.StatusForbidden)
                return fmt.Errorf("request rejected by approval workflow")
            }
        }
    }

    // Forward to backend
    forwardToBackend(...)
}
```

---

### 3. PostgreSQL Approval Flow (handlePostgresWebSocket)

**Step-by-Step**:

```go
func (s *Server) handlePostgresWebSocket(w http.ResponseWriter, r *http.Request) {
    // 1. Upgrade HTTP → WebSocket
    wsConn, _ := upgrader.Upgrade(w, r, nil)

    // 2. Wrap WebSocket as net.Conn
    wsNetConn := &websocketConn{ws: wsConn}

    // 3. Create PostgreSQL proxy (protocol-aware)
    pgProxy := proxy.NewPostgresAuthProxy(config, ...)
    pgProxy.SetApprovalManager(approvalMgr)

    // 4. Handle PostgreSQL protocol over WebSocket
    //    The PostgreSQL proxy reads from wsNetConn as if it's TCP
    pgProxy.HandleConnection(wsNetConn)
}

// In internal/proxy/postgres_auth.go
func (p *PostgresAuthProxy) HandleConnection(conn net.Conn) {
    for {
        // Read PostgreSQL protocol message
        msgType, data := readPostgresMessage(conn)

        if msgType == 'Q' {  // Query message
            query := string(data)

            // Check whitelist
            if !isQueryAllowed(query) {
                sendError(conn, "Query blocked by whitelist")
                continue
            }

            // Check if approval required
            if p.approvalMgr != nil {
                normalizedQuery := strings.TrimSpace(query)

                if p.approvalMgr.RequiresApproval(normalizedQuery, tags) {
                    // Request approval (blocks until approved/rejected)
                    approved := p.approvalMgr.RequestApproval(...)

                    if !approved {
                        sendError(conn, "Query blocked by approval workflow")
                        continue
                    }
                }
            }

            // Forward query to backend
            forwardToBackend(query)
        }
    }
}
```

---

## Key Differences: HTTP vs PostgreSQL

| Aspect | HTTP | PostgreSQL |
|--------|------|------------|
| **Protocol Parsing** | Parse HTTP request line/headers | Parse PostgreSQL wire protocol |
| **Approval Target** | Method + Path (e.g., "DELETE /api/users") | SQL Query (e.g., "DELETE FROM users") |
| **Blocking** | Wait for approval before forwarding request | Wait for approval before executing query |
| **Error Response** | HTTP 403 Forbidden | PostgreSQL error message |
| **Existing Logic** | Reuses `HTTPProxy.HandleRequest()` | Reuses `PostgresAuthProxy.HandleConnection()` |

---

## Approval Workflow Timeline

### Example: HTTP DELETE Request

```
Time   CLI                  API Server                Mock Approval Server
────   ───                  ──────────                ────────────────────
0ms    Connect nginx-server
       ↓
1ms    → WebSocket upgrade  ✓ Upgrade accepted
       ← 101 Switching
2ms    ✓ Connected


100ms  DELETE /api/david
       ↓
101ms  → WebSocket frame    Parse HTTP request
         (binary)            Method: DELETE
                            Path: /api/david

                            Check approval pattern:
                            ^DELETE.* matches!

                            ↓ POST /webhook
                            {
                              "method": "DELETE",
                              "path": "/api/david",
                              "user": "admin",
                              "tags": ["env:dev"]
                            }
                                                      ← Receive request

                                                      Auto-approve (0ms delay)

                                                      → Approve callback
102ms                        ← GET /api/approvals/.../approve

                            ✓ Approved!

                            Forward to backend:
                            → DELETE /api/david
                                                     (Nginx backend)
                            ← 200 OK
                            {"deleted": true}

103ms  ← WebSocket frame
       (binary)

       200 OK
       {"deleted": true}
```

---

## Testing Results

### HTTP DELETE (with approval)
```bash
$ curl -X DELETE http://localhost:8084/api/david -v
< HTTP/1.1 200 OK
< Content-Length: 90
{"status":"success","message":"Hello from Nginx!"}
```

**Audit Log**:
```json
{"action":"http_approval_requested","metadata":{"method":"DELETE","path":"/api/david"}}
{"action":"http_approval_granted","metadata":{"method":"DELETE"}}
```

---

### PostgreSQL DELETE (with approval)
```bash
$ psql -h localhost -p 5435 -U admin -d testdb -c "DELETE FROM users WHERE id = 999;"
DELETE 0
```

**Audit Log**:
```json
{"action":"postgres_approval_requested","metadata":{"query":"DELETE FROM users WHERE id = 999;"}}
{"action":"postgres_approval_granted","metadata":{"query":"DELETE FROM users WHERE id = 999;"}}
```

---

## Benefits of This Architecture

1. ✅ **ALB Compatible**: WebSocket works perfectly with ALB
2. ✅ **Protocol Aware**: Full HTTP/PostgreSQL parsing
3. ✅ **Approval Support**: All approval patterns work
4. ✅ **Whitelist Support**: All whitelist patterns work
5. ✅ **Audit Logging**: Complete visibility into requests/queries
6. ✅ **Credential Hiding**: Backend credentials never exposed
7. ✅ **Reuses Existing Logic**: No duplication of approval/whitelist code

---

## Code Organization

```
internal/api/
├── proxy_stream.go              # Main WebSocket routing
│   ├── handleProxyStream()      # Routes by connection type
│   ├── handleHTTPWebSocket()    # HTTP-aware WebSocket handler
│   ├── handlePostgresWebSocket() # PostgreSQL-aware WebSocket handler
│   ├── websocketConn{}          # WebSocket → net.Conn adapter
│   └── handleHTTPOverWebSocket() # HTTP parsing loop
│
├── proxy_http_stream.go         # Legacy HTTP handler (non-WebSocket)
└── proxy_postgres.go            # Legacy PostgreSQL handler (non-WebSocket)

internal/proxy/
├── http.go                      # HTTPProxy.HandleRequest()
│   └── Checks whitelist + approval
│
└── postgres_auth.go             # PostgresAuthProxy.HandleConnection()
    └── Parses queries, checks whitelist + approval
```

---

## Comparison: Before vs After

| Feature | Before (Broken) | After (Fixed) |
|---------|----------------|---------------|
| ALB Support | ✅ Yes | ✅ Yes |
| HTTP Parsing | ❌ No | ✅ Yes |
| HTTP Approval | ❌ Broken | ✅ Working |
| PostgreSQL Parsing | ❌ No | ✅ Yes |
| PostgreSQL Approval | ❌ Broken | ✅ Working |
| Audit Logging | ⚠️ Partial | ✅ Complete |
| Performance | Fast | Fast (minimal overhead) |

---

## Future Enhancements

1. **Compression**: Enable WebSocket `permessage-deflate` extension
2. **Multiplexing**: Multiple connections over one WebSocket
3. **Better Buffering**: Adaptive buffer sizes based on traffic patterns
4. **Connection Pooling**: Reuse backend connections for better performance

---

## Summary

The key insight is that **WebSocket doesn't mean "dumb tunnel"** - we can still parse protocols by:

1. Wrapping WebSocket as `net.Conn` using an adapter
2. Passing it to existing protocol-aware handlers
3. Reusing all existing security logic (approval, whitelist, audit)

This gives us the best of both worlds:
- ✅ ALB compatibility (WebSocket)
- ✅ Full protocol awareness (HTTP/PostgreSQL parsing)
- ✅ Complete security (approval + whitelist)


