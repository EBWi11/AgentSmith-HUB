/**
 * Anti-duplicate trigger utility with improved UX
 * Prevents function execution within a specified cooldown period
 */

class AntiDuplicateTrigger {
  constructor(cooldown = 1000) {
    this.cooldown = cooldown
    this.lastExecutionTimes = new Map()
    this.callbacks = {
      onBlocked: null,  // Called when execution is blocked
      onExecuted: null  // Called when execution succeeds
    }
  }

  /**
   * Set callbacks for better UX feedback
   * @param {Object} callbacks - Callback functions
   * @param {Function} callbacks.onBlocked - Called when execution is blocked
   * @param {Function} callbacks.onExecuted - Called when execution succeeds
   */
  setCallbacks(callbacks) {
    this.callbacks = { ...this.callbacks, ...callbacks }
  }

  /**
   * Execute function with anti-duplicate protection
   * @param {string} key - Unique identifier for the function/operation
   * @param {Function} fn - Function to execute
   * @param {...any} args - Arguments to pass to the function
   * @returns {any} - Result of function execution or false if blocked
   */
  execute(key, fn, ...args) {
    const currentTime = Date.now()
    const lastTime = this.lastExecutionTimes.get(key) || 0
    const timeSinceLastExecution = currentTime - lastTime
    
    if (timeSinceLastExecution < this.cooldown) {
      const remainingTime = this.cooldown - timeSinceLastExecution
      console.log(`Anti-duplicate trigger: ${key} ignored - too frequent (${remainingTime}ms remaining)`)
      
      // Call blocked callback for better UX
      if (this.callbacks.onBlocked) {
        this.callbacks.onBlocked(key, remainingTime)
      }
      
      return false
    }
    
    this.lastExecutionTimes.set(key, currentTime)
    
    try {
      const result = fn(...args)
      
      // Call executed callback for confirmation
      if (this.callbacks.onExecuted) {
        this.callbacks.onExecuted(key)
      }
      
      return result
    } catch (error) {
      console.error(`Anti-duplicate trigger: Error executing ${key}:`, error)
      throw error
    }
  }

  /**
   * Execute function with Promise support and better error handling
   * @param {string} key - Unique identifier for the function/operation
   * @param {Function} fn - Function to execute (can be async)
   * @param {...any} args - Arguments to pass to the function
   * @returns {Promise<any>} - Result of function execution or rejected promise if blocked
   */
  async executeAsync(key, fn, ...args) {
    const currentTime = Date.now()
    const lastTime = this.lastExecutionTimes.get(key) || 0
    const timeSinceLastExecution = currentTime - lastTime
    
    if (timeSinceLastExecution < this.cooldown) {
      const remainingTime = this.cooldown - timeSinceLastExecution
      console.log(`Anti-duplicate trigger: ${key} ignored - too frequent (${remainingTime}ms remaining)`)
      
      // Call blocked callback for better UX
      if (this.callbacks.onBlocked) {
        this.callbacks.onBlocked(key, remainingTime)
      }
      
      return Promise.reject(new Error(`Operation blocked: please wait ${Math.ceil(remainingTime / 100) / 10}s`))
    }
    
    this.lastExecutionTimes.set(key, currentTime)
    
    try {
      const result = await fn(...args)
      
      // Call executed callback for confirmation
      if (this.callbacks.onExecuted) {
        this.callbacks.onExecuted(key)
      }
      
      return result
    } catch (error) {
      console.error(`Anti-duplicate trigger: Error executing ${key}:`, error)
      throw error
    }
  }

  /**
   * Check if operation can be executed without executing it
   * @param {string} key - Unique identifier for the operation
   * @returns {Object} - Status object with canExecute flag and remaining time
   */
  getExecutionStatus(key) {
    const currentTime = Date.now()
    const lastTime = this.lastExecutionTimes.get(key) || 0
    const timeSinceLastExecution = currentTime - lastTime
    const canExecute = timeSinceLastExecution >= this.cooldown
    const remainingTime = canExecute ? 0 : this.cooldown - timeSinceLastExecution
    
    return {
      canExecute,
      remainingTime,
      lastExecutionTime: lastTime
    }
  }

  /**
   * Check if operation can be executed (legacy method for compatibility)
   * @param {string} key - Unique identifier for the operation
   * @returns {boolean} - Whether the operation can be executed
   */
  canExecute(key) {
    return this.getExecutionStatus(key).canExecute
  }

  /**
   * Reset cooldown for a specific key
   * @param {string} key - Unique identifier to reset
   */
  reset(key) {
    this.lastExecutionTimes.delete(key)
  }

  /**
   * Clear all cooldowns
   */
  clear() {
    this.lastExecutionTimes.clear()
  }

  /**
   * Get all tracked keys and their status
   * @returns {Map} - Map of key to execution status
   */
  getAllStatus() {
    const status = new Map()
    for (const [key] of this.lastExecutionTimes) {
      status.set(key, this.getExecutionStatus(key))
    }
    return status
  }
}

// Global instance for save operations with user-friendly feedback
export const saveAntiDuplicate = new AntiDuplicateTrigger(1000)

// Set up global callbacks for better UX
if (typeof window !== 'undefined') {
  saveAntiDuplicate.setCallbacks({
    onBlocked: (key, remainingTime) => {
      // Show a subtle toast or message
      if (window.$toast) {
        window.$toast.info(`请稍等 ${Math.ceil(remainingTime / 100) / 10} 秒后再保存`, {
          duration: 1000,
          position: 'top-center'
        })
      }
    }
  })
}

// Export the class for custom instances
export default AntiDuplicateTrigger
