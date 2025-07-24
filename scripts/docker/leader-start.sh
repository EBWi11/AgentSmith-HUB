#!/bin/bash

# AgentSmith-HUB Leader Mode Startup Script
# This script starts both the backend in leader mode and the web frontend

set -e

echo "Starting AgentSmith-HUB in Leader Mode..."

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

# Start nginx for web frontend
echo "Starting nginx web server..."
mkdir -p /tmp/nginx
sed "s|__WEB_ROOT__|/opt/agentsmith-hub/web/dist|g" /etc/nginx/http.d/default.conf > /tmp/nginx/default.conf
nginx -c /tmp/nginx/default.conf

# Start the backend in leader mode
echo "Starting backend in leader mode..."
exec ./agentsmith-hub \
    --config_root "$CONFIG_ROOT" \
    --port 8080 \
    --mode leader \
    --log_level "$LOG_LEVEL" \
    --node_id "$NODE_ID"