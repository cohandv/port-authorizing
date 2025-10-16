# Kubernetes Deployment for Port Authorizing

This directory contains Kubernetes manifests for deploying Port Authorizing with Kubernetes ConfigMap-based storage backend.

## Prerequisites

1. **Local Kubernetes cluster** (Docker Desktop, Minikube, Kind, etc.)
2. **kubectl** configured to access your cluster
3. **Docker** for building the image

## Quick Start

### 1. Build Docker Image with K8s Support

The Kubernetes storage backend requires building with the `k8s` build tag:

```bash
# From the project root
docker build -t port-authorizing:latest \
  --build-arg BUILD_TAGS=k8s \
  -f Dockerfile.k8s .
```

**Note:** You'll need to create `Dockerfile.k8s` (see below) or modify the existing Dockerfile.

### 2. Deploy to Kubernetes

```bash
# Apply all manifests
kubectl apply -f deploy/k8s/

# Or apply in order:
kubectl apply -f deploy/k8s/namespace.yaml
kubectl apply -f deploy/k8s/rbac.yaml
kubectl apply -f deploy/k8s/configmap.yaml
kubectl apply -f deploy/k8s/deployment.yaml
kubectl apply -f deploy/k8s/service.yaml
```

### 3. Verify Deployment

```bash
# Check if pods are running
kubectl get pods -n port-authorizing

# Check logs
kubectl logs -n port-authorizing deployment/port-authorizing -f

# Check service
kubectl get svc -n port-authorizing
```

### 4. Access the Application

#### Option A: Port Forward (Recommended for testing)

```bash
kubectl port-forward -n port-authorizing svc/port-authorizing 8080:8080
```

Then access:
- **API**: http://localhost:8080/api
- **Admin UI**: http://localhost:8080/admin
- **Health**: http://localhost:8080/api/health

#### Option B: NodePort (For local clusters)

Change service type to `NodePort` in `service.yaml`:

```yaml
spec:
  type: NodePort
  ports:
    - name: http
      port: 8080
      targetPort: http
      nodePort: 30080  # Optional: specify port
```

Then apply and access via `http://<node-ip>:30080`

#### Option C: LoadBalancer (For cloud providers)

Change service type to `LoadBalancer` in `service.yaml` and get external IP:

```bash
kubectl get svc -n port-authorizing port-authorizing
```

## Testing the K8s Storage Backend

### 1. Login to Admin UI

```bash
# Port forward if needed
kubectl port-forward -n port-authorizing svc/port-authorizing 8080:8080

# Open browser
open http://localhost:8080/admin
```

Login with:
- **Username**: `admin`
- **Password**: `admin123`

### 2. Test Configuration Updates

1. Navigate to **Connections** tab
2. Add/edit a connection
3. Save changes
4. Check that the ConfigMap was updated:

```bash
kubectl get configmap -n port-authorizing port-authorizing-config -o yaml
```

You should see annotations with version metadata!

### 3. Test Version History

1. Make several configuration changes
2. Navigate to **Config Versions** tab
3. You should see version history
4. Test rollback functionality

### 4. Verify Version Backups

Check for version backup ConfigMaps:

```bash
# List all ConfigMaps (should see versioned backups)
kubectl get configmaps -n port-authorizing

# Example output:
# port-authorizing-config               # Current
# port-authorizing-config-20240115-143025  # Version 1
# port-authorizing-config-20240115-143126  # Version 2
```

### 5. Test Hot Reload

1. Make a config change via Admin UI
2. Watch logs to verify reload:

```bash
kubectl logs -n port-authorizing deployment/port-authorizing -f
```

3. Verify existing connections remain active

## Architecture

### Storage Backend

The app uses Kubernetes ConfigMap API to:
- **Read** current configuration from ConfigMap
- **Write** updates back to ConfigMap
- **Create** versioned backup ConfigMaps with timestamps
- **Rotate** old versions (keeps last 5)
- **Rollback** by copying old version to current

### RBAC Permissions

The ServiceAccount has permissions to:
- `get`, `list`, `watch` ConfigMaps (read current config)
- `create`, `update`, `patch` ConfigMaps (save changes)
- `delete` ConfigMaps (rotate old versions)

### Single Instance

**Important**: Run only 1 replica! Multiple replicas writing to the same ConfigMap would cause conflicts.

For HA, consider:
- Using a Secret with optimistic locking
- External configuration storage (etcd, database)
- Leader election for config writes

## Dockerfile with K8s Support

Create `Dockerfile.k8s`:

```dockerfile
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build with k8s tag
RUN CGO_ENABLED=0 GOOS=linux go build -tags k8s -o port-authorizing cmd/port-authorizing/main.go

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy binary
COPY --from=builder /build/port-authorizing .

# Create non-root user
RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser && \
    chown -R appuser:appuser /app

USER appuser

EXPOSE 8080

ENTRYPOINT ["/app/port-authorizing"]
CMD ["server"]
```

Build:

```bash
docker build -t port-authorizing:latest -f Dockerfile.k8s .
```

For local Kubernetes (Docker Desktop/Minikube), the image is already available.

For remote clusters, push to a registry:

```bash
docker tag port-authorizing:latest <registry>/port-authorizing:latest
docker push <registry>/port-authorizing:latest
```

## Troubleshooting

### Pod Not Starting

```bash
# Check pod status
kubectl describe pod -n port-authorizing <pod-name>

# Check logs
kubectl logs -n port-authorizing <pod-name>
```

Common issues:
- **Image not found**: Build the image with correct tag
- **RBAC errors**: Verify ServiceAccount and RoleBinding are applied
- **ConfigMap not found**: Apply configmap.yaml first

### Storage Backend Errors

Check logs for errors:

```bash
kubectl logs -n port-authorizing deployment/port-authorizing | grep -i "storage\|configmap"
```

Common errors:
- `kubernetes backend not available`: App not built with `-tags k8s`
- `failed to get configmap`: RBAC permissions missing
- `namespace is required`: ConfigMap missing or wrong namespace

### Configuration Not Updating

1. Check ConfigMap was updated:
   ```bash
   kubectl get configmap -n port-authorizing port-authorizing-config -o yaml
   ```

2. Check app reloaded:
   ```bash
   kubectl logs -n port-authorizing deployment/port-authorizing | tail -20
   ```

3. Restart pod if needed:
   ```bash
   kubectl rollout restart -n port-authorizing deployment/port-authorizing
   ```

## Cleanup

```bash
# Delete all resources
kubectl delete -f deploy/k8s/

# Or delete namespace (deletes everything)
kubectl delete namespace port-authorizing
```

## Production Considerations

### Security

1. **Change default passwords** in ConfigMap
2. **Use Secrets** instead of ConfigMap for sensitive data:
   ```yaml
   storage:
     type: kubernetes
     resource_type: secret  # Use Secret instead of ConfigMap
   ```
3. **Enable TLS** with Ingress
4. **Restrict RBAC** to minimum required permissions
5. **Use network policies** to limit access

### High Availability

1. **External storage**: Consider external database for config
2. **Leader election**: For multi-replica deployments
3. **Backup strategy**: Regularly backup ConfigMaps/Secrets
4. **Monitoring**: Add Prometheus metrics

### Scaling

1. **Horizontal Pod Autoscaler**: Scale based on CPU/memory
2. **Separate read/write** paths for config
3. **Cache configuration** in memory
4. **Distributed locking** for config updates

## Next Steps

1. **Add Ingress** for external access with TLS
2. **Configure monitoring** with Prometheus
3. **Set up GitOps** with ArgoCD/Flux for config management
4. **Implement backup** automation for ConfigMaps
5. **Add health checks** for config validation

