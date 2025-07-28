import { createStore } from 'vuex'
import { hubApi } from '../api'
import eventManager from '../utils/eventManager'

const store = createStore({
  state: {
    // Static data (not cache)
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
      { value: 'EQU', detail: 'Equal check (case insensitive)' },
      { value: 'NEQ', detail: 'Not equal check (case insensitive)' },
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
      		{ value: 'EXCLUDE', detail: 'Exclude rule type' }
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
    ]
  },
  getters: {
    getNodeTypes: (state) => state.nodeTypes,
    getLogicTypes: (state) => state.logicTypes,
    getCountTypes: (state) => state.countTypes,
    getRootTypes: (state) => state.rootTypes,
    getCommonFields: (state) => state.commonFields
  },
  mutations: {},
  actions: {},
  modules: {}
})

export default store 