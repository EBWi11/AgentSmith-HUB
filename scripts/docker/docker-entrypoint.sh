#!/bin/bash

# AgentSmith-HUB Docker Entrypoint Script
# This script determines whether to start in leader or follower mode based on environment variables

set -e

echo "AgentSmith-HUB Docker Entrypoint Starting..."

# Check if we should run in leader mode
if [ "$MODE" = "leader" ] || [ "$LEADER_MODE" = "true" ] || [ "$LEADER_MODE" = "1" ]; then
    echo "Starting in LEADER mode..."
    exec ./leader-start.sh
else
    echo "Starting in FOLLOWER mode..."
    exec ./follower-start.sh
fi 