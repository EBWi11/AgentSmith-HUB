import { defineStore } from 'pinia'
import { hubApi } from '../api'
import eventManager from '../utils/eventManager'

// Add at top-level (outside Pinia store) a non-reactive map to track in-flight requests
const ongoingRequests = new Map()

// Priority refresh queue to ensure operation-triggered refreshes have highest priority
const priorityRefreshQueue = new Set()

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
    
    // Available plugins cache (moved from Vuex)
    availablePlugins: {
      data: [],
      timestamp: 0,
      loading: false
    },
    
    // Ruleset fields cache (moved from Vuex) - Store as Map with LRU mechanism
    rulesetFields: new Map(),
    
    // Test data caches (consolidated from cacheUtils)
    testCaches: {
      rulesets: new Map(),
      projects: new Map()
    },
    
    // UI state persistence (basic replacement for stateManager)
    uiStates: {
      sidebarCollapsed: {
        inputs: true,
        outputs: true,
        rulesets: true,
        plugins: true,
        projects: true,
        settings: true,
        builtinPlugins: true
      },
      sidebarSearch: '',
      lastUpdate: 0
    },
    
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
    
    // Settings badges cache
    settingsBadges: {
      data: {
        'pending-changes': 0,
        'load-local-components': 0,
        'error-logs': 0
      },
      timestamp: 0,
      loading: false
    },
    
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
        
        // Normalize type to plural form
        // Handle both singular ('input') and plural ('inputs') forms
        let normalizedType = type
        if (!type.endsWith('s')) {
          // Convert singular to plural
          if (type === 'ruleset') {
            normalizedType = 'rulesets'
          } else {
            normalizedType = type + 's'
          }
        }
        
        switch (action) {
          case 'created':
            this.clearComponentCache(normalizedType)
            // HIGHEST PRIORITY: Force immediate refresh after creation
            setTimeout(() => {
              this.fetchComponents(normalizedType, true, true) // isPriorityRefresh = true
            }, 150)
            break
          case 'updated':
            this.clearComponentCache(normalizedType)
            // Also clear the specific component detail cache
            if (id) {
              const detailCacheKey = `detail_${normalizedType}_${id}`
              ongoingRequests.delete(detailCacheKey)
            }
            // HIGHEST PRIORITY: Force immediate refresh after update
            setTimeout(() => {
              this.fetchComponents(normalizedType, true, true) // isPriorityRefresh = true
            }, 150)
            break
          case 'deleted':
            // Use specialized method for deletion
            if (id) {
              this.clearComponentRelatedCaches(normalizedType, id)
            } else {
              this.clearComponentCache(normalizedType)
            }
            // HIGHEST PRIORITY: Force immediate refresh after deletion
            setTimeout(() => {
              this.fetchComponents(normalizedType, true, true) // isPriorityRefresh = true
            }, 150)
            break
        }
        
        // For projects, also clear cluster info cache as project status might change
        if (normalizedType === 'projects') {
          this.clearCache('clusterInfo')
          this.clearCache('clusterProjectStates')
        }
        
        // Handle ruleset-specific caches
        if (normalizedType === 'rulesets' && id) {
          this.clearRulesetFields(id)
          // Test cache is now cleared automatically in clearComponentRelatedCaches
        }
        
        // Handle plugin-specific caches
        if (normalizedType === 'plugins') {
          this.clearCache('availablePlugins')
        }
        
        // Handle project-specific caches
        if (normalizedType === 'projects' && id) {
          // Test cache is now cleared automatically in clearComponentRelatedCaches
        }
      })
      
      const pendingChangesCleanup = eventManager.on('pendingChangesApplied', (data) => {
        const { types } = data
        
        if (Array.isArray(types)) {
          types.forEach(type => {
            this.clearComponentCache(type)
            // HIGHEST PRIORITY: Force immediate refresh after pending changes applied
            setTimeout(() => {
              this.fetchComponents(type, true, true) // isPriorityRefresh = true
            }, 150)
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
            // HIGHEST PRIORITY: Force immediate refresh after local changes loaded
            setTimeout(() => {
              this.fetchComponents(type, true, true) // isPriorityRefresh = true
            }, 150)
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
    async fetchComponents(type, forceRefresh = false, isPriorityRefresh = false) {
      // Initialize event listeners on first use
      this.initializeEventListeners()
      
      const cacheKey = `components_${type}`
      
      // HIGHEST PRIORITY: If this is a priority refresh, cancel existing request and proceed immediately
      if (isPriorityRefresh) {
        priorityRefreshQueue.add(cacheKey)
        if (ongoingRequests.has(cacheKey)) {
          // Cancel existing request to prioritize this one
          ongoingRequests.delete(cacheKey)
        }
      } else {
        // If there is already an in-flight request for this key, return the same Promise
        if (ongoingRequests.has(cacheKey)) {
          return ongoingRequests.get(cacheKey)
        }
        
        // If this is in priority queue, don't proceed with normal request
        if (priorityRefreshQueue.has(cacheKey)) {
          // Wait a bit and try again
          await new Promise(resolve => setTimeout(resolve, 50))
          return this.fetchComponents(type, forceRefresh, false)
        }
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
      
      // Dynamic TTL based on component type
      const componentTTL = type === 'projects' ? 30000 : 60000 // Projects: 30s, Others: 60s
      
      // If data not expired and not forcing refresh, return cached data
      if (!forceRefresh && !this.isComponentExpired(type, componentTTL)) {
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
          // Clean up priority queue
          priorityRefreshQueue.delete(cacheKey)
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
        30000, // 30s TTL - optimized for sidebar refresh intervals
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
      
      // Clear test caches automatically
      if (type === 'rulesets') {
        this.clearTestCache('rulesets', id)
      } else if (type === 'projects') {
        this.clearTestCache('projects', id)
      }
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
      
      // Clear all test caches for this component type
      if (type === 'rulesets') {
        this.clearTestCache('rulesets')
      } else if (type === 'projects') {
        this.clearTestCache('projects')
      }
    },
    
    // Update component cache
    updateComponentCache(type, data) {
      if (!this.components[type]) {
        this.components[type] = { data: [], timestamp: 0, loading: false }
      }
      this.components[type].data = data
      this.components[type].timestamp = Date.now()
      this.components[type].loading = false
    },
    
    // Clear all cache (but preserve UI states)
    clearAllCache() {
      // Clear component cache
      Object.keys(this.components).forEach(type => {
        this.clearComponentCache(type)
      })
      
      // Clear other cache
      this.clearCache('systemMetrics')
      this.clearCache('messageStats')
      this.clearCache('clusterInfo')
      this.clearCache('clusterProjectStates')
      this.clearCache('pendingChanges')
      this.clearCache('localChanges')
      this.clearCache('availablePlugins')
      this.clearCache('settingsBadges')
      
      // Clear Map-based caches
      this.pluginStats.clear()
      this.rulesetFields.clear()
      this.operationsHistory.clear()
      this.testCaches.rulesets.clear()
      this.testCaches.projects.clear()
      
      // Clear ongoing requests
      ongoingRequests.clear()
      priorityRefreshQueue.clear()
      
      // Note: UI states are preserved to maintain user experience
    },
    
    // Alias for backward compatibility
    clearAll() {
      this.clearAllCache()
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
      if (cache && cache.data) {
        const index = cache.data.findIndex(item => (item.id || item.name) === id)
        if (index !== -1) {
          cache.data.splice(index, 1)
        }
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
    },
    
    // Available plugins methods (moved from Vuex)
    async fetchAvailablePlugins(forceRefresh = false) {
      return this.fetchWithCache(
        'availablePlugins',
        () => hubApi.getAvailablePlugins(),
        300000, // 5min TTL, plugins don't change often
        forceRefresh
      )
    },
    
    // Ruleset fields methods (moved from Vuex)
    async fetchRulesetFields(rulesetId, forceRefresh = false) {
      const cacheKey = `rulesetFields_${rulesetId}`
      
      // If there is already an in-flight request for this key, return the same Promise
      if (ongoingRequests.has(cacheKey)) {
        return ongoingRequests.get(cacheKey)
      }
      
      // Check if cache exists and is not expired (reduced TTL to 1 minute for better responsiveness)
      const cache = this.rulesetFields.get(rulesetId)
      if (!forceRefresh && cache && cache.data && (Date.now() - cache.timestamp) <= 60000) {
        // Move to end (LRU)
        this.rulesetFields.delete(rulesetId)
        this.rulesetFields.set(rulesetId, cache)
        return cache.data
      }
      
      // Create a Promise for the fetcher and store it in the map to deduplicate
      const requestPromise = (async () => {
        // Create cache entry if not exists
        let cacheEntry = this.rulesetFields.get(rulesetId) || { data: { fieldKeys: [], sampleCount: 0 }, timestamp: 0, loading: false }
        cacheEntry.loading = true
        this.rulesetFields.set(rulesetId, cacheEntry)
        
        try {
          const data = await hubApi.getRulesetFields(rulesetId)
          
          cacheEntry.data = data || { fieldKeys: [], sampleCount: 0 }
          cacheEntry.timestamp = Date.now()
          cacheEntry.loading = false
          
          // If no field keys returned, schedule a retry after 10 minutes
          if (cacheEntry.data.fieldKeys && cacheEntry.data.fieldKeys.length === 0) {
            setTimeout(() => {
              this.fetchRulesetFields(rulesetId, true) // Force refresh
            }, 600000)
          }
          
          // LRU cleanup: keep only last 20 entries
          if (this.rulesetFields.size > 20) {
            const firstKey = this.rulesetFields.keys().next().value
            this.rulesetFields.delete(firstKey)
          }
          
          return cacheEntry.data
        } catch (error) {
          console.warn(`Failed to fetch fields for ruleset ${rulesetId}:`, error)
          const fallbackData = { fieldKeys: [], sampleCount: 0 }
          cacheEntry.data = fallbackData
          cacheEntry.loading = false
          return fallbackData
        } finally {
          ongoingRequests.delete(cacheKey)
        }
      })()

      ongoingRequests.set(cacheKey, requestPromise)
      return requestPromise
    },
    
    // Clear ruleset fields cache
    clearRulesetFields(rulesetId = null) {
      if (rulesetId) {
        this.rulesetFields.delete(rulesetId)
      } else {
        this.rulesetFields.clear()
      }
    },
    
    // Test cache methods (consolidated from cacheUtils)
    getTestCache(type, id) {
      if (this.testCaches[type]) {
        const cache = this.testCaches[type].get(id)
        if (cache) {
          // Check if cache is expired (30 minutes TTL)
          const ttl = 30 * 60 * 1000
          if (Date.now() - cache.timestamp > ttl) {
            this.testCaches[type].delete(id)
            return null
          }
          return cache.data
        }
      }
      return null
    },
    
    setTestCache(type, id, data) {
      if (this.testCaches[type]) {
        this.testCaches[type].set(id, {
          data,
          timestamp: Date.now()
        })
        
        // LRU cleanup: keep only last 10 entries per type
        if (this.testCaches[type].size > 10) {
          const firstKey = this.testCaches[type].keys().next().value
          this.testCaches[type].delete(firstKey)
        }
      }
    },
    
    clearTestCache(type, id = null) {
      if (this.testCaches[type]) {
        if (id) {
          this.testCaches[type].delete(id)
        } else {
          this.testCaches[type].clear()
        }
      }
    },
    
    // UI state management methods
    saveSidebarState(collapsed, search) {
      if (collapsed) {
        Object.assign(this.uiStates.sidebarCollapsed, collapsed)
      }
      if (search !== undefined) {
        this.uiStates.sidebarSearch = search
      }
      this.uiStates.lastUpdate = Date.now()
      
      // Save to localStorage for persistence across page refreshes
      try {
        const stateToSave = {
          collapsed: { ...this.uiStates.sidebarCollapsed },
          search: this.uiStates.sidebarSearch,
          timestamp: Date.now()
        }
        localStorage.setItem('agentsmith_sidebar_state', JSON.stringify(stateToSave))
      } catch (error) {
        console.warn('Failed to save sidebar state to localStorage:', error)
      }
    },
    
    restoreSidebarState() {
      // First try to restore from localStorage (persistent across refreshes)
      try {
        const savedState = localStorage.getItem('agentsmith_sidebar_state')
        if (savedState) {
          const parsedState = JSON.parse(savedState)
          // Check if saved state is not too old (24 hours)
          const maxAge = 24 * 60 * 60 * 1000 // 24 hours
          if (Date.now() - parsedState.timestamp < maxAge) {
            return {
              collapsed: parsedState.collapsed || { ...this.uiStates.sidebarCollapsed },
              search: parsedState.search || ''
            }
          }
        }
      } catch (error) {
        console.warn('Failed to restore sidebar state from localStorage:', error)
      }
      
      // Fallback to memory state (only if saved within last 5 minutes)
      const maxAge = 5 * 60 * 1000 // 5 minutes
      if (Date.now() - this.uiStates.lastUpdate > maxAge) {
        return null
      }
      
      return {
        collapsed: { ...this.uiStates.sidebarCollapsed },
        search: this.uiStates.sidebarSearch
      }
    },
    
    clearUIStates() {
      this.uiStates.sidebarCollapsed = {
        inputs: true,
        outputs: true,
        rulesets: true,
        plugins: true,
        projects: true,
        settings: true,
        builtinPlugins: true
      }
      this.uiStates.sidebarSearch = ''
      this.uiStates.lastUpdate = 0
      
      // Also clear localStorage state
      try {
        localStorage.removeItem('agentsmith_sidebar_state')
      } catch (error) {
        console.warn('Failed to clear sidebar state from localStorage:', error)
      }
    },

    // Fetch settings badges data
    async fetchSettingsBadges(force = false) {
      // No caching for badges - always fetch fresh data
      if (this.settingsBadges.loading) {
        return this.settingsBadges.data
      }

      this.settingsBadges.loading = true

      try {
        // Get pending changes count from API (always fresh data for badges)
        let pendingCount = 0
        try {
          const pendingData = await hubApi.fetchEnhancedPendingChanges()
          pendingCount = Array.isArray(pendingData) ? pendingData.length : 0
        } catch (e) {
          console.warn('Failed to fetch pending changes for badge:', e)
          // Fallback to cache if API fails
          if (this.pendingChanges.data && Array.isArray(this.pendingChanges.data)) {
            pendingCount = this.pendingChanges.data.length
          }
        }

        // Get local changes count from lightweight API (always fresh data for badges)
        let localCount = 0
        try {
          localCount = await hubApi.fetchLocalChangesCount()
        } catch (e) {
          console.warn('Failed to fetch local changes count for badge:', e)
          // Fallback to cache if API fails
          if (this.localChanges.data && Array.isArray(this.localChanges.data)) {
            localCount = this.localChanges.data.length
          }
        }

        // Get error logs count for last hour
        let errorCount = 0
        try {
          const oneHourAgo = new Date(Date.now() - 60 * 60 * 1000).toISOString()
          const params = {
            start_time: oneHourAgo,
            limit: '1000'
          }
          
          const errorData = await hubApi.getErrorLogs(params)
          errorCount = errorData?.total_count || 0
        } catch (e) {
          console.warn('Failed to fetch error logs for badge:', e)
        }

        this.settingsBadges.data = {
          'pending-changes': pendingCount,
          'load-local-components': localCount,
          'error-logs': errorCount
        }
        this.settingsBadges.timestamp = Date.now()

        return this.settingsBadges.data
      } catch (error) {
        console.error('Failed to fetch settings badges:', error)
        throw error
      } finally {
        this.settingsBadges.loading = false
      }
    }
  }
}) 