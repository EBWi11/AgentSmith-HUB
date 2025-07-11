<template>
  <div class="h-full flex flex-col bg-white">
    <!-- Header -->
    <div class="flex items-center justify-between p-4 border-b border-gray-200">
      <h1 class="text-xl font-semibold text-gray-900">Operations History</h1>
      <div class="flex items-center space-x-2">
        <button 
          @click="refreshHistory" 
          :disabled="loading"
          class="btn btn-secondary btn-sm"
        >
          <svg v-if="loading" class="w-4 h-4 animate-spin mr-2" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
          </svg>
          <svg v-else class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"></path>
          </svg>
          Refresh
        </button>
        <button
          @click="exportOperations"
          :disabled="operations.length === 0"
          class="btn btn-secondary btn-sm"
        >
          <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 10v6m0 0l-3-3m3 3l3-3m2 8H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"></path>
          </svg>
          Export
        </button>
      </div>
    </div>

    <!-- Filters -->
    <div class="p-4 border-b border-gray-200 bg-gray-50">
      <!-- (View mode toggle removed) -->

      <div class="grid grid-cols-1 md:grid-cols-3 lg:grid-cols-6 gap-4">
        <!-- Operation Type Filter -->
        <div>
          <label class="block text-sm font-medium text-gray-700 mb-1">Operation Type</label>
          <select v-model="filters.operationType" @change="applyFilters" class="filter-select">
            <option value="">All Types</option>
            <option value="change_push">Change Push</option>
            <option value="local_push">Local Push</option>
            <option value="project_start">Project Start</option>
            <option value="project_stop">Project Stop</option>
            <option value="project_restart">Project Restart</option>
          </select>
        </div>

        <!-- Component Type Filter -->
        <div>
          <label class="block text-sm font-medium text-gray-700 mb-1">Component Type</label>
          <select v-model="filters.componentType" @change="applyFilters" class="filter-select">
            <option value="">All Components</option>
            <option value="input">Input</option>
            <option value="output">Output</option>
            <option value="ruleset">Ruleset</option>
            <option value="plugin">Plugin</option>
            <option value="project">Project</option>
          </select>
        </div>

        <!-- Status Filter -->
        <div>
          <label class="block text-sm font-medium text-gray-700 mb-1">Status</label>
          <select v-model="filters.status" @change="applyFilters" class="filter-select">
            <option value="">All Status</option>
            <option value="success">Success</option>
            <option value="failed">Failed</option>
          </select>
        </div>

        <!-- Node Filter -->
        <div>
          <label class="block text-sm font-medium text-gray-700 mb-1">Node</label>
          <select v-model="filters.nodeId" @change="applyFilters" class="filter-select">
            <option value="all">All Nodes</option>
            <option v-for="nodeId in availableNodes" :key="nodeId" :value="nodeId">
              {{ nodeId }}
            </option>
          </select>
        </div>

        <!-- Time Range Filter -->
        <div>
          <label class="block text-sm font-medium text-gray-700 mb-1">Time Range</label>
          <select v-model="filters.timeRange" @change="handleTimeRangeChange" class="filter-select">
            <option value="1h">Last Hour</option>
            <option value="12h">Last 12 Hours</option>
            <option value="24h">Last 24 Hours</option>
            <option value="7d">Last 7 Days</option>
            <option value="30d">Last 30 Days</option>
            <option value="custom">Custom Range</option>
          </select>
        </div>

        <!-- Search -->
        <div>
          <label class="block text-sm font-medium text-gray-700 mb-1">Search</label>
          <input 
            v-model="filters.keyword" 
            @input="debouncedSearch"
            type="text" 
            placeholder="Search operations, components, errors, or node IDs..."
            class="filter-input"
          >
        </div>
      </div>

      <!-- Custom Date Range (shown when custom is selected) -->
      <div v-if="filters.timeRange === 'custom'" class="mt-4 grid grid-cols-1 md:grid-cols-2 gap-4">
        <div>
          <label class="block text-sm font-medium text-gray-700 mb-1">Start Date</label>
          <input 
            v-model="filters.startDate" 
            @change="applyFilters"
            type="datetime-local" 
            class="filter-input"
          >
        </div>
        <div>
          <label class="block text-sm font-medium text-gray-700 mb-1">End Date</label>
          <input 
            v-model="filters.endDate" 
            @change="applyFilters"
            type="datetime-local" 
            class="filter-input"
          >
        </div>
      </div>
      
      <!-- Node Statistics section removed per requirement -->
    </div>

    <!-- Content -->
    <div class="flex-1 overflow-y-auto">
      <div v-if="loading && !operations.length" class="flex items-center justify-center h-64">
        <div class="text-gray-500">Loading operations history...</div>
      </div>
      
      <div v-else-if="error" class="p-4 bg-red-50 border border-red-200 text-red-700 text-sm">
        {{ error }}
      </div>
      
      <div v-else-if="!operations.length" class="flex-1 flex items-center justify-center text-gray-500">
        No operations found
      </div>
      
      <div v-else class="space-y-2 p-4">
        <div 
          v-for="operation in operations" 
          :key="getOperationKey(operation)" 
          class="border border-gray-200 rounded-lg overflow-hidden hover:border-gray-300 transition-colors"
        >
          <div class="flex items-center justify-between p-3 bg-gray-50 border-b border-gray-200 cursor-pointer"
               @click="toggleOperationDetail(operation)">
            <div class="flex items-center space-x-3">
              <div class="flex items-center space-x-2">
                <!-- Operation Type Icon -->
                <div class="flex items-center justify-center w-8 h-8 rounded-full" :class="getOperationTypeClass(operation.type)">
                  <svg class="w-4 h-4 text-white" fill="currentColor" viewBox="0 0 20 20" v-html="getOperationTypeIcon(operation.type)"></svg>
                </div>
                
                <!-- Operation Info -->
                <div>
                  <h3 class="font-medium text-gray-900">
                    {{ getOperationTypeLabel(operation.type) }}
                    <span v-if="operation.component_type && operation.component_id" class="ml-2 text-sm font-normal text-gray-600">
                      {{ operation.component_type }}: {{ operation.component_id }}
                    </span>
                    <span v-else-if="operation.project_id" class="ml-2 text-sm font-normal text-gray-600">
                      Project: {{ operation.project_id }}
                    </span>
                  </h3>
                  <div class="flex items-center space-x-2 text-sm text-gray-500">
                    <span>{{ formatTimestamp(operation.timestamp) }}</span>
                    <span v-if="operation.details?.node_id || operation.node_id" class="text-xs text-blue-600 bg-blue-100 px-2 py-1 rounded font-medium">
                      {{ operation.details?.node_id || operation.node_id }}
                    </span>
                  </div>
                </div>
              </div>
            </div>
            
            <div class="flex items-center space-x-2">
              <!-- Status Badge -->
              <span class="px-2 py-1 text-xs font-medium rounded-full" :class="getStatusClass(operation.status)">
                {{ operation.status }}
              </span>
              
              <!-- Expand/Collapse Icon -->
              <svg class="w-4 h-4 text-gray-400 transform transition-transform" 
                   :class="{ 'rotate-90': expandedOperations.has(getOperationKey(operation)) }"
                   fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"></path>
              </svg>
            </div>
          </div>
          
          <!-- Operation Details (expanded) -->
          <div v-if="expandedOperations.has(getOperationKey(operation))" class="p-4 bg-white">
            <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
              <!-- Basic Info -->
              <div>
                <h4 class="text-sm font-medium text-gray-900 mb-2">Operation Details</h4>
                <dl class="space-y-1 text-sm">

                  <div class="grid grid-cols-3 gap-1">
                    <dt class="text-gray-500">Type:</dt>
                    <dd class="col-span-2 text-gray-900">{{ getOperationTypeLabel(operation.type) }}</dd>
                  </div>
                  <div v-if="operation.component_type" class="grid grid-cols-3 gap-1">
                    <dt class="text-gray-500">Component:</dt>
                    <dd class="col-span-2 text-gray-900">{{ operation.component_type }}</dd>
                  </div>
                  <div v-if="operation.component_id" class="grid grid-cols-3 gap-1">
                    <dt class="text-gray-500">Component ID:</dt>
                    <dd class="col-span-2 text-gray-900">{{ operation.component_id }}</dd>
                  </div>
                  <div v-if="operation.project_id" class="grid grid-cols-3 gap-1">
                    <dt class="text-gray-500">Project ID:</dt>
                    <dd class="col-span-2 text-gray-900">{{ operation.project_id }}</dd>
                  </div>
                  <div v-if="isClusterMode && operation.details?.node_id" class="grid grid-cols-3 gap-1">
                    <dt class="text-gray-500">Node ID:</dt>
                    <dd class="col-span-2 text-gray-900">{{ operation.details.node_id }}</dd>
                  </div>
                  <div v-if="isClusterMode && operation.details?.node_address" class="grid grid-cols-3 gap-1">
                    <dt class="text-gray-500">Node Address:</dt>
                    <dd class="col-span-2 text-gray-900">{{ operation.details.node_address }}</dd>
                  </div>
                  <div class="grid grid-cols-3 gap-1">
                    <dt class="text-gray-500">Status:</dt>
                    <dd class="col-span-2">
                      <span class="px-2 py-1 text-xs font-medium rounded-full" :class="getStatusClass(operation.status)">
                        {{ operation.status }}
                      </span>
                    </dd>
                  </div>
                  <div class="grid grid-cols-3 gap-1">
                    <dt class="text-gray-500">Timestamp:</dt>
                    <dd class="col-span-2 text-gray-900">{{ formatFullTimestamp(operation.timestamp) }}</dd>
                  </div>

                </dl>
              </div>

              <!-- Error (if any) -->
              <div v-if="operation.error">
                <h4 class="text-sm font-medium text-red-900 mb-2">Error Details</h4>
                <div class="bg-red-50 border border-red-200 rounded-md p-3">
                  <pre class="text-sm text-red-700 whitespace-pre-wrap">{{ operation.error }}</pre>
                </div>
              </div>
            </div>

            <!-- Diff View (for change operations) -->
            <div v-if="operation.diff || (operation.old_content && operation.new_content)" class="mt-4">
              <h4 class="text-sm font-medium text-gray-900 mb-2">Changes</h4>
              <div class="bg-gray-100 rounded-md" style="height: 300px;">
                <MonacoEditor 
                  :key="`diff-${getOperationKey(operation)}`"
                  :value="operation.new_content || ''" 
                  :original-value="operation.old_content || ''"
                  :language="getLanguageForComponent(operation.component_type)" 
                  :read-only="true" 
                  :diff-mode="true"
                  style="height: 100%; width: 100%;"
                />
              </div>
            </div>

            <!-- Content View (for single content operations) -->
            <div v-else-if="operation.new_content" class="mt-4">
              <h4 class="text-sm font-medium text-gray-900 mb-2">Content</h4>
              <div class="bg-gray-100 rounded-md" style="height: 300px;">
                <MonacoEditor 
                  :key="`content-${getOperationKey(operation)}`"
                  :value="operation.new_content" 
                  :language="getLanguageForComponent(operation.component_type)" 
                  :read-only="true" 
                  :diff-mode="false"
                  style="height: 100%; width: 100%;"
                />
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Pagination -->
    <div v-if="totalCount > 0" class="flex items-center justify-between p-4 border-t border-gray-200 bg-gray-50">
      <div class="text-sm text-gray-700">
        Showing {{ ((currentPage - 1) * pageSize) + 1 }} to {{ Math.min(currentPage * pageSize, totalCount) }} of {{ totalCount }} operations
      </div>
      <div class="flex items-center space-x-2">
        <button 
          @click="previousPage" 
          :disabled="currentPage <= 1 || loading"
          class="btn btn-secondary btn-sm"
        >
          Previous
        </button>
        <span class="text-sm text-gray-700">
          Page {{ currentPage }} of {{ totalPages }}
        </span>
        <button 
          @click="nextPage" 
          :disabled="currentPage >= totalPages || loading"
          class="btn btn-secondary btn-sm"
        >
          Next
        </button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, nextTick, inject } from 'vue'
import axios from 'axios'
import { hubApi } from '../api'
import { useDataCacheStore } from '../stores/dataCache'
import MonacoEditor from './MonacoEditor.vue'
import { debounce, createOptimizedApiCall } from '../utils/performance'

// State
const operations = ref([])
const loading = ref(false)
const error = ref(null)
const totalCount = ref(0)
const currentPage = ref(1)
const pageSize = ref(20)
const expandedOperations = ref(new Set())
// Node statistics no longer used (feature removed)
const nodeStats = ref({}) // kept empty for backward compatibility
const availableNodes = ref([])
const isClusterMode = computed(() => availableNodes.value.length > 1)

// Global message component
const $message = inject('$message', window?.$toast)

// Filters
const filters = ref({
  operationType: '',
  componentType: '',
  status: '',
  timeRange: '1h',
  startDate: '',
  endDate: '',
  keyword: '',
  nodeId: 'all'
})

// Computed properties
const totalPages = computed(() => Math.ceil(totalCount.value / pageSize.value))

// Lifecycle hooks
const dataCache = useDataCacheStore()
const clusterInfo = ref({})

async function loadClusterInfo() {
  try {
    clusterInfo.value = await dataCache.fetchClusterInfo()
  } catch (e) {
    clusterInfo.value = {}
  }
}

onMounted(() => {
  setDefaultTimeRange()
  loadClusterInfo().then(fetchOperationsHistory)
})

// Create optimized API call
const optimizedFetchHistory = createOptimizedApiCall(
  (params) => hubApi.getOperationsHistory(params),
  500 // 500ms debounce
)

// Methods
function toLocalISOString(date) {
  // Generate local timezone yyyy-MM-ddTHH:mm string
  const pad = n => n < 10 ? '0' + n : n
  return date.getFullYear() + '-' + pad(date.getMonth() + 1) + '-' + pad(date.getDate()) + 'T' + pad(date.getHours()) + ':' + pad(date.getMinutes())
}

function setDefaultTimeRange() {
  const now = new Date()
  const oneHourAgo = new Date(now.getTime() - 60 * 60 * 1000)
  
  filters.value.startDate = toLocalISOString(oneHourAgo)
  filters.value.endDate = toLocalISOString(now)
}

function handleTimeRangeChange() {
  if (filters.value.timeRange !== 'custom') {
    const now = new Date()
    let startTime
    
    switch (filters.value.timeRange) {
      case '1h':
        startTime = new Date(now.getTime() - 60 * 60 * 1000)
        break
      case '12h':
        startTime = new Date(now.getTime() - 12 * 60 * 60 * 1000)
        break
      case '24h':
        startTime = new Date(now.getTime() - 24 * 60 * 60 * 1000)
        break
      case '7d':
        startTime = new Date(now.getTime() - 7 * 24 * 60 * 60 * 1000)
        break
      case '30d':
        startTime = new Date(now.getTime() - 30 * 24 * 60 * 60 * 1000)
        break
      default:
        startTime = new Date(now.getTime() - 60 * 60 * 1000)
    }
    
    filters.value.startDate = toLocalISOString(startTime)
    filters.value.endDate = toLocalISOString(now)
  }
  
  applyFilters()
}

async function fetchOperationsHistory() {
  loading.value = true
  error.value = null
  
  try {
    const params = new URLSearchParams()
    
    if (filters.value.operationType) {
      params.append('operation_type', filters.value.operationType)
    }
    if (filters.value.componentType) {
      params.append('component_type', filters.value.componentType)
    }
    if (filters.value.status) {
      params.append('status', filters.value.status)
    }
    if (filters.value.keyword) {
      params.append('keyword', filters.value.keyword)
    }
    if (filters.value.nodeId && filters.value.nodeId !== 'all') {
      params.append('node_id', filters.value.nodeId)
    }
    // Use local time string concatenated with ISO format, with local timezone
    if (filters.value.startDate) {
      const start = new Date(filters.value.startDate)
      params.append('start_time', start.toISOString())
    }
    if (filters.value.endDate) {
      const end = new Date(filters.value.endDate)
      params.append('end_time', end.toISOString())
    }
    
    params.append('limit', pageSize.value.toString())
    params.append('offset', ((currentPage.value - 1) * pageSize.value).toString())

    let response
    const needCluster = filters.value.nodeId === 'all'
    const isLeaderNode = clusterInfo.value.status === 'leader'

    if (needCluster) {
      if (isLeaderNode) {
        // call local leader endpoint
        response = await hubApi.getClusterOperationsHistory(params.toString())
        // Node statistics feature removed – ignore response.node_stats
      } else if (clusterInfo.value.leader_address) {
        // call remote leader address directly
        const leaderBase = `http://${clusterInfo.value.leader_address}`
        const token = localStorage.getItem('auth_token') || ''
        const instance = axios.create({ baseURL: leaderBase, timeout: 15000, headers: { token } })
        const url = '/cluster-operations-history' + (params ? '?' + params.toString() : '')
        const res = await instance.get(url)
        response = res.data
        // ignore node stats
      }
    }

    // if response still undefined fall back to local history
    if (!response) {
      response = await optimizedFetchHistory(params.toString())
      nodeStats.value = {}
    }
    
    operations.value = response.operations || []
    totalCount.value = response.total_count || 0
    
    // Update available nodes information
    if (response.available_nodes) {
      availableNodes.value = response.available_nodes
    } else if (response.node_stats) {
      availableNodes.value = Object.keys(response.node_stats)
    } else {
      const nodesSet = new Set()
      operations.value.forEach(op => {
        if (op.details?.node_id) nodesSet.add(op.details.node_id)
      })
      availableNodes.value = Array.from(nodesSet)
    }
    
    // Wait for DOM update then refresh editor layout
    await nextTick()
    refreshEditorsLayout()
  } catch (e) {
    error.value = 'Failed to fetch operations history: ' + (e?.message || 'Unknown error')
    $message?.error?.('Failed to fetch operations history')
  } finally {
    loading.value = false
  }
}

function refreshEditorsLayout() {
  // Give editors some time to render
  setTimeout(() => {
    const editorElements = document.querySelectorAll('.monaco-editor-container')
    editorElements.forEach(el => {
      const editor = el.__vue__?.exposed
      if (editor) {
        const monacoEditor = editor.getEditor()
        const diffEditor = editor.getDiffEditor()
        
        if (monacoEditor) {
          monacoEditor.layout()
        }
        
        if (diffEditor) {
          diffEditor.layout()
        }
      }
    })
  }, 300)
}

function applyFilters() {
  currentPage.value = 1
  fetchOperationsHistory()
}

// handleViewModeChange removed (no longer needed)

const debouncedSearch = debounce(() => {
  applyFilters()
}, 500)

function refreshHistory() {
  fetchOperationsHistory()
}

function exportOperations() {
  try {
    // Create CSV headers
    const headers = ['Type', 'Component Type', 'Component ID', 'Project ID', 'Status', 'Timestamp', 'Error']
    
    // Convert operations to CSV format
    const csvData = operations.value.map(op => [
      getOperationTypeLabel(op.type),
      op.component_type || '',
      op.component_id || '',
      op.project_id || '',
      op.status,
      formatFullTimestamp(op.timestamp),
      op.error || ''
    ])
    
    // Add headers to data
    csvData.unshift(headers)
    
    // Convert to CSV string
    const csvString = csvData.map(row => 
      row.map(cell => 
        typeof cell === 'string' && cell.includes(',') ? `"${cell}"` : cell
      ).join(',')
    ).join('\n')
    
    // Create and download file
    const blob = new Blob([csvString], { type: 'text/csv;charset=utf-8;' })
    const link = document.createElement('a')
    
    if (link.download !== undefined) {
      const url = URL.createObjectURL(blob)
      link.setAttribute('href', url)
      link.setAttribute('download', `operations_history_${new Date().toISOString().split('T')[0]}.csv`)
      link.style.visibility = 'hidden'
      document.body.appendChild(link)
      link.click()
      document.body.removeChild(link)
      URL.revokeObjectURL(url)
    }
    
    $message?.success?.('Operations history exported successfully')
  } catch (e) {
    $message?.error?.('Failed to export operations history')
  }
}

function toggleOperationDetail(operation) {
  const key = getOperationKey(operation)
  if (expandedOperations.value.has(key)) {
    expandedOperations.value.delete(key)
  } else {
    expandedOperations.value.add(key)
  }
  
  // Refresh editor layout after expansion
  if (expandedOperations.value.has(key)) {
    nextTick(() => {
      refreshEditorsLayout()
    })
  }
}

function previousPage() {
  if (currentPage.value > 1) {
    currentPage.value--
    fetchOperationsHistory()
  }
}

function nextPage() {
  if (currentPage.value < totalPages.value) {
    currentPage.value++
    fetchOperationsHistory()
  }
}

// Helper functions
function getOperationTypeLabel(type) {
  const labels = {
    'change_push': 'Change Push',
    'local_push': 'Local Push',
    'project_start': 'Project Start',
    'project_stop': 'Project Stop',
    'project_restart': 'Project Restart'
  }
  return labels[type] || type
}

function getOperationTypeClass(type) {
  const classes = {
    'change_push': 'bg-blue-500',
    'local_push': 'bg-purple-500',
    'project_start': 'bg-green-500',
    'project_stop': 'bg-red-500',
    'project_restart': 'bg-orange-500'
  }
  return classes[type] || 'bg-gray-500'
}

function getOperationTypeIcon(type) {
  const icons = {
    'change_push': '<path d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M9 12l2 2 4-4"/>',
    'local_push': '<path d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M13 13l3-3-3-3m-4 6L6 10l3-3"/>',
    'project_start': '<path d="M14.828 14.828a4 4 0 01-5.656 0M9 10h1m4 0h1m-6 4h8a2 2 0 002-2V8a2 2 0 00-2-2H8a2 2 0 00-2 2v4a2 2 0 002 2z"/>',
    'project_stop': '<path d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/><path d="M9 10a1 1 0 011-1h4a1 1 0 011 1v4a1 1 0 01-1 1h-4a1 1 0 01-1-1v-4z"/>',
    'project_restart': '<path d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"/>'
  }
  return icons[type] || '<path d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/>'
}

function getStatusClass(status) {
  const classes = {
    'success': 'bg-green-100 text-green-800',
    'failed': 'bg-red-100 text-red-800'
  }
  return classes[status] || 'bg-gray-100 text-gray-800'
}

function getLanguageForComponent(componentType) {
  switch (componentType) {
    case 'ruleset':
      return 'xml'
    case 'plugin':
      return 'go'
    default:
      return 'yaml'
  }
}

function formatTimestamp(timestamp) {
  const date = new Date(timestamp)
  const now = new Date()
  const diff = now - date
  
  if (diff < 60000) { // Less than 1 minute
    return 'just now'
  } else if (diff < 3600000) { // Less than 1 hour
    return `${Math.floor(diff / 60000)} minutes ago`
  } else if (diff < 86400000) { // Less than 1 day
    return `${Math.floor(diff / 3600000)} hours ago`
  } else {
    return date.toLocaleString('en-US', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false
    })
  }
}

function formatFullTimestamp(timestamp) {
  const date = new Date(timestamp)
  return date.toLocaleString('en-US', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false
  })
}

function getOperationKey(operation) {
  // Create a unique key for each operation using timestamp and operation details
  return `${operation.timestamp}-${operation.type}-${operation.component_type || ''}-${operation.component_id || ''}-${operation.project_id || ''}`
}
</script>

<style scoped>
.filter-select {
  @apply w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500 text-sm;
}

.filter-input {
  @apply w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500 text-sm;
}

.btn {
  @apply px-3 py-2 text-sm font-medium rounded-md focus:outline-none focus:ring-2 focus:ring-offset-2 transition-colors;
}

.btn-secondary {
  @apply bg-white border border-gray-300 text-gray-700 hover:bg-gray-50 focus:ring-gray-500;
}

.btn-sm {
  @apply px-2 py-1 text-xs;
}

.btn:disabled {
  @apply opacity-50 cursor-not-allowed;
}
</style> 