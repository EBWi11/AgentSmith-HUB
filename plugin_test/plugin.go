package main

import (
	"AgentSmith-HUB/common"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"
)

func main() {
	rb, err := common.NewRingBuffer("/tmp/test.mmap")
	if err != nil {
		panic(err)
	}
	defer rb.Close()
	rand.Seed(time.Now().UnixNano())

	// Add write retry count
	maxRetries := 3
	retryDelay := time.Microsecond * 2 // Use shorter retry delay

	// Add statistics
	var totalWrites int
	var failedWrites int

	for {
		r := rand.Intn(10000)
		msg := map[string]interface{}{
			"type":    "greeting",
			"content": "hello from plugin_test.go!",
			"rand":    r,
			"ts":      time.Now().Format(time.RFC3339Nano),
		}
		jsonData, err := json.Marshal(msg)
		if err != nil {
			fmt.Println("marshal error:", err)
			continue
		}

		totalWrites++
		// Add retry mechanism
		for i := 0; i < maxRetries; i++ {
			if rb.WriteMsg(jsonData) {
				fmt.Println("[plugin_test.go] Wrote JSON:", string(jsonData))
				break
			}
			failedWrites++
			fmt.Printf("[plugin_test.go] ringbuffer full, write failed (attempt %d/%d) head=%d tail=%d\n",
				i+1, maxRetries, rb.GetHead(), rb.GetTail())

			// Use short retry delay
			time.Sleep(retryDelay)
		}
	}
}
