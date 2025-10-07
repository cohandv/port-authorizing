# Approval Workflow

The approval workflow adds a human-in-the-loop security layer that requires explicit approval for sensitive operations before they are executed.

## Overview

When enabled, certain HTTP requests can be configured to require approval before being proxied to the backend. The request is sent to one or more approval providers (Slack, webhook) and waits for an approval/rejection decision.

## Flow Diagram

```
┌──────┐     1. Request      ┌─────────────┐
│ User │────────────────────>│   Proxy     │
└──────┘                     └─────────────┘
                                    │
                                    │ 2. Whitelist check
                                    ▼
                             ┌─────────────┐
                             │ Whitelist OK│
                             └─────────────┘
                                    │
                                    │ 3. Check approval required?
                                    ▼
                             ┌─────────────┐
                       ┌─────│  Requires   │─────┐
                       │     │  Approval?  │     │
                       │     └─────────────┘     │
                    Yes│                         │No
                       ▼                         ▼
           ┌──────────────────────┐      ┌──────────────┐
           │ Send to Slack/Webhook│      │ Proxy to     │
           └──────────────────────┘      │ Backend      │
                       │                 └──────────────┘
                       │ 4. Wait for decision
                       ▼
           ┌──────────────────────┐
           │ Approved / Rejected? │
           └──────────────────────┘
              │                │
          Approved         Rejected
              │                │
              ▼                ▼
      ┌──────────┐      ┌──────────┐
      │ Proxy to │      │  Return  │
      │ Backend  │      │   403    │
      └──────────┘      └──────────┘
```

## Configuration

### Basic Configuration

Add to `config.yaml`:

```yaml
server:
  port: 8080
  base_url: "https://your-api-domain.com"  # Required for Slack buttons

approval:
  enabled: true

  # Define which requests require approval
  patterns:
    # Require approval for all DELETE operations
    - pattern: "^DELETE /.*"
      timeout_seconds: 300  # 5 minutes

    # Require approval for sensitive POST endpoints
    - pattern: "^POST /(users|admin)/.*"
      timeout_seconds: 600  # 10 minutes

    # Require approval for database DROP commands
    - pattern: "^POST .* DROP .*"
      timeout_seconds: 300
```

### Pattern Format

Patterns use regular expressions to match HTTP requests in the format:
```
METHOD /path
```

Examples:
- `^DELETE /.*` - All DELETE requests
- `^POST /api/users/.*` - POST requests to user endpoints
- `^(PUT|PATCH) /.*` - All PUT or PATCH requests
- `^GET /admin/.*` - GET requests to admin endpoints

### Approval Providers

#### 1. Generic Webhook

Send approval requests to any webhook endpoint:

```yaml
approval:
  enabled: true
  webhook:
    url: "https://your-approval-service.com/webhook"
  patterns:
    - pattern: "^DELETE /.*"
      timeout_seconds: 300
```

**Webhook Payload:**

```json
{
  "request_id": "550e8400-e29b-41d4-a716-446655440000",
  "username": "alice",
  "connection_id": "conn-123",
  "method": "DELETE",
  "path": "/api/users/5",
  "requested_at": "2025-01-15T10:30:00Z",
  "metadata": {
    "connection_name": "production-api",
    "connection_type": "http"
  },
  "approval_url": "/api/approvals/550e8400-e29b-41d4-a716-446655440000"
}
```

**To approve/reject, make a request:**

```bash
# Approve
curl -X POST "https://your-api-domain.com/api/approvals/{request_id}/approve?approver=bob&reason=approved"

# Reject
curl -X POST "https://your-api-domain.com/api/approvals/{request_id}/reject?approver=bob&reason=too risky"
```

#### 2. Slack Integration

Send approval requests to Slack with interactive buttons:

```yaml
approval:
  enabled: true
  slack:
    webhook_url: "https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK"
  patterns:
    - pattern: "^DELETE /.*"
      timeout_seconds: 300
```

**Slack Message Example:**

When a request requires approval, a Slack message is sent with:
- Request details (user, method, path, connection)
- Interactive "Approve" and "Reject" buttons
- Timeout information

Users click the buttons directly in Slack, which calls the approval API endpoint.

#### 3. Multiple Providers

You can enable both webhook and Slack simultaneously:

```yaml
approval:
  enabled: true
  webhook:
    url: "https://your-approval-service.com/webhook"
  slack:
    webhook_url: "https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK"
  patterns:
    - pattern: "^DELETE /.*"
      timeout_seconds: 300
```

When a request requires approval, it will be sent to **all** configured providers. The first approval/rejection received will determine the outcome.

## API Endpoints

### Approve a Request

```http
GET/POST /api/approvals/{request_id}/approve
```

**Query Parameters:**
- `approver` (optional): Name of the person approving
- `reason` (optional): Reason for approval

**Response:**
- `200 OK` - HTML page confirming approval
- `200 OK` (with Accept: application/json) - JSON response

**Example:**

```bash
curl "https://api.example.com/api/approvals/550e8400-e29b-41d4-a716-446655440000/approve?approver=bob"
```

### Reject a Request

```http
GET/POST /api/approvals/{request_id}/reject
```

**Query Parameters:**
- `approver` (optional): Name of the person rejecting
- `reason` (optional): Reason for rejection

**Example:**

```bash
curl "https://api.example.com/api/approvals/550e8400-e29b-41d4-a716-446655440000/reject?approver=bob&reason=unauthorized"
```

### Get Pending Approvals Count

```http
GET /api/approvals/pending
```

**Requires:** Authentication

**Response:**
```json
{
  "pending_count": 3
}
```

## Audit Logging

All approval-related events are logged to the audit log:

- `http_approval_requested` - When approval is requested
- `http_approval_granted` - When request is approved
- `http_approval_rejected` - When request is rejected or times out

**Example audit log entry:**

```json
{
  "timestamp": "2025-01-15T10:30:00Z",
  "username": "alice",
  "action": "http_approval_requested",
  "resource": "production-api",
  "metadata": {
    "connection_id": "conn-123",
    "method": "DELETE",
    "path": "/api/users/5",
    "timeout": "5m0s"
  }
}
```

## Timeouts

Each approval pattern can have its own timeout. When a request requires approval:

1. The proxy sends the approval request to all providers
2. The proxy waits for up to `timeout_seconds` for a decision
3. If no decision is received within the timeout, the request is **automatically rejected**

**Best practices:**
- Use shorter timeouts (1-5 minutes) for frequently needed operations
- Use longer timeouts (10-30 minutes) for rare/sensitive operations
- Consider your team's response time when setting timeouts

## Use Cases

### 1. Production Database Deletions

```yaml
patterns:
  - pattern: "^DELETE /.*"
    timeout_seconds: 300
```

Require approval for any DELETE operation on production databases.

### 2. Sensitive Admin Operations

```yaml
patterns:
  - pattern: "^POST /admin/.*"
    timeout_seconds: 600
  - pattern: "^DELETE /admin/.*"
    timeout_seconds: 600
```

Require approval for all admin operations.

### 3. Database Schema Changes

```yaml
patterns:
  - pattern: "^POST .* (DROP|ALTER|CREATE) .*"
    timeout_seconds: 900  # 15 minutes
```

Require approval for SQL DDL commands.

### 4. User Management

```yaml
patterns:
  - pattern: "^POST /users/[0-9]+/roles"
    timeout_seconds: 300
  - pattern: "^DELETE /users/.*"
    timeout_seconds: 300
```

Require approval for role changes and user deletions.

## Security Considerations

### 1. Approval Endpoint Security

The approval endpoints (`/api/approvals/{request_id}/approve|reject`) are **intentionally not authenticated** to allow Slack buttons and webhooks to work.

**Security measures:**
- Request IDs are UUIDs - difficult to guess
- Requests automatically expire after timeout
- Each request can only be approved/rejected once
- All approvals are logged with approver name

### 2. Slack Webhook Security

- Keep your Slack webhook URL secret
- Use Slack's IP whitelist if your infrastructure supports it
- Monitor audit logs for unexpected approval patterns

### 3. Generic Webhook Security

- Use HTTPS for webhook URLs
- Implement authentication on your webhook endpoint
- Validate the `request_id` in your approval service
- Set up rate limiting

## Testing Approval Workflow

### 1. Quick Testing with Mock Approval Server (Recommended)

The easiest way to test approvals without setting up Slack:

```bash
# Terminal 1: Start mock approval server
./bin/mock-approval-server

# Terminal 2: Start API server
./bin/port-authorizing

# Terminal 3: Configure and test
# Update config.yaml:
approval:
  enabled: true
  webhook:
    url: "http://localhost:9000/webhook"
  patterns:
    - pattern: "^DELETE /.*"
      tags: ["env:test"]
      timeout_seconds: 300

# Start CLI proxy
./bin/port-authorizing connect nginx-server -l 8081

# Terminal 4: Make DELETE request
curl -X DELETE http://localhost:8081/api/users/123

# Result: Request is auto-approved and proceeds!
```

**Mock Server Modes:**

```bash
# Auto-approve (default) - instant approval
./bin/mock-approval-server

# Interactive mode - you decide!
./bin/mock-approval-server -interactive
# When request comes in, type: approve, reject, or skip

# Auto-approve with delay (simulate human response time)
./bin/mock-approval-server -delay 5s

# Manual mode - just log URLs
./bin/mock-approval-server -auto-approve=false

# Custom API URL
./bin/mock-approval-server -api-url http://192.168.1.100:8080

# Custom approver name
./bin/mock-approval-server -approver "Alice"
```

**Interactive Mode Example:**

```bash
./bin/mock-approval-server -interactive

# When request arrives, you'll see:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
❓ Decision required for request abc-123
   DELETE /api/users/123 by admin

   Type your decision:
   • 'approve' or 'a' - Approve this request
   • 'reject' or 'r'  - Reject this request
   • 'skip' or 's'    - Skip (timeout)

👉 Decision: approve ✅
```

### 2. Local Testing with Slack

```bash
# 1. Create a Slack incoming webhook
# 2. Add to config.yaml
approval:
  enabled: true
  slack:
    webhook_url: "https://hooks.slack.com/services/YOUR/WEBHOOK"
  patterns:
    - pattern: "^DELETE /.*"
      timeout_seconds: 300

# 3. Make a DELETE request through the proxy
curl -X DELETE http://localhost:8081/api/users/123

# 4. Check Slack for the approval message
# 5. Click "Approve" or "Reject"
```

### 3. Testing with Generic Webhook

```bash
# 1. Set up a webhook receiver (e.g., webhook.site)
# 2. Add to config
approval:
  enabled: true
  webhook:
    url: "https://webhook.site/your-unique-id"

# 3. Make a request that requires approval
# 4. Check webhook.site for the payload
# 5. Manually approve:
curl -X POST "http://localhost:8080/api/approvals/{request_id}/approve"
```

### 4. Testing Timeout Behavior

```bash
# Set short timeout
approval:
  patterns:
    - pattern: "^DELETE /.*"
      timeout_seconds: 5

# Start mock server with longer delay (will timeout)
./bin/mock-approval-server -delay 10s

# Make request
curl -X DELETE http://localhost:8081/api/

# Expected: Request times out after 5s and returns 403
# Mock server will try to approve at 10s, but too late!
```

### 5. CI/CD Testing

```bash
# Fully automated testing
./bin/mock-approval-server -verbose=false &
MOCK_PID=$!

# Run your tests
./run-integration-tests.sh

# Cleanup
kill $MOCK_PID
```

## Troubleshooting

### Approvals Not Working

1. **Check config is valid:**
   ```bash
   # Enable approval in config.yaml
   approval:
     enabled: true
   ```

2. **Check patterns match:**
   ```bash
   # Test your regex pattern
   echo "DELETE /api/users" | grep -E "^DELETE /.*"
   ```

3. **Check base_url is set:**
   ```yaml
   server:
     base_url: "https://your-domain.com"  # Required for Slack buttons
   ```

### Slack Messages Not Appearing

1. Verify webhook URL is correct
2. Check audit logs for errors
3. Test webhook URL with curl:
   ```bash
   curl -X POST https://hooks.slack.com/services/YOUR/WEBHOOK \
     -H 'Content-Type: application/json' \
     -d '{"text": "Test message"}'
   ```

### Approval Buttons Not Working

1. Ensure `server.base_url` is set correctly
2. Ensure approval endpoints are accessible from internet
3. Check firewall/security group rules

## Future Enhancements

Planned features:
- Microsoft Teams integration
- Email approval notifications
- PagerDuty integration
- Approval delegation/escalation
- Approval based on user roles
- Multi-approver requirements (2-of-3, etc.)

