package main

import (
	"AgentSmith-HUB/common"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"
)

func main() {
	rb, err := common.NewRingBuffer(common.RingBufferSize)
	if err != nil {
		panic(err)
	}
	defer rb.Close()
	rand.Seed(time.Now().UnixNano())
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
		if rb.WriteMsg(jsonData) {
			fmt.Println("[plugin_test.go] 写入 JSON:", string(jsonData))
		} else {
			fmt.Println("[plugin_test.go] ringbuffer 满，写入失败")
		}
		time.Sleep(time.Millisecond * 5)
	}
}
