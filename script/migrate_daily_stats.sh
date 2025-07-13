#!/bin/bash

# Daily Stats Migration Script
# Migrates from old format (counter + metadata) to new Hash format

REDIS_HOST=${REDIS_HOST:-localhost}
REDIS_PORT=${REDIS_PORT:-6379}
REDIS_PASSWORD=${REDIS_PASSWORD:-toor}

echo "Daily Stats Migration Script"
echo "==========================="
echo "Redis Host: $REDIS_HOST:$REDIS_PORT"
echo ""

# Function to connect to Redis
redis_cmd() {
    if [ -n "$REDIS_PASSWORD" ]; then
        redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" -a "$REDIS_PASSWORD" --no-auth-warning "$@"
    else
        redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" "$@"
    fi
}

# Check Redis connection
echo -n "Checking Redis connection... "
if redis_cmd ping > /dev/null 2>&1; then
    echo "OK"
else
    echo "FAILED"
    echo "Error: Cannot connect to Redis"
    exit 1
fi

# Get all daily stats keys (old format)
echo ""
echo "Scanning for old format keys..."
OLD_KEYS=$(redis_cmd --scan --pattern "hub:daily_stats:*_*_*_*" | grep -v ":counter$" | grep -v ":metadata$" | grep -v ":hash:")

if [ -z "$OLD_KEYS" ]; then
    echo "No old format keys found. Migration may have already been completed."
    exit 0
fi

# Count keys
KEY_COUNT=$(echo "$OLD_KEYS" | wc -l | tr -d ' ')
echo "Found $KEY_COUNT old format keys to migrate"

# Ask for confirmation
echo ""
read -p "Do you want to proceed with migration? (y/N) " -n 1 -r
echo ""
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Migration cancelled"
    exit 0
fi

echo ""
echo "Starting migration..."
echo ""

MIGRATED=0
FAILED=0

# Process each key
while IFS= read -r KEY; do
    if [ -z "$KEY" ]; then
        continue
    fi
    
    echo -n "Processing $KEY... "
    
    # Get the JSON data
    JSON_DATA=$(redis_cmd get "$KEY" 2>/dev/null)
    if [ -z "$JSON_DATA" ]; then
        echo "SKIP (empty)"
        continue
    fi
    
    # Parse JSON to extract fields
    NODE_ID=$(echo "$JSON_DATA" | jq -r '.node_id // empty')
    PROJECT_ID=$(echo "$JSON_DATA" | jq -r '.project_id // empty')
    COMPONENT_ID=$(echo "$JSON_DATA" | jq -r '.component_id // empty')
    COMPONENT_TYPE=$(echo "$JSON_DATA" | jq -r '.component_type // empty')
    PROJECT_NODE_SEQUENCE=$(echo "$JSON_DATA" | jq -r '.project_node_sequence // empty')
    DATE=$(echo "$JSON_DATA" | jq -r '.date // empty')
    TOTAL_MESSAGES=$(echo "$JSON_DATA" | jq -r '.total_messages // 0')
    
    # Validate required fields
    if [ -z "$NODE_ID" ] || [ -z "$PROJECT_ID" ] || [ -z "$COMPONENT_TYPE" ] || [ -z "$COMPONENT_ID" ] || [ -z "$PROJECT_NODE_SEQUENCE" ] || [ -z "$DATE" ]; then
        echo "FAILED (missing required fields)"
        ((FAILED++))
        continue
    fi
    
    # Create Hash key and field
    HASH_KEY="hub:daily_stats:hash:$DATE"
    FIELD="${NODE_ID}|${PROJECT_ID}|${COMPONENT_TYPE}|${COMPONENT_ID}|${PROJECT_NODE_SEQUENCE}"
    
    # Check if counter key exists and use it if available
    COUNTER_KEY="${KEY}:counter"
    COUNTER_VALUE=$(redis_cmd get "$COUNTER_KEY" 2>/dev/null)
    if [ -n "$COUNTER_VALUE" ] && [ "$COUNTER_VALUE" != "" ]; then
        TOTAL_MESSAGES=$COUNTER_VALUE
    fi
    
    # Write to new Hash format
    if redis_cmd hset "$HASH_KEY" "$FIELD" "$TOTAL_MESSAGES" > /dev/null 2>&1; then
        # Set expiration (30 days)
        redis_cmd expire "$HASH_KEY" 2592000 > /dev/null 2>&1
        
        echo "OK (messages: $TOTAL_MESSAGES)"
        ((MIGRATED++))
        
        # Optionally delete old keys (uncomment to enable)
        # redis_cmd del "$KEY" > /dev/null 2>&1
        # redis_cmd del "${KEY}:counter" > /dev/null 2>&1
        # redis_cmd del "${KEY}:metadata" > /dev/null 2>&1
    else
        echo "FAILED (write error)"
        ((FAILED++))
    fi
    
done <<< "$OLD_KEYS"

echo ""
echo "Migration Summary"
echo "================"
echo "Total keys processed: $KEY_COUNT"
echo "Successfully migrated: $MIGRATED"
echo "Failed: $FAILED"
echo ""
echo "Note: Old keys have been preserved. To delete them, uncomment the deletion lines in the script."
echo ""
echo "To verify migration, run:"
echo "  redis-cli --scan --pattern 'hub:daily_stats:hash:*' | head -10" 