package rules_engine

import (
	"sync"
	"testing"
	"time"
)

// This regression test is most valuable under `go test -race`.
// The previous implementation iterated r.DownStream directly while project
// lifecycle code could mutate the map concurrently.
func TestSendToDownstreamConcurrentMutation(t *testing.T) {
	chA := make(chan map[string]interface{}, 1024)
	chB := make(chan map[string]interface{}, 1024)

	r := &Ruleset{
		RulesetID:  "race-ruleset",
		DownStream: make(map[string]*chan map[string]interface{}),
	}
	r.SetDownstream("OUTPUT.a", &chA)

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
			r.sendToDownstream(map[string]interface{}{"message": "test"})
		}
	}()

	go func() {
		defer workers.Done()
		deadline := time.Now().Add(300 * time.Millisecond)
		for time.Now().Before(deadline) {
			r.DeleteDownstream("OUTPUT.a")
			r.SetDownstream("OUTPUT.b", &chB)
			r.DeleteDownstream("OUTPUT.b")
			r.SetDownstream("OUTPUT.a", &chA)
		}
	}()

	workers.Wait()
	close(stopDrain)
	drainWG.Wait()

	if got := len(r.CopyDownstream()); got != 1 {
		t.Fatalf("expected one downstream after mutation loop, got %d", got)
	}
}
