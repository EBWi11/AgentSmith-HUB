package rules_engine

import (
	"sync"
	"time"
)

// localThresholdCounter is a synchronous TTL-aware integer counter map.
// It replaces ristretto.Cache[string, int] for local-cache threshold operations,
// eliminating the async Set + blocking Wait() pattern that serialised all threshold
// updates through the global ruleset mutex.
//
// Each method is protected by its own fine-grained mutex, so different keys can
// be operated on concurrently without contention on the ruleset-level r.mu lock.
type localThresholdCounter struct {
	mu      sync.RWMutex
	entries map[string]counterEntry
}

type counterEntry struct {
	value     int
	expiresAt time.Time
}

func newLocalThresholdCounter() *localThresholdCounter {
	return &localThresholdCounter{
		entries: make(map[string]counterEntry),
	}
}

// Get returns the current value and whether the key exists and has not expired.
func (c *localThresholdCounter) Get(key string) (int, bool) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	if !ok {
		c.mu.RUnlock()
		return 0, false
	}
	if time.Now().After(entry.expiresAt) {
		c.mu.RUnlock()
		c.mu.Lock()
		defer c.mu.Unlock()
		entry, ok = c.entries[key]
		if !ok || time.Now().After(entry.expiresAt) {
			delete(c.entries, key)
			return 0, false
		}
		return entry.value, true
	}
	c.mu.RUnlock()
	return entry.value, true
}

// GetTTL returns the remaining TTL for an existing non-expired key.
func (c *localThresholdCounter) GetTTL(key string) (time.Duration, bool) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	if !ok {
		c.mu.RUnlock()
		return 0, false
	}
	remaining := time.Until(entry.expiresAt)
	if remaining <= 0 {
		c.mu.RUnlock()
		c.mu.Lock()
		defer c.mu.Unlock()
		entry, ok = c.entries[key]
		if !ok {
			return 0, false
		}
		remaining = time.Until(entry.expiresAt)
		if remaining <= 0 {
			delete(c.entries, key)
			return 0, false
		}
		return remaining, true
	}
	c.mu.RUnlock()
	return remaining, true
}

// SetWithTTL stores value with the given TTL, overwriting any existing entry.
func (c *localThresholdCounter) SetWithTTL(key string, value int, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = counterEntry{value: value, expiresAt: time.Now().Add(ttl)}
}

// Del removes a key from the counter map.
func (c *localThresholdCounter) Del(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

// Close clears all entries (mirrors ristretto's Close signature).
func (c *localThresholdCounter) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]counterEntry)
}

// ---

// localClassifyCounter is a synchronous TTL-aware set map.
// It replaces ristretto.Cache[string, map[string]bool] for CLASSIFY threshold
// operations, with the same guarantees as localThresholdCounter.
type localClassifyCounter struct {
	mu      sync.RWMutex
	entries map[string]classifyEntry
}

type classifyEntry struct {
	keys      map[string]bool
	expiresAt time.Time
}

func newLocalClassifyCounter() *localClassifyCounter {
	return &localClassifyCounter{
		entries: make(map[string]classifyEntry),
	}
}

// Get returns a copy of the key set and whether the entry exists and has not expired.
// A copy is returned to allow the caller to mutate it safely.
func (c *localClassifyCounter) Get(key string) (map[string]bool, bool) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	if !ok {
		c.mu.RUnlock()
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		c.mu.RUnlock()
		c.mu.Lock()
		defer c.mu.Unlock()
		entry, ok = c.entries[key]
		if !ok || time.Now().After(entry.expiresAt) {
			delete(c.entries, key)
			return nil, false
		}
		cp := make(map[string]bool, len(entry.keys))
		for k, v := range entry.keys {
			cp[k] = v
		}
		return cp, true
	}
	cp := make(map[string]bool, len(entry.keys))
	for k, v := range entry.keys {
		cp[k] = v
	}
	c.mu.RUnlock()
	return cp, true
}

// SetWithTTL stores a key set with the given TTL, overwriting any existing entry.
func (c *localClassifyCounter) SetWithTTL(key string, keys map[string]bool, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = classifyEntry{keys: keys, expiresAt: time.Now().Add(ttl)}
}

// Del removes a key from the classify map.
func (c *localClassifyCounter) Del(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

// Close clears all entries.
func (c *localClassifyCounter) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]classifyEntry)
}
