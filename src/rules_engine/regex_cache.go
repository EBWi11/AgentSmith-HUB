package rules_engine

import (
	"container/list"
	"hash/fnv"
	"sync"

	regexp "github.com/BurntSushi/rure-go"
)

// regexCacheEntry stores a compiled regex and its list element key.
type regexCacheEntry struct {
	pattern string
	regex   *regexp.Regex
}

const regexCacheShardCount = 32

type regexCacheShard struct {
	mu      sync.Mutex
	cache   map[string]*list.Element
	order   *list.List
	maxSize int
}

// regexCache is a sharded thread-safe LRU cache for compiled regular expressions.
type regexCache struct {
	shards []regexCacheShard
}

// Global regex cache instance
var globalRegexCache = newRegexCache(1000)

func newRegexCache(maxSize int) *regexCache {
	shardCount := regexCacheShardCount
	if maxSize > 0 && maxSize < shardCount {
		shardCount = 1
	}
	rc := &regexCache{
		shards: make([]regexCacheShard, shardCount),
	}
	perShardSize := maxSize / shardCount
	if perShardSize < 16 {
		if shardCount == 1 {
			perShardSize = maxSize
		} else {
			perShardSize = 16
		}
	}
	for i := range rc.shards {
		rc.shards[i] = regexCacheShard{
			cache:   make(map[string]*list.Element),
			order:   list.New(),
			maxSize: perShardSize,
		}
	}
	return rc
}

func (rc *regexCache) shardFor(pattern string) *regexCacheShard {
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(pattern))
	return &rc.shards[hasher.Sum32()%uint32(len(rc.shards))]
}

// getCompiledRegex retrieves a compiled regex from cache or compiles and caches it.
func (rc *regexCache) getCompiledRegex(pattern string) (*regexp.Regex, error) {
	shard := rc.shardFor(pattern)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	if elem, exists := shard.cache[pattern]; exists {
		shard.order.MoveToFront(elem)
		return elem.Value.(*regexCacheEntry).regex, nil
	}

	compiledRegex, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	entry := &regexCacheEntry{pattern: pattern, regex: compiledRegex}
	elem := shard.order.PushFront(entry)
	shard.cache[pattern] = elem

	// Evict least recently used entries when over capacity
	if shard.order.Len() > shard.maxSize {
		oldest := shard.order.Back()
		if oldest != nil {
			shard.order.Remove(oldest)
			delete(shard.cache, oldest.Value.(*regexCacheEntry).pattern)
		}
	}

	return compiledRegex, nil
}

// getCacheStats returns current cache size for monitoring.
func (rc *regexCache) getCacheStats() int {
	total := 0
	for i := range rc.shards {
		shard := &rc.shards[i]
		shard.mu.Lock()
		total += shard.order.Len()
		shard.mu.Unlock()
	}
	return total
}

// clearCache removes all entries from the cache.
func (rc *regexCache) clearCache() {
	for i := range rc.shards {
		shard := &rc.shards[i]
		shard.mu.Lock()
		shard.cache = make(map[string]*list.Element)
		shard.order = list.New()
		shard.mu.Unlock()
	}
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
