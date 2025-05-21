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

	// 添加写入重试次数
	maxRetries := 3
	retryDelay := time.Microsecond * 5 // 使用更短的重试延迟

	// 添加统计信息
	var totalWrites int
	var failedWrites int
	lastStatsTime := time.Now()

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
		// 添加重试机制
		success := false
		for i := 0; i < maxRetries; i++ {
			if rb.WriteMsg(jsonData) {
				fmt.Println("[plugin_test.go] 写入 JSON:", string(jsonData))
				success = true
				break
			}
			failedWrites++
			fmt.Printf("[plugin_test.go] ringbuffer 满，写入失败 (尝试 %d/%d) head=%d tail=%d\n",
				i+1, maxRetries, rb.GetHead(), rb.GetTail())

			// 使用短暂的重试延迟
			time.Sleep(retryDelay)
		}

		if !success {
			fmt.Printf("[plugin_test.go] 写入失败，跳过当前消息 head=%d tail=%d\n",
				rb.GetHead(), rb.GetTail())
		}

		// 每5秒打印一次统计信息
		if time.Since(lastStatsTime) > 5*time.Second {
			successRate := float64(totalWrites-failedWrites) / float64(totalWrites) * 100
			fmt.Printf("[plugin_test.go] 统计: 总写入=%d, 失败=%d, 成功率=%.2f%%\n",
				totalWrites, failedWrites, successRate)
			totalWrites = 0
			failedWrites = 0
			lastStatsTime = time.Now()
		}
	}
}
