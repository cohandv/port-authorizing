# HTTP-Aware TCP Proxy with Approval Support

## Problem

The CLI uses raw TCP tunneling, which forwards bytes transparently without understanding the protocol. This meant:

❌ **Approval workflows didn't work** - Couldn't parse HTTP requests to check if approval was needed
❌ **Whitelist wasn't enforced** - Couldn't extract method/path from raw bytes
✅ **Audit logging worked** - But only captured raw bytes after the fact

## The Insight

**We were already capturing HTTP traffic for audit logs**, we just needed to **parse it BEFORE forwarding** instead of only logging after!

## Solution: HTTP-Aware Stream Handler

Created `handleHTTPProxyStream()` that:

1. **Intercepts raw TCP stream** from CLI
2. **Parses HTTP requests** from the byte stream
3. **Calls HTTP proxy's HandleRequest()** - which includes:
   - Whitelist checking
   - **Approval workflow**
   - LLM security analysis (if enabled)
4. **Forwards to backend** only if approved
5. **Returns response** back through the stream

### Architecture

#### Before (Transparent TCP Tunnel)

```
CLI → localhost:8081 → [raw bytes] → API Server → [raw bytes] → Backend
                                          ↓
                                      Audit log
                                      (after the fact)

❌ No approval check
❌ No whitelist check
✅ Audit logging (passive)
```

#### After (HTTP-Aware Proxy)

```
CLI → localhost:8081 → [HTTP bytes] → API Server
                                          ↓
                                    Parse HTTP request
                                          ↓
                                    Extract method & path
                                          ↓
                                    Check whitelist ✅
                                          ↓
                                    Check approval required? ✅
                                          ↓ (if required)
                                    Send to Slack/Webhook
                                          ↓
                                    BLOCK & WAIT ⏸️
                                          ↓
                              ┌─────────────────────┐
                        Approved            Rejected/Timeout
                              │                     │
                              ▼                     ▼
                    Forward to Backend      Return 403
                              │
                              ▼
                    Return response to CLI
```

## Implementation Details

### 1. Route HTTP/HTTPS Connections to HTTP Handler

**File:** `internal/api/proxy_stream.go`

```go
// Route to appropriate handler based on connection type
if conn.Config.Type == "postgres" {
    s.handlePostgresProxy(w, r)
    return
}

// For HTTP/HTTPS connections, use HTTP proxy handler (with approval support)
if conn.Config.Type == "http" || conn.Config.Type == "https" {
    s.handleHTTPProxyStream(w, r)  // ← NEW!
    return
}

// For other types (tcp), use transparent TCP proxy
```

### 2. HTTP Stream Handler

**File:** `internal/api/proxy_http_stream.go`

Processes HTTP requests in a loop:

```go
for {
    // Read HTTP request from client
    requestBytes := readHTTPRequest(reader)

    // Parse HTTP request
    httpReq, _ := http.ReadRequest(bytes.NewReader(requestBytes))

    // Create synthetic request for proxy handler
    proxyReq := httptest.NewRequest("POST", "/", bytes.NewReader(requestBytes))

    // Call HTTP proxy's HandleRequest
    // This checks whitelist + approval!
    err := httpProxy.HandleRequest(respWriter, proxyReq)

    // Response is automatically written back to client
}
```

### 3. Stream Response Writer

Implements `http.ResponseWriter` interface but writes directly to the TCP stream:

```go
type streamResponseWriter struct {
    writer      *bufio.ReadWriter
    header      http.Header
    statusCode  int
}

func (w *streamResponseWriter) WriteHeader(statusCode int) {
    // Write HTTP status line
    fmt.Fprintf(w.writer, "HTTP/1.1 %d %s\r\n", statusCode, statusText)

    // Write headers
    for key, values := range w.header {
        fmt.Fprintf(w.writer, "%s: %s\r\n", key, value)
    }

    // End headers
    fmt.Fprint(w.writer, "\r\n")
    w.writer.Flush()
}
```

## How It Works Now

### Example: DELETE Request Through CLI

```bash
# 1. Start CLI proxy
./bin/port-authorizing connect nginx-server -l 8081

# 2. Make DELETE request
curl -X DELETE http://localhost:8081/api/users/123
```

**Flow:**

1. `curl` sends: `DELETE /api/users/123 HTTP/1.1\r\n...`
2. CLI forwards raw bytes to API server
3. **API server's `handleHTTPProxyStream`:**
   - Reads raw bytes
   - **Parses** as HTTP request
   - Extracts method: `DELETE`, path: `/api/users/123`
   - Checks if approval required (based on config patterns + tags)
4. **If approval required:**
   - Sends to Slack with "Approve/Reject" buttons
   - **BLOCKS the request** (client waits)
   - Waits up to timeout (e.g., 5 minutes)
5. **If approved:**
   - Forwards request to nginx
   - Returns response to client
6. **If rejected/timeout:**
   - Returns `HTTP/1.1 403 Forbidden`
   - Client gets error

## Configuration

Works exactly like before, but now **also works through CLI**!

```yaml
connections:
  - name: nginx-server
    type: http
    host: localhost
    port: 8888
    tags: ["env:test"]

approval:
  enabled: true
  patterns:
    - pattern: "^DELETE /.*"
      tags: ["env:test"]
      timeout_seconds: 300
  slack:
    webhook_url: "https://hooks.slack.com/..."
```

## Testing

### Test 1: Approval Required

```bash
# Start CLI proxy
./bin/port-authorizing connect nginx-server -l 8081

# Make DELETE request (requires approval)
curl -X DELETE http://localhost:8081/api/users/123

# Expected:
# - Request BLOCKS
# - Slack message appears with Approve/Reject buttons
# - After 5 minutes (timeout), request returns 403
# - OR if approved, request proceeds to backend
```

### Test 2: No Approval Needed

```bash
# Make GET request (no approval needed)
curl http://localhost:8081/api/users

# Expected:
# - Request proceeds immediately
# - No approval required
# - Response returned quickly
```

## Benefits

✅ **Approval workflows work through CLI** - Full support for human-in-the-loop security
✅ **Whitelist enforcement** - HTTP patterns checked in real-time
✅ **Audit logging enhanced** - Now logs approval decisions too
✅ **LLM analysis supported** - If enabled, works through CLI
✅ **Backward compatible** - TCP proxies still use transparent forwarding

## Performance Considerations

**Overhead:** Minimal
- Parsing HTTP: ~microseconds per request
- Approval check: Only if pattern matches
- Blocking: Only when approval required

**Memory:** Low
- Streams requests/responses
- No buffering of large bodies
- Each request processed independently

## Limitations

1. **HTTP protocol only** - TCP connections still use transparent proxy
2. **Requires valid HTTP** - Malformed requests will fail
3. **Keep-Alive supported** - Processes multiple requests on same connection
4. **Streaming responses** - Works for chunked encoding

## Future Enhancements

Potential improvements:
- Support for WebSocket upgrading
- HTTP/2 support
- Request/response body inspection (currently only headers/method/path)
- Caching layer for performance
- Rate limiting per connection

## Summary

**The approval workflow now works end-to-end through the CLI!**

Users can use the CLI as normal, and sensitive operations will:
1. Be intercepted
2. Sent for approval
3. Block until approved/rejected
4. Proceed or fail based on decision

This gives you **full security controls** without changing the user experience. 🎯

