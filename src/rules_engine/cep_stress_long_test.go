package rules_engine

import (
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestCEP_Stress_10Minutes runs a mixed workload for 10 minutes to validate
// CEP stability/performance under pressure and edge cases.
//
// Run with: go test -run TestCEP_Stress_10Minutes -timeout 15m
// Override duration: CEP_STRESS_DURATION_SECONDS=30 go test -run TestCEP_Stress_10Minutes -timeout 60s
func TestCEP_Stress_10Minutes(t *testing.T) {
	xml := `<root name="stress" type="DETECTION">
		<rule id="r_seq" name="login then exfil">
			<sequence within="6s" group_by="user_id" local_cache="true">
				<event id="a" event_time="ts">
					<check type="EQU" field="event_type">login</check>
				</event>
				<event id="b" event_time="ts">
					<check type="EQU" field="event_type">file_transfer</check>
				</event>
				<condition>a -> b</condition>
			</sequence>
		</rule>
	</root>`

	rs := buildStressRuleset(t, xml)
	t.Cleanup(func() {
		if rs != nil {
			rs.cleanup()
		}
	})

	duration := 10 * time.Minute
	if v := os.Getenv("CEP_STRESS_DURATION_SECONDS"); v != "" {
		if sec, err := strconv.Atoi(v); err == nil && sec > 0 {
			duration = time.Duration(sec) * time.Second
		}
	}
	const workers = 4
	const cardinality = 5000
	hotKeys := []string{"hot-0", "hot-1", "hot-2", "hot-3"}

	// Pre-generate a large payload to stress snapshot storage path.
	largeBuf := make([]byte, 2048)
	for i := range largeBuf {
		largeBuf[i] = byte('a' + (i % 26))
	}
	largePayload := string(largeBuf)

	var totalEvents atomic.Uint64
	var totalHits atomic.Uint64
	var totalPanics atomic.Uint64

	stopAt := time.Now().Add(duration)
	var wg sync.WaitGroup

	workerFn := func(workerID int) {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				totalPanics.Add(1)
				t.Logf("worker %d panic: %v", workerID, r)
			}
		}()

		r := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)*997))
		for time.Now().Before(stopAt) {
			nowNs := time.Now().UnixNano()
			uid := fmt.Sprintf("u-%d", r.Intn(cardinality))
			if workerID%3 == 0 {
				uid = hotKeys[r.Intn(len(hotKeys))]
			}

			roll := r.Intn(100)
			switch {
			// Normal sequence path: generate a->b frequently to drive hits.
			case roll < 55:
				_ = rs.EngineCheck(map[string]interface{}{
					"user_id":    uid,
					"event_type": "login",
					"ts":         fmt.Sprintf("%d", nowNs),
					"blob":       largePayload,
					"worker":     workerID,
				})
				totalEvents.Add(1)

				// Keep completion ratio moderate to avoid overloading delete path.
				if r.Intn(100) < 20 {
					res := rs.EngineCheck(map[string]interface{}{
						"user_id":    uid,
						"event_type": "file_transfer",
						"ts":         fmt.Sprintf("%d", nowNs+int64(1*time.Millisecond)),
						"direction":  "outbound",
						"worker":     workerID,
					})
					totalEvents.Add(1)
					if len(res) > 0 {
						totalHits.Add(uint64(len(res)))
					}
				}

			// Edge cases: malformed or missing timestamp/group field.
			default:
				ev := map[string]interface{}{
					"event_type": "login",
					"user_id":    uid,
					"noise":      largePayload,
				}
				if r.Intn(2) == 0 {
					ev["ts"] = "not-a-time"
				}
				if r.Intn(5) == 0 {
					delete(ev, "user_id")
				}
				_ = rs.EngineCheck(ev)
				totalEvents.Add(1)
			}
			time.Sleep(1 * time.Millisecond)
		}
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go workerFn(i)
	}

	// Periodic metrics log.
	metricTicker := time.NewTicker(30 * time.Second)
	defer metricTicker.Stop()
	for time.Now().Before(stopAt) {
		<-metricTicker.C
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		t.Logf(
			"stress-progress events=%d hits=%d panics=%d heap_alloc_mb=%.2f num_gc=%d goroutines=%d",
			totalEvents.Load(),
			totalHits.Load(),
			totalPanics.Load(),
			float64(m.HeapAlloc)/1024.0/1024.0,
			m.NumGC,
			runtime.NumGoroutine(),
		)
	}

	wg.Wait()

	t.Logf("stress-final events=%d hits=%d panics=%d", totalEvents.Load(), totalHits.Load(), totalPanics.Load())
	if totalPanics.Load() != 0 {
		t.Fatalf("stress test observed %d panic(s)", totalPanics.Load())
	}
	minEvents := uint64(duration.Seconds() * 120)
	if totalEvents.Load() < minEvents {
		t.Fatalf("stress test generated too few events: %d", totalEvents.Load())
	}
	if totalHits.Load() == 0 {
		t.Fatal("stress test produced zero sequence hits")
	}
}
