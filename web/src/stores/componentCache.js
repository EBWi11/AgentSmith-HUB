import { defineStore } from 'pinia'
import { hubApi } from '../api'

export const useComponentCacheStore = defineStore('componentCache', {
  state: () => ({
    // Component data cache
    components: {
      inputs: [],
      outputs: [],
      rulesets: [],
      plugins: [],
      projects: []
    },
    
    // Component parameters cache (for autocomplete)
    parameters: {
      inputs: {}, // inputId -> { fields: [], configs: [] }
      outputs: {}, // outputId -> { fields: [], configs: [] }
      rulesets: {}, // rulesetId -> { fields: [], nodes: [] }
      plugins: {}, // pluginId -> [{ name, type, required }]
      projects: {} // projectId -> { inputs: [], outputs: [], rulesets: [] }
    },
    
    // Cache metadata
    lastUpdated: {
      inputs: 0,
      outputs: 0,
      rulesets: 0,
      plugins: 0,
      projects: 0
    },
    
    // Cache TTL in milliseconds
    cacheTTL: {
      inputs: 5 * 60 * 1000, // 5 minutes
      outputs: 5 * 60 * 1000, // 5 minutes
      rulesets: 5 * 60 * 1000, // 5 minutes
      plugins: 2 * 60 * 1000, // 2 minutes (more dynamic)
      projects: 3 * 60 * 1000 // 3 minutes
    },
    
    // Ongoing requests to prevent duplicates
    ongoingRequests: new Map()
  }),
  
  getters: {
    // Check if cache is valid
    isCacheValid: (state) => (componentType) => {
      const now = Date.now()
      const lastUpdate = state.lastUpdated[componentType] || 0
      const ttl = state.cacheTTL[componentType] || 300000 // 5 minutes default
      return (now - lastUpdate) < ttl
    },
    
    // Get components by type
    getComponents: (state) => (componentType) => {
      return state.components[componentType] || []
    },
    
    // Get component parameters
    getParameters: (state) => (componentType, componentId) => {
      return state.parameters[componentType]?.[componentId] || null
    },
    
    // Get all parameters for a component type
    getAllParameters: (state) => (componentType) => {
      return state.parameters[componentType] || {}
    }
  },
  
  actions: {
    // Fetch components with caching
    async fetchComponents(componentType, options = {}) {
      const { forceRefresh = false } = options
      
      // Check cache validity
      if (!forceRefresh && this.isCacheValid(componentType)) {
        return this.components[componentType]
      }
      
      // Prevent duplicate requests
      const requestKey = `fetch-${componentType}`
      if (this.ongoingRequests.has(requestKey)) {
        return this.ongoingRequests.get(requestKey)
      }
      
      try {
        // Create request promise
        const requestPromise = this._fetchComponentsFromAPI(componentType)
        this.ongoingRequests.set(requestKey, requestPromise)
        
        const components = await requestPromise
        
        // Update cache
        this.components[componentType] = components
        this.lastUpdated[componentType] = Date.now()
        
        return components
      } catch (error) {
        console.error(`Failed to fetch ${componentType}:`, error)
        throw error
      } finally {
        this.ongoingRequests.delete(requestKey)
      }
    },
    
    // Fetch component parameters with batch API
    async fetchComponentParameters(componentType, componentIds = [], options = {}) {
      const { forceRefresh = false } = options
      
      if (!componentIds.length) {
        return {}
      }
      
      // Filter out already cached parameters (unless force refresh)
      const idsToFetch = forceRefresh ? componentIds : 
        componentIds.filter(id => !this.parameters[componentType]?.[id])
      
      if (!idsToFetch.length) {
        // Return cached parameters
        const result = {}
        componentIds.forEach(id => {
          const cached = this.parameters[componentType]?.[id]
          if (cached) {
            result[id] = cached
          }
        })
        return result
      }
      
      // Prevent duplicate requests
      const requestKey = `fetch-params-${componentType}-${idsToFetch.join(',')}`
      if (this.ongoingRequests.has(requestKey)) {
        return this.ongoingRequests.get(requestKey)
      }
      
      try {
        // Create request promise
        const requestPromise = this._fetchParametersFromAPI(componentType, idsToFetch)
        this.ongoingRequests.set(requestKey, requestPromise)
        
        const parameters = await requestPromise
        
        // Update cache
        if (!this.parameters[componentType]) {
          this.parameters[componentType] = {}
        }
        
        Object.entries(parameters).forEach(([id, params]) => {
          this.parameters[componentType][id] = params
        })
        
        return parameters
      } catch (error) {
        console.error(`Failed to fetch ${componentType} parameters:`, error)
        throw error
      } finally {
        this.ongoingRequests.delete(requestKey)
      }
    },
    
    // Invalidate cache for a component type
    invalidateCache(componentType) {
      this.lastUpdated[componentType] = 0
      if (componentType) {
        this.components[componentType] = []
        this.parameters[componentType] = {}
      }
    },
    
    // Invalidate all caches
    invalidateAllCaches() {
      Object.keys(this.components).forEach(type => {
        this.invalidateCache(type)
      })
    },
    
    // Update single component in cache
    updateComponent(componentType, componentId, componentData) {
      const components = this.components[componentType] || []
      const index = components.findIndex(c => c.id === componentId)
      
      if (index >= 0) {
        components[index] = { ...components[index], ...componentData }
      } else {
        components.push({ id: componentId, ...componentData })
      }
      
      this.components[componentType] = components
    },
    
    // Remove component from cache
    removeComponent(componentType, componentId) {
      const components = this.components[componentType] || []
      this.components[componentType] = components.filter(c => c.id !== componentId)
      
      // Also remove parameters
      if (this.parameters[componentType]?.[componentId]) {
        delete this.parameters[componentType][componentId]
      }
    },
    
    // Private methods for API calls
    async _fetchComponentsFromAPI(componentType) {
      switch (componentType) {
        case 'inputs':
          return await hubApi.fetchInputs()
        case 'outputs':
          return await hubApi.fetchOutputs()
        case 'rulesets':
          return await hubApi.fetchRulesets()
        case 'plugins':
          return await hubApi.fetchPlugins()
        case 'projects':
          return await hubApi.fetchProjects()
        default:
          throw new Error(`Unsupported component type: ${componentType}`)
      }
    },
    
    async _fetchParametersFromAPI(componentType, componentIds) {
      switch (componentType) {
        case 'plugins':
          return await hubApi.getBatchPluginParameters(componentIds)
        case 'rulesets':
          // For rulesets, we use the batch API to fetch field keys
          return await hubApi.getBatchRulesetFields(componentIds)
        case 'inputs':
        case 'outputs':
          // For inputs/outputs, we might need to fetch configuration schemas
          // For now, return empty objects
          const emptyResult = {}
          componentIds.forEach(id => {
            emptyResult[id] = { fields: [], configs: [] }
          })
          return emptyResult
        case 'projects':
          // For projects, we might need to fetch component sequences
          const projectResult = {}
          for (const id of componentIds) {
            try {
              const components = await hubApi.getProjectComponents(id)
              projectResult[id] = components
            } catch (error) {
              console.warn(`Failed to fetch components for project ${id}:`, error)
              projectResult[id] = { inputs: [], outputs: [], rulesets: [] }
            }
          }
          return projectResult
        default:
          throw new Error(`Unsupported component type for parameters: ${componentType}`)
      }
    }
  }
}) 