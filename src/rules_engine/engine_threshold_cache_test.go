package rules_engine

import (
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// localThresholdCounter
// ---------------------------------------------------------------------------

func TestLocalThresholdCounter_GetMissing(t *testing.T) {
	c := newLocalThresholdCounter()
	v, ok := c.Get("missing")
	if ok || v != 0 {
		t.Fatalf("expected (0, false), got (%d, %v)", v, ok)
	}
}

func TestLocalThresholdCounter_SetGet(t *testing.T) {
	c := newLocalThresholdCounter()
	c.SetWithTTL("k", 42, time.Minute)
	v, ok := c.Get("k")
	if !ok || v != 42 {
		t.Fatalf("expected (42, true), got (%d, %v)", v, ok)
	}
}

func TestLocalThresholdCounter_Overwrite(t *testing.T) {
	c := newLocalThresholdCounter()
	c.SetWithTTL("k", 1, time.Minute)
	c.SetWithTTL("k", 99, time.Minute)
	v, ok := c.Get("k")
	if !ok || v != 99 {
		t.Fatalf("expected (99, true), got (%d, %v)", v, ok)
	}
}

func TestLocalThresholdCounter_Expired(t *testing.T) {
	c := newLocalThresholdCounter()
	c.SetWithTTL("k", 7, 1*time.Millisecond)
	time.Sleep(5 * time.Millisecond)
	v, ok := c.Get("k")
	if ok || v != 0 {
		t.Fatalf("expected (0, false) after expiry, got (%d, %v)", v, ok)
	}
}

func TestLocalThresholdCounter_GetTTL_Active(t *testing.T) {
	c := newLocalThresholdCounter()
	c.SetWithTTL("k", 1, time.Minute)
	ttl, ok := c.GetTTL("k")
	if !ok || ttl <= 0 || ttl > time.Minute {
		t.Fatalf("unexpected TTL: ok=%v ttl=%v", ok, ttl)
	}
}

func TestLocalThresholdCounter_GetTTL_Missing(t *testing.T) {
	c := newLocalThresholdCounter()
	_, ok := c.GetTTL("no-such-key")
	if ok {
		t.Fatal("expected false for missing key")
	}
}

func TestLocalThresholdCounter_GetTTL_Expired(t *testing.T) {
	c := newLocalThresholdCounter()
	c.SetWithTTL("k", 1, 1*time.Millisecond)
	time.Sleep(5 * time.Millisecond)
	_, ok := c.GetTTL("k")
	if ok {
		t.Fatal("expected false for expired key")
	}
}

func TestLocalThresholdCounter_Del(t *testing.T) {
	c := newLocalThresholdCounter()
	c.SetWithTTL("k", 5, time.Minute)
	c.Del("k")
	_, ok := c.Get("k")
	if ok {
		t.Fatal("key should be gone after Del")
	}
}

func TestLocalThresholdCounter_Close(t *testing.T) {
	c := newLocalThresholdCounter()
	c.SetWithTTL("a", 1, time.Minute)
	c.SetWithTTL("b", 2, time.Minute)
	c.Close()
	if _, ok := c.Get("a"); ok {
		t.Fatal("key 'a' should be gone after Close")
	}
	if _, ok := c.Get("b"); ok {
		t.Fatal("key 'b' should be gone after Close")
	}
}

func TestLocalThresholdCounter_Concurrent(t *testing.T) {
	c := newLocalThresholdCounter()
	const goroutines = 20
	const ops = 200

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := "shared"
			for j := 0; j < ops; j++ {
				c.SetWithTTL(key, id*ops+j, time.Minute)
				c.Get(key)
				c.GetTTL(key)
			}
		}(i)
	}
	wg.Wait()
	// No race / panic = pass
}

// ---------------------------------------------------------------------------
// localClassifyCounter
// ---------------------------------------------------------------------------

func TestLocalClassifyCounter_GetMissing(t *testing.T) {
	c := newLocalClassifyCounter()
	keys, ok := c.Get("missing")
	if ok || keys != nil {
		t.Fatalf("expected (nil, false), got (%v, %v)", keys, ok)
	}
}

func TestLocalClassifyCounter_SetGet(t *testing.T) {
	c := newLocalClassifyCounter()
	in := map[string]bool{"alpha": true, "beta": true}
	c.SetWithTTL("k", in, time.Minute)
	out, ok := c.Get("k")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if len(out) != 2 || !out["alpha"] || !out["beta"] {
		t.Fatalf("unexpected keys: %v", out)
	}
}

func TestLocalClassifyCounter_GetReturnsCopy(t *testing.T) {
	c := newLocalClassifyCounter()
	c.SetWithTTL("k", map[string]bool{"x": true}, time.Minute)

	got, _ := c.Get("k")
	got["injected"] = true // mutate the returned copy

	// Second Get should not see the injected key
	got2, _ := c.Get("k")
	if got2["injected"] {
		t.Fatal("mutation of returned copy affected stored entry")
	}
}

func TestLocalClassifyCounter_Expired(t *testing.T) {
	c := newLocalClassifyCounter()
	c.SetWithTTL("k", map[string]bool{"x": true}, 1*time.Millisecond)
	time.Sleep(5 * time.Millisecond)
	keys, ok := c.Get("k")
	if ok || keys != nil {
		t.Fatalf("expected (nil, false) after expiry")
	}
}

func TestLocalClassifyCounter_Del(t *testing.T) {
	c := newLocalClassifyCounter()
	c.SetWithTTL("k", map[string]bool{"x": true}, time.Minute)
	c.Del("k")
	_, ok := c.Get("k")
	if ok {
		t.Fatal("key should be gone after Del")
	}
}

func TestLocalClassifyCounter_Close(t *testing.T) {
	c := newLocalClassifyCounter()
	c.SetWithTTL("a", map[string]bool{"x": true}, time.Minute)
	c.SetWithTTL("b", map[string]bool{"y": true}, time.Minute)
	c.Close()
	if _, ok := c.Get("a"); ok {
		t.Fatal("key 'a' should be gone after Close")
	}
}

func TestLocalClassifyCounter_Concurrent(t *testing.T) {
	c := newLocalClassifyCounter()
	const goroutines = 20
	const ops = 100

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < ops; j++ {
				c.SetWithTTL("shared", map[string]bool{"k": true}, time.Minute)
				c.Get("shared")
				c.Del("shared")
			}
		}(i)
	}
	wg.Wait()
}
