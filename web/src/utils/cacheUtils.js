/**
 * Cache boundary condition handling utilities
 * Unified handling of network errors, empty data, concurrent requests, and other edge cases
 */

/**
 * Safe cached data fetch
 * @param {Function} fetcher - Data fetching function
 * @param {any} fallback - Fallback data on failure
 * @param {object} options - Configuration options
 */
export async function safeCacheFetch(fetcher, fallback = null, options = {}) {
  const {
    retries = 1,
    timeout = 10000,
    silent = false
  } = options

  let lastError = null

  for (let attempt = 0; attempt <= retries; attempt++) {
    try {
      // Set timeout
      const timeoutPromise = new Promise((_, reject) =>
        setTimeout(() => reject(new Error('Request timeout')), timeout)
      )

      const result = await Promise.race([fetcher(), timeoutPromise])
      
      // Validate result
      if (result === null || result === undefined) {
        if (!silent) {
          console.warn('[CacheUtils] Received null/undefined data, using fallback')
        }
        return fallback
      }

      return result
    } catch (error) {
      lastError = error
      
      if (attempt < retries) {
        const delay = Math.min(1000 * Math.pow(2, attempt), 5000) // Exponential backoff, max 5 seconds
        if (!silent) {
          console.warn(`[CacheUtils] Attempt ${attempt + 1} failed, retrying in ${delay}ms:`, error.message)
        }
        await new Promise(resolve => setTimeout(resolve, delay))
      }
    }
  }

  // All retries failed
  if (!silent) {
    console.error('[CacheUtils] All attempts failed:', lastError)
  }
  
  return fallback
}

/**
 * Normalize component data format
 * @param {any} data - Raw data
 * @param {string} type - Component type
 */
export function normalizeComponentData(data, type) {
  if (!Array.isArray(data)) {
    console.warn(`[CacheUtils] Expected array for ${type}, got:`, typeof data)
    return []
  }

  return data.filter(item => {
    // Basic validation
    if (!item) return false
    
    // Component must have ID or name
    const id = item.id || item.name
    if (!id) {
      console.warn(`[CacheUtils] ${type} item missing id/name:`, item)
      return false
    }

    // Ensure hasTemp property exists
    if (item.hasTemp === undefined) {
      item.hasTemp = false
    }

    return true
  }).sort((a, b) => {
    // Unified sorting logic
    const idA = (a.id || a.name || '').toString()
    const idB = (b.id || b.name || '').toString()
    return idA.localeCompare(idB)
  })
}

/**
 * Check if data is expired
 * @param {number} timestamp - Data timestamp
 * @param {number} ttl - Time to live (milliseconds)
 */
export function isDataExpired(timestamp, ttl = 30000) {
  if (!timestamp || timestamp <= 0) return true
  return Date.now() - timestamp > ttl
}

/**
 * Cache key generator for deduplicating concurrent requests
 * @param {string} prefix - Key prefix
 * @param {...any} params - Parameters
 */
export function generateCacheKey(prefix, ...params) {
  const key = params
    .filter(p => p !== null && p !== undefined)
    .map(p => String(p))
    .join('_')
  
  return prefix ? `${prefix}_${key}` : key
}

/**
 * Safe JSON parsing
 * @param {string} json - JSON string
 * @param {any} fallback - Fallback value on failure
 */
export function safeJsonParse(json, fallback = null) {
  try {
    return JSON.parse(json)
  } catch (error) {
    console.warn('[CacheUtils] JSON parse failed:', error.message)
    return fallback
  }
}

/**
 * Delayed function execution (for debouncing)
 * @param {Function} func - Function to execute
 * @param {number} delay - Delay time (milliseconds)
 */
export function debounce(func, delay = 300) {
  let timeoutId
  return function (...args) {
    clearTimeout(timeoutId)
    timeoutId = setTimeout(() => func.apply(this, args), delay)
  }
}

/**
 * Memory-safe cache cleanup
 * @param {Map|Object} cache - Cache object
 * @param {number} maxSize - Maximum cache size
 */
export function cleanupCache(cache, maxSize = 100) {
  if (cache instanceof Map) {
    // LRU cleanup for Map type cache
    if (cache.size > maxSize) {
      const deleteCount = cache.size - maxSize
      const keys = Array.from(cache.keys()).slice(0, deleteCount)
      keys.forEach(key => cache.delete(key))
      console.log(`[CacheUtils] Cleaned up ${deleteCount} cache entries`)
    }
  } else if (typeof cache === 'object' && cache !== null) {
    // Cleanup for Object type cache
    const keys = Object.keys(cache)
    if (keys.length > maxSize) {
      const deleteCount = keys.length - maxSize
      const deleteKeys = keys.slice(0, deleteCount)
      deleteKeys.forEach(key => delete cache[key])
      console.log(`[CacheUtils] Cleaned up ${deleteCount} cache entries`)
    }
  }
}

/**
 * Network error handling
 * @param {Error} error - Error object
 * @param {string} operation - Operation description
 * @param {any} fallback - Fallback value
 */
export function handleNetworkError(error, operation, fallback = null) {
  let errorMessage = 'Unknown error'
  let shouldRetry = false

  if (error.response) {
    // HTTP error
    const status = error.response.status
    errorMessage = error.response.data?.error || `HTTP ${status} error`
    shouldRetry = status >= 500 || status === 429 // Server error or rate limiting
  } else if (error.request) {
    // Network error
    errorMessage = 'Network error or server not responding'
    shouldRetry = true
  } else {
    // Other error
    errorMessage = error.message || 'Unknown error'
    shouldRetry = false
  }

  console.error(`[CacheUtils] ${operation} failed: ${errorMessage}`)

  return {
    error: errorMessage,
    shouldRetry,
    fallback
  }
} 