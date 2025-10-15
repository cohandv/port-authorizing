# WebSocket Reverse Tunnel Architecture

## Overview

The **WebSocket Reverse Tunnel** is a crucial design that enables `port-authorizing` to work seamlessly with **AWS Application Load Balancers (ALB)** and other Layer 7 load balancers that don't fully support long-lived HTTP CONNECT tunnels.

## The Problem with HTTP CONNECT

Traditional HTTP CONNECT tunneling (used by many proxies) doesn't work well with ALBs because:

1. **ALB idle timeout** (default 60s) kills long-lived connections
2. **Limited HTTP CONNECT support** - ALBs are optimized for HTTP/HTTPS, not raw TCP tunneling
3. **Connection direction** - Traditional proxies establish connections FROM the API server TO the CLI, which doesn't work with NAT/firewalls

## The WebSocket Solution

### Architecture

```
┌─────────────────┐
│   Local App     │ (e.g., psql, curl)
│  localhost:5432 │
└────────┬────────┘
         │
         ↓
┌────────────────────────────────────────────────────────────┐
│                  CLI (port-authorizing)                    │
│  1. Opens local port (localhost:5432)                      │
│  2. Establishes persistent WebSocket connection to API     │
│  3. Forwards local traffic through WebSocket               │
└────────┬───────────────────────────────────────────────────┘
         │ WebSocket (outbound from CLI)
         │ ws://api.example.com/api/proxy/{connectionID}
         ↓
┌────────────────────────────────────────────────────────────┐
│                    AWS Application Load Balancer          │
│  - WebSocket support: ✅ Fully supported                  │
│  - Sticky sessions: ✅ Maintains connection                │
│  - Idle timeout: Handled by ping/pong keepalive           │
└────────┬───────────────────────────────────────────────────┘
         │
         ↓
┌────────────────────────────────────────────────────────────┐
│              API Server (port-authorizing)                 │
│  1. Upgrades HTTP to WebSocket                             │
│  2. Forwards WebSocket messages to backend                 │
│  3. Enforces security (auth, whitelist, approval)          │
│  4. Sends ping frames every 30s to keep connection alive   │
└────────┬───────────────────────────────────────────────────┘
         │
         ↓
┌────────────────────────────────────────────────────────────┐
│                    Backend Service                         │
│  (PostgreSQL, HTTP API, Redis, etc.)                       │
└────────────────────────────────────────────────────────────┘
```

### Key Design Principles

1. **Reverse Tunnel**: CLI initiates and maintains the connection TO the API (outbound)
   - Works through NAT/firewalls
   - No need for API to connect back to CLI
   - Compatible with corporate networks

2. **WebSocket Protocol**: Uses standard WebSocket (RFC 6455)
   - Fully supported by ALB
   - Bidirectional full-duplex communication
   - Binary message framing for efficiency

3. **Keepalive Mechanism**: Ping/Pong every 30 seconds
   - CLI sends PING frames every 30s
   - API responds with PONG
   - Prevents ALB idle timeout (default 60s)
   - Resets read deadline to 60s on each pong

4. **Authentication**: JWT token in WebSocket upgrade handshake
   - `Authorization: Bearer <token>` header
   - Validated before WebSocket upgrade
   - No separate auth after upgrade

## Implementation Details

### CLI Side (`internal/cli/connect.go`)

```go
// Convert HTTP URL to WebSocket URL
wsURL := strings.Replace(apiURL, "http://", "ws://", 1)
wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
wsURL = fmt.Sprintf("%s/api/proxy/%s", wsURL, connectionID)

// Establish WebSocket with auth header
headers := http.Header{}
headers.Add("Authorization", fmt.Sprintf("Bearer %s", token))

wsConn, _, err := dialer.Dial(wsURL, headers)

// Setup keepalive (ping every 30s)
ticker := time.NewTicker(30 * time.Second)
go func() {
    for range ticker.C {
        wsConn.WriteMessage(websocket.PingMessage, nil)
    }
}()

// Bidirectional forwarding
// Local → WebSocket (32KB buffer)
buf := make([]byte, 32768)
n, _ := localConn.Read(buf)
wsConn.WriteMessage(websocket.BinaryMessage, buf[:n])

// WebSocket → Local
messageType, data, _ := wsConn.ReadMessage()
if messageType == websocket.BinaryMessage {
    localConn.Write(data)
}
```

### API Server Side (`internal/api/proxy_stream.go`)

```go
// Detect WebSocket upgrade request
isWebSocket := r.Header.Get("Upgrade") == "websocket"

// Upgrade HTTP to WebSocket
upgrader := websocket.Upgrader{
    ReadBufferSize:  32768,  // 32KB
    WriteBufferSize: 32768,  // 32KB
    CheckOrigin: func(r *http.Request) bool {
        return true  // Auth handled via JWT
    },
}

wsConn, _ := upgrader.Upgrade(w, r, nil)

// Setup pong handler (respond to client pings)
wsConn.SetPongHandler(func(string) error {
    wsConn.SetReadDeadline(time.Now().Add(60 * time.Second))
    return nil
})

// Connect to backend
targetConn, _ := net.Dial("tcp", fmt.Sprintf("%s:%d", host, port))

// Bidirectional forwarding
// WebSocket → Backend
messageType, data, _ := wsConn.ReadMessage()
if messageType == websocket.BinaryMessage {
    targetConn.Write(data)
}

// Backend → WebSocket
buf := make([]byte, 32768)
n, _ := targetConn.Read(buf)
wsConn.WriteMessage(websocket.BinaryMessage, buf[:n])
```

## ALB Configuration

### Required ALB Settings

#### 1. **Enable WebSocket Support**

WebSocket is automatically supported on ALB, but ensure:

```yaml
# ALB Target Group Settings
Health Check:
  Protocol: HTTP
  Path: /api/health
  Interval: 30s
  Timeout: 5s
  Healthy Threshold: 2
  Unhealthy Threshold: 2

Attributes:
  deregistration_delay.timeout_seconds: 30
  stickiness.enabled: true  # IMPORTANT: Maintain WebSocket connection to same backend
  stickiness.type: lb_cookie
  stickiness.lb_cookie.duration_seconds: 3600  # 1 hour (match connection expiry)
```

#### 2. **Idle Timeout Configuration**

Set ALB idle timeout to at least 60 seconds (default is 60s):

```bash
aws elbv2 modify-load-balancer-attributes \
  --load-balancer-arn <alb-arn> \
  --attributes Key=idle_timeout.timeout_seconds,Value=60
```

Our ping/pong keepalive (every 30s) ensures the connection stays active.

#### 3. **Security Group Rules**

```yaml
# ALB Security Group
Ingress:
  - Port: 443 (HTTPS)
    Protocol: TCP
    Source: 0.0.0.0/0
  - Port: 80 (HTTP)
    Protocol: TCP
    Source: 0.0.0.0/0

# API Server Security Group
Ingress:
  - Port: 8080
    Protocol: TCP
    Source: <ALB Security Group>
```

### Terraform Example

```hcl
resource "aws_lb_target_group" "port_authorizing" {
  name     = "port-authorizing-tg"
  port     = 8080
  protocol = "HTTP"
  vpc_id   = var.vpc_id

  health_check {
    enabled             = true
    healthy_threshold   = 2
    unhealthy_threshold = 2
    timeout             = 5
    interval            = 30
    path                = "/api/health"
    protocol            = "HTTP"
    matcher             = "200"
  }

  # Enable sticky sessions for WebSocket
  stickiness {
    enabled         = true
    type            = "lb_cookie"
    cookie_duration = 3600  # 1 hour
  }

  deregistration_delay = 30
}

resource "aws_lb" "port_authorizing" {
  name               = "port-authorizing-alb"
  internal           = false
  load_balancer_type = "application"
  security_groups    = [aws_security_group.alb.id]
  subnets            = var.public_subnet_ids

  # Set idle timeout (60s default is fine with our 30s ping)
  idle_timeout = 60
}

resource "aws_lb_listener" "https" {
  load_balancer_arn = aws_lb.port_authorizing.arn
  port              = "443"
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-TLS-1-2-2017-01"
  certificate_arn   = var.acm_certificate_arn

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.port_authorizing.arn
  }
}
```

### Kubernetes/EKS Ingress Example

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: port-authorizing
  annotations:
    kubernetes.io/ingress.class: alb
    alb.ingress.kubernetes.io/scheme: internet-facing
    alb.ingress.kubernetes.io/target-type: ip
    
    # Health check
    alb.ingress.kubernetes.io/healthcheck-path: /api/health
    alb.ingress.kubernetes.io/healthcheck-interval-seconds: '30'
    alb.ingress.kubernetes.io/healthcheck-timeout-seconds: '5'
    alb.ingress.kubernetes.io/healthy-threshold-count: '2'
    alb.ingress.kubernetes.io/unhealthy-threshold-count: '2'
    
    # Sticky sessions for WebSocket
    alb.ingress.kubernetes.io/target-group-attributes: stickiness.enabled=true,stickiness.lb_cookie.duration_seconds=3600
    
    # SSL/TLS
    alb.ingress.kubernetes.io/certificate-arn: arn:aws:acm:us-east-1:123456789:certificate/xxx
    alb.ingress.kubernetes.io/listen-ports: '[{"HTTP": 80}, {"HTTPS": 443}]'
    alb.ingress.kubernetes.io/ssl-redirect: '443'
    
spec:
  rules:
  - host: api.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: port-authorizing
            port:
              number: 8080
```

## Testing

### Local Testing (without ALB)

```bash
# Start API server
./port-authorizing server --config config.yaml

# Login
./port-authorizing login -u admin -p admin123

# Connect via WebSocket tunnel
./port-authorizing connect postgres-prod -l 5432

# Use the connection
psql -h localhost -p 5432 -U myuser mydb
```

### Testing with ALB

```bash
# Set API URL to ALB endpoint
export API_URL="https://api.example.com"

# Login
./port-authorizing login --api-url $API_URL -u admin -p admin123

# Connect through ALB
./port-authorizing connect postgres-prod -l 5432

# Use the connection
psql -h localhost -p 5432 -U myuser mydb
```

### Monitoring WebSocket Connections

Check audit logs for WebSocket activity:

```bash
tail -f audit.log | grep websocket
```

Expected entries:
- `proxy_stream_websocket` - WebSocket upgrade successful
- `websocket_upgrade_failed` - WebSocket upgrade failed
- `backend_connect_failed` - Backend connection failed
- `proxy_session_websocket` - WebSocket session ended

## Performance

### Throughput

- **Buffer size**: 32KB (optimized for balance between memory and throughput)
- **PostgreSQL**: Can handle multiple queries per second
- **HTTP APIs**: Suitable for REST API calls, file downloads (up to 100s of MB)
- **Streaming**: Limited by WebSocket frame size (max 32KB per message)

### Latency

- **WebSocket overhead**: ~1-2ms per message
- **Ping/pong overhead**: Negligible (30s interval)
- **Total latency**: Primarily network latency (CLI → ALB → API → Backend)

### Scalability

- **Concurrent connections**: Limited by API server resources
- **ALB scaling**: Automatically scales to handle traffic
- **Stateful connections**: Each CLI maintains one WebSocket per active connection
- **Memory usage**: ~64KB per connection (2x 32KB buffers)

## Troubleshooting

### Connection Fails with "bad handshake"

**Cause**: WebSocket upgrade failed

**Solutions**:
1. Check ALB configuration (sticky sessions enabled?)
2. Verify API server is reachable
3. Check JWT token is valid
4. Look for `websocket_upgrade_failed` in audit logs

### Connection Drops After 60 Seconds

**Cause**: ALB idle timeout

**Solutions**:
1. Verify ping/pong is working (check CLI logs)
2. Increase ALB idle timeout: `aws elbv2 modify-load-balancer-attributes --attributes Key=idle_timeout.timeout_seconds,Value=120`
3. Check network isn't blocking WebSocket traffic

### High Latency

**Cause**: Multiple network hops

**Solutions**:
1. Deploy API server in same region as backend
2. Use AWS VPC peering/PrivateLink for backend connectivity
3. Consider NLB instead of ALB for lower latency (but loses WebSocket benefits)

### Backend Connection Fails

**Cause**: API server can't reach backend

**Solutions**:
1. Check security groups allow traffic from API server to backend
2. Verify backend is reachable from API server: `telnet backend-host backend-port`
3. Look for `backend_connect_failed` in audit logs

## Comparison with Other Approaches

| Approach | ALB Support | NAT/Firewall Friendly | Latency | Complexity |
|----------|-------------|------------------------|---------|------------|
| **WebSocket Reverse Tunnel** ✅ | ✅ Full support | ✅ Works everywhere | Low | Medium |
| HTTP CONNECT | ❌ Limited | ✅ Usually works | Very Low | Low |
| Direct Connection | ⚠️ Bypasses ALB | ❌ Requires VPN/routing | Very Low | Medium |
| SSH Tunneling | ❌ Not supported | ✅ Works everywhere | Medium | High |
| VPN | ❌ Not applicable | ✅ Works everywhere | Low | Very High |

## Security Considerations

1. **Authentication**: JWT tokens in WebSocket upgrade handshake
2. **Authorization**: Enforced per connection before upgrade
3. **Encryption**: Use WSS (WebSocket Secure) with TLS in production
4. **Audit Logging**: All connections and traffic logged
5. **Approval Workflow**: Still enforced for sensitive operations (PostgreSQL DELETE, etc.)
6. **Connection Timeout**: Server enforces expiry, closes WebSocket gracefully

## Future Enhancements

- **Compression**: Enable WebSocket permessage-deflate extension
- **Multiplexing**: Multiple logical connections over one WebSocket
- **Bandwidth Limiting**: Rate limit per connection
- **Connection Pooling**: Reuse backend connections for better performance

## References

- [RFC 6455 - WebSocket Protocol](https://datatracker.ietf.org/doc/html/rfc6455)
- [AWS ALB WebSocket Support](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-listeners.html#websocket-listener-rules)
- [Gorilla WebSocket](https://github.com/gorilla/websocket)


