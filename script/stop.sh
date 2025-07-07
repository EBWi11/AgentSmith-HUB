#!/bin/bash

# AgentSmith-HUB Stop Script
# This script stops the AgentSmith-HUB services

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

# Function to show help
show_help() {
    echo "AgentSmith-HUB Stop Script"
    echo ""
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Options:"
    echo "  --help, -h     Show this help message"
    echo "  --force, -f    Force kill processes immediately"
    echo "  --check, -c    Check for running processes without stopping"
    echo ""
    echo "This script stops all AgentSmith-HUB processes gracefully."
    echo "If graceful shutdown fails, it will force terminate the processes."
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
        local wait_time=120
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

# Main function
main() {
    local force_mode=false
    local check_only=false
    
    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            --help|-h)
                show_help
                exit 0
                ;;
            --force|-f)
                force_mode=true
                shift
                ;;
            --check|-c)
                check_only=true
                shift
                ;;
            *)
                print_error "Unknown option: $1"
                echo "Use --help for usage information"
                exit 1
                ;;
        esac
    done
    
    print_info "Checking for AgentSmith-HUB processes..."
    
    # Check for running processes
    local pids=$(check_processes)
    if [ -z "$pids" ]; then
        print_info "No running AgentSmith-HUB processes found."
        exit 0
    fi
    
    # Show process information
    show_process_info "$pids"
    
    # If check only mode, exit here
    if [ "$check_only" = true ]; then
        exit 0
    fi
    
    print_info "Stopping AgentSmith-HUB processes..."
    
    if [ "$force_mode" = true ]; then
        # Force mode: kill immediately
        if force_stop "$pids"; then
            print_info "All processes stopped successfully."
        else
            print_error "Failed to stop some processes."
            exit 1
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
                exit 1
            fi
        fi
    fi
    
    print_info "AgentSmith-HUB stopped."
}

# Run main function with all arguments
main "$@" 