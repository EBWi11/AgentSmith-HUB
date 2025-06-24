#!/bin/bash

# AgentSmith-HUB Run Script
# This script starts the AgentSmith-HUB services

set -e

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Configuration
CONFIG_ROOT="config"
BINARY_NAME="agentsmith-hub"
BUILD_DIR="build"
DIST_DIR="dist"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
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

# Function to check if binary exists in different locations
find_binary() {
    # Check dist directory first (production build)
    if [ -f "$DIST_DIR/$BINARY_NAME" ]; then
        echo "$DIST_DIR/$BINARY_NAME"
        return 0
    fi
    
    # Check build directory (development build)
    if [ -f "$BUILD_DIR/$BINARY_NAME" ]; then
        echo "$BUILD_DIR/$BINARY_NAME"
        return 0
    fi
    
    # Check current directory
    if [ -f "$BINARY_NAME" ]; then
        echo "$BINARY_NAME"
        return 0
    fi
    
    return 1
}

# Function to set library path
setup_library_path() {
    local binary_dir="$(dirname "$1")"
    
    # Check for lib directory relative to binary
    if [ -d "$binary_dir/lib" ]; then
        export LD_LIBRARY_PATH="$binary_dir/lib:${LD_LIBRARY_PATH}"
        print_info "Using library path: $binary_dir/lib"
    elif [ -d "lib/linux" ]; then
        export LD_LIBRARY_PATH="$(pwd)/lib/linux:${LD_LIBRARY_PATH}"
        print_info "Using library path: $(pwd)/lib/linux"
    else
        print_warn "No lib directory found, continuing without setting LD_LIBRARY_PATH"
    fi
}

# Function to check config directory
check_config() {
    local binary_dir="$(dirname "$1")"
    
    # Check for config directory relative to binary
    if [ -d "$binary_dir/$CONFIG_ROOT" ]; then
        CONFIG_ROOT="$binary_dir/$CONFIG_ROOT"
        print_info "Using config directory: $CONFIG_ROOT"
    elif [ -d "$CONFIG_ROOT" ]; then
        print_info "Using config directory: $(pwd)/$CONFIG_ROOT"
    else
        print_error "Config directory not found!"
        print_error "Please ensure the config directory is present with proper configuration files."
        echo ""
        echo "Expected locations:"
        echo "  - $binary_dir/$CONFIG_ROOT"
        echo "  - $(pwd)/$CONFIG_ROOT"
        exit 1
    fi
}

# Main function
main() {
    print_info "Starting AgentSmith-HUB..."
    
    # Find binary
    BINARY_PATH=$(find_binary)
    if [ $? -ne 0 ]; then
        print_error "AgentSmith-HUB binary not found!"
        echo ""
        echo "Please build the project first:"
        echo "  make all          # For production build"
        echo "  make backend      # For development build"
        echo ""
        echo "Or ensure the binary is in one of these locations:"
        echo "  - $DIST_DIR/$BINARY_NAME"
        echo "  - $BUILD_DIR/$BINARY_NAME"
        echo "  - ./$BINARY_NAME"
        exit 1
    fi
    
    print_info "Found binary: $BINARY_PATH"
    
    # Determine run mode
    if [ -n "$LEADER_ADDR" ]; then
        print_info "Running in FOLLOWER mode, connecting to leader: $LEADER_ADDR"
        RUN_MODE="follower"
    else
        print_info "Running in LEADER mode (default)"
        RUN_MODE="leader"
        # Check and setup configuration for leader mode
        check_config "$BINARY_PATH"
    fi
    
    # Setup library path
    setup_library_path "$BINARY_PATH"
    
    # Make binary executable
    chmod +x "$BINARY_PATH"
    
    # Show version information
    print_info "Version information:"
    if "$BINARY_PATH" -version 2>/dev/null; then
        echo ""
    else
        print_warn "Could not retrieve version information"
    fi
    
    print_info "Working directory: $SCRIPT_DIR"
    print_info "Library path: ${LD_LIBRARY_PATH:-'not set'}"
    
    if [ "$RUN_MODE" = "leader" ]; then
        print_info "Config root: $CONFIG_ROOT"
    fi
    echo ""
    
    # Start the application based on mode
    if [ "$RUN_MODE" = "follower" ]; then
        print_info "Starting AgentSmith-HUB in FOLLOWER mode..."
        print_info "Connecting to leader: $LEADER_ADDR"
        print_info "Press Ctrl+C to stop"
        echo ""
        
        # Start as follower
        cd "$(dirname "$BINARY_PATH")"
        exec "./$BINARY_NAME" -leader "$LEADER_ADDR"
    else
        print_info "Starting AgentSmith-HUB in LEADER mode..."
        print_info "Web interface will be available at: http://localhost:8080"
        print_info "Press Ctrl+C to stop"
        echo ""
        
        # Calculate relative config path from binary location
        BINARY_DIR="$(dirname "$BINARY_PATH")"
        RELATIVE_CONFIG_ROOT=$(realpath --relative-to="$BINARY_DIR" "$CONFIG_ROOT")
        
        # Start as leader
        cd "$(dirname "$BINARY_PATH")"
        exec "./$BINARY_NAME" -config_root "$RELATIVE_CONFIG_ROOT"
    fi
}

# Parse command line arguments
LEADER_ADDR=""
while [[ $# -gt 0 ]]; do
    case $1 in
        --help|-h)
            echo "AgentSmith-HUB Run Script"
            echo ""
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --help, -h           Show this help message"
            echo "  --version, -v        Show version information and exit"
            echo "  --check, -c          Check dependencies and configuration"
            echo "  --follower ADDR      Run as follower, connecting to leader at ADDR"
            echo ""
            echo "Default Mode: Leader (requires config directory)"
            echo "Follower Mode: --follower <leader_address>"
            echo ""
            echo "This script automatically detects the binary location and configuration."
            echo "It will look for the binary in the following order:"
            echo "  1. $DIST_DIR/$BINARY_NAME (production build)"
            echo "  2. $BUILD_DIR/$BINARY_NAME (development build)"
            echo "  3. ./$BINARY_NAME (current directory)"
            echo ""
            echo "Examples:"
            echo "  $0                           # Start as leader (default)"
            echo "  $0 --follower 192.168.1.100 # Start as follower"
            echo ""
            exit 0
            ;;
        --follower)
            LEADER_ADDR="$2"
            shift 2
            ;;
        --version|-v)
            BINARY_PATH=$(find_binary)
            if [ $? -eq 0 ]; then
                "$BINARY_PATH" -version
            else
                print_error "Binary not found, cannot show version"
                exit 1
            fi
            exit 0
            ;;
        --check|-c)
            print_info "Checking dependencies and configuration..."
            
            # Check binary
            BINARY_PATH=$(find_binary)
            if [ $? -eq 0 ]; then
                print_info "✓ Binary found: $BINARY_PATH"
            else
                print_error "✗ Binary not found"
            fi
            
            # Check config
            if check_config "${BINARY_PATH:-./}" 2>/dev/null; then
                print_info "✓ Config directory found: $CONFIG_ROOT"
            else
                print_error "✗ Config directory not found"
            fi
            
            # Check libraries
            setup_library_path "${BINARY_PATH:-./}"
            if [ -n "${LD_LIBRARY_PATH:-}" ]; then
                print_info "✓ Library path set: $LD_LIBRARY_PATH"
            else
                print_warn "⚠ Library path not set (may be okay for some builds)"
            fi
            
            exit 0
            ;;
        *)
            print_error "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done


# Run main function
main "$@" 