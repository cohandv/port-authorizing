.PHONY: build deploy undeploy logs shell port-forward test clean

# Docker image settings
IMAGE_NAME := port-authorizing
IMAGE_TAG := latest
NAMESPACE := port-authorizing

# Build Docker image with K8s support
build:
	@echo "Building Docker image with Kubernetes support..."
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) -f ../../Dockerfile.k8s ../..
	@echo "✓ Image built: $(IMAGE_NAME):$(IMAGE_TAG)"

# Deploy to Kubernetes
deploy:
	@echo "Deploying to Kubernetes..."
	kubectl apply -f namespace.yaml
	kubectl apply -f rbac.yaml
	kubectl apply -f configmap.yaml
	kubectl apply -f deployment.yaml
	kubectl apply -f service.yaml
	@echo "✓ Deployed to namespace: $(NAMESPACE)"
	@echo ""
	@echo "Waiting for pod to be ready..."
	kubectl wait --for=condition=ready pod -l app=$(IMAGE_NAME) -n $(NAMESPACE) --timeout=60s
	@echo ""
	@echo "✓ Deployment complete!"
	@echo ""
	@echo "Access the application:"
	@echo "  kubectl port-forward -n $(NAMESPACE) svc/$(IMAGE_NAME) 8080:8080"
	@echo "  Then open: http://localhost:8080/admin"

# Undeploy from Kubernetes
undeploy:
	@echo "Removing deployment..."
	kubectl delete -f service.yaml --ignore-not-found=true
	kubectl delete -f deployment.yaml --ignore-not-found=true
	kubectl delete -f configmap.yaml --ignore-not-found=true
	kubectl delete -f rbac.yaml --ignore-not-found=true
	kubectl delete -f namespace.yaml --ignore-not-found=true
	@echo "✓ Undeployed"

# View logs
logs:
	kubectl logs -n $(NAMESPACE) -l app=$(IMAGE_NAME) -f

# Get a shell in the pod
shell:
	kubectl exec -it -n $(NAMESPACE) $$(kubectl get pod -n $(NAMESPACE) -l app=$(IMAGE_NAME) -o jsonpath='{.items[0].metadata.name}') -- /bin/sh

# Port forward to local machine
port-forward:
	@echo "Port forwarding to localhost:8080..."
	@echo "Access admin UI: http://localhost:8080/admin"
	@echo "Login: admin / admin123"
	kubectl port-forward -n $(NAMESPACE) svc/$(IMAGE_NAME) 8080:8080

# Test the deployment
test:
	@echo "Testing deployment..."
	@echo ""
	@echo "1. Checking pod status..."
	kubectl get pods -n $(NAMESPACE)
	@echo ""
	@echo "2. Checking service..."
	kubectl get svc -n $(NAMESPACE)
	@echo ""
	@echo "3. Checking ConfigMap..."
	kubectl get configmap -n $(NAMESPACE)
	@echo ""
	@echo "4. Testing health endpoint..."
	kubectl run -it --rm --restart=Never curl-test --image=curlimages/curl:latest -n $(NAMESPACE) -- \
		curl -s http://$(IMAGE_NAME).$(NAMESPACE).svc.cluster.local:8080/api/health || echo "Health check failed"
	@echo ""
	@echo "✓ Tests complete"

# Clean everything including namespace
clean: undeploy
	@echo "Cleaning up Docker images..."
	docker rmi $(IMAGE_NAME):$(IMAGE_TAG) 2>/dev/null || true
	@echo "✓ Cleanup complete"

# Full deployment (build + deploy)
all: build deploy
	@echo ""
	@echo "🚀 Full deployment complete!"
	@echo ""
	@echo "Next steps:"
	@echo "  make port-forward  # Access the UI"
	@echo "  make logs          # View logs"
	@echo "  make test          # Run tests"

# Show status
status:
	@echo "=== Namespace ==="
	kubectl get namespace $(NAMESPACE) 2>/dev/null || echo "Not deployed"
	@echo ""
	@echo "=== Pods ==="
	kubectl get pods -n $(NAMESPACE) 2>/dev/null || echo "No pods"
	@echo ""
	@echo "=== Services ==="
	kubectl get svc -n $(NAMESPACE) 2>/dev/null || echo "No services"
	@echo ""
	@echo "=== ConfigMaps ==="
	kubectl get configmap -n $(NAMESPACE) 2>/dev/null || echo "No configmaps"
	@echo ""
	@echo "=== Events ==="
	kubectl get events -n $(NAMESPACE) --sort-by='.lastTimestamp' | tail -10 2>/dev/null || echo "No events"

# Help
help:
	@echo "Kubernetes Deployment Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make build          Build Docker image with K8s support"
	@echo "  make deploy         Deploy to Kubernetes"
	@echo "  make undeploy       Remove from Kubernetes"
	@echo "  make all            Build and deploy"
	@echo "  make logs           View application logs"
	@echo "  make port-forward   Port forward to localhost:8080"
	@echo "  make shell          Get shell in pod"
	@echo "  make test           Run tests"
	@echo "  make status         Show deployment status"
	@echo "  make clean          Clean everything"
	@echo "  make help           Show this help"

