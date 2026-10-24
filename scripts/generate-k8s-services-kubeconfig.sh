#!/bin/bash
# Generate a kubeconfig file for the port-authorizing-discovery service account
# This kubeconfig can be used for k8s_services connections without needing AWS CLI
#
# Usage: ./generate-k8s-services-kubeconfig.sh [output-file]
#
# Prerequisites:
#   1. kubectl configured and connected to your cluster
#   2. RBAC resources applied: kubectl apply -f deploy/k8s/k8s-services-rbac.yaml

set -e

OUTPUT_FILE="${1:-kubeconfig-port-authorizing.yaml}"
NAMESPACE="port-authorizing"
SERVICE_ACCOUNT="port-authorizing-discovery"
TOKEN_DURATION="8760h"  # 1 year

echo "🔧 Generating kubeconfig for k8s_services..."

# Check if namespace exists
if ! kubectl get namespace "$NAMESPACE" &>/dev/null; then
    echo "❌ Namespace '$NAMESPACE' not found. Apply RBAC first:"
    echo "   kubectl apply -f deploy/k8s/k8s-services-rbac.yaml"
    exit 1
fi

# Get cluster info from current context
CURRENT_CONTEXT=$(kubectl config current-context)
CLUSTER_NAME=$(kubectl config view -o jsonpath="{.contexts[?(@.name=='$CURRENT_CONTEXT')].context.cluster}")
CLUSTER_SERVER=$(kubectl config view -o jsonpath="{.clusters[?(@.name=='$CLUSTER_NAME')].cluster.server}")
CLUSTER_CA=$(kubectl config view --raw -o jsonpath="{.clusters[?(@.name=='$CLUSTER_NAME')].cluster.certificate-authority-data}")

# Generate a long-lived token
echo "📝 Creating token (valid for $TOKEN_DURATION)..."
TOKEN=$(kubectl create token "$SERVICE_ACCOUNT" -n "$NAMESPACE" --duration="$TOKEN_DURATION")

# Generate kubeconfig
cat > "$OUTPUT_FILE" <<EOF
apiVersion: v1
kind: Config
current-context: port-authorizing
clusters:
  - name: port-authorizing-cluster
    cluster:
      server: $CLUSTER_SERVER
      certificate-authority-data: $CLUSTER_CA
contexts:
  - name: port-authorizing
    context:
      cluster: port-authorizing-cluster
      user: port-authorizing-discovery
users:
  - name: port-authorizing-discovery
    user:
      token: $TOKEN
EOF

echo "✅ Kubeconfig generated: $OUTPUT_FILE"
echo ""
echo "📋 To use with Docker:"
echo "   1. Copy to a location Docker can access:"
echo "      cp $OUTPUT_FILE ~/.kube/port-authorizing-config"
echo ""
echo "   2. Update docker-compose.yml volume mount:"
echo "      - ~/.kube/port-authorizing-config:/home/portauth/.kube/config:ro"
echo ""
echo "   3. Or set KUBECONFIG env var:"
echo "      export KUBECONFIG=$PWD/$OUTPUT_FILE"
echo ""
echo "📋 To test:"
echo "   KUBECONFIG=$OUTPUT_FILE kubectl get namespaces"
echo "   KUBECONFIG=$OUTPUT_FILE kubectl get services --all-namespaces"
