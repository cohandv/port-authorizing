# Kubernetes Services Discovery

Port Authorizing can automatically discover and proxy Kubernetes services across namespaces, making them accessible to developers through secure, controlled connections.

## Overview

The `k8s_services` connection type enables:

- **Automatic service discovery** across all namespaces (with configurable filters)
- **Dynamic port forwarding** for each discovered service
- **Protocol-aware proxying** with whitelist/approval support for HTTP services
- **Service annotations** for per-service configuration

## Quick Start

### 1. Create a k8s_services Connection

In your `config.yaml`:

```yaml
connections:
  - name: k8s-dev
    type: k8s_services
    tags:
      - env:dev
    k8s_include_namespaces:
      - default
      - app
      - staging
    # Or use exclusions (if include is empty):
    # k8s_exclude_namespaces:
    #   - kube-system
    #   - monitoring
```

### 2. Configure Port Mappings (Optional)

Specify which protocol to use for each port to enable whitelist filtering:

```yaml
connections:
  - name: k8s-dev
    type: k8s_services
    tags:
      - env:dev
    k8s_port_mappings:
      - port_name: http
        protocol: http        # Apply HTTP whitelist rules
      - port_name: web
        protocol: http
      - port: 8080
        protocol: http
      - port_name: postgres
        protocol: postgres    # PostgreSQL protocol
      - port_name: redis
        protocol: redis       # Redis protocol
```

**Important:** Without explicit port mappings, all ports default to **TCP** (raw byte forwarding with no filtering).

### 3. Connect from CLI

```bash
port-authorizing connect k8s-dev
```

This will:
1. Discover all services in allowed namespaces
2. Open a local port for each service/port combination
3. Display an index page at the base port (e.g., `http://localhost:9001`)

## Service Annotations

Service owners can configure port-authorizer behavior directly on their Kubernetes services using annotations:

### Protocol Type Annotation

Specify the protocol handler for a port:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-api
  annotations:
    # By port number
    cohandv.port-authorizer.io/port.8080.type: http
    cohandv.port-authorizer.io/port.5432.type: postgres
    
    # By port name
    cohandv.port-authorizer.io/port.web.type: http
    cohandv.port-authorizer.io/port.grpc.type: tcp
spec:
  ports:
    - name: web
      port: 8080
    - name: grpc
      port: 9090
```

**Supported protocols:**
- `http` - HTTP proxy with whitelist support
- `https` - HTTPS proxy with whitelist support
- `postgres` - PostgreSQL protocol
- `redis` - Redis protocol
- `tcp` - Raw TCP (default, no filtering)

### Expose/Hide Annotation

Control whether a port is exposed to developers:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-api
  annotations:
    # Hide admin port from discovery
    cohandv.port-authorizer.io/port.admin.expose: "false"
    cohandv.port-authorizer.io/port.8081.expose: "false"
    
    # Explicitly expose (default is true)
    cohandv.port-authorizer.io/port.http.expose: "true"
spec:
  ports:
    - name: http
      port: 8080
    - name: admin
      port: 8081
    - name: metrics
      port: 9090
```

**Default behavior:** All ports are exposed unless explicitly set to `"false"`.

## Configuration Priority

Protocol determination follows this priority order:

1. **Connection config** (`k8s_port_mappings`) - Highest priority
2. **Service annotations** (`cohandv.port-authorizer.io/port.*.type`)
3. **Default to TCP** - No filtering

This allows:
- Organization-wide defaults via connection config
- Service-specific overrides via annotations
- Safe fallback to TCP for unknown protocols

## Admin UI Configuration

The admin UI supports full configuration of k8s_services connections:

1. Go to **Connections** tab
2. Click **Add Connection**
3. Select **K8s Services Discovery** as the type
4. Configure:
   - **Include Namespaces** - Only discover services in these namespaces
   - **Exclude Namespaces** - Exclude these namespaces (if include is empty)
   - **Port Mappings** - Click "+ Add Port Mapping" to configure protocols

## RBAC Requirements

The port-authorizing server needs permissions to discover services. Apply the following RBAC:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: port-authorizing-discovery
  namespace: port-authorizing
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: port-authorizing-discovery
rules:
  - apiGroups: [""]
    resources: ["namespaces"]
    verbs: ["list"]
  - apiGroups: [""]
    resources: ["services", "endpoints"]
    verbs: ["get", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: port-authorizing-discovery
subjects:
  - kind: ServiceAccount
    name: port-authorizing-discovery
    namespace: port-authorizing
roleRef:
  kind: ClusterRole
  name: port-authorizing-discovery
  apiGroup: rbac.authorization.k8s.io
```

## Example: Full Service Configuration

Here's a complete example of a Kubernetes service with port-authorizer annotations:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: backend-api
  namespace: app
  labels:
    app: backend
  annotations:
    # Configure port-authorizer protocols
    cohandv.port-authorizer.io/port.http.type: http
    cohandv.port-authorizer.io/port.grpc.type: tcp
    cohandv.port-authorizer.io/port.metrics.expose: "false"
spec:
  selector:
    app: backend
  ports:
    - name: http
      port: 8080
      targetPort: 8080
    - name: grpc
      port: 9090
      targetPort: 9090
    - name: metrics      # Hidden from port-authorizer
      port: 9091
      targetPort: 9091
```

## How It Works

### Architecture

```
┌─────────────────┐     WebSocket      ┌────────────────────┐
│   CLI Client    │◄──────────────────►│  Port-Authorizing  │
│                 │                    │       Server       │
│  Local Ports:   │                    │                    │
│  - 9001 (index) │                    │  ┌──────────────┐  │
│  - 9002 (svc1)  │                    │  │ K8s Discovery│  │
│  - 9003 (svc2)  │                    │  │              │  │
│  - ...          │                    │  └──────┬───────┘  │
└─────────────────┘                    │         │          │
                                       │         ▼          │
                                       │  ┌──────────────┐  │
                                       │  │ K8s Services │  │
                                       │  │ (in cluster) │  │
                                       │  └──────────────┘  │
                                       └────────────────────┘
```

### Flow

1. **Connection Request**: CLI requests a k8s_services connection
2. **Discovery**: Server discovers all services in allowed namespaces
3. **Service List**: Server sends service list to CLI over WebSocket
4. **Local Ports**: CLI allocates a local port for each service/port
5. **Index Page**: CLI serves an HTML index at the base port
6. **Proxying**: When traffic hits a local port:
   - CLI forwards to server over WebSocket
   - Server applies whitelist/approval rules
   - Server forwards to the K8s service
   - Response flows back through the same path

### Protocol Handling

For **HTTP/HTTPS** ports (when configured):
- Server buffers incoming data until a complete HTTP request is received
- Request is parsed and validated against whitelist rules
- If allowed, request is forwarded to the backend

For **TCP** ports (default):
- Raw byte forwarding with no inspection
- No whitelist rules apply
- Useful for databases, custom protocols, etc.

## Whitelist Rules

HTTP whitelist rules apply to k8s_services connections. Configure them in the **Whitelisted Policies** section:

```yaml
policies:
  - name: allow-read-apis
    connections: ["k8s-dev"]
    patterns:
      - "^GET /api/.*"
      - "^GET /health"
    roles: []  # Empty = all roles
```

## Troubleshooting

### Services Not Discovered

1. Check RBAC permissions are applied
2. Verify namespace filters in connection config
3. Check server logs for discovery errors:
   ```bash
   kubectl logs -n port-authorizing deployment/port-authorizing | grep k8s_services
   ```

### Connection Hanging

1. Verify the server can reach K8s services (network/DNS)
2. Check if the service has endpoints:
   ```bash
   kubectl get endpoints -n <namespace> <service>
   ```
3. Verify port mapping is correct

### Whitelist Not Applied

1. Ensure port mapping specifies `protocol: http` or `protocol: https`
2. Check whitelist patterns in policies
3. Verify the connection is authorized for the user

### Port Hidden Unexpectedly

1. Check service annotations for `expose: "false"`
2. Verify annotation key format matches exactly:
   ```
   cohandv.port-authorizer.io/port.<port>.expose
   ```

## Best Practices

1. **Use include lists** in production to limit scope
2. **Configure port mappings** for HTTP services to enable whitelist
3. **Add annotations** to services for self-documenting configuration
4. **Hide sensitive ports** (admin, metrics) via annotations
5. **Use approval workflows** for production namespaces
