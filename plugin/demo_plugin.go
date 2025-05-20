package main

import (
	"AgentSmith-HUB/common"
	"encoding/json"
	"fmt"
)

func main() {
	rb, err := common.NewRingBuffer(common.RingBufferSize)
	if err != nil {
		panic(err)
	}
	for {
		head, tail := rb.GetHeadTail() // 新增日志
		msg, ok := rb.ReadMsg()
		if !ok {
			fmt.Printf("[demo_plugin] head=%d tail=%d 无消息\n", head, tail)
			continue // busy loop，无sleep
		}
		fmt.Printf("[demo_plugin] head=%d tail=%d 读到消息长度=%d\n", head, tail, len(msg))
		var m map[string]interface{}
		err = json.Unmarshal(msg, &m)
		if err != nil {
			fmt.Println("[demo_plugin] JSON 解析失败:", err)
			continue
		}
		fmt.Println("[demo_plugin] 读取到 JSON 消息:", m)
	}
}
