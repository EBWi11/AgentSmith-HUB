#!/bin/bash

# AgentSmith-HUB Unified Kubernetes Cleanup Script
# Cleans up all resources including persistent storage

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

print_warning "This will delete all AgentSmith-HUB resources from the agentsmith-hub namespace."
print_warning "This includes persistent storage (PVC) which will permanently delete all configuration data."
read -p "Are you sure you want to continue? (y/N): " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    print_status "Cleanup cancelled."
    exit 0
fi

print_status "Starting cleanup of AgentSmith-HUB resources..."

# Check if namespace exists
if kubectl get namespace agentsmith-hub &> /dev/null; then
    print_status "Found agentsmith-hub namespace, proceeding with cleanup..."
    
    # Delete PVCs explicitly to ensure proper cleanup
    print_status "Deleting Persistent Volume Claims..."
    kubectl delete pvc --all -n agentsmith-hub --ignore-not-found=true
    
    # Delete all other resources in the namespace
    print_status "Deleting all other resources in agentsmith-hub namespace..."
    kubectl delete all --all -n agentsmith-hub --ignore-not-found=true
    
    # Delete ConfigMaps and Secrets
    print_status "Deleting ConfigMaps and Secrets..."
    kubectl delete configmap --all -n agentsmith-hub --ignore-not-found=true
    kubectl delete secret --all -n agentsmith-hub --ignore-not-found=true
    
    # Delete Ingress
    print_status "Deleting Ingress..."
    kubectl delete ingress --all -n agentsmith-hub --ignore-not-found=true
    
    # Finally delete the namespace
    print_status "Deleting agentsmith-hub namespace..."
    kubectl delete namespace agentsmith-hub --ignore-not-found=true
    
    # Wait for namespace deletion to complete
    print_status "Waiting for namespace deletion to complete..."
    kubectl wait --for=delete namespace/agentsmith-hub --timeout=60s 2>/dev/null || true
    
else
    print_status "agentsmith-hub namespace not found, nothing to clean up."
fi

print_status "Cleanup completed successfully!"
print_status "All AgentSmith-HUB resources including persistent storage have been removed from the cluster." 