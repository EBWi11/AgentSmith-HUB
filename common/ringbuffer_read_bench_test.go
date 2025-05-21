package common

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
)

func BenchmarkRingBufferConcurrentWriteSingleRead(b *testing.B) {
	rb, err := NewRingBuffer("/tmp/bench_test.mmap")
	if err != nil {
		b.Fatalf("NewRingBuffer error: %v", err)
	}
	b.ResetTimer()
	var wg sync.WaitGroup
	writeCount := 16
	writerNum := 8
	perWriter := writeCount / writerNum
	start := make(chan struct{})

	// Start concurrent write goroutines
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
					// busy wait until buffer has space
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

	close(start) // All write goroutines start at the same time
	wg.Wait()
	// Wait for read goroutine to finish to avoid main thread exiting early
	readWg.Wait()
	b.Logf("All %d messages written and read", writeCount)
}
