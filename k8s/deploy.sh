#!/bin/bash

# AgentSmith-HUB Unified Kubernetes Deployment Script
# Single image deployment for both leader and follower modes

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if kubectl is available
if ! command -v kubectl &> /dev/null; then
    print_error "kubectl is not installed or not in PATH"
    exit 1
fi

# Check if we can connect to Kubernetes cluster
if ! kubectl cluster-info &> /dev/null; then
    print_error "Cannot connect to Kubernetes cluster. Please check your kubeconfig."
    exit 1
fi

print_status "Starting AgentSmith-HUB unified deployment..."

# Step 1: Deploy all components (Redis, Leader, Follower)
print_status "Deploying AgentSmith-HUB components..."
kubectl apply -f k8s-deployment.yaml

# Step 2: Wait for deployments to be ready
print_status "Waiting for deployments to be ready..."

# Wait for Redis
print_status "Waiting for Redis deployment..."
kubectl wait --for=condition=available --timeout=300s deployment/agentsmith-hub-redis -n agentsmith-hub

# Wait for Leader
print_status "Waiting for Leader deployment..."
kubectl wait --for=condition=available --timeout=300s deployment/agentsmith-hub-leader -n agentsmith-hub

# Wait for Followers
print_status "Waiting for Follower deployments..."
kubectl wait --for=condition=available --timeout=300s deployment/agentsmith-hub-follower -n agentsmith-hub

# Step 3: Show deployment status
print_status "Deployment completed! Showing status..."
echo ""
kubectl get all -n agentsmith-hub

echo ""
print_status "Services:"
kubectl get services -n agentsmith-hub

echo ""
print_status "Pods:"
kubectl get pods -n agentsmith-hub

echo ""
print_warning "Important notes:"
echo "1. Make sure you have the correct Docker image available:"
echo "   - ghcr.io/ebwi11/agentsmith-hub:latest"
echo "   - redis:7-alpine"
echo ""
echo "2. Image address is already configured: ghcr.io/ebwi11/agentsmith-hub:latest"
echo "3. Leader configuration is persisted using PVC"
echo "4. The default token is: 9ef0c170-069e-44dd-a406-2d85eca0a0b2"
echo ""
print_status "To access the application:"
echo "- Frontend: kubectl port-forward svc/agentsmith-hub-leader 8080:80 -n agentsmith-hub"
echo "- API: kubectl port-forward svc/agentsmith-hub-leader 8081:8080 -n agentsmith-hub"
echo ""
print_status "To view logs:"
echo "- Leader: kubectl logs -f deployment/agentsmith-hub-leader -n agentsmith-hub"
echo "- Follower: kubectl logs -f deployment/agentsmith-hub-follower -n agentsmith-hub"
echo "- Redis: kubectl logs -f deployment/agentsmith-hub-redis -n agentsmith-hub" 