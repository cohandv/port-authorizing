# Quick Start - Kubernetes Deployment

## TL;DR

```bash
cd deploy/k8s

# Option 1: Use Makefile
make all          # Build + Deploy
make port-forward # Access UI

# Option 2: Use script
./deploy.sh

# Option 3: Manual
docker build -t port-authorizing:latest -f ../../Dockerfile.k8s ../..
kubectl apply -f .
kubectl port-forward -n port-authorizing svc/port-authorizing 8080:8080
```

Then open: **http://localhost:8080/admin**
- Username: `admin`
- Password: `admin123`

## What This Deploys

1. **Namespace**: `port-authorizing`
2. **ServiceAccount** with RBAC for ConfigMap read/write
3. **ConfigMap**: Initial configuration (managed by the app itself!)
4. **Deployment**: Single pod (do not scale beyond 1 replica)
5. **Service**: ClusterIP on port 8080

## Test the K8s Storage Backend

### 1. Access Admin UI

```bash
kubectl port-forward -n port-authorizing svc/port-authorizing 8080:8080
```

Open http://localhost:8080/admin and login.

### 2. Make Configuration Changes

- Go to **Connections** tab
- Add/edit a connection
- Click Save

### 3. Verify ConfigMap Updated

```bash
kubectl get configmap -n port-authorizing port-authorizing-config -o yaml
```

You should see:
- Updated configuration in `data.config.yaml`
- New annotations with version metadata
- Timestamp of the change

### 4. Check Version History

```bash
# List all ConfigMaps (versions show as separate ConfigMaps)
kubectl get configmaps -n port-authorizing

# Should show:
# port-authorizing-config                  <- Current
# port-authorizing-config-20240115-143025  <- Version 1
# port-authorizing-config-20240115-143126  <- Version 2
```

### 5. Test Rollback

In the Admin UI:
1. Go to **Config Versions** tab
2. Click **Rollback** on an older version
3. Verify config reverted

## Common Commands

```bash
# View logs
kubectl logs -n port-authorizing -l app=port-authorizing -f

# Get pod status
kubectl get pods -n port-authorizing

# Exec into pod
kubectl exec -it -n port-authorizing deployment/port-authorizing -- /bin/sh

# Restart deployment
kubectl rollout restart -n port-authorizing deployment/port-authorizing

# Clean up
kubectl delete namespace port-authorizing
```

## Troubleshooting

### "kubernetes backend not available"

**Cause**: App not built with `-tags k8s`

**Fix**:
```bash
docker build -t port-authorizing:latest -f Dockerfile.k8s .
```

### Pod CrashLoopBackOff

**Check logs**:
```bash
kubectl logs -n port-authorizing -l app=port-authorizing
```

Common causes:
- ConfigMap not found (apply configmap.yaml first)
- RBAC permissions missing (apply rbac.yaml)
- Image not found (build with correct tag)

### Can't Access UI

**Ensure port-forward is running**:
```bash
kubectl port-forward -n port-authorizing svc/port-authorizing 8080:8080
```

**Check service**:
```bash
kubectl get svc -n port-authorizing
```

## Next Steps

1. ✅ **Test version management** - make changes and check ConfigMap versions
2. ✅ **Test rollback** - revert to previous version
3. ✅ **Check audit logs** - view in Admin UI (goes to stdout in K8s)
4. 📝 **Customize config** - edit configmap.yaml for your needs
5. 🔒 **Secure it** - change passwords, use Secrets, add TLS

## Architecture Notes

- **Single instance only**: No horizontal scaling (ConfigMap doesn't support multi-writer)
- **Version backups**: Creates new ConfigMaps with timestamp suffixes
- **Hot reload**: Config changes apply without pod restart
- **Existing connections**: Preserved during config reload
- **RBAC**: Minimal permissions (only ConfigMap access in same namespace)

Enjoy! 🎉

