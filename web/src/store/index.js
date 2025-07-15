import { createStore } from 'vuex'
import { hubApi } from '../api'
import eventManager from '../utils/eventManager'

const store = createStore({
  state: {
    // components moved to dataCache for unified management
    availablePlugins: [],
    nodeTypes: [
      { value: 'INCL', detail: 'String contains check' },
      { value: 'NI', detail: 'String not contains check' },
      { value: 'END', detail: 'String ends with check' },
      { value: 'START', detail: 'String starts with check' },
      { value: 'NEND', detail: 'String not ends with check' },
      { value: 'NSTART', detail: 'String not starts with check' },
      { value: 'NCS_INCL', detail: 'Case-insensitive contains check' },
      { value: 'NCS_NI', detail: 'Case-insensitive not contains check' },
      { value: 'NCS_END', detail: 'Case-insensitive ends with check' },
      { value: 'NCS_START', detail: 'Case-insensitive starts with check' },
      { value: 'NCS_NEND', detail: 'Case-insensitive not ends with check' },
      { value: 'NCS_NSTART', detail: 'Case-insensitive not starts with check' },
      { value: 'REGEX', detail: 'Regular expression check' },
      { value: 'ISNULL', detail: 'Field is null check' },
      { value: 'NOTNULL', detail: 'Field is not null check' },
      { value: 'EQU', detail: 'Equal check' },
      { value: 'NEQ', detail: 'Not equal check' },
      { value: 'NCS_EQU', detail: 'Case-insensitive equal check' },
      { value: 'NCS_NEQ', detail: 'Case-insensitive not equal check' },
      { value: 'MT', detail: 'More than check' },
      { value: 'LT', detail: 'Less than check' },
      { value: 'PLUGIN', detail: 'Plugin check' }
    ],
    logicTypes: [
      { value: 'AND', detail: 'All values must match' },
      { value: 'OR', detail: 'Any value can match' }
    ],
    countTypes: [
      { value: 'SUM', detail: 'Sum values' },
      { value: 'CLASSIFY', detail: 'Count unique values' }
    ],
    rootTypes: [
      { value: 'DETECTION', detail: 'Detection rule type' },
      { value: 'WHITELIST', detail: 'Whitelist rule type' }
    ],
    commonFields: [
      { value: 'data', detail: 'Data field' },
      { value: 'data_type', detail: 'Data type field' },
      { value: 'exe', detail: 'Executable field' },
      { value: 'dip', detail: 'Destination IP field' },
      { value: 'sip', detail: 'Source IP field' },
      { value: 'dport', detail: 'Destination port field' },
      { value: 'sport', detail: 'Source port field' },
      { value: 'pid', detail: 'Process ID field' }
    ],
    // pendingChanges moved to dataCache for unified management
    rulesetFields: {}, // Cache for ruleset field keys: { rulesetId: { fieldKeys: [...], sampleCount: 0 } }
    _eventCleanupFunctions: []
  },
  getters: {
    // getComponents moved to dataCache
    getAvailablePlugins: (state) => {
      return state.availablePlugins
    },
    getNodeTypes: (state) => {
      return state.nodeTypes
    },
    getLogicTypes: (state) => {
      return state.logicTypes
    },
    getCountTypes: (state) => {
      return state.countTypes
    },
    getRootTypes: (state) => {
      return state.rootTypes
    },
    getCommonFields: (state) => {
      return state.commonFields
    },
    // getPendingChanges moved to dataCache
    getRulesetFields: (state) => (rulesetId) => {
      return state.rulesetFields[rulesetId] || { fieldKeys: [], sampleCount: 0 }
    }
  },
  mutations: {
    // setComponents moved to dataCache
    setAvailablePlugins(state, plugins) {
      state.availablePlugins = plugins
    },
    // setPendingChanges moved to dataCache
    setRulesetFields(state, { rulesetId, fieldData }) {
      state.rulesetFields[rulesetId] = fieldData
    },
    clearRulesetFields(state, rulesetId) {
      if (rulesetId) {
        delete state.rulesetFields[rulesetId]
      } else {
        // Clear all ruleset fields
        state.rulesetFields = {}
      }
    },
    setEventCleanupFunctions(state, functions) {
      state._eventCleanupFunctions = functions
    }
  },
  actions: {
    // Initialize event listeners using the unified event manager (focused on Vuex-specific caches)
    initializeEventListeners({ commit, state }) {
      if (state._eventCleanupFunctions.length > 0) return // Already initialized
      
      // Use unified event manager for Vuex-specific caches only
      const componentChangedCleanup = eventManager.on('componentChanged', (data) => {
        const { action, type, id } = data
        // console.log(`[VuexStore] Component ${action}: ${type}/${id}`)
        
        // Only handle Vuex-specific caches (avoid overlap with dataCache)
        if (type === 'rulesets') {
          // Clear field cache for the specific ruleset
          if (id) {
            commit('clearRulesetFields', id)
          }
        } else if (type === 'plugins') {
          // Clear available plugins cache
          commit('setAvailablePlugins', [])
        }
      })
      
      const pendingChangesCleanup = eventManager.on('pendingChangesApplied', (data) => {
        const { types } = data
        // console.log(`[VuexStore] Pending changes applied for types:`, types)
        
        if (Array.isArray(types)) {
          types.forEach(type => {
            if (type === 'rulesets') {
              commit('clearRulesetFields')
            } else if (type === 'plugins') {
              commit('setAvailablePlugins', [])
            }
          })
        }
      })
      
      const localChangesCleanup = eventManager.on('localChangesLoaded', (data) => {
        const { types } = data
        // console.log(`[VuexStore] Local changes loaded for types:`, types)
        
        if (Array.isArray(types)) {
          types.forEach(type => {
            if (type === 'rulesets') {
              commit('clearRulesetFields')
            } else if (type === 'plugins') {
              commit('setAvailablePlugins', [])
            }
          })
        }
      })
      
      // Store cleanup functions
      commit('setEventCleanupFunctions', [
        componentChangedCleanup,
        pendingChangesCleanup,
        localChangesCleanup
      ])
      
      // console.log('[VuexStore] Event listeners initialized via EventManager')
    },
    
    // Cleanup event listeners
    cleanupEventListeners({ commit, state }) {
      state._eventCleanupFunctions.forEach(cleanup => cleanup())
      commit('setEventCleanupFunctions', [])
      // console.log('[VuexStore] Event listeners cleaned up')
    },

    // fetchComponents removed - use dataCache.fetchComponents() instead
    async fetchAvailablePlugins({ commit, dispatch, state }) {
      // Initialize event listeners if needed
      if (state._eventCleanupFunctions.length === 0) {
        dispatch('initializeEventListeners')
      }
      
      try {
        // Use the already updated getAvailablePlugins function
        const plugins = await hubApi.getAvailablePlugins()
        commit('setAvailablePlugins', plugins)
      } catch (error) {
        commit('setAvailablePlugins', [])
      }
    },
    // fetchAllComponents removed - use dataCache for component fetching
    // fetchPendingChanges moved to dataCache.fetchPendingChanges()
    // fetchClusterStatus moved to dataCache.fetchClusterInfo()
    async fetchRulesetFields({ commit, dispatch, state }, rulesetId) {
      // Initialize event listeners if needed
      if (state._eventCleanupFunctions.length === 0) {
        dispatch('initializeEventListeners')
      }
      
      try {
        const fieldData = await hubApi.getRulesetFields(rulesetId)
        commit('setRulesetFields', { rulesetId, fieldData })
        return fieldData
      } catch (error) {
        // console.warn(`Failed to fetch fields for ruleset ${rulesetId}:`, error)
        const fallbackData = { fieldKeys: [], sampleCount: 0 }
        commit('setRulesetFields', { rulesetId, fieldData: fallbackData })
        return fallbackData
      }
    }
  },
  modules: {}
})

export default store 