#!/bin/bash

# AgentSmith-HUB Deploy Script
# This script builds and prepares the project for deployment

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_step() {
    echo -e "${BLUE}[STEP]${NC} $1"
}

# Get script directory and project root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Change to project root
cd "$PROJECT_ROOT"

print_info "AgentSmith-HUB Deploy Script"
print_info "Project root: $PROJECT_ROOT"
print_info "Current version: $(cat VERSION 2>/dev/null || echo 'unknown')"
echo ""

# Check if make is available
if ! command -v make >/dev/null 2>&1; then
    print_error "make command not found. Please install build tools."
    exit 1
fi

# Parse command line arguments
BUILD_TYPE="all"
SKIP_DEPS=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --help|-h)
            echo "AgentSmith-HUB Deploy Script"
            echo ""
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --help, -h          Show this help message"
            echo "  --backend-only      Build backend only"
            echo "  --frontend-only     Build frontend only"
            echo "  --skip-deps         Skip dependency installation"
            echo "  --clean             Clean before build"
            echo ""
            echo "Examples:"
            echo "  $0                  # Full build with dependencies"
            echo "  $0 --clean          # Clean and full build"
            echo "  $0 --backend-only   # Build backend only"
            exit 0
            ;;
        --backend-only)
            BUILD_TYPE="backend"
            shift
            ;;
        --frontend-only)
            BUILD_TYPE="frontend"
            shift
            ;;
        --skip-deps)
            SKIP_DEPS=true
            shift
            ;;
        --clean)
            print_step "Cleaning previous builds..."
            make clean
            shift
            ;;
        *)
            print_error "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Install dependencies if not skipped
if [ "$SKIP_DEPS" = false ]; then
    print_step "Installing dependencies..."
    if ! make install-deps; then
        print_error "Failed to install dependencies"
        exit 1
    fi
    echo ""
fi

# Build based on type
case $BUILD_TYPE in
    "backend")
        print_step "Building backend..."
        if ! make backend; then
            print_error "Backend build failed"
            exit 1
        fi
        ;;
    "frontend")
        print_step "Building frontend..."
        if ! make frontend; then
            print_error "Frontend build failed"
            exit 1
        fi
        ;;
    "all")
        print_step "Building all components..."
        if ! make all; then
            print_error "Build failed"
            exit 1
        fi
        ;;
esac

echo ""
print_step "Creating deployment package..."
if ! make package; then
    print_error "Package creation failed"
    exit 1
fi

echo ""
print_step "Copying scripts to deployment directory..."
if [ -d "dist" ]; then
    mkdir -p dist/script
    cp "$SCRIPT_DIR/run.sh" dist/script/
    cp "$SCRIPT_DIR/stop.sh" dist/script/
    chmod +x dist/script/run.sh dist/script/stop.sh
    
    # Create convenience symlinks in dist root
    cd dist
    ln -sf script/run.sh run.sh
    ln -sf script/stop.sh stop.sh
    cd ..
fi

echo ""
print_info "✅ Build completed successfully!"
print_info "📦 Deployment files are ready in: dist/"
print_info "🚀 To start the service: cd dist && ./run.sh"
print_info "🛑 To stop the service: cd dist && ./stop.sh"

# Show deployment archive if created
if [ -f "agentsmith-hub-deployment.tar.gz" ]; then
    print_info "📦 Deployment archive: agentsmith-hub-deployment.tar.gz"
fi

echo ""
print_info "Deployment package contents:"
if [ -d "dist" ]; then
    ls -la dist/ | grep -E '^(d|-)' | awk '{print "  " $9 " (" $5 " bytes)"}'
fi 