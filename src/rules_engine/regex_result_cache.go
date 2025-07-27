package rules_engine

import (
	"container/list"
	"crypto/md5"
	"fmt"
	"sync"

	regexp "github.com/BurntSushi/rure-go"
)

// RegexMatchResult represents a cached regex match result
type RegexMatchResult struct {
	matched bool
	error   error
}

// RegexResultCache is a thread-safe LRU cache for regex match results
type RegexResultCache struct {
	mutex     sync.RWMutex
	capacity  int
	cache     map[string]*list.Element
	lruList   *list.List
	hitCount  uint64
	missCount uint64
}

// cacheItem represents an item in the LRU cache
type cacheItem struct {
	key    string
	result RegexMatchResult
}

// NewRegexResultCache creates a new regex result cache with specified capacity
func NewRegexResultCache(capacity int) *RegexResultCache {
	return &RegexResultCache{
		capacity:  capacity,
		cache:     make(map[string]*list.Element),
		lruList:   list.New(),
		hitCount:  0,
		missCount: 0,
	}
}

// generateCacheKey creates a cache key from regex pattern and input value
func generateCacheKey(regexPattern, inputValue string) string {
	// Use MD5 hash to create a compact key
	data := fmt.Sprintf("%s|%s", regexPattern, inputValue)
	hash := md5.Sum([]byte(data))
	return fmt.Sprintf("%x", hash)
}

// Get retrieves a cached result if it exists
func (c *RegexResultCache) Get(regexPattern, inputValue string) (RegexMatchResult, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	key := generateCacheKey(regexPattern, inputValue)
	if element, exists := c.cache[key]; exists {
		// Move to front (most recently used)
		c.lruList.MoveToFront(element)
		item := element.Value.(*cacheItem)
		c.hitCount++
		return item.result, true
	}
	c.missCount++
	return RegexMatchResult{}, false
}

// Put stores a result in the cache
func (c *RegexResultCache) Put(regexPattern, inputValue string, result RegexMatchResult) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	key := generateCacheKey(regexPattern, inputValue)

	// Check if key already exists
	if element, exists := c.cache[key]; exists {
		// Update existing entry and move to front
		c.lruList.MoveToFront(element)
		item := element.Value.(*cacheItem)
		item.result = result
		return
	}

	// Add new entry
	newItem := &cacheItem{
		key:    key,
		result: result,
	}
	element := c.lruList.PushFront(newItem)
	c.cache[key] = element

	// Remove least recently used item if capacity exceeded
	if c.lruList.Len() > c.capacity {
		oldest := c.lruList.Back()
		if oldest != nil {
			c.lruList.Remove(oldest)
			oldItem := oldest.Value.(*cacheItem)
			delete(c.cache, oldItem.key)
		}
	}
}

// Clear removes all entries from the cache
func (c *RegexResultCache) Clear() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.cache = make(map[string]*list.Element)
	c.lruList = list.New()
	c.hitCount = 0
	c.missCount = 0
}

// Size returns the current number of entries in the cache
func (c *RegexResultCache) Size() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.lruList.Len()
}

// CachedRegexMatch performs regex matching with caching for non-raw values
// Uses the same logic as REGEX function to ensure compatibility
func CachedRegexMatch(cache *RegexResultCache, regexPattern, inputValue string, isFromRaw bool) (bool, error) {
	// Only cache results for non-raw values (static values that might be reused)
	if !isFromRaw && cache != nil {
		// Try to get from cache first
		if result, found := cache.Get(regexPattern, inputValue); found {
			return result.matched, result.error
		}
	}

	// Perform actual regex match using more efficient IsMatch() method
	// since we only need the boolean result, not the matched content
	regex, err := regexp.Compile(regexPattern)
	if err != nil {
		result := RegexMatchResult{matched: false, error: err}
		// Cache the error result only for non-raw values
		if !isFromRaw && cache != nil {
			cache.Put(regexPattern, inputValue, result)
		}
		return false, err
	}

	// Use IsMatch() for better performance since we only need boolean result
	matched := regex.IsMatch(inputValue)
	result := RegexMatchResult{matched: matched, error: nil}

	// Cache the result only for non-raw values
	if !isFromRaw && cache != nil {
		cache.Put(regexPattern, inputValue, result)
	}

	return matched, nil
}

// CachedRegexMatchWithPrecompiled performs regex matching using pre-compiled regex
// This is for static regex patterns that are already compiled
// NOTE: Caching is disabled for performance reasons - direct regex matching is faster
func CachedRegexMatchWithPrecompiled(cache *RegexResultCache, compiledRegex *regexp.Regex, regexPattern, inputValue string) bool {
	// Disabled caching for static regex - direct matching is more efficient
	// Try to get from cache first
	// if cache != nil {
	//     if result, found := cache.Get(regexPattern, inputValue); found && result.error == nil {
	//         return result.matched
	//     }
	// }

	// Use IsMatch() for better performance since we only need boolean result
	matched := compiledRegex.IsMatch(inputValue)

	// Disabled result caching - overhead outweighs benefits for pre-compiled regex
	// if cache != nil {
	//     result := RegexMatchResult{matched: matched, error: nil}
	//     cache.Put(regexPattern, inputValue, result)
	// }

	return matched
}

// GetRegexResultCacheStats returns cache statistics for monitoring
func GetRegexResultCacheStats(cache *RegexResultCache) map[string]interface{} {
	if cache == nil {
		return map[string]interface{}{
			"size":     0,
			"capacity": 0,
		}
	}

	cache.mutex.RLock()
	defer cache.mutex.RUnlock()

	return map[string]interface{}{
		"size":     cache.lruList.Len(),
		"capacity": cache.capacity,
	}
}

// SetRegexResultCacheCapacity updates the cache capacity
func SetRegexResultCacheCapacity(cache *RegexResultCache, capacity int) {
	if cache == nil {
		return
	}

	cache.mutex.Lock()
	defer cache.mutex.Unlock()

	cache.capacity = capacity

	// Remove excess entries if new capacity is smaller
	for cache.lruList.Len() > capacity {
		oldest := cache.lruList.Back()
		if oldest != nil {
			cache.lruList.Remove(oldest)
			oldItem := oldest.Value.(*cacheItem)
			delete(cache.cache, oldItem.key)
		}
	}
}

// ClearGlobalRegexResultCache is now deprecated - kept for compatibility
// Use cache.Clear() directly on specific cache instances
func ClearGlobalRegexResultCache() {
	// This function is now a no-op since we don't have global cache
}
