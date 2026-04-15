package input

import (
	"sync"
	"testing"
	"time"
)

// This regression test exercises downstream mutation while dispatching.
// It is most valuable under `go test -race`, where the old implementation
// reported a data race and could crash in dispatchMessage.
func TestDispatchMessageConcurrentDownstreamMutation(t *testing.T) {
	chA := make(chan map[string]interface{}, 1024)
	chB := make(chan map[string]interface{}, 1024)

	in := &Input{
		Id:         "race-input",
		DownStream: make(map[string]*chan map[string]interface{}),
	}
	in.SetDownstream("RULESET.a", &chA)

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
			in.dispatchMessage(map[string]interface{}{"message": "test"})
		}
	}()

	go func() {
		defer workers.Done()
		deadline := time.Now().Add(300 * time.Millisecond)
		for time.Now().Before(deadline) {
			in.DeleteDownstream("RULESET.a")
			in.SetDownstream("RULESET.b", &chB)
			in.DeleteDownstream("RULESET.b")
			in.SetDownstream("RULESET.a", &chA)
		}
	}()

	workers.Wait()
	close(stopDrain)
	drainWG.Wait()

	if got := in.DownstreamCount(); got != 1 {
		t.Fatalf("expected one downstream after mutation loop, got %d", got)
	}
}
