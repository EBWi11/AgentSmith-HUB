<template>
  <div class="h-full flex flex-col bg-white">
    <div class="flex items-center justify-between p-4 border-b border-gray-200">
      <h1 class="text-xl font-semibold text-gray-900">Error Logs</h1>
      <div class="flex items-center space-x-2">
        <button
          @click="refreshLogs"
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
          @click="exportLogs"
          :disabled="logs.length === 0"
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
      <div class="grid grid-cols-1 md:grid-cols-3 lg:grid-cols-5 gap-4">
        <!-- Source Filter -->
        <div>
          <label class="block text-sm font-medium text-gray-700 mb-1">Source</label>
          <select v-model="filters.source" @change="applyFilters" class="filter-select">
            <option value="all">All Sources</option>
            <option value="hub">Hub</option>
            <option value="plugin">Plugin</option>
          </select>
        </div>

        <!-- Node Filter -->
        <div>
          <label class="block text-sm font-medium text-gray-700 mb-1">Node</label>
          <select v-model="filters.nodeId" @change="applyFilters" class="filter-select">
            <option value="all">All Nodes</option>
            <option v-for="node in availableNodes" :key="node.id" :value="node.id">
              {{ node.name || node.id }}
            </option>
          </select>
        </div>



        <!-- Time Range Filter -->
        <div>
          <label class="block text-sm font-medium text-gray-700 mb-1">Time Range</label>
          <select v-model="filters.timeRange" @change="handleTimeRangeChange" class="filter-select">
            <option value="1h">Last Hour</option>
            <option value="6h">Last 6 Hours</option>
            <option value="12h">Last 12 Hours</option>
            <option value="24h">Last 24 Hours</option>
            <option value="7d">Last 7 Days</option>
            <option value="30d">Last 30 Days</option>
            <option value="custom">Custom Range</option>
          </select>
        </div>

        <!-- Search (fill remaining columns) -->
        <div class="lg:col-span-2">
          <label class="block text-sm font-medium text-gray-700 mb-1">Search</label>
          <input 
            v-model="filters.keyword" 
            @input="debouncedSearch"
            type="text" 
            placeholder="Search messages..."
            class="filter-input w-full"
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

      <!-- Node statistics section removed per requirement -->
    </div>

    <!-- Content -->
    <div class="flex-1 overflow-y-auto">
      <div v-if="loading && !logs.length" class="flex items-center justify-center h-64">
        <div class="text-gray-500">Loading error logs...</div>
      </div>
      
      <div v-else-if="error" class="p-4 bg-red-50 border border-red-200 text-red-700 text-sm">
        {{ error }}
      </div>
      
      <div v-else-if="!logs.length" class="flex-1 flex items-center justify-center text-gray-500">
        No error logs found
      </div>
      
      <div v-else class="space-y-2 p-4">
        <div
          v-for="(log, index) in logs"
          :key="`${log.node_id}-${log.source}-${log.line}-${index}`"
          class="border border-gray-200 rounded-lg overflow-hidden hover:border-gray-300 transition-colors"
        >
          <div class="flex items-center justify-between p-3 bg-gray-50 border-b border-gray-200 cursor-pointer"
               @click="toggleLogDetail(index)">
            <div class="flex items-center space-x-3">
              <div class="flex items-center space-x-2">
                <!-- Source Icon -->
                <div class="flex items-center justify-center w-8 h-8 rounded-full" :class="getSourceClass(log.source)">
                  <span class="text-white text-xs font-medium">{{ log.source === 'hub' ? 'H' : 'P' }}</span>
                </div>
                
                <!-- Log Info -->
                <div>
                  <h3 class="font-medium text-gray-900">{{ log.message }}</h3>
                  <div class="flex items-center space-x-2 text-sm text-gray-500">
                    <span>{{ formatTimestamp(log.timestamp) }}</span>
                    <span v-if="log.node_id" class="text-xs text-blue-600 bg-blue-100 px-2 py-1 rounded font-medium">
                      {{ log.node_id }}
                    </span>
                    <span class="text-xs">Line: {{ log.line }}</span>
                  </div>
                </div>
              </div>
            </div>
            
            <div class="flex items-center space-x-2">
              <!-- Level Badge -->
              <span class="px-2 py-1 text-xs font-medium rounded-full" :class="getLevelClass(log.level)">
                {{ log.level }}
              </span>
              
              <!-- Expand/Collapse Icon -->
              <svg class="w-4 h-4 text-gray-400 transform transition-transform" 
                   :class="{ 'rotate-90': expandedLogs.has(index) }"
                   fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"></path>
              </svg>
            </div>
          </div>
          
          <!-- Log Details (expanded) -->
          <div v-if="expandedLogs.has(index)" class="p-4 bg-white">
            <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
              <!-- Basic Info -->
              <div>
                <h4 class="text-sm font-medium text-gray-900 mb-2">Log Details</h4>
                <dl class="space-y-1 text-sm">
                  <div class="grid grid-cols-3 gap-1">
                    <dt class="text-gray-500">Source:</dt>
                    <dd class="col-span-2 text-gray-900">{{ log.source }}</dd>
                  </div>
                  <div class="grid grid-cols-3 gap-1">
                    <dt class="text-gray-500">Level:</dt>
                    <dd class="col-span-2">
                      <span class="px-2 py-1 text-xs font-medium rounded-full" :class="getLevelClass(log.level)">
                        {{ log.level }}
                      </span>
                    </dd>
                  </div>
                  <div v-if="log.node_id" class="grid grid-cols-3 gap-1">
                    <dt class="text-gray-500">Node:</dt>
                    <dd class="col-span-2 text-gray-900">{{ log.node_id }}</dd>
                  </div>
                  <div class="grid grid-cols-3 gap-1">
                    <dt class="text-gray-500">Line:</dt>
                    <dd class="col-span-2 text-gray-900">{{ log.line }}</dd>
                  </div>
                  <div class="grid grid-cols-3 gap-1">
                    <dt class="text-gray-500">Timestamp:</dt>
                    <dd class="col-span-2 text-gray-900">{{ formatFullTimestamp(log.timestamp) }}</dd>
                  </div>
                </dl>
              </div>

              <!-- Context (if any) -->
              <div v-if="log.context && log.context !== log.message">
                <h4 class="text-sm font-medium text-gray-900 mb-2">Context</h4>
                <div class="bg-gray-50 border border-gray-200 rounded-md p-3">
                  <pre class="text-sm text-gray-700 whitespace-pre-wrap break-all">{{ log.context }}</pre>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Pagination -->
    <div v-if="totalCount > 0" class="flex items-center justify-between p-4 border-t border-gray-200 bg-gray-50">
      <div class="text-sm text-gray-700">
        Showing {{ ((currentPage - 1) * pageSize) + 1 }} to {{ Math.min(currentPage * pageSize, totalCount) }} of {{ totalCount }} logs
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
import { ref, reactive, onMounted, inject, computed } from 'vue'
import { hubApi } from '@/api'
import { debounce } from '../utils/common'
import { useDataCacheStore } from '../stores/dataCache'
import axios from 'axios'

// Inject global message service
const $message = inject('$message')

// Reactive state
const loading = ref(false)
const error = ref(null)
const logs = ref([])
// Node statistics feature removed; keep placeholder for compatibility
const nodeStats = ref({})
const totalCount = ref(0)
const currentPage = ref(1)
const pageSize = ref(20)
const availableNodes = ref([])
const dataCache = useDataCacheStore()
const clusterInfo = ref({})
const expandedLogs = ref(new Set())

// Filters
const filters = reactive({
  source: 'all',
  nodeId: 'all',
  timeRange: '24h',
  keyword: '',
  startDate: '',
  endDate: ''
})

// Computed properties
const isClusterMode = computed(() => {
  return availableNodes.value.length > 1
})

const totalPages = computed(() => Math.ceil(totalCount.value / pageSize.value))

// Helper functions
const formatTimestamp = (timestamp) => {
  return new Date(timestamp).toLocaleString('en-US', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false
  })
}

const formatFullTimestamp = (timestamp) => {
  return new Date(timestamp).toLocaleString('en-US', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
    timeZoneName: 'short'
  })
}

function toLocalISOString(date) {
  // 生成本地时区的 yyyy-MM-ddTHH:mm 字符串
  const pad = n => n < 10 ? '0' + n : n
  return date.getFullYear() + '-' + pad(date.getMonth() + 1) + '-' + pad(date.getDate()) + 'T' + pad(date.getHours()) + ':' + pad(date.getMinutes())
}

function setDefaultTimeRange() {
  const now = new Date()
  const oneDayAgo = new Date(now.getTime() - 24 * 60 * 60 * 1000)
  
  filters.startDate = toLocalISOString(oneDayAgo)
  filters.endDate = toLocalISOString(now)
}

function handleTimeRangeChange() {
  if (filters.timeRange !== 'custom') {
    const now = new Date()
    let startTime
    
    switch (filters.timeRange) {
      case '1h':
        startTime = new Date(now.getTime() - 60 * 60 * 1000)
        break
      case '6h':
        startTime = new Date(now.getTime() - 6 * 60 * 60 * 1000)
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
    
    filters.startDate = toLocalISOString(startTime)
    filters.endDate = toLocalISOString(now)
  }
  
  applyFilters()
}

const getTimeRangeParams = () => {
  const params = {}
  
  // 用本地时间字符串拼接ISO格式，带本地时区
  if (filters.startDate) {
    const start = new Date(filters.startDate)
    params.start_time = start.toISOString()
  }
  if (filters.endDate) {
    const end = new Date(filters.endDate)
    params.end_time = end.toISOString()
  }
  
  return params
}

const buildApiParams = () => {
  const params = {
    limit: pageSize.value,
    offset: (currentPage.value - 1) * pageSize.value
  }

  if (filters.source !== 'all') params.source = filters.source
  if (filters.nodeId !== 'all') params.node_id = filters.nodeId
  if (filters.keyword) params.keyword = filters.keyword

  Object.assign(params, getTimeRangeParams())

  return params
}

// API functions
const fetchErrorLogs = async () => {
  loading.value = true
  error.value = null

  try {
    const params = buildApiParams()
    
    // Use unified error logs endpoint - always call /error-logs
    // The backend will handle cluster aggregation automatically
    const response = await hubApi.getErrorLogs(params)
    
    logs.value = response.logs || []
    totalCount.value = response.total_count || 0
    
    // Extract available nodes from logs
    const nodes = new Set()
    logs.value.forEach(log => {
      if (log.node_id) {
        nodes.add(log.node_id)
      }
    })
    
    availableNodes.value = Array.from(nodes).map(nodeId => ({
      id: nodeId,
      name: nodeId
    }))

    // Ensure current node is in the list if no logs found
    if (availableNodes.value.length === 0 && clusterInfo.value.self_id) {
      availableNodes.value = [{ id: clusterInfo.value.self_id, name: clusterInfo.value.self_id }]
    }

  } catch (err) {
    error.value = err.message
    
    // Check if this is a follower node access error
    if (err.message.includes('only available on the leader node')) {
      error.value = 'Error logs are only available on the leader node. Please access the leader node to view error logs from all cluster nodes.'
    }
    
    $message?.error?.('Failed to fetch error logs: ' + err.message)
  } finally {
    loading.value = false
  }
}

const applyFilters = async () => {
  currentPage.value = 1
  expandedLogs.value.clear()
  await fetchErrorLogs()
}

const debouncedSearch = debounce(() => {
  applyFilters()
}, 500)

const refreshLogs = async () => {
  await fetchErrorLogs()
}

function toggleLogDetail(index) {
  if (expandedLogs.value.has(index)) {
    expandedLogs.value.delete(index)
  } else {
    expandedLogs.value.add(index)
  }
}

function previousPage() {
  if (currentPage.value > 1) {
    currentPage.value--
    fetchErrorLogs()
  }
}

function nextPage() {
  if (currentPage.value < totalPages.value) {
    currentPage.value++
    fetchErrorLogs()
  }
}

function getSourceClass(source) {
  const classes = {
    'hub': 'bg-blue-500',
    'plugin': 'bg-purple-500'
  }
  return classes[source] || 'bg-gray-500'
}

function getLevelClass(level) {
  const classes = {
    'ERROR': 'bg-red-100 text-red-800',
    'FATAL': 'bg-red-100 text-red-800'
  }
  return classes[level] || 'bg-gray-100 text-gray-800'
}

const exportLogs = () => {
  if (logs.value.length === 0) return

  const csvContent = [
    ['Timestamp', 'Source', 'Level', 'Node', 'Message', 'Context'].join(','),
    ...logs.value.map(log => [
      log.timestamp,
      log.source,
      log.level,
      log.node_id || '',
      `"${log.message.replace(/"/g, '""')}"`,
      `"${(log.context || '').replace(/"/g, '""')}"`
    ].join(','))
  ].join('\n')

  const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' })
  const link = document.createElement('a')
  const url = URL.createObjectURL(blob)
  link.setAttribute('href', url)
  link.setAttribute('download', `error-logs-${new Date().toISOString().slice(0, 19)}.csv`)
  link.style.visibility = 'hidden'
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
}

// Lifecycle
onMounted(async () => {
  setDefaultTimeRange()
  try {
    clusterInfo.value = await dataCache.fetchClusterInfo()
  } catch {}
  await fetchErrorLogs()
})
</script>

<style scoped>
.filter-select {
  @apply block w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500;
}

.filter-input {
  @apply block w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500;
}

.btn {
  @apply inline-flex items-center border font-medium rounded focus:outline-none focus:ring-2 focus:ring-offset-2 transition-colors;
}

.btn-secondary {
  @apply border-gray-300 text-gray-700 bg-white hover:bg-gray-50 focus:ring-blue-500;
}

.btn-sm {
  @apply px-2 py-1 text-xs;
}
</style> 