#!/bin/bash

echo "=== Testing ProjectNodeSequence Fix ==="
echo

# Clean up old Redis data first
echo "1. Cleaning up old daily stats data..."
redis-cli -a toor keys "hub:daily_stats:*" 2>/dev/null | xargs -r redis-cli -a toor del 2>/dev/null
echo "Old data cleaned up."
echo

# Restart the project to trigger new statistics collection
echo "2. Restarting demo project..."
curl -s -X PUT -H "token: 9ef0c170-069e-44dd-a406-2d85eca0a0b2" http://localhost:8000/api/project/demo/restart | jq '.'
echo

# Wait a bit for statistics to be collected
echo "3. Waiting 15 seconds for statistics collection..."
sleep 15
echo

# Check new Redis keys
echo "4. Checking new Redis keys pattern:"
DATE=$(date +%Y-%m-%d)
redis-cli -a toor keys "hub:daily_stats:${DATE}_*" 2>/dev/null | sort
echo

# Check metadata content
echo "5. Checking metadata content for each component:"
redis-cli -a toor keys "hub:daily_stats:${DATE}_*:metadata" 2>/dev/null | while read key; do
    echo "Key: $key"
    redis-cli -a toor get "$key" 2>/dev/null | jq '.'
    echo "---"
done

# Check counter values
echo "6. Checking counter values:"
redis-cli -a toor keys "hub:daily_stats:${DATE}_*:counter" 2>/dev/null | while read key; do
    echo "Key: $key"
    echo "Value: $(redis-cli -a toor get "$key" 2>/dev/null)"
    echo "---"
done

# Get daily stats via API
echo "7. Getting daily stats via API:"
curl -s -H "token: 9ef0c170-069e-44dd-a406-2d85eca0a0b2" "http://localhost:8000/api/getDailyStats?date=${DATE}" | jq '.'
echo

# Check project flow display
echo "8. Checking project flow display:"
curl -s -H "token: 9ef0c170-069e-44dd-a406-2d85eca0a0b2" http://localhost:8000/api/project/demo/components | jq '.components | to_entries[] | {name: .key, type: .value.type, dailyMessages: .value.daily_messages}' 