package main

import (
	"AgentSmith-HUB/common"
	"encoding/json"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
)

func main() {
	rb, err := common.NewRingBuffer(common.RingBufferSize)
	if err != nil {
		panic(err)
	}
	defer rb.Close()
	efd := rb.Eventfd()
	for {
		// 优先消费 ringbuffer 中所有可读消息
		for {
			head, tail := rb.GetHeadTail()
			msg, ok := rb.ReadMsg()
			if !ok {
				break
			}
			fmt.Printf("[demo_plugin] head=%d tail=%d read message length=%d\n", head, tail, len(msg))
			var m map[string]interface{}
			err = json.Unmarshal(msg, &m)
			if err != nil {
				fmt.Println("[demo_plugin] JSON unmarshal failed:", err)
				continue
			}
			fmt.Println("[demo_plugin] Read JSON message:", m)
		}
		// ringbuffer 已空，阻塞等待新事件
		var buf [8]byte
		_, err := unix.Read(efd, buf[:])
		if err != nil && err != unix.EAGAIN {
			fmt.Fprintf(os.Stderr, "[demo_plugin] eventfd/pipe read error: %v\n", err)
		}
	}
}
