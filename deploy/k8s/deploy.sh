#!/bin/bash
set -e

echo "🚀 Deploying Port Authorizing to Kubernetes..."
echo ""

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check if kubectl is available
if ! command -v kubectl &> /dev/null; then
    echo "❌ kubectl not found. Please install kubectl first."
    exit 1
fi

# Check if Docker is available
if ! command -v docker &> /dev/null; then
    echo "❌ docker not found. Please install docker first."
    exit 1
fi

echo "Step 1: Building Docker image with Kubernetes support..."
docker build -t port-authorizing:latest -f ../../Dockerfile.k8s ../.. || {
    echo "❌ Docker build failed"
    exit 1
}
echo -e "${GREEN}✓ Image built successfully${NC}"
echo ""

echo "Step 2: Deploying to Kubernetes..."
kubectl apply -f namespace.yaml
kubectl apply -f rbac.yaml
kubectl apply -f configmap.yaml
kubectl apply -f deployment.yaml
kubectl apply -f service.yaml
echo -e "${GREEN}✓ Resources created${NC}"
echo ""

echo "Step 3: Waiting for pod to be ready..."
kubectl wait --for=condition=ready pod -l app=port-authorizing -n port-authorizing --timeout=120s || {
    echo ""
    echo -e "${YELLOW}⚠️  Pod not ready after 120s. Checking status...${NC}"
    kubectl get pods -n port-authorizing
    echo ""
    echo "Logs:"
    kubectl logs -n port-authorizing -l app=port-authorizing --tail=50
    exit 1
}
echo -e "${GREEN}✓ Pod is ready${NC}"
echo ""

echo "Step 4: Verifying deployment..."
kubectl get all -n port-authorizing
echo ""

echo -e "${GREEN}✅ Deployment complete!${NC}"
echo ""
echo "========================================="
echo "Access the application:"
echo "========================================="
echo ""
echo "Run port-forward:"
echo "  ${YELLOW}kubectl port-forward -n port-authorizing svc/port-authorizing 8080:8080${NC}"
echo ""
echo "Then access:"
echo "  • Admin UI:  http://localhost:8080/admin"
echo "  • API:       http://localhost:8080/api"
echo "  • Health:    http://localhost:8080/api/health"
echo ""
echo "Login credentials:"
echo "  • Username: admin"
echo "  • Password: admin123"
echo ""
echo "========================================="
echo "Useful commands:"
echo "========================================="
echo ""
echo "View logs:"
echo "  ${YELLOW}kubectl logs -n port-authorizing -l app=port-authorizing -f${NC}"
echo ""
echo "Check ConfigMaps (including versions):"
echo "  ${YELLOW}kubectl get configmaps -n port-authorizing${NC}"
echo ""
echo "View current config:"
echo "  ${YELLOW}kubectl get configmap -n port-authorizing port-authorizing-config -o yaml${NC}"
echo ""
echo "Clean up:"
echo "  ${YELLOW}kubectl delete namespace port-authorizing${NC}"
echo ""

