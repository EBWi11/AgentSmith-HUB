#!/bin/bash

# AgentSmith-HUB Native Kubernetes Deployment Script
# Simple deployment without Helm complexity

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

print_status "Starting AgentSmith-HUB deployment..."

# Step 1: Create namespace
print_status "Creating namespace..."
kubectl apply -f k8s-deployment.yaml --dry-run=client -o yaml | kubectl apply -f -

# Step 2: Wait for namespace to be ready
print_status "Waiting for namespace to be ready..."
kubectl wait --for=condition=Ready namespace/agentsmith-hub --timeout=30s

# Step 3: Deploy all resources
print_status "Deploying AgentSmith-HUB components..."
kubectl apply -f k8s-deployment.yaml

# Step 4: Wait for deployments to be ready
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

# Wait for Frontend
print_status "Waiting for Frontend deployment..."
kubectl wait --for=condition=available --timeout=300s deployment/agentsmith-hub-frontend -n agentsmith-hub

# Step 5: Show deployment status
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
echo "1. Make sure you have the correct Docker images available:"
echo "   - agentsmith-hub:0.1.6"
echo "   - agentsmith-hub-frontend:latest"
echo "   - redis:7-alpine"
echo ""
echo "2. Update the ConfigMap with your actual configuration files"
echo "3. Update the hostname in Ingress if needed"
echo "4. The default token is: 9ef0c170-069e-44dd-a406-2d85eca0a0b2"
echo ""
print_status "To access the application:"
echo "- Frontend: kubectl port-forward svc/agentsmith-hub-frontend 8080:80 -n agentsmith-hub"
echo "- API: kubectl port-forward svc/agentsmith-hub-leader 8081:8080 -n agentsmith-hub"
echo ""
print_status "To view logs:"
echo "- Leader: kubectl logs -f deployment/agentsmith-hub-leader -n agentsmith-hub"
echo "- Follower: kubectl logs -f deployment/agentsmith-hub-follower -n agentsmith-hub"
echo "- Frontend: kubectl logs -f deployment/agentsmith-hub-frontend -n agentsmith-hub"
echo "- Redis: kubectl logs -f deployment/agentsmith-hub-redis -n agentsmith-hub" 