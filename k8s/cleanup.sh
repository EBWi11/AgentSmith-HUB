#!/bin/bash

# AgentSmith-HUB Native Kubernetes Cleanup Script

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
read -p "Are you sure you want to continue? (y/N): " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    print_status "Cleanup cancelled."
    exit 0
fi

print_status "Starting cleanup of AgentSmith-HUB resources..."

# Delete all resources in the namespace
print_status "Deleting all resources in agentsmith-hub namespace..."
kubectl delete namespace agentsmith-hub --ignore-not-found=true

# Wait for namespace deletion to complete
print_status "Waiting for namespace deletion to complete..."
kubectl wait --for=delete namespace/agentsmith-hub --timeout=60s 2>/dev/null || true

print_status "Cleanup completed successfully!"
print_status "All AgentSmith-HUB resources have been removed from the cluster." 