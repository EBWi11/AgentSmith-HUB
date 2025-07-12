/**
 * Global event manager - Unified handling of component change events
 * Solves duplicate registration and memory leak issues
 */

class EventManager {
  constructor() {
    this.listeners = new Map() // eventType -> Set<callback>
    this.initialized = false
  }

  /**
   * Initialize event manager (only once)
   */
  initialize() {
    if (this.initialized) return
    
    // Listen for component change events
    window.addEventListener('componentChanged', this.handleComponentChanged.bind(this))
    window.addEventListener('pendingChangesApplied', this.handlePendingChangesApplied.bind(this))
    window.addEventListener('localChangesLoaded', this.handleLocalChangesLoaded.bind(this))
    
    this.initialized = true
    // console.log('[EventManager] Global event listeners initialized')
  }

  /**
   * Register event listener
   * @param {string} eventType - Event type
   * @param {Function} callback - Callback function
   * @returns {Function} - Function to unregister the listener
   */
  on(eventType, callback) {
    if (!this.listeners.has(eventType)) {
      this.listeners.set(eventType, new Set())
    }
    
    const callbacks = this.listeners.get(eventType)
    callbacks.add(callback)
    
    // Ensure event manager is initialized
    this.initialize()
    
    // Return function to unregister the listener
    return () => {
      callbacks.delete(callback)
      if (callbacks.size === 0) {
        this.listeners.delete(eventType)
      }
    }
  }

  /**
   * Unregister event listener
   * @param {string} eventType - Event type
   * @param {Function} callback - Callback function
   */
  off(eventType, callback) {
    const callbacks = this.listeners.get(eventType)
    if (callbacks) {
      callbacks.delete(callback)
      if (callbacks.size === 0) {
        this.listeners.delete(eventType)
      }
    }
  }

  /**
   * Trigger event callbacks
   * @param {string} eventType - Event type
   * @param {any} eventData - Event data
   */
  emit(eventType, eventData) {
    const callbacks = this.listeners.get(eventType)
    if (callbacks) {
      callbacks.forEach(callback => {
        try {
          callback(eventData)
        } catch (error) {
          console.error(`[EventManager] Error in ${eventType} callback:`, error)
        }
      })
    }
  }

  /**
   * Handle component change events
   */
  handleComponentChanged(event) {
    const { action, type, id, timestamp } = event.detail
    // console.log(`[EventManager] Component ${action}: ${type}/${id}`)
    
    this.emit('componentChanged', { action, type, id, timestamp })
  }

  /**
   * Handle pending changes applied events
   */
  handlePendingChangesApplied(event) {
    const { types } = event.detail
    // console.log(`[EventManager] Pending changes applied for types:`, types)
    
    this.emit('pendingChangesApplied', { types })
  }

  /**
   * Handle local changes loaded events
   */
  handleLocalChangesLoaded(event) {
    const { types, type } = event.detail
    // console.log(`[EventManager] Local changes loaded for types:`, types || [type])
    
    const affectedTypes = types || [type]
    this.emit('localChangesLoaded', { types: affectedTypes })
  }

  /**
   * Destroy event manager (cleanup resources)
   */
  destroy() {
    if (this.initialized) {
      window.removeEventListener('componentChanged', this.handleComponentChanged.bind(this))
      window.removeEventListener('pendingChangesApplied', this.handlePendingChangesApplied.bind(this))
      window.removeEventListener('localChangesLoaded', this.handleLocalChangesLoaded.bind(this))
      
      this.listeners.clear()
      this.initialized = false
      // console.log('[EventManager] Event listeners cleaned up')
    }
  }
}

// Create global singleton
const eventManager = new EventManager()

export default eventManager 