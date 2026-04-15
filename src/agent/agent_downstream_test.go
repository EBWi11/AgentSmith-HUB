package agent

import (
	"sync"
	"testing"
	"time"
)

// This regression test is most valuable under `go test -race`.
// The previous implementation iterated a.DownStream directly while project
// stop/hot-reload paths could mutate the same map concurrently.
func TestForwardDownstreamConcurrentMutation(t *testing.T) {
	chA := make(chan map[string]interface{}, 1024)
	chB := make(chan map[string]interface{}, 1024)

	a := &Agent{
		Id:         "race-agent",
		DownStream: make(map[string]*chan map[string]interface{}),
	}
	a.SetDownstream("OUTPUT.a", &chA)

	stopDrain := make(chan struct{})
	var drainWG sync.WaitGroup
	for _, ch := range []chan map[string]interface{}{chA, chB} {
		drainWG.Add(1)
		go func(ch chan map[string]interface{}) {
			defer drainWG.Done()
			for {
				select {
				case <-stopDrain:
					return
				case <-ch:
				}
			}
		}(ch)
	}

	var workers sync.WaitGroup
	workers.Add(2)

	go func() {
		defer workers.Done()
		deadline := time.Now().Add(300 * time.Millisecond)
		for time.Now().Before(deadline) {
			a.forwardDownstream(map[string]interface{}{"message": "test"})
		}
	}()

	go func() {
		defer workers.Done()
		deadline := time.Now().Add(300 * time.Millisecond)
		for time.Now().Before(deadline) {
			a.DeleteDownstream("OUTPUT.a")
			a.SetDownstream("OUTPUT.b", &chB)
			a.DeleteDownstream("OUTPUT.b")
			a.SetDownstream("OUTPUT.a", &chA)
		}
	}()

	workers.Wait()
	close(stopDrain)
	drainWG.Wait()

	if got := len(a.CopyDownstream()); got != 1 {
		t.Fatalf("expected one downstream after mutation loop, got %d", got)
	}
}
