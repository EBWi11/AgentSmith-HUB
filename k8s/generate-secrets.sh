#!/bin/bash

# AgentSmith-HUB Kubernetes Secret Generator
# This script generates random strong passwords for Kubernetes secrets

set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

print_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

# Function to generate random password
generate_password() {
    # Generate a strong password with:
    # - 24 characters
    # - Mix of uppercase, lowercase, numbers, and special characters
    # Use gshuf on macOS, shuf on Linux
    if command -v gshuf >/dev/null 2>&1; then
        SHUF_CMD="gshuf"
    elif command -v shuf >/dev/null 2>&1; then
        SHUF_CMD="shuf"
    else
        # Fallback for systems without shuf
        openssl rand -base64 32 | tr -d "=+/" | cut -c1-32 | sed 's/^/Ag3ntSm1th-Hub-/' | sed 's/$/-2024!@#/'
        return
    fi
    
    openssl rand -base64 18 | tr -d "=+/" | cut -c1-24 | sed 's/./&\n/g' | $SHUF_CMD | tr -d '\n' | sed 's/^/Ag3ntSm1th-Hub-/' | sed 's/$/-2024!@#/'
}

# Function to encode to base64
encode_base64() {
    echo -n "$1" | base64
}

print_info "Generating random strong passwords for AgentSmith-HUB..."

# Generate Redis password
REDIS_PASSWORD=$(generate_password)
REDIS_PASSWORD_B64=$(encode_base64 "$REDIS_PASSWORD")

# Generate AgentSmith token (UUID format)
AGENTSMITH_TOKEN=$(uuidgen)
AGENTSMITH_TOKEN_B64=$(encode_base64 "$AGENTSMITH_TOKEN")

print_info "Generated passwords:"
echo ""
echo "=== Redis Password ==="
echo "Plain: $REDIS_PASSWORD"
echo "Base64: $REDIS_PASSWORD_B64"
echo ""
echo "=== AgentSmith Token ==="
echo "Plain: $AGENTSMITH_TOKEN"
echo "Base64: $AGENTSMITH_TOKEN_B64"
echo ""

# Create secrets.yaml file
print_info "Creating secrets.yaml file..."
cat > secrets.yaml << EOF
---
# AgentSmith-HUB Kubernetes Secrets
# Generated on: $(date)
# WARNING: Keep this file secure and do not commit to version control

apiVersion: v1
kind: Secret
metadata:
  name: agentsmith-hub-redis-secret
  namespace: agentsmith-hub
type: Opaque
data:
  password: "$REDIS_PASSWORD_B64"

---
apiVersion: v1
kind: Secret
metadata:
  name: agentsmith-hub-token-secret
  namespace: agentsmith-hub
type: Opaque
data:
  token: "$AGENTSMITH_TOKEN_B64"
EOF

print_info "Secrets file created: secrets.yaml"
print_warning "IMPORTANT: Keep this file secure and do not commit it to version control!"
echo ""
print_info "To apply these secrets:"
echo "kubectl apply -f secrets.yaml"
echo ""
print_info "To use these values in your deployment:"
echo "1. Update k8s-deployment.yaml with the new token value"
echo "2. The Redis password will be automatically used from the secret"
echo ""
print_info "Generated values summary:"
echo "- Redis Password: $REDIS_PASSWORD"
echo "- AgentSmith Token: $AGENTSMITH_TOKEN" 