package rules_engine

import (
	"sync"

	regexp "github.com/BurntSushi/rure-go"
)

// regexCacheEntry represents a compiled regex with reference count
type regexCacheEntry struct {
	regex *regexp.Regex
	count int // reference count for LRU eviction
}

// regexCache provides a thread-safe LRU cache for compiled regular expressions
type regexCache struct {
	mu        sync.RWMutex
	cache     map[string]*regexCacheEntry
	order     []string // LRU order, most recently used at the end
	maxSize   int
	hitCount  uint64
	missCount uint64
}

// Global regex cache instance
var globalRegexCache = &regexCache{
	cache:   make(map[string]*regexCacheEntry),
	order:   make([]string, 0),
	maxSize: 1000, // Maximum 1000 compiled regex patterns
}

// getCompiledRegex retrieves a compiled regex from cache or compiles and caches it
func (rc *regexCache) getCompiledRegex(pattern string) (*regexp.Regex, error) {
	// Fast path: read lock for cache hit
	rc.mu.RLock()
	if entry, exists := rc.cache[pattern]; exists {
		entry.count++
		rc.hitCount++
		rc.mu.RUnlock()

		// Move to end of LRU order (most recently used)
		rc.mu.Lock()
		rc.moveToEnd(pattern)
		rc.mu.Unlock()

		return entry.regex, nil
	}
	rc.mu.RUnlock()

	// Cache miss: compile and cache
	rc.mu.Lock()
	defer rc.mu.Unlock()

	// Double-check after acquiring write lock
	if entry, exists := rc.cache[pattern]; exists {
		entry.count++
		rc.hitCount++
		rc.moveToEnd(pattern)
		return entry.regex, nil
	}

	// Compile the regex
	compiledRegex, err := regexp.Compile(pattern)
	if err != nil {
		rc.missCount++
		return nil, err
	}

	// Add to cache
	rc.cache[pattern] = &regexCacheEntry{
		regex: compiledRegex,
		count: 1,
	}
	rc.order = append(rc.order, pattern)
	rc.missCount++

	// Evict oldest entries if cache is full
	if len(rc.cache) > rc.maxSize {
		rc.evictOldest()
	}

	return compiledRegex, nil
}

// moveToEnd moves a pattern to the end of the LRU order
func (rc *regexCache) moveToEnd(pattern string) {
	// Find and remove the pattern from current position
	for i, p := range rc.order {
		if p == pattern {
			// Remove from current position
			rc.order = append(rc.order[:i], rc.order[i+1:]...)
			break
		}
	}
	// Add to end (most recently used)
	rc.order = append(rc.order, pattern)
}

// evictOldest removes the least recently used entries to maintain cache size
func (rc *regexCache) evictOldest() {
	// Calculate how many entries to evict (10% of maxSize)
	evictCount := rc.maxSize / 10
	if evictCount < 1 {
		evictCount = 1
	}

	for i := 0; i < evictCount && len(rc.order) > 0; i++ {
		// Remove the oldest (least recently used) entry
		oldest := rc.order[0]
		rc.order = rc.order[1:]
		delete(rc.cache, oldest)
	}
}

// getCacheStats returns cache statistics for monitoring
func (rc *regexCache) getCacheStats() (hitCount, missCount uint64, cacheSize int) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc.hitCount, rc.missCount, len(rc.cache)
}

// clearCache clears all entries from the cache
func (rc *regexCache) clearCache() {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.cache = make(map[string]*regexCacheEntry)
	rc.order = make([]string, 0)
	rc.hitCount = 0
	rc.missCount = 0
}

// GetCompiledRegex is the public interface to get a compiled regex with caching
func GetCompiledRegex(pattern string) (*regexp.Regex, error) {
	return globalRegexCache.getCompiledRegex(pattern)
}

// GetRegexCacheStats returns cache statistics for monitoring
func GetRegexCacheStats() (hitCount, missCount uint64, cacheSize int) {
	return globalRegexCache.getCacheStats()
}

// ClearRegexCache clears the global regex cache
func ClearRegexCache() {
	globalRegexCache.clearCache()
}
