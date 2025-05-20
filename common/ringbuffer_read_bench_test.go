package common

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
)

func BenchmarkRingBufferConcurrentWriteSingleRead(b *testing.B) {
	rb, err := NewRingBuffer(RingBufferSize)
	if err != nil {
		b.Fatalf("NewRingBuffer error: %v", err)
	}
	b.ResetTimer()
	var wg sync.WaitGroup
	writeCount := 16
	writerNum := 8
	perWriter := writeCount / writerNum
	start := make(chan struct{})

	// 启动并发写goroutine
	for w := 0; w < writerNum; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			<-start
			for i := 0; i < perWriter; i++ {
				msg := map[string]interface{}{
					"type":    "bench",
					"content": "hello world",
					"writer":  id,
					"i":       i,
				}
				jsonData, err := json.Marshal(msg)
				if err != nil {
					b.Fatalf("json.Marshal error: %v", err)
				}
				for !rb.WriteMsg(jsonData) {
					// busy wait until buffer有空位
				}
			}
		}(w)
	}

	var readTotal int64
	readWg := sync.WaitGroup{}
	readWg.Add(1)
	go func() {
		for atomic.LoadInt64(&readTotal) < int64(writeCount)-1 {
			if _, ok := rb.ReadMsg(); ok {
				atomic.AddInt64(&readTotal, 1)
			}
		}
		readWg.Done()
	}()

	close(start) // 所有写goroutine同时开始
	wg.Wait()
	// 等待读goroutine完成，避免主线程提前退出
	readWg.Wait()
	b.Logf("All %d messages written and read", writeCount)
}
