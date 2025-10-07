# Approval Timeout and Tag-Based Approval

## What is the Approval Timeout?

The **timeout** is **how long the proxy will block and wait for a human to approve/reject** the request before automatically rejecting it.

### Flow with Timeout

```
┌─────────┐     1. DELETE /api/users/123
│  User   │────────────────────────────────>┌──────────┐
└─────────┘                                  │  Proxy   │
                                             └──────────┘
                                                   │
                                                   ▼
                                      ┌────────────────────────┐
                                      │ Approval Required!     │
                                      │ Timeout: 300 seconds   │
                                      └────────────────────────┘
                                                   │
                                                   │ 2. Send to Slack
                                                   ▼
                          ┌────────────────────────────────────┐
                          │   Slack Message                    │
                          │   "⚠️ Approval Required"            │
                          │   [Approve] [Reject]               │
                          │   Timeout in 5 minutes             │
                          └────────────────────────────────────┘
                                                   │
                          ┌────────────────────────┴─────────────────┐
                          │                                          │
            3a. Within 5 minutes                       3b. After 5 minutes
                          │                                          │
                   ┌──────┴──────┐                              ┌────┴────┐
                   │  Approve    │                              │ Timeout │
                   └─────┬───────┘                              └────┬────┘
                         │                                           │
                         ▼                                           ▼
                 ┌──────────────┐                          ┌─────────────────┐
                 │ Request      │                          │ Request Auto-   │
                 │ PROCEEDS     │                          │ REJECTED (403)  │
                 │ to Backend   │                          └─────────────────┘
                 └──────────────┘
```

### Important: Request is BLOCKED During Timeout

**The user's request is frozen/blocked** waiting for approval:

```
User Makes Request
      │
      ▼
┌─────────────────────────────┐
│  Proxy BLOCKS request       │ ◄─── User is waiting here
│  Waiting for approval...    │
│  Time remaining: 4m 32s     │
└─────────────────────────────┘
      │
      │ (After approval or timeout)
      ▼
Response Returned to User
```

### Timeout Examples

```yaml
patterns:
  # Quick approval needed - only wait 1 minute
  - pattern: "^GET /admin/logs"
    timeout_seconds: 60

  # Standard approval - 5 minutes
  - pattern: "^DELETE /.*"
    timeout_seconds: 300

  # Critical operation - wait up to 30 minutes for approval
  - pattern: "^DROP DATABASE .*"
    timeout_seconds: 1800
```

### Choosing the Right Timeout

| Operation Type | Recommended Timeout | Reason |
|---|---|---|
| Frequent operations | 1-2 minutes | Team needs to respond quickly |
| Standard deletions | 5 minutes | Balance between urgency and response time |
| Rare/critical ops | 10-30 minutes | Allows time to contact decision maker |
| Emergency-only | 5 minutes | Should be fast, but rare |

**Note:** If timeout is too short, legitimate requests get auto-rejected. If too long, users wait forever.

---

## Tag-Based Approval

Tag-based approval lets you require approval **only for specific connections** based on their tags.

### Why Tags?

You want different approval rules for different environments:

- ✅ **Production databases**: Require approval for DELETE
- ❌ **Dev/staging databases**: Allow DELETE without approval
- ✅ **Critical systems**: Require approval for ALL changes
- ❌ **Test systems**: Allow everything

### Configuration Examples

#### Example 1: Approval Only on Production

```yaml
connections:
  - name: "prod-db"
    type: "postgres"
    tags: ["env:production"]  # Tagged as production

  - name: "dev-db"
    type: "postgres"
    tags: ["env:dev"]  # Tagged as dev

approval:
  patterns:
    # DELETE requires approval ONLY on production
    - pattern: "^DELETE /.*"
      tags: ["env:production"]  # Only matches prod-db
      timeout_seconds: 300
```

**Result:**
- `DELETE` on `prod-db` → **Requires approval** ✋
- `DELETE` on `dev-db` → **No approval needed** ✅

#### Example 2: Multiple Tags with "All" Match

```yaml
connections:
  - name: "prod-payment-db"
    tags: ["env:production", "system:payment", "criticality:high"]

  - name: "prod-logs-db"
    tags: ["env:production", "system:logs"]

approval:
  patterns:
    # Require approval only for production AND payment system
    - pattern: "^DELETE /.*"
      tags: ["env:production", "system:payment"]
      tag_match: all  # Must have BOTH tags
      timeout_seconds: 300
```

**Result:**
- `DELETE` on `prod-payment-db` → **Requires approval** (has both tags) ✋
- `DELETE` on `prod-logs-db` → **No approval** (missing system:payment) ✅

#### Example 3: Multiple Tags with "Any" Match

```yaml
connections:
  - name: "prod-db"
    tags: ["env:production"]

  - name: "backend-team-db"
    tags: ["team:backend"]

  - name: "frontend-team-db"
    tags: ["team:frontend"]

approval:
  patterns:
    # Require approval for production OR backend team
    - pattern: "^DROP .*"
      tags: ["env:production", "team:backend"]
      tag_match: any  # Matches if has ANY of these tags
      timeout_seconds: 600
```

**Result:**
- `DROP` on `prod-db` → **Requires approval** (has env:production) ✋
- `DROP` on `backend-team-db` → **Requires approval** (has team:backend) ✋
- `DROP` on `frontend-team-db` → **No approval** (has neither tag) ✅

#### Example 4: No Tags = Applies to All

```yaml
approval:
  patterns:
    # Require approval for admin operations on ALL connections
    - pattern: "^POST /admin/.*"
      tags: []  # Empty = matches ALL connections
      timeout_seconds: 300
```

**Result:**
- Admin operations on **any connection** require approval

### Tag Match Modes

| Mode | Behavior | Use Case |
|---|---|---|
| `all` (default) | Must have **ALL** specified tags | Narrow targeting: "production AND payment" |
| `any` | Must have **ANY** specified tag | Broad targeting: "production OR critical" |

### Real-World Examples

#### Scenario 1: Stricter Rules for Production

```yaml
approval:
  patterns:
    # Production: Approval for ALL DELETE operations
    - pattern: "^DELETE /.*"
      tags: ["env:production"]
      timeout_seconds: 300

    # Dev/Staging: No approval needed (no pattern matches)
```

#### Scenario 2: Team-Based Approvals

```yaml
approval:
  patterns:
    # Backend team databases need approval for schema changes
    - pattern: "^(ALTER|DROP|CREATE) TABLE .*"
      tags: ["team:backend"]
      timeout_seconds: 600

    # Data team can do anything without approval
```

#### Scenario 3: Criticality-Based

```yaml
connections:
  - name: "user-db"
    tags: ["criticality:high", "env:production"]

  - name: "analytics-db"
    tags: ["criticality:low", "env:production"]

approval:
  patterns:
    # Only high-criticality systems need approval
    - pattern: "^DELETE /.*"
      tags: ["criticality:high"]
      timeout_seconds: 300
```

**Result:**
- `DELETE` on `user-db` → **Approval required** ✋
- `DELETE` on `analytics-db` → **No approval** ✅

#### Scenario 4: Multi-Environment, Multi-Team

```yaml
connections:
  - name: "prod-payment"
    tags: ["env:production", "team:payments", "region:us-east"]

  - name: "prod-analytics"
    tags: ["env:production", "team:analytics", "region:us-west"]

  - name: "staging-payment"
    tags: ["env:staging", "team:payments", "region:us-east"]

approval:
  patterns:
    # Approval for production payments in ANY region
    - pattern: "^DELETE /.*"
      tags: ["env:production", "team:payments"]
      tag_match: all
      timeout_seconds: 300

    # Approval for ANY production database in us-east
    - pattern: "^DROP .*"
      tags: ["env:production", "region:us-east"]
      tag_match: all
      timeout_seconds: 600
```

### Best Practices

1. **Use descriptive tags:**
   - ✅ `env:production`, `team:backend`, `criticality:high`
   - ❌ `prod`, `be`, `important`

2. **Consistent tag naming:**
   - Always use `category:value` format
   - Examples: `env:dev`, `team:frontend`, `region:us-east`

3. **Layer your approval rules:**
   ```yaml
   patterns:
     # Layer 1: All production requires approval for DROP
     - pattern: "^DROP .*"
       tags: ["env:production"]
       timeout_seconds: 300

     # Layer 2: High-criticality requires approval for DELETE too
     - pattern: "^DELETE /.*"
       tags: ["criticality:high"]
       timeout_seconds: 300
   ```

4. **Document your tags:**
   - Create a `docs/TAGS.md` documenting your tag schema
   - Keep tags consistent across all connections

### Testing Tag-Based Approvals

```bash
# 1. Add tags to your connection
connections:
  - name: "test-db"
    tags: ["env:test", "team:qa"]

# 2. Add approval pattern
approval:
  patterns:
    - pattern: "^DELETE /.*"
      tags: ["env:test"]
      timeout_seconds: 60

# 3. Make a DELETE request
curl -X DELETE http://localhost:8080/api/users/123

# 4. Check Slack for approval message
# 5. Verify it only triggers for connections with matching tags
```

---

## Summary

### Timeout
- **What**: How long to wait for human approval before auto-rejecting
- **When**: User's request is BLOCKED during this time
- **Choose wisely**: Balance between response time and operational urgency

### Tags
- **What**: Filter which connections require approval
- **Match modes**:
  - `all`: Must have ALL tags (narrow)
  - `any`: Must have ANY tag (broad)
- **Power**: Different rules for prod vs dev, critical vs non-critical, etc.

### Combined Power

```yaml
# DELETE on production payment databases waits 5 minutes for approval
- pattern: "^DELETE /.*"
  tags: ["env:production", "system:payment"]
  tag_match: all
  timeout_seconds: 300
```

This gives you **fine-grained control** over approval requirements! 🎯

