#!/bin/bash

# AgentSmith-HUB Follower Mode Startup Script
# This script starts only the backend in follower mode

set -e

echo "Starting AgentSmith-HUB in Follower Mode..."

# Ensure config directory exists
mkdir -p "$CONFIG_ROOT"

# Copy default config if not exists
if [ ! -f "$CONFIG_ROOT/config.yaml" ]; then
    echo "Creating default config.yaml..."
    cp config/config.yaml "$CONFIG_ROOT/"
fi

# Copy default MCP config if not exists
if [ ! -f "$CONFIG_ROOT/mcp_config.json" ]; then
    echo "Creating default MCP config..."
    cp mcp_config/cline_mcp_settings.json "$CONFIG_ROOT/mcp_config.json"
fi

# Start the backend in follower mode
echo "Starting backend in follower mode..."
exec ./agentsmith-hub \
    --config_root "$CONFIG_ROOT" \
    --api_listen "0.0.0.0:8080" \
    --log_level "$LOG_LEVEL" \
    --node_id "$NODE_ID"