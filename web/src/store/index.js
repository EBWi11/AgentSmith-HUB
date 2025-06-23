import { createStore } from 'vuex'
import { hubApi } from '../api'

export default createStore({
  state: {
    components: {
      inputs: [],
      outputs: [],
      rulesets: [],
      projects: [],
      plugins: []
    },
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
    pendingChanges: [],
    clusterStatus: {},
    clusterInfo: {},
    rulesetFields: {} // Cache for ruleset field keys: { rulesetId: { fieldKeys: [...], sampleCount: 0 } }
  },
  getters: {
    getComponents: (state) => (type) => {
      return state.components[type] || []
    },
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
    getPendingChanges: (state) => {
      return state.pendingChanges
    },
    getClusterStatus: (state) => {
      return state.clusterStatus
    },
    getClusterInfo: (state) => {
      return state.clusterInfo
    },
    getRulesetFields: (state) => (rulesetId) => {
      return state.rulesetFields[rulesetId] || { fieldKeys: [], sampleCount: 0 }
    }
  },
  mutations: {
    setComponents(state, { type, components }) {
      state.components[type] = components
    },
    setAvailablePlugins(state, plugins) {
      state.availablePlugins = plugins
    },
    setPendingChanges(state, changes) {
      state.pendingChanges = changes
    },
    setClusterStatus(state, status) {
      state.clusterStatus = status
    },
    setClusterInfo(state, info) {
      state.clusterInfo = info
    },
    setRulesetFields(state, { rulesetId, fieldData }) {
      state.rulesetFields[rulesetId] = fieldData
    }
  },
  actions: {
    async fetchComponents({ commit }, type) {
      try {
        let components = []
        switch (type) {
          case 'inputs':
            components = await hubApi.fetchInputs()
            break
          case 'outputs':
            components = await hubApi.fetchOutputs()
            break
          case 'rulesets':
            components = await hubApi.fetchRulesets()
            break
          case 'projects':
            components = await hubApi.fetchProjects()
            break
          case 'plugins':
            components = await hubApi.fetchPlugins()
            break
        }
        // 对组件列表按照ID排序
        if (Array.isArray(components)) {
          components.sort((a, b) => {
            const idA = a.id || a.name || ''
            const idB = b.id || b.name || ''
            return idA.localeCompare(idB)
          })
        }
        commit('setComponents', { type, components })
      } catch (error) {
      }
    },
    async fetchAvailablePlugins({ commit }) {
      try {
        const plugins = await hubApi.getAvailablePlugins()
        commit('setAvailablePlugins', plugins)
      } catch (error) {
        commit('setAvailablePlugins', [])
      }
    },
    async fetchAllComponents({ dispatch }) {
      await Promise.all([
        dispatch('fetchComponents', 'inputs'),
        dispatch('fetchComponents', 'outputs'),
        dispatch('fetchComponents', 'rulesets'),
        dispatch('fetchComponents', 'projects'),
        dispatch('fetchComponents', 'plugins'),
        dispatch('fetchAvailablePlugins')
      ])
    },
    async fetchPendingChanges({ commit }) {
      try {
        const changes = await hubApi.fetchPendingChanges()
        commit('setPendingChanges', changes)
      } catch (error) {
        commit('setPendingChanges', [])
      }
    },
    async fetchClusterStatus({ commit }) {
      try {
        const status = await hubApi.fetchClusterStatus()
        commit('setClusterStatus', status)
      } catch (error) {
        commit('setClusterStatus', {})
      }
    },
    async fetchClusterInfo({ commit }) {
      try {
        const info = await hubApi.fetchClusterStatus()
        commit('setClusterInfo', info)
      } catch (error) {
        commit('setClusterInfo', {})
      }
    },
    async fetchRulesetFields({ commit }, rulesetId) {
      try {
        const fieldData = await hubApi.getRulesetFields(rulesetId)
        commit('setRulesetFields', { rulesetId, fieldData })
        return fieldData
      } catch (error) {
        console.warn(`Failed to fetch fields for ruleset ${rulesetId}:`, error)
        const fallbackData = { fieldKeys: [], sampleCount: 0 }
        commit('setRulesetFields', { rulesetId, fieldData: fallbackData })
        return fallbackData
      }
    }
  },
  modules: {}
}) 