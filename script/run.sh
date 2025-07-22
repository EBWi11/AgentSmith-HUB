#!/bin/bash

# AgentSmith-HUB Run Script
# This script starts the AgentSmith-HUB services

set -e

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Configuration
CONFIG_ROOT="/opt/hub_config"
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

# Function to detect system architecture
detect_architecture() {
    local arch=$(uname -m)
    case "$arch" in
        x86_64)
            echo "amd64"
            ;;
        aarch64|arm64)
            echo "arm64"
            ;;
        *)
            print_warn "Unknown architecture: $arch, defaulting to amd64"
            echo "amd64"
            ;;
    esac
}

# Function to set library path with architecture detection
setup_library_path() {
    local binary_dir="$(dirname "$1")"
    local system_arch=$(detect_architecture)
    
    # Check for lib directory relative to binary (preferred)
    if [ -d "$binary_dir/lib" ]; then
        export LD_LIBRARY_PATH="$binary_dir/lib:${LD_LIBRARY_PATH}"
        print_info "Using library path: $binary_dir/lib"
    # Check for architecture-specific lib directory in project root
    elif [ -d "lib/linux/$system_arch" ]; then
        export LD_LIBRARY_PATH="$(pwd)/lib/linux/$system_arch:${LD_LIBRARY_PATH}"
        print_info "Using library path: $(pwd)/lib/linux/$system_arch (detected architecture: $system_arch)"
    # Fallback to generic linux lib directory
    elif [ -d "lib/linux" ]; then
        export LD_LIBRARY_PATH="$(pwd)/lib/linux:${LD_LIBRARY_PATH}"
        print_info "Using library path: $(pwd)/lib/linux (fallback)"
    else
        print_warn "No lib directory found, continuing without setting LD_LIBRARY_PATH"
        print_warn "Expected locations:"
        print_warn "  - $binary_dir/lib (preferred)"
        print_warn "  - $(pwd)/lib/linux/$system_arch (architecture-specific)"
        print_warn "  - $(pwd)/lib/linux (fallback)"
    fi
}

# Function to check for running processes
check_processes() {
    local pids=$(pgrep -f "agentsmith-hub" 2>/dev/null)
    if [ -n "$pids" ]; then
        echo "$pids"
        return 0
    else
        return 1
    fi
}

# Function to show process information
show_process_info() {
    local pids="$1"
    if [ -n "$pids" ]; then
        print_info "Found running AgentSmith-HUB processes:"
        echo "$pids" | while read pid; do
            if [ -n "$pid" ]; then
                ps -p "$pid" -o pid,ppid,cmd --no-headers 2>/dev/null || echo "PID $pid (process info unavailable)"
            fi
        done
    fi
}

# Function to gracefully stop processes
graceful_stop() {
    local pids="$1"
    if [ -n "$pids" ]; then
        print_info "Sending TERM signal to processes..."
        echo "$pids" | xargs kill -TERM 2>/dev/null || true
        
        # Wait for graceful shutdown
        local wait_time=20
        print_info "Waiting ${wait_time} seconds for graceful shutdown..."
        sleep $wait_time
        
        # Check if processes are still running
        local remaining_pids=$(check_processes)
        if [ -n "$remaining_pids" ]; then
            return 1
        else
            return 0
        fi
    fi
    return 0
}

# Function to force kill processes
force_stop() {
    local pids="$1"
    if [ -n "$pids" ]; then
        print_warn "Force killing remaining processes..."
        echo "$pids" | xargs kill -KILL 2>/dev/null || true
        sleep 1
        
        # Final check
        local remaining_pids=$(check_processes)
        if [ -n "$remaining_pids" ]; then
            print_error "Some processes could not be stopped:"
            show_process_info "$remaining_pids"
            return 1
        fi
    fi
    return 0
}

# Function to stop existing processes
stop_existing_processes() {
    local force_mode="$1"
    
    print_info "Checking for existing AgentSmith-HUB processes..."
    
    # Check for running processes
    local pids=$(check_processes)
    if [ -z "$pids" ]; then
        print_info "No running AgentSmith-HUB processes found."
        return 0
    fi
    
    # Show process information
    show_process_info "$pids"
    
    print_info "Stopping existing AgentSmith-HUB processes..."
    
    if [ "$force_mode" = "true" ]; then
        # Force mode: kill immediately
        if force_stop "$pids"; then
            print_info "All processes stopped successfully."
        else
            print_error "Failed to stop some processes."
            return 1
        fi
    else
        # Normal mode: try graceful first, then force
        if graceful_stop "$pids"; then
            print_info "All processes stopped gracefully."
        else
            print_warn "Graceful shutdown failed, attempting force stop..."
            local remaining_pids=$(check_processes)
            if force_stop "$remaining_pids"; then
                print_info "All processes stopped successfully."
            else
                print_error "Failed to stop some processes."
                return 1
            fi
        fi
    fi
    
    return 0
}

# Function to check config directory
check_config() {
    local binary_dir="$(dirname "$1")"
    
    # Check for absolute config path first (preferred)
    if [ -d "$CONFIG_ROOT" ]; then
        print_info "Using config directory: $CONFIG_ROOT"
    # Check for config directory relative to binary (backward compatibility)
    elif [ -d "$binary_dir/config" ]; then
        CONFIG_ROOT="$binary_dir/config"
        print_info "Using config directory: $CONFIG_ROOT (relative to binary)"
    # Check for config directory in current working directory (backward compatibility)
    elif [ -d "config" ]; then
        CONFIG_ROOT="$(pwd)/config"
        print_info "Using config directory: $CONFIG_ROOT (current directory)"
    else
        print_error "Config directory not found!"
        print_error "Please ensure the config directory is present with proper configuration files."
        echo ""
        echo "Expected locations (in order of preference):"
        echo "  - $CONFIG_ROOT (preferred)"
        echo "  - $binary_dir/config (relative to binary)"
        echo "  - $(pwd)/config (current directory)"
        echo ""
        echo "To create the default config directory:"
        echo "  sudo mkdir -p $CONFIG_ROOT"
        echo "  sudo chown \$(whoami):\$(whoami) $CONFIG_ROOT"
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
    
    # Determine run mode based on command line flag
    if [ "$IS_FOLLOWER" = "true" ]; then
        print_info "Running in FOLLOWER mode"
        RUN_MODE="follower"
    else
        print_info "Running in LEADER mode (default)"
        RUN_MODE="leader"
    fi
    
    # Check and setup configuration directory (both leader and follower need config)
    check_config "$BINARY_PATH"
    
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
    
    # Show system architecture information
    local system_arch=$(detect_architecture)
    print_info "System architecture: $(uname -m) (mapped to: $system_arch)"
    print_info "Working directory: $SCRIPT_DIR"
    print_info "Library path: ${LD_LIBRARY_PATH:-'not set'}"
    print_info "Config root: $CONFIG_ROOT"
    echo ""
    
    # Stop existing processes if requested
    if [ "$STOP_EXISTING" = "true" ]; then
        if ! stop_existing_processes "$FORCE_STOP"; then
            print_error "Failed to stop existing processes. Exiting."
            exit 1
        fi
        echo ""
    fi
    
    # Calculate config path for binary
    BINARY_DIR="$(dirname "$BINARY_PATH")"
    
    # If CONFIG_ROOT is absolute, use it directly; otherwise calculate relative path
    if [[ "$CONFIG_ROOT" = /* ]]; then
        # Absolute path - use as is
        CONFIG_ARG="$CONFIG_ROOT"
    else
        # Relative path - calculate relative to binary location
        CONFIG_ARG=$(realpath --relative-to="$BINARY_DIR" "$CONFIG_ROOT")
    fi
    
    # Start the application based on mode
    cd "$(dirname "$BINARY_PATH")"
    if [ "$RUN_MODE" = "follower" ]; then
        print_info "Starting AgentSmith-HUB in FOLLOWER mode..."
        print_info "Will auto-discover cluster via Redis"
        print_info "Press Ctrl+C to stop"
        echo ""
        
        # Start as follower (no -leader flag)
        exec "./$BINARY_NAME" -config_root "$CONFIG_ARG"
    else
        print_info "Starting AgentSmith-HUB in LEADER mode..."
        print_info "Web interface will be available at: http://localhost:8080"
        print_info "Press Ctrl+C to stop"
        echo ""
        
        # Start as leader (with -leader flag)
        exec "./$BINARY_NAME" -config_root "$CONFIG_ARG" -leader
    fi
}

# Parse command line arguments
IS_FOLLOWER="false"
STOP_EXISTING="false"
FORCE_STOP="false"
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
            echo "  --follower           Run as follower node (auto-discovers cluster via Redis)"
            echo "  --restart            Stop existing processes before starting (graceful shutdown)"
            echo "  --force-restart      Force stop existing processes before starting"
            echo ""
            echo "Default Mode: Leader (starts with web interface on port 8080)"
            echo "Follower Mode: Connects to existing cluster via Redis configuration"
            echo ""
            echo "Process Management:"
            echo "  --restart            Check for running processes and stop them gracefully before starting"
            echo "  --force-restart      Force kill any running processes before starting"
            echo ""
            echo "This script automatically detects the binary location and configuration."
            echo "It will look for the binary in the following order:"
            echo "  1. $DIST_DIR/$BINARY_NAME (production build)"
            echo "  2. $BUILD_DIR/$BINARY_NAME (development build)"
            echo "  3. ./$BINARY_NAME (current directory)"
            echo ""
            echo "Configuration directory search order:"
            echo "  1. /opt/hub_config (preferred system location)"
            echo "  2. <binary_dir>/config (relative to binary)"
            echo "  3. ./config (current directory)"
            echo ""
            echo "Library path search order:"
            echo "  1. <binary_dir>/lib (preferred - architecture-specific libraries)"
            echo "  2. ./lib/linux/<arch> (architecture-specific: amd64/arm64)"
            echo "  3. ./lib/linux (fallback)"
            echo ""
            echo "Architecture Detection:"
            echo "  - Current system: $(uname -m)"
            echo "  - Mapped to: $(detect_architecture)"
            echo ""
            echo "Examples:"
            echo "  $0                    # Start as leader (default)"
            echo "  $0 --follower         # Start as follower"
            echo "  $0 --restart          # Stop existing processes and restart as leader"
            echo "  $0 --force-restart    # Force stop existing processes and restart as leader"
            echo "  $0 --follower --restart # Stop existing processes and restart as follower"
            echo ""
            echo "Note: Both leader and follower nodes need the same Redis configuration"
            echo "      in their config.yaml file to join the same cluster."
            echo ""
            exit 0
            ;;
        --follower)
            IS_FOLLOWER="true"
            shift
            ;;
        --restart)
            STOP_EXISTING="true"
            FORCE_STOP="false"
            shift
            ;;
        --force-restart)
            STOP_EXISTING="true"
            FORCE_STOP="true"
            shift
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
            
            # Show system information
            local system_arch=$(detect_architecture)
            print_info "System architecture: $(uname -m) (mapped to: $system_arch)"
            
            # Check binary
            BINARY_PATH=$(find_binary)
            if [ $? -eq 0 ]; then
                print_info "✓ Binary found: $BINARY_PATH"
            else
                print_error "✗ Binary not found"
            fi
            
            # Check for running processes
            local pids=$(check_processes)
            if [ -n "$pids" ]; then
                print_info "✓ Found running AgentSmith-HUB processes:"
                show_process_info "$pids"
            else
                print_info "✓ No running AgentSmith-HUB processes found"
            fi
            
            # Check config
            # Save original CONFIG_ROOT
            ORIGINAL_CONFIG_ROOT="$CONFIG_ROOT"
            
            # Try to find config
            if [ -d "$CONFIG_ROOT" ]; then
                print_info "✓ Config directory found: $CONFIG_ROOT"
            elif [ -d "$(dirname "${BINARY_PATH:-./}")/config" ]; then
                print_info "✓ Config directory found: $(dirname "${BINARY_PATH:-./}")/config (relative to binary)"
            elif [ -d "config" ]; then
                print_info "✓ Config directory found: $(pwd)/config (current directory)"
            else
                print_error "✗ Config directory not found"
                echo "    Expected locations:"
                echo "      - $ORIGINAL_CONFIG_ROOT (preferred)"
                echo "      - $(dirname "${BINARY_PATH:-./}")/config (relative to binary)"
                echo "      - $(pwd)/config (current directory)"
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