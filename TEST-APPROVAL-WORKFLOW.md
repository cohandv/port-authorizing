# Testing Approval Workflow - Fixed Configuration

## What Was Wrong

1. **Developer had DELETE permission in whitelist** (line 142 in config.yaml: `^DELETE /api/.*`)
2. **Mock approval server was auto-approving** everything (`-auto-approve=true` by default)

## What Was Actually Working

✅ **Whitelist checking** - Working perfectly
✅ **Approval workflow** - Working perfectly
✅ **Audit logging** - Working perfectly

The issue was **configuration**, not code!

## New Configuration

### Developer whitelist (fixed):
```yaml
whitelist:
  - "^SELECT.*"
  - "^EXPLAIN.*"
  - "^GET /.*"
  - "^POST /api/.*"
  - "^PUT /api/.*"
  - "^PATCH /api/.*"
  # DELETE removed - developer should not have permission
```

### Admin whitelist (unchanged):
```yaml
whitelist:
  - ".*"  # Everything allowed
```

### Approval patterns (unchanged):
```yaml
approval:
  enabled: true
  patterns:
    - pattern: "^DELETE /.*"
      tags: ["env:test"]
      timeout_seconds: 10
```

## How to Test

### Terminal 1: Start API Server
```bash
go run cmd/api/main.go
```

### Terminal 2: Start Mock Approval Server (Interactive Mode)
```bash
cd tools/mock-approval-server
go run main.go -interactive=true
```

### Terminal 3: Test as Developer

```bash
# Login as developer
port-authorizing login developer

# Connect to nginx-server
port-authorizing connect nginx-server

# Try DELETE (should be BLOCKED by whitelist)
curl -X DELETE http://localhost:XXXX/api/test

# Expected result: 403 Forbidden
# Expected audit log: "http_request_blocked"
```

### Terminal 4: Test as Admin

```bash
# Login as admin
port-authorizing login admin

# Connect to nginx-server
port-authorizing connect nginx-server

# Try DELETE (should request APPROVAL)
curl -X DELETE http://localhost:XXXX/api/test

# Expected result: Request WAITS for approval
# In Terminal 2: You'll see prompt to approve/reject
# Type "approve" → request succeeds
# Type "reject" → request fails with 403
```

## Expected Audit Logs

### Developer DELETE (Blocked by Whitelist):
```json
{
  "action": "http_request_blocked",
  "username": "developer",
  "method": "DELETE",
  "path": "/api/test",
  "reason": "does not match whitelist"
}
```

### Admin DELETE (Approval Workflow):
```json
{
  "action": "http_request",
  "username": "admin",
  "method": "DELETE",
  "path": "/api/test",
  "allowed": true
}
{
  "action": "http_approval_requested",
  "method": "DELETE",
  "path": "/api/test",
  "timeout": "10s"
}
// Then either:
{
  "action": "http_approval_granted",
  "approved_by": "your-name"
}
// Or:
{
  "action": "http_approval_rejected",
  "rejected_by": "your-name"
}
```

## Mock Approval Server Modes

### Auto-Approve (Default - for testing)
```bash
go run main.go -auto-approve=true
# Automatically approves everything (instant)
```

### Auto-Approve with Delay (Simulates human)
```bash
go run main.go -auto-approve=true -delay=3s
# Automatically approves after 3 second delay
```

### Manual Mode (Need to click URLs)
```bash
go run main.go -auto-approve=false
# Prints approve/reject URLs you must visit
```

### Interactive Mode (Recommended for testing)
```bash
go run main.go -interactive=true
# Prompts you to type approve/reject for each request
```

## Expected Behavior Summary

| User      | Request        | Whitelist | Approval | Result                           |
|-----------|----------------|-----------|----------|----------------------------------|
| developer | GET /api/test  | ✅ Pass   | N/A      | ✅ Success (no approval needed)  |
| developer | DELETE /api/x  | ❌ Block  | N/A      | ❌ 403 Forbidden (blocked)       |
| admin     | GET /api/test  | ✅ Pass   | N/A      | ✅ Success (no approval needed)  |
| admin     | DELETE /api/x  | ✅ Pass   | ⏳ Wait  | Depends on approval decision     |

## Architecture Recap

```
Client Request
     ↓
JWT Authentication ✅
     ↓
Get Whitelist for User's Roles
     ↓
Whitelist Check (regex patterns)
     ↓
   Block ❌ ←─────┐
   Pass ✅        │
     ↓            │
Approval Check    │
     ↓            │
Needs Approval?   │
     ↓            │
   No → Forward to Backend
     ↓            │
   Yes → Request Approval
     ↓            │
Wait for Decision │
     ↓            │
Approved? ────────┤
     ↓            │
   No → Block ❌ ─┘
     ↓
   Yes → Forward to Backend
```

## Verifying the Fix

After restarting the API server with the new config:

1. **Developer DELETE should be blocked immediately** (no approval request)
2. **Admin DELETE should wait for approval** (interactive prompt in Terminal 2)

Check `audit.log` to verify the flow!

