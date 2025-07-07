// Debounce function - delay execution, only execute the last call among multiple calls
export function debounce(func, delay = 300) {
  let timeoutId
  return function (...args) {
    clearTimeout(timeoutId)
    timeoutId = setTimeout(() => func.apply(this, args), delay)
  }
}

// Throttle function - limit execution frequency, execute only once within specified time
export function throttle(func, limit = 300) {
  let inThrottle
  return function (...args) {
    if (!inThrottle) {
      func.apply(this, args)
      inThrottle = true
      setTimeout(() => inThrottle = false, limit)
    }
  }
}

// Factory method to create debounced functions
export function createDebouncedFunction(func, delay = 300) {
  return debounce(func, delay)
}

// Factory method to create throttled functions
export function createThrottledFunction(func, limit = 300) {
  return throttle(func, limit)
}

// Debounce decorator - for Vue component methods
export function debouncedMethod(delay = 300) {
  return function (target, propertyKey, descriptor) {
    const originalMethod = descriptor.value
    descriptor.value = debounce(originalMethod, delay)
    return descriptor
  }
}

// Throttle decorator - for Vue component methods
export function throttledMethod(limit = 300) {
  return function (target, propertyKey, descriptor) {
    const originalMethod = descriptor.value
    descriptor.value = throttle(originalMethod, limit)
    return descriptor
  }
}

// Debounce Promise - for async operations
export function debouncePromise(func, delay = 300) {
  let timeoutId
  let rejectPrevious
  
  return function (...args) {
    return new Promise((resolve, reject) => {
      // Cancel previous Promise
      if (rejectPrevious) {
        rejectPrevious(new Error('Debounced'))
      }
      
      clearTimeout(timeoutId)
      rejectPrevious = reject
      
      timeoutId = setTimeout(async () => {
        try {
          const result = await func.apply(this, args)
          resolve(result)
        } catch (error) {
          reject(error)
        } finally {
          rejectPrevious = null
        }
      }, delay)
    })
  }
}

// Throttle Promise - for async operations
export function throttlePromise(func, limit = 300) {
  let inThrottle = false
  let pendingPromise = null
  
  return function (...args) {
    if (inThrottle) {
      // If in throttle, return current executing Promise
      return pendingPromise || Promise.resolve()
    }
    
    inThrottle = true
    pendingPromise = func.apply(this, args)
    
    // Ensure pendingPromise is a Promise
    if (!(pendingPromise instanceof Promise)) {
      pendingPromise = Promise.resolve(pendingPromise)
    }
    
    // On success: keep throttle window for the specified limit
    pendingPromise.then(() => {
      setTimeout(() => {
        inThrottle = false
        pendingPromise = null
      }, limit)
    })
    // On failure: release immediately so caller can retry
    .catch(() => {
      inThrottle = false
      pendingPromise = null
    })
    
    return pendingPromise
  }
}

// Smart debounce - dynamically adjust delay based on user behavior
export function smartDebounce(func, baseDelay = 300, maxDelay = 1000) {
  let timeoutId
  let callCount = 0
  let lastCallTime = 0
  
  return function (...args) {
    const now = Date.now()
    const timeSinceLastCall = now - lastCallTime
    
    // If continuous calls, increase delay
    if (timeSinceLastCall < baseDelay) {
      callCount++
    } else {
      callCount = 1
    }
    
    lastCallTime = now
    
    // Calculate dynamic delay
    const dynamicDelay = Math.min(baseDelay * Math.pow(1.2, callCount - 1), maxDelay)
    
    clearTimeout(timeoutId)
    timeoutId = setTimeout(() => {
      func.apply(this, args)
      callCount = 0
    }, dynamicDelay)
  }
}

// Batch debounce - collect multiple operations, execute in batch
export function batchDebounce(func, delay = 300) {
  let timeoutId
  let batch = []
  
  return function (item) {
    batch.push(item)
    
    clearTimeout(timeoutId)
    timeoutId = setTimeout(() => {
      if (batch.length > 0) {
        func.call(this, [...batch])
        batch = []
      }
    }, delay)
  }
}

// Request deduplication - prevent duplicate API requests
export function requestDeduplication() {
  const pendingRequests = new Map()
  
  return function (key, requestFunc) {
    // If same request is already in progress, return that Promise
    if (pendingRequests.has(key)) {
      return pendingRequests.get(key)
    }
    
    // Create new request
    const promise = requestFunc()
    pendingRequests.set(key, promise)
    
    // Clear after request completes
    promise.finally(() => {
      pendingRequests.delete(key)
    })
    
    return promise
  }
}

// Combined usage: debounce + request deduplication
export function createOptimizedApiCall(apiFunc, delay = 300) {
  const debouncedFunc = debouncePromise(apiFunc, delay)
  const deduplicatedFunc = requestDeduplication()
  
  return function (...args) {
    const key = JSON.stringify(args)
    return deduplicatedFunc(key, () => debouncedFunc.apply(this, args))
  }
}

// Performance monitoring decorator
export function performanceMonitor(name) {
  return function (target, propertyKey, descriptor) {
    const originalMethod = descriptor.value
    
    descriptor.value = async function (...args) {
      const startTime = performance.now()
      console.log(`🚀 ${name} started`)
      
      try {
        const result = await originalMethod.apply(this, args)
        const endTime = performance.now()
        console.log(`✅ ${name} completed in ${(endTime - startTime).toFixed(2)}ms`)
        return result
      } catch (error) {
        const endTime = performance.now()
        console.error(`❌ ${name} failed in ${(endTime - startTime).toFixed(2)}ms:`, error)
        throw error
      }
    }
    
    return descriptor
  }
}

// User activity detection
export function createUserActivityDetector() {
  let lastActivity = Date.now()
  let isActive = true
  const callbacks = []
  
  // Listen to user activity
  const events = ['mousedown', 'mousemove', 'keypress', 'scroll', 'touchstart', 'click']
  
  const updateActivity = throttle(() => {
    lastActivity = Date.now()
    if (!isActive) {
      isActive = true
      callbacks.forEach(callback => callback(true))
    }
  }, 1000)
  
  events.forEach(event => {
    document.addEventListener(event, updateActivity, { passive: true })
  })
  
  // Check inactive state
  const checkInactivity = () => {
    const now = Date.now()
    if (now - lastActivity > 60000 && isActive) { // 1 minute inactive
      isActive = false
      callbacks.forEach(callback => callback(false))
    }
  }
  
  setInterval(checkInactivity, 30000) // Check every 30 seconds
  
  return {
    isActive: () => isActive,
    getLastActivity: () => lastActivity,
    onActivityChange: (callback) => {
      callbacks.push(callback)
      return () => {
        const index = callbacks.indexOf(callback)
        if (index > -1) callbacks.splice(index, 1)
      }
    }
  }
}

// Network status detection
export function createNetworkDetector() {
  let isOnline = navigator.onLine
  const callbacks = []
  
  const handleOnline = () => {
    isOnline = true
    callbacks.forEach(callback => callback(true))
  }
  
  const handleOffline = () => {
    isOnline = false
    callbacks.forEach(callback => callback(false))
  }
  
  window.addEventListener('online', handleOnline)
  window.addEventListener('offline', handleOffline)
  
  return {
    isOnline: () => isOnline,
    onNetworkChange: (callback) => {
      callbacks.push(callback)
      return () => {
        const index = callbacks.indexOf(callback)
        if (index > -1) callbacks.splice(index, 1)
      }
    }
  }
} 