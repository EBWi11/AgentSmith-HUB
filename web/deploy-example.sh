#!/bin/bash

# AgentSmith-HUB Frontend Deployment Script Example
# This script demonstrates how to deploy the frontend with runtime configuration

set -e

# Configuration
DEPLOYMENT_ENV=${DEPLOYMENT_ENV:-production}
BUILD_DIR=${BUILD_DIR:-dist}
TARGET_DIR=${TARGET_DIR:-/var/www/html}

echo "🚀 Starting AgentSmith-HUB Frontend Deployment"
echo "Environment: $DEPLOYMENT_ENV"
echo "Build directory: $BUILD_DIR"
echo "Target directory: $TARGET_DIR"

# Step 1: Build the application
echo "📦 Building application..."
npm run build

# Step 2: Create runtime configuration based on environment
echo "⚙️  Creating runtime configuration..."

case $DEPLOYMENT_ENV in
  "development")
    cat > $BUILD_DIR/config.json << EOF
{
  "apiBaseUrl": "http://localhost:8080",
  "apiTimeout": 30000,
  "enableDebugMode": true,
  "enableClusterMode": true,
  "theme": "light",
  "language": "en"
}
EOF
    ;;
  
  "staging")
    cat > $BUILD_DIR/config.json << EOF
{
  "apiBaseUrl": "https://staging-api.example.com:8080",
  "apiTimeout": 45000,
  "enableDebugMode": false,
  "enableClusterMode": true,
  "theme": "light",
  "language": "en"
}
EOF
    ;;
  
  "production")
    # Use environment variables for production
    export API_BASE_URL=${API_BASE_URL:-"https://api.example.com:8080"}
    export API_TIMEOUT=${API_TIMEOUT:-60000}
    export DEBUG_MODE=${DEBUG_MODE:-false}
    export CLUSTER_MODE=${CLUSTER_MODE:-true}
    export THEME=${THEME:-light}
    export LANGUAGE=${LANGUAGE:-en}
    
    # Generate config from template
    envsubst < config.template.json > $BUILD_DIR/config.json
    ;;
  
  *)
    echo "❌ Unknown environment: $DEPLOYMENT_ENV"
    exit 1
    ;;
esac

# Step 3: Deploy files
echo "📁 Deploying files..."
mkdir -p $TARGET_DIR
cp -r $BUILD_DIR/* $TARGET_DIR/

# Step 4: Set proper permissions
echo "🔐 Setting permissions..."
chmod -R 644 $TARGET_DIR/*
find $TARGET_DIR -type d -exec chmod 755 {} \;

# Step 5: Verify deployment
echo "✅ Verifying deployment..."
if [ -f "$TARGET_DIR/config.json" ]; then
  echo "Configuration file created successfully:"
  cat $TARGET_DIR/config.json | jq .
else
  echo "⚠️  No configuration file found, using default configuration"
fi

# Step 6: Test accessibility
if command -v curl &> /dev/null; then
  echo "🌐 Testing configuration endpoint..."
  if curl -s -f "$TARGET_DIR/config.json" > /dev/null; then
    echo "✅ Configuration endpoint accessible"
  else
    echo "⚠️  Configuration endpoint not accessible (this is normal for file-based deployments)"
  fi
fi

echo "🎉 Deployment completed successfully!"
echo ""
echo "📋 Deployment Summary:"
echo "- Environment: $DEPLOYMENT_ENV"
echo "- Target directory: $TARGET_DIR"
echo "- Configuration: $TARGET_DIR/config.json"
echo ""
echo "🔧 To modify configuration after deployment:"
echo "Edit $TARGET_DIR/config.json and restart your web server if needed"
echo ""
echo "💡 For containerized deployments, consider using environment variables:"
echo "docker run -e API_BASE_URL=https://your-api.com:8080 your-image" 