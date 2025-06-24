#!/bin/bash

# AgentSmith-HUB Script Generator
# This script creates deployment scripts for the AgentSmith-HUB package

set -e

# Check if target directory is provided
if [ $# -ne 1 ]; then
    echo "Usage: $0 <target_directory>"
    echo "This script creates start.sh and stop.sh in the target directory"
    exit 1
fi

TARGET_DIR="$1"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

print_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

# Ensure target directory exists
mkdir -p "$TARGET_DIR"

print_info "Creating deployment scripts in $TARGET_DIR"

# Create start.sh (based on run.sh)
if [ -f "$SCRIPT_DIR/run.sh" ]; then
    print_info "Creating start.sh..."
    cp "$SCRIPT_DIR/run.sh" "$TARGET_DIR/start.sh"
    chmod +x "$TARGET_DIR/start.sh"
else
    print_warn "Source run.sh not found, creating basic start.sh..."
    cat > "$TARGET_DIR/start.sh" << 'EOF'
#!/bin/bash

# AgentSmith-HUB Start Script

set -e

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Configuration
BINARY_NAME="agentsmith-hub"
CONFIG_ROOT="config"

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

print_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if binary exists
if [ ! -f "$BINARY_NAME" ]; then
    print_error "Binary $BINARY_NAME not found!"
    exit 1
fi

# Set library path if lib directory exists
if [ -d "lib" ]; then
    export LD_LIBRARY_PATH="$(pwd)/lib:${LD_LIBRARY_PATH}"
    print_info "Set library path: $(pwd)/lib"
fi

# Check config directory
if [ ! -d "$CONFIG_ROOT" ]; then
    print_error "Config directory $CONFIG_ROOT not found!"
    exit 1
fi

# Make binary executable
chmod +x "$BINARY_NAME"

print_info "Starting AgentSmith-HUB..."
print_info "Web interface will be available at: http://localhost:8080"
print_info "Press Ctrl+C to stop"

# Start the application
exec "./$BINARY_NAME" -config_root "$CONFIG_ROOT"
EOF
    chmod +x "$TARGET_DIR/start.sh"
fi

# Create stop.sh
if [ -f "$SCRIPT_DIR/stop.sh" ]; then
    print_info "Creating stop.sh..."
    cp "$SCRIPT_DIR/stop.sh" "$TARGET_DIR/stop.sh"
    chmod +x "$TARGET_DIR/stop.sh"
else
    print_warn "Source stop.sh not found, creating basic stop.sh..."
    cat > "$TARGET_DIR/stop.sh" << 'EOF'
#!/bin/bash

# AgentSmith-HUB Stop Script

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

print_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_info "Stopping AgentSmith-HUB..."

# Find and stop all agentsmith-hub processes
PIDS=$(pgrep -f "agentsmith-hub" 2>/dev/null || true)

if [ -z "$PIDS" ]; then
    print_info "No running AgentSmith-HUB processes found."
    exit 0
fi

print_info "Found running processes: $PIDS"

# Try graceful shutdown first
print_info "Sending TERM signal..."
echo "$PIDS" | xargs kill -TERM 2>/dev/null || true

# Wait a bit for graceful shutdown
sleep 3

# Check if processes are still running
REMAINING=$(pgrep -f "agentsmith-hub" 2>/dev/null || true)
if [ -n "$REMAINING" ]; then
    print_warn "Some processes still running, force killing..."
    echo "$REMAINING" | xargs kill -KILL 2>/dev/null || true
    sleep 1
fi

# Final check
FINAL_CHECK=$(pgrep -f "agentsmith-hub" 2>/dev/null || true)
if [ -z "$FINAL_CHECK" ]; then
    print_info "AgentSmith-HUB stopped successfully."
else
    print_error "Failed to stop some processes."
    exit 1
fi
EOF
    chmod +x "$TARGET_DIR/stop.sh"
fi

# Create README for deployment
print_info "Creating deployment README..."
cat > "$TARGET_DIR/README.md" << 'EOF'
# AgentSmith-HUB Deployment

This directory contains a complete AgentSmith-HUB deployment package.

## Files

- `agentsmith-hub` - Main application binary (Linux amd64)
- `web/` - Frontend web interface files
- `lib/` - Required shared libraries
- `config/` - Configuration files
- `start.sh` - Script to start the services
- `stop.sh` - Script to stop the services

## Quick Start

1. Make sure you're on a Linux amd64 system
2. Start the services:
   ```bash
   ./start.sh
   ```
3. Open your browser and navigate to: http://localhost:8080
4. To stop the services:
   ```bash
   ./stop.sh
   ```

## Advanced Usage

### Leader Mode (Default)
```bash
./start.sh
```

### Follower Mode
```bash
LEADER_ADDR=<leader_ip:port> ./start.sh --follower <leader_ip:port>
```

### Check Status
```bash
./stop.sh --check
```

## Configuration

Configuration files are located in the `config/` directory. Modify them according to your needs before starting the services.

## Logs

Application logs are written to stdout. To save logs to a file:
```bash
./start.sh > agentsmith-hub.log 2>&1 &
```

## Troubleshooting

If you encounter issues:

1. Check that all files have execute permissions:
   ```bash
   chmod +x agentsmith-hub start.sh stop.sh
   ```

2. Verify library path (if needed):
   ```bash
   export LD_LIBRARY_PATH=$(pwd)/lib:$LD_LIBRARY_PATH
   ```

3. Check configuration files in `config/` directory

For more information, visit: https://github.com/EBWi11/AgentSmith-HUB
EOF

print_info "Deployment scripts created successfully!"
print_info "Created files:"
print_info "  - $TARGET_DIR/start.sh"
print_info "  - $TARGET_DIR/stop.sh" 
print_info "  - $TARGET_DIR/README.md" 