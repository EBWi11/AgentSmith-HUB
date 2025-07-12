import { defineStore } from 'pinia'
import { hubApi } from '../api'
import eventManager from '../utils/eventManager'

// Add at top-level (outside Pinia store) a non-reactive map to track in-flight requests
const ongoingRequests = new Map()

export const useDataCacheStore = defineStore('dataCache', {
  state: () => ({
    // Component data cache
    components: {
      inputs: { data: [], timestamp: 0, loading: false },
      outputs: { data: [], timestamp: 0, loading: false },
      rulesets: { data: [], timestamp: 0, loading: false },
      plugins: { data: [], timestamp: 0, loading: false },
      projects: { data: [], timestamp: 0, loading: false }
    },
    
    // System metrics cache
    systemMetrics: {
      data: {},
      timestamp: 0,
      loading: false
    },
    
    // Message statistics cache
    messageStats: {
      data: {},
      timestamp: 0,
      loading: false
    },
    
    // Plugin statistics cache - Store as Map with LRU mechanism
    pluginStats: new Map(),
    
    // Cluster status cache
    clusterInfo: {
      data: {},
      timestamp: 0,
      loading: false
    },
    clusterProjectStates: { data: {}, timestamp: 0, loading: false },
    
    // Pending changes cache
    pendingChanges: {
      data: [],
      timestamp: 0,
      loading: false
    },
    
    // Local changes cache
    localChanges: {
      data: [],
      timestamp: 0,
      loading: false
    },
    
    // Operations history cache - Store as Map with LRU mechanism
    operationsHistory: new Map(),
    
    // Event cleanup functions
    _eventCleanupFunctions: []
  }),
  
  getters: {
    // Check if data is expired
    isExpired: (state) => (key, ttl = 60000) => {
      const cache = state[key]
      if (!cache) return true
      return Date.now() - cache.timestamp > ttl
    },
    
    // Get component data
    getComponentData: (state) => (type) => {
      return state.components[type]?.data || []
    },
    
    // Check if component data is expired
    isComponentExpired: (state) => (type, ttl = 60000) => {
      const cache = state.components[type]
      if (!cache) return true
      return Date.now() - cache.timestamp > ttl
    }
  },
  
  actions: {
    // Initialize event listeners using the unified event manager
    initializeEventListeners() {
      if (this._eventCleanupFunctions.length > 0) return // Already initialized
      
      // Use unified event manager instead of direct window listeners
      const componentChangedCleanup = eventManager.on('componentChanged', (data) => {
        const { action, type, id } = data
        
        switch (action) {
          case 'created':
            this.clearComponentCache(type)
            break
          case 'updated':
            this.clearComponentCache(type)
            // Also clear the specific component detail cache
            if (id) {
              const detailCacheKey = `detail_${type}_${id}`
              ongoingRequests.delete(detailCacheKey)
            }
            break
          case 'deleted':
            // Use specialized method for deletion
            if (id) {
              this.clearComponentRelatedCaches(type, id)
            } else {
              this.clearComponentCache(type)
            }
            break
        }
        
        // For projects, also clear cluster info cache as project status might change
        if (type === 'projects') {
          this.clearCache('clusterInfo')
          this.clearCache('clusterProjectStates')
        }
      })
      
      const pendingChangesCleanup = eventManager.on('pendingChangesApplied', (data) => {
        const { types } = data
        
        if (Array.isArray(types)) {
          types.forEach(type => {
            this.clearComponentCache(type)
          })
        }
        
        // Also clear pending changes cache
        this.clearCache('pendingChanges')
      })
      
      const localChangesCleanup = eventManager.on('localChangesLoaded', (data) => {
        const { types } = data
        
        if (Array.isArray(types)) {
          types.forEach(type => {
            this.clearComponentCache(type)
          })
        }
        
        // Also clear local changes cache
        this.clearCache('localChanges')
      })
      
      // Store cleanup functions
      this._eventCleanupFunctions.push(
        componentChangedCleanup,
        pendingChangesCleanup,
        localChangesCleanup
      )
      
    },
    
    // Cleanup event listeners
    cleanupEventListeners() {
      this._eventCleanupFunctions.forEach(cleanup => cleanup())
      this._eventCleanupFunctions = []
    },

    // Generic cache fetch method
    async fetchWithCache(key, fetcher, ttl = 60000, forceRefresh = false) {
      // Initialize event listeners on first use
      this.initializeEventListeners()
      
      const cacheKey = key
      // If there is already an in-flight request for this key, return the same Promise
      if (ongoingRequests.has(cacheKey)) {
        return ongoingRequests.get(cacheKey)
      }

      const cache = this[key]
      
      // If cache doesn't exist, return error
      if (!cache) {
        console.error(`Cache key '${key}' not found in dataCache store`)
        throw new Error(`Cache key '${key}' not found`)
      }
      
      // If data not expired and not forcing refresh, return cached data
      if (!forceRefresh && cache && !this.isExpired(key, ttl)) {
        return cache.data
      }

      // Create a Promise for the fetcher and store it in the map to deduplicate
      const requestPromise = (async () => {
        cache.loading = true
        try {
          const data = await fetcher()
          cache.data = data
          cache.timestamp = Date.now()
          return data
        } catch (error) {
          console.error(`Failed to fetch ${key}:`, error)
          // If has cached data, return cached data
          if (cache.data) {
            return cache.data
          }
          throw error
        } finally {
          cache.loading = false
          ongoingRequests.delete(cacheKey)
        }
      })()

      ongoingRequests.set(cacheKey, requestPromise)
      return requestPromise
    },
    
    // Get component data
    async fetchComponents(type, forceRefresh = false) {
      // Initialize event listeners on first use
      this.initializeEventListeners()
      
      const cacheKey = `components_${type}`
      
      // If there is already an in-flight request for this key, return the same Promise
      if (ongoingRequests.has(cacheKey)) {
        return ongoingRequests.get(cacheKey)
      }

      const cache = this.components[type]
      
      // If cache doesn't exist, check if it's a special UI type
      if (!cache) {
        // Special UI types that are not actual component types
        const uiTypes = ['settings', 'cluster', 'pending-changes', 'load-local-components', 'operations-history', 'error-logs', 'tutorial', 'home']
        if (uiTypes.includes(type)) {
          console.warn(`Attempted to fetch '${type}' as component type, but it's a UI type. Returning empty array.`)
          return []
        }
        
        console.error(`Component type '${type}' not found in dataCache store`)
        console.error('Call stack:', new Error().stack)
        throw new Error(`Component type '${type}' not found`)
      }
      
      // If data not expired and not forcing refresh, return cached data
      if (!forceRefresh && !this.isComponentExpired(type, 60000)) {
        return cache.data
      }
      
      // Create a Promise for the fetcher and store it in the map to deduplicate
      const requestPromise = (async () => {
        cache.loading = true
        try {
          const data = await hubApi.fetchComponentsWithTempInfo(type)
          cache.data = data
          cache.timestamp = Date.now()
          return data
        } catch (error) {
          console.error(`Failed to fetch ${type}:`, error)
          // If has cached data, return cached data
          if (cache.data && cache.data.length > 0) {
            return cache.data
          }
          throw error
        } finally {
          cache.loading = false
          ongoingRequests.delete(cacheKey)
        }
      })()

      ongoingRequests.set(cacheKey, requestPromise)
      return requestPromise
    },
    
    // Get system metrics
    async fetchSystemMetrics(forceRefresh = false) {
      return this.fetchWithCache(
        'systemMetrics',
        // 使用集群系统指标接口可返回各节点详细数据（leader 节点可用；follower 节点将返回 400 并在调用处兜底）
        () => hubApi.getClusterSystemMetrics(),
        15000, // 15s TTL, system metrics update frequently
        forceRefresh
      )
    },
    
    // Get message statistics
    async fetchMessageStats(forceRefresh = false) {
      return this.fetchWithCache(
        'messageStats',
        () => hubApi.getAggregatedDailyMessages(),
        60000, // 1min TTL
        forceRefresh
      )
    },
    
    // Get plugin statistics with LRU cache
    async fetchPluginStats(date, forceRefresh = false) {
      const cacheKey = `pluginStats_${date}`
      
      // If there is already an in-flight request for this key, return the same Promise
      if (ongoingRequests.has(cacheKey)) {
        return ongoingRequests.get(cacheKey)
      }
      
      // Check if cache exists and is not expired
      const cache = this.pluginStats.get(date)
      if (!forceRefresh && cache && (Date.now() - cache.timestamp) <= 60000) {
        // Move to end (LRU)
        this.pluginStats.delete(date)
        this.pluginStats.set(date, cache)
        return cache.data
      }
      
      // Create a Promise for the fetcher and store it in the map to deduplicate
      const requestPromise = (async () => {
        // Create cache entry if not exists
        let cacheEntry = this.pluginStats.get(date) || { data: {}, timestamp: 0, loading: false }
        cacheEntry.loading = true
        this.pluginStats.set(date, cacheEntry)
        
        try {
          const data = await hubApi.getPluginStats({ date })
          cacheEntry.data = data
          cacheEntry.timestamp = Date.now()
          cacheEntry.loading = false
          
          // LRU cleanup: keep only last 10 entries
          if (this.pluginStats.size > 10) {
            const firstKey = this.pluginStats.keys().next().value
            this.pluginStats.delete(firstKey)
          }
          
          return data
        } catch (error) {
          console.error(`Failed to fetch plugin stats for ${date}:`, error)
          // If has cached data, return cached data
          if (cacheEntry.data && Object.keys(cacheEntry.data).length > 0) {
            return cacheEntry.data
          }
          throw error
        } finally {
          cacheEntry.loading = false
          ongoingRequests.delete(cacheKey)
        }
      })()

      ongoingRequests.set(cacheKey, requestPromise)
      return requestPromise
    },
    
    // Get cluster information
    async fetchClusterInfo(forceRefresh = false) {
      return this.fetchWithCache(
        'clusterInfo',
        () => hubApi.fetchClusterInfo(),
        60000, // 1min TTL
        forceRefresh
      )
    },

    // Get cluster project states (leader only)
    async fetchClusterProjectStates(forceRefresh = false) {
      return this.fetchWithCache(
        'clusterProjectStates',
        () => hubApi.getClusterProjectStates(),
        10000, // 10s TTL – project state can change quickly
        forceRefresh
      )
    },
    
    // Get pending changes
    async fetchPendingChanges(forceRefresh = false) {
      return this.fetchWithCache(
        'pendingChanges',
        () => hubApi.fetchEnhancedPendingChanges(),
        10000, // 10s TTL, change status updates frequently
        forceRefresh
      )
    },
    
    // Get local changes
    async fetchLocalChanges(forceRefresh = false) {
      return this.fetchWithCache(
        'localChanges',
        () => hubApi.fetchLocalChanges(),
        10000, // 10s TTL
        forceRefresh
      )
    },
    
    // Get operations history with LRU cache
    async fetchOperationsHistory(params, forceRefresh = false) {
      const cacheKey = `operationsHistory_${JSON.stringify(params)}`
      const paramsKey = JSON.stringify(params)
      
      // If there is already an in-flight request for this key, return the same Promise
      if (ongoingRequests.has(cacheKey)) {
        return ongoingRequests.get(cacheKey)
      }
      
      // Check if cache exists and is not expired
      const cache = this.operationsHistory.get(paramsKey)
      if (!forceRefresh && cache && (Date.now() - cache.timestamp) <= 60000) {
        // Move to end (LRU)
        this.operationsHistory.delete(paramsKey)
        this.operationsHistory.set(paramsKey, cache)
        return { operations: cache.data, total_count: cache.totalCount }
      }
      
      // Create a Promise for the fetcher and store it in the map to deduplicate
      const requestPromise = (async () => {
        // Create cache entry if not exists
        let cacheEntry = this.operationsHistory.get(paramsKey) || { data: [], totalCount: 0, timestamp: 0, loading: false }
        cacheEntry.loading = true
        this.operationsHistory.set(paramsKey, cacheEntry)
        
        try {
          const data = await hubApi.getOperationsHistory(params)
          cacheEntry.data = data.operations || []
          cacheEntry.totalCount = data.total_count || 0
          cacheEntry.timestamp = Date.now()
          cacheEntry.loading = false
          
          // LRU cleanup: keep only last 5 entries
          if (this.operationsHistory.size > 5) {
            const firstKey = this.operationsHistory.keys().next().value
            this.operationsHistory.delete(firstKey)
          }
          
          return data
        } catch (error) {
          console.error(`Failed to fetch operations history:`, error)
          // If has cached data, return cached data
          if (cacheEntry.data && cacheEntry.data.length > 0) {
            return { operations: cacheEntry.data, total_count: cacheEntry.totalCount }
          }
          throw error
        } finally {
          cacheEntry.loading = false
          ongoingRequests.delete(cacheKey)
        }
      })()

      ongoingRequests.set(cacheKey, requestPromise)
      return requestPromise
    },
    
    // Get single component detail with caching
    async fetchComponentDetail(type, id, forceRefresh = false) {
      const cacheKey = `detail_${type}_${id}`
      
      // If there is already an in-flight request for this key, return the same Promise
      if (ongoingRequests.has(cacheKey)) {
        return ongoingRequests.get(cacheKey)
      }
      
      // Check component cache first - if we have recent list data, use it
      const listCache = this.components[type]
      if (!forceRefresh && listCache && listCache.data && (Date.now() - listCache.timestamp) <= 60000) {
        // Try to find the component in the list cache first
        const cachedComponent = listCache.data.find(item => {
          const itemId = item.id || item.name
          return itemId === id
        })
        
        if (cachedComponent) {
          // If it's a basic list item, we still need full details
          // But we can skip the request if we already have full data
          if (cachedComponent.raw) {
            return cachedComponent
          }
        }
      }
      
      // Create a Promise for the fetcher and store it in the map to deduplicate
      const requestPromise = (async () => {
        try {
          let data
          
          // Call appropriate detail API
          switch (type) {
            case 'inputs':
              data = await hubApi.getInput(id)
              break
            case 'outputs':
              data = await hubApi.getOutput(id)
              break
            case 'rulesets':
              data = await hubApi.getRuleset(id)
              break
            case 'projects':
              data = await hubApi.getProject(id)
              
              // Get project status from cluster info
              try {
                const clusterStatus = await this.fetchClusterInfo()
                if (clusterStatus && clusterStatus.projects) {
                  const projectStatus = clusterStatus.projects.find(p => p.id === id)
                  if (projectStatus) {
                    data.status = projectStatus.status || 'stopped'
                  } else {
                    data.status = 'stopped'
                  }
                }
              } catch (statusError) {
                console.error('Failed to fetch project status:', statusError)
                data.status = 'unknown'
              }
              break
            case 'plugins':
              data = await hubApi.getPlugin(id)
              break
            default:
              throw new Error(`Unsupported component type: ${type}`)
          }
          
          // Check if this is a temporary file
          if (data && data.path) {
            data.isTemporary = data.path.endsWith('.new')
          }
          
          return data
        } catch (error) {
          console.error(`Failed to fetch ${type} detail for ${id}:`, error)
          throw error
        } finally {
          ongoingRequests.delete(cacheKey)
        }
      })()

      ongoingRequests.set(cacheKey, requestPromise)
      return requestPromise
    },
    
    // Clear specific cache
    clearCache(key) {
      if (this[key]) {
        this[key].data = Array.isArray(this[key].data) ? [] : {}
        this[key].timestamp = 0
        this[key].loading = false
      }
    },
    
    // Clear all caches related to a specific component
    clearComponentRelatedCaches(type, id) {
      // Clear from component list cache
      this.removeComponentItem(type, id)
      
      // Clear component detail cache
      const detailCacheKey = `detail_${type}_${id}`
      ongoingRequests.delete(detailCacheKey)
      
    },
    
    // Clear component cache
    clearComponentCache(type) {
      if (this.components[type]) {
        this.components[type].data = []
        this.components[type].timestamp = 0
        this.components[type].loading = false
      }
      
      // Also clear component detail caches for this type
      const keysToDelete = []
      for (const [key] of ongoingRequests) {
        if (key.startsWith(`detail_${type}_`)) {
          keysToDelete.push(key)
        }
      }
      keysToDelete.forEach(key => ongoingRequests.delete(key))
      
    },
    
    // Clear all cache
    clearAllCache() {
      // Clear component cache
      Object.keys(this.components).forEach(type => {
        this.clearComponentCache(type)
      })
      
      // Clear other cache
      this.clearCache('systemMetrics')
      this.clearCache('messageStats')
      this.clearCache('clusterInfo')
      this.clearCache('pendingChanges')
      this.clearCache('localChanges')
      
      // Clear Map-based caches
      this.pluginStats.clear()
      this.operationsHistory.clear()
    },
    
    // Incremental update component data
    updateComponentItem(type, item) {
      const cache = this.components[type]
      if (!cache.data) return
      
      const index = cache.data.findIndex(existing => existing.id === item.id)
      if (index >= 0) {
        cache.data[index] = { ...cache.data[index], ...item }
      } else {
        cache.data.push(item)
      }
      
      // Update timestamp
      cache.timestamp = Date.now()
    },
    
    // Remove component data
    removeComponentItem(type, id) {
      const cache = this.components[type]
      if (!cache.data) return
      
      const index = cache.data.findIndex(item => item.id === id)
      if (index >= 0) {
        cache.data.splice(index, 1)
        cache.timestamp = Date.now()
      }
    },
    
    // Batch update data
    batchUpdate(updates) {
      updates.forEach(({ type, action, data }) => {
        switch (action) {
          case 'update':
            if (type.startsWith('component_')) {
              const componentType = type.replace('component_', '')
              this.updateComponentItem(componentType, data)
            } else {
              this[type].data = data
              this[type].timestamp = Date.now()
            }
            break
          case 'remove':
            if (type.startsWith('component_')) {
              const componentType = type.replace('component_', '')
              this.removeComponentItem(componentType, data.id)
            }
            break
        }
      })
    }
  }
}) 