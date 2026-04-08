package rules_engine

import (
	"container/list"
	"sync"

	regexp "github.com/BurntSushi/rure-go"
)

// regexCacheEntry stores a compiled regex and its list element key.
type regexCacheEntry struct {
	pattern string
	regex   *regexp.Regex
}

// regexCache is a thread-safe LRU cache for compiled regular expressions.
// All operations (hit or miss) require a write to maintain LRU order,
// so a single sync.Mutex is used throughout — no RWMutex needed.
type regexCache struct {
	mu      sync.Mutex
	cache   map[string]*list.Element
	order   *list.List // front = most recently used
	maxSize int
}

// Global regex cache instance
var globalRegexCache = &regexCache{
	cache:   make(map[string]*list.Element),
	order:   list.New(),
	maxSize: 1000,
}

// getCompiledRegex retrieves a compiled regex from cache or compiles and caches it.
func (rc *regexCache) getCompiledRegex(pattern string) (*regexp.Regex, error) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	if elem, exists := rc.cache[pattern]; exists {
		rc.order.MoveToFront(elem)
		return elem.Value.(*regexCacheEntry).regex, nil
	}

	compiledRegex, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	entry := &regexCacheEntry{pattern: pattern, regex: compiledRegex}
	elem := rc.order.PushFront(entry)
	rc.cache[pattern] = elem

	// Evict least recently used entries when over capacity
	if rc.order.Len() > rc.maxSize {
		oldest := rc.order.Back()
		if oldest != nil {
			rc.order.Remove(oldest)
			delete(rc.cache, oldest.Value.(*regexCacheEntry).pattern)
		}
	}

	return compiledRegex, nil
}

// getCacheStats returns current cache size for monitoring.
func (rc *regexCache) getCacheStats() int {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return rc.order.Len()
}

// clearCache removes all entries from the cache.
func (rc *regexCache) clearCache() {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.cache = make(map[string]*list.Element)
	rc.order = list.New()
}

// GetCompiledRegex is the public interface to get a compiled regex with caching.
func GetCompiledRegex(pattern string) (*regexp.Regex, error) {
	return globalRegexCache.getCompiledRegex(pattern)
}

// GetRegexCacheStats returns the current number of cached patterns.
func GetRegexCacheStats() int {
	return globalRegexCache.getCacheStats()
}

// ClearRegexCache clears the global regex cache.
func ClearRegexCache() {
	globalRegexCache.clearCache()
}
