# Mock Approval Server

A simple HTTP server that receives approval requests and automatically approves them. Perfect for testing the approval workflow without needing to manually click buttons in Slack.

## Features

- ✅ **Auto-approve** - Automatically approves all incoming requests
- ⏱️ **Configurable delay** - Add delay before approving (simulate human response time)
- 📊 **Verbose logging** - See all approval requests in real-time
- 🔧 **Flexible** - Can also run in manual mode (just log, don't approve)

## Usage

### Quick Start

```bash
# Build
cd tools/mock-approval-server
go build -o mock-approval-server

# Run with defaults (auto-approve, port 9000)
./mock-approval-server
```

### Configuration Options

```bash
# Custom port
./mock-approval-server -port 9001

# Custom API URL (if not localhost:8080)
./mock-approval-server -api-url http://localhost:8080

# Add delay before approving (simulate human response time)
./mock-approval-server -delay 2s

# Custom approver name
./mock-approval-server -approver "Bob"

# Disable auto-approve (just log requests)
./mock-approval-server -auto-approve=false

# Quiet mode (less verbose)
./mock-approval-server -verbose=false
```

### All Options

| Flag | Default | Description |
|------|---------|-------------|
| `-port` | 9000 | Port to listen on |
| `-api-url` | http://localhost:8080 | Port authorizing API URL |
| `-auto-approve` | true | Automatically approve all requests |
| `-delay` | 0 | Delay before approving (e.g., 2s, 1m) |
| `-approver` | mock-server | Name of the approver |
| `-verbose` | true | Verbose logging |

## Testing Approval Workflow

### Step 1: Start Port Authorizing API

```bash
# Terminal 1
./bin/port-authorizing
```

### Step 2: Start Mock Approval Server

```bash
# Terminal 2
cd tools/mock-approval-server
go build && ./mock-approval-server
```

### Step 3: Configure Port Authorizing

Update `config.yaml`:

```yaml
approval:
  enabled: true
  patterns:
    - pattern: "^DELETE /.*"
      tags: ["env:test"]
      timeout_seconds: 300
  webhook:
    url: "http://localhost:9000/webhook"
```

### Step 4: Test with CLI

```bash
# Terminal 3 - Start proxy
./bin/port-authorizing connect nginx-server -l 8081

# Terminal 4 - Make DELETE request
curl -X DELETE http://localhost:8081/api/users/123
```

### Expected Output

**Mock Approval Server (Terminal 2):**
```
🚀 Mock Approval Server started on :9000
📡 API URL: http://localhost:8080
✅ Auto-approve: true
👤 Approver name: mock-server

Waiting for approval requests...

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
📥 Approval Request Received
   Request ID:    550e8400-e29b-41d4-a716-446655440000
   User:          admin
   Connection:    nginx-server
   Method:        DELETE
   Path:          /api/users/123
   Requested At:  2025-10-06T14:30:00Z
   Metadata:
     connection_name: nginx-server
     connection_type: http
🔄 Sending approval to: http://localhost:8080/api/approvals/550e8400-e29b-41d4-a716-446655440000/approve
✅ Request 550e8400-e29b-41d4-a716-446655440000 APPROVED by mock-server
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

**CLI (Terminal 4):**
```
{"status":"success","message":"Deleted user 123"}
```

## Use Cases

### 1. Development Testing

Quickly test approval workflow without Slack:
```bash
./mock-approval-server
```

### 2. CI/CD Testing

Automated tests with instant approval:
```bash
./mock-approval-server -verbose=false
```

### 3. Simulate Human Delay

Test timeout behavior:
```bash
# Approve after 10 seconds
./mock-approval-server -delay 10s

# Test timeout (if timeout is 5s, request will be rejected)
./mock-approval-server -delay 10s  # > timeout
```

### 4. Manual Testing

Log requests but don't auto-approve:
```bash
./mock-approval-server -auto-approve=false

# Then manually approve using curl:
curl http://localhost:8080/api/approvals/{request_id}/approve
```

## Integration with Slack

You can run both the mock server AND Slack simultaneously:

```yaml
approval:
  webhook:
    url: "http://localhost:9000/webhook"  # Mock server
  slack:
    webhook_url: "https://hooks.slack.com/..."  # Real Slack
```

This sends to both:
- Mock server auto-approves (for testing)
- Slack sends to your team (for visibility)

First approval (from mock server) wins!

## Health Check

```bash
curl http://localhost:9000/health
```

Returns:
```json
{
  "status": "ok",
  "service": "mock-approval-server",
  "auto_approve": true
}
```

## Example: Testing Timeout

```bash
# Set approval timeout to 5 seconds in config.yaml
approval:
  patterns:
    - pattern: "^DELETE /.*"
      timeout_seconds: 5

# Start mock server with 10 second delay (> timeout)
./mock-approval-server -delay 10s

# Make request
curl -X DELETE http://localhost:8081/api/

# Expected: Request times out after 5s and returns 403
# Mock server will still try to approve at 10s, but too late!
```

## Docker Support

```dockerfile
# Dockerfile
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o mock-approval-server

FROM alpine:latest
COPY --from=builder /app/mock-approval-server /usr/local/bin/
ENTRYPOINT ["mock-approval-server"]
```

Build and run:
```bash
docker build -t mock-approval-server .
docker run -p 9000:9000 mock-approval-server -api-url http://host.docker.internal:8080
```

## Troubleshooting

### "Connection refused" when approving

**Problem:** Mock server can't reach the API server.

**Solution:** Check `-api-url` flag:
```bash
# If API is on different host
./mock-approval-server -api-url http://192.168.1.100:8080

# If using Docker
./mock-approval-server -api-url http://host.docker.internal:8080
```

### Requests not reaching mock server

**Problem:** Webhook URL not configured correctly.

**Solution:** Verify `config.yaml`:
```yaml
approval:
  webhook:
    url: "http://localhost:9000/webhook"  # Must match -port flag
```

### Approval not working

**Problem:** Auto-approve disabled or delay too long.

**Solution:**
```bash
# Enable auto-approve
./mock-approval-server -auto-approve=true

# Reduce delay
./mock-approval-server -delay 1s
```

## Advanced: Custom Logic

Modify `main.go` to add custom approval logic:

```go
func approveRequest(requestID, approver string) {
    // Custom logic
    if shouldApprove(requestID) {
        // Call approval API
        approvalURL := fmt.Sprintf("%s/api/approvals/%s/approve", *apiURL, requestID)
        http.Post(approvalURL, "application/json", nil)
    } else {
        // Call rejection API
        rejectURL := fmt.Sprintf("%s/api/approvals/%s/reject", *apiURL, requestID)
        http.Post(rejectURL, "application/json", nil)
    }
}
```

## Summary

The mock approval server is perfect for:
- ✅ **Development** - Test approval workflow without Slack setup
- ✅ **CI/CD** - Automated testing with instant approvals
- ✅ **Debugging** - See exactly what approval requests look like
- ✅ **Load testing** - Test approval system under load
- ✅ **Timeout testing** - Test what happens when approvals are slow
- ✅ **Interactive testing** - Manually approve/reject each request via stdin

## Quick Reference

| Flag | Default | Description |
|------|---------|-------------|
| `-port` | 9000 | Server port |
| `-api-url` | http://localhost:8080 | API server URL |
| `-auto-approve` | true | Auto-approve all requests |
| `-interactive` | false | Prompt for each approval via stdin |
| `-delay` | 0 | Delay before auto-approving |
| `-approver` | mock-server | Approver name |
| `-verbose` | true | Verbose logging |

## Common Commands

```bash
# Quick start - auto-approve everything
./bin/mock-approval-server

# Interactive mode - you decide
./bin/mock-approval-server -interactive

# Simulate slow approver
./bin/mock-approval-server -delay 5s

# Point to remote API
./bin/mock-approval-server -api-url http://prod.example.com:8080

# Quiet mode for CI/CD
./bin/mock-approval-server -verbose=false
```

Happy testing! 🎉

