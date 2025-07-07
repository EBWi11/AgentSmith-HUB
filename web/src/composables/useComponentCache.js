import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useComponentCacheStore } from '../stores/componentCache'

export function useComponentCache(componentTypes = [], options = {}) {
  const {
    enableAutoRefresh = true,
    refreshInterval = 120000, // 2 minutes
    enableEventRefresh = true,
    debug = false
  } = options

  const store = useComponentCacheStore()
  const isRefreshing = ref(false)
  const lastRefreshTime = ref(0)
  const refreshTimer = ref(null)
  const eventListeners = ref([])

  // Log function
  const log = (message, ...args) => {
    if (debug) {
      console.log(`[ComponentCache] ${message}`, ...args)
    }
  }

  // Computed properties for each component type
  const components = computed(() => {
    const result = {}
    componentTypes.forEach(type => {
      result[type] = store.getComponents(type)
    })
    return result
  })

  const parameters = computed(() => {
    const result = {}
    componentTypes.forEach(type => {
      result[type] = store.getAllParameters(type)
    })
    return result
  })

  // Fetch components with caching
  const fetchComponents = async (type, options = {}) => {
    if (isRefreshing.value) {
      log(`Already refreshing, skipping fetch for ${type}`)
      return
    }

    try {
      isRefreshing.value = true
      log(`Fetching components: ${type}`)
      
      const components = await store.fetchComponents(type, options)
      lastRefreshTime.value = Date.now()
      
      log(`Successfully fetched ${components.length} ${type}`)
      return components
    } catch (error) {
      log(`Failed to fetch ${type}:`, error)
      throw error
    } finally {
      isRefreshing.value = false
    }
  }

  // Fetch component parameters
  const fetchParameters = async (type, componentIds, options = {}) => {
    if (!componentIds || !componentIds.length) {
      return {}
    }

    try {
      log(`Fetching parameters for ${type}:`, componentIds)
      
      const parameters = await store.fetchComponentParameters(type, componentIds, options)
      
      log(`Successfully fetched parameters for ${Object.keys(parameters).length} ${type}`)
      return parameters
    } catch (error) {
      log(`Failed to fetch ${type} parameters:`, error)
      throw error
    }
  }

  // Refresh all component types
  const refreshAll = async (options = {}) => {
    if (isRefreshing.value) {
      log('Already refreshing, skipping refresh all')
      return
    }

    try {
      isRefreshing.value = true
      log('Refreshing all component types:', componentTypes)
      
      // Fetch all component types in parallel
      const promises = componentTypes.map(type => 
        store.fetchComponents(type, { forceRefresh: true, ...options })
      )
      
      await Promise.all(promises)
      
      // Fetch parameters for components that need them
      const parameterPromises = []
      
      componentTypes.forEach(type => {
        const componentList = store.getComponents(type)
        if (componentList.length > 0) {
          const ids = componentList.filter(c => !c.hasTemp).map(c => c.id)
          if (ids.length > 0) {
            parameterPromises.push(
              store.fetchComponentParameters(type, ids, { forceRefresh: true, ...options })
            )
          }
        }
      })
      
      if (parameterPromises.length > 0) {
        await Promise.all(parameterPromises)
      }
      
      lastRefreshTime.value = Date.now()
      log('Successfully refreshed all component types')
      
    } catch (error) {
      log('Failed to refresh all components:', error)
      throw error
    } finally {
      isRefreshing.value = false
    }
  }

  // Force refresh specific component type
  const forceRefresh = async (type) => {
    return await fetchComponents(type, { forceRefresh: true })
  }

  // Invalidate cache
  const invalidateCache = (type) => {
    if (type) {
      store.invalidateCache(type)
      log(`Cache invalidated for ${type}`)
    } else {
      store.invalidateAllCaches()
      log('All caches invalidated')
    }
  }

  // Setup periodic refresh
  const setupPeriodicRefresh = () => {
    if (!enableAutoRefresh) return

    if (refreshTimer.value) {
      clearInterval(refreshTimer.value)
    }

    refreshTimer.value = setInterval(async () => {
      try {
        log('Periodic refresh triggered')
        await refreshAll()
      } catch (error) {
        log('Periodic refresh failed:', error)
      }
    }, refreshInterval)

    log(`Periodic refresh setup with ${refreshInterval}ms interval`)
  }

  // Setup event-driven refresh
  const setupEventRefresh = () => {
    if (!enableEventRefresh) return

    const handleComponentChange = async (event) => {
      const { detail } = event
      const { type, action } = detail || {}
      
      if (!type || !componentTypes.includes(type)) {
        return
      }

      log(`Component change event: ${action} ${type}`, detail)
      
      try {
        // Refresh the specific component type
        await forceRefresh(type)
        
        // If it's a plugin change, also refresh parameters
        if (type === 'plugins') {
          const pluginList = store.getComponents('plugins')
          const ids = pluginList.filter(c => !c.hasTemp).map(c => c.id)
          if (ids.length > 0) {
            await fetchParameters('plugins', ids, { forceRefresh: true })
          }
        }
      } catch (error) {
        log(`Failed to refresh after ${action} ${type}:`, error)
      }
    }

    // Listen for component change events
    const events = ['pluginChanged', 'componentChanged', 'pendingChangesApplied', 'localChangesLoaded']
    
    events.forEach(eventName => {
      window.addEventListener(eventName, handleComponentChange)
      eventListeners.value.push({
        event: eventName,
        handler: handleComponentChange
      })
    })

    log('Event-driven refresh setup complete')
  }

  // Cleanup function
  const cleanup = () => {
    if (refreshTimer.value) {
      clearInterval(refreshTimer.value)
      refreshTimer.value = null
      log('Periodic refresh timer cleared')
    }

    eventListeners.value.forEach(({ event, handler }) => {
      window.removeEventListener(event, handler)
    })
    eventListeners.value = []
    log('Event listeners cleaned up')
  }

  // Initialize
  const initialize = async () => {
    log('Initializing component cache for types:', componentTypes)
    
    // Setup periodic and event-driven refresh
    setupPeriodicRefresh()
    setupEventRefresh()
    
    // Initial fetch
    try {
      await refreshAll()
    } catch (error) {
      log('Initial fetch failed:', error)
    }
  }

  // Lifecycle hooks
  onMounted(() => {
    initialize()
  })

  onUnmounted(() => {
    cleanup()
  })

  // Watch for component type changes
  watch(() => componentTypes, (newTypes, oldTypes) => {
    if (JSON.stringify(newTypes) !== JSON.stringify(oldTypes)) {
      log('Component types changed, reinitializing')
      cleanup()
      initialize()
    }
  }, { deep: true })

  return {
    // State
    isRefreshing,
    lastRefreshTime,
    components,
    parameters,
    
    // Methods
    fetchComponents,
    fetchParameters,
    refreshAll,
    forceRefresh,
    invalidateCache,
    
    // Store access
    store
  }
}

// Specialized hooks for different use cases
export function useMonacoComponentCache(componentType = 'unknown') {
  const needsPluginCompletion = ['plugin', 'plugins', 'project', 'projects', 'ruleset', 'rulesets'].includes(componentType)
  
  const componentTypes = ['inputs', 'outputs', 'rulesets']
  if (needsPluginCompletion) {
    componentTypes.push('plugins')
  }
  
  return useComponentCache(componentTypes, {
    enableAutoRefresh: true,
    refreshInterval: 120000, // 2 minutes
    enableEventRefresh: true,
    debug: false
  })
}

export function useSidebarComponentCache() {
  return useComponentCache(['inputs', 'outputs', 'rulesets', 'plugins', 'projects'], {
    enableAutoRefresh: true,
    refreshInterval: 180000, // 3 minutes
    enableEventRefresh: true,
    debug: false
  })
}

export function useDashboardComponentCache() {
  return useComponentCache(['inputs', 'outputs', 'rulesets', 'plugins', 'projects'], {
    enableAutoRefresh: true,
    refreshInterval: 300000, // 5 minutes
    enableEventRefresh: true,
    debug: false
  })
} 