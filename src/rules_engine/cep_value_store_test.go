package rules_engine

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestPebbleCEPValueStore_PutGetDelete(t *testing.T) {
	store, err := NewPebbleCEPValueStore("test-put-get-delete")
	if err != nil {
		t.Fatalf("NewPebbleCEPValueStore failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	ref, err := store.PutSnapshot(map[string]interface{}{
		"event_type": "login",
		"user":       "alice",
	}, time.Now().Add(2*time.Minute).UnixNano())
	if err != nil {
		t.Fatalf("PutSnapshot failed: %v", err)
	}
	if ref == "" {
		t.Fatal("expected non-empty ref")
	}

	data, err := store.GetSnapshot(ref)
	if err != nil {
		t.Fatalf("GetSnapshot failed: %v", err)
	}
	if data["event_type"] != "login" || data["user"] != "alice" {
		t.Fatalf("unexpected snapshot data: %+v", data)
	}

	if err := store.DeleteSnapshot(ref); err != nil {
		t.Fatalf("DeleteSnapshot failed: %v", err)
	}
	if _, err := store.GetSnapshot(ref); err == nil {
		t.Fatal("expected error after delete, got nil")
	}
}

func TestPebbleCEPValueStore_CleanupExpired(t *testing.T) {
	store, err := NewPebbleCEPValueStore("test-cleanup-expired")
	if err != nil {
		t.Fatalf("NewPebbleCEPValueStore failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	pastRef, err := store.PutSnapshot(map[string]interface{}{"k": "expired"}, time.Now().Add(-1*time.Minute).UnixNano())
	if err != nil {
		t.Fatalf("PutSnapshot expired failed: %v", err)
	}
	futureRef, err := store.PutSnapshot(map[string]interface{}{"k": "active"}, time.Now().Add(5*time.Minute).UnixNano())
	if err != nil {
		t.Fatalf("PutSnapshot active failed: %v", err)
	}

	if err := store.cleanupExpired(time.Now().UnixNano(), 1000); err != nil {
		t.Fatalf("cleanupExpired failed: %v", err)
	}

	if _, err := store.GetSnapshot(pastRef); err == nil {
		t.Fatal("expected expired ref to be cleaned up")
	}
	active, err := store.GetSnapshot(futureRef)
	if err != nil {
		t.Fatalf("expected active ref to remain, got error: %v", err)
	}
	if active["k"] != "active" {
		t.Fatalf("unexpected active snapshot data: %+v", active)
	}
}

func TestPebbleCEPValueStore_ConcurrentPutAndGet(t *testing.T) {
	store, err := NewPebbleCEPValueStore("test-concurrent-put-get")
	if err != nil {
		t.Fatalf("NewPebbleCEPValueStore failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	const n = 200
	type item struct {
		ref string
		val string
	}
	items := make([]item, n)

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			val := fmt.Sprintf("v-%d", i)
			ref, putErr := store.PutSnapshot(map[string]interface{}{"v": val}, time.Now().Add(2*time.Minute).UnixNano())
			if putErr != nil {
				t.Errorf("PutSnapshot(%d) failed: %v", i, putErr)
				return
			}
			items[i] = item{ref: ref, val: val}
		}()
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		if items[i].ref == "" {
			t.Fatalf("missing ref for index %d", i)
		}
		data, getErr := store.GetSnapshot(items[i].ref)
		if getErr != nil {
			t.Fatalf("GetSnapshot(%d) failed: %v", i, getErr)
		}
		if data["v"] != items[i].val {
			t.Fatalf("value mismatch at %d: want=%s got=%v", i, items[i].val, data["v"])
		}
	}
}

func TestPebbleCEPValueStore_CloseIdempotent(t *testing.T) {
	store, err := NewPebbleCEPValueStore("test-close-idempotent")
	if err != nil {
		t.Fatalf("NewPebbleCEPValueStore failed: %v", err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("first Close failed: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("second Close should be idempotent, got: %v", err)
	}
}

