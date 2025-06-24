<template>
  <div class="h-full flex flex-col bg-gray-50">
    <!-- Header -->
    <div class="bg-white shadow-sm border-b border-gray-200 p-4">
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-2xl font-semibold text-gray-900">Error Logs</h1>
          <p class="mt-1 text-sm text-gray-600">Monitor and analyze error logs from hub and plugin components across cluster nodes</p>
        </div>
        <div class="flex items-center space-x-4">
          <!-- Refresh Button -->
          <button
            @click="refreshLogs"
            :disabled="loading"
            class="inline-flex items-center px-3 py-2 border border-gray-300 rounded-md text-sm font-medium text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500 disabled:opacity-50"
          >
            <svg class="w-4 h-4 mr-2" :class="{ 'animate-spin': loading }" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"></path>
            </svg>
            Refresh
          </button>

          <!-- Export Button -->
          <button
            @click="exportLogs"
            :disabled="logs.length === 0"
            class="inline-flex items-center px-3 py-2 border border-gray-300 rounded-md text-sm font-medium text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500 disabled:opacity-50"
          >
            <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 10v6m0 0l-3-3m3 3l3-3m2 8H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"></path>
            </svg>
            Export
          </button>
        </div>
      </div>
    </div>

    <!-- Filters -->
    <div class="bg-white border-b border-gray-200 p-4">
      <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        <!-- Source Filter -->
        <div>
          <label class="block text-sm font-medium text-gray-700 mb-1">Source</label>
          <select
            v-model="filters.source"
            @change="applyFilters"
            class="block w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
          >
            <option value="all">All Sources</option>
            <option value="hub">Hub</option>
            <option value="plugin">Plugin</option>
          </select>
        </div>

        <!-- Node Filter -->
        <div>
          <label class="block text-sm font-medium text-gray-700 mb-1">Node</label>
          <select
            v-model="filters.nodeId"
            @change="applyFilters"
            class="block w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
          >
            <option value="all">All Nodes</option>
            <option v-for="node in availableNodes" :key="node.id" :value="node.id">
              {{ node.name || node.id }}
            </option>
          </select>
        </div>

        <!-- Time Range Filter -->
        <div>
          <label class="block text-sm font-medium text-gray-700 mb-1">Time Range</label>
          <select
            v-model="filters.timeRange"
            @change="applyTimeRange"
            class="block w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
          >
            <option value="1h">Last Hour</option>
            <option value="6h">Last 6 Hours</option>
            <option value="24h">Last 24 Hours</option>
            <option value="7d">Last 7 Days</option>
            <option value="30d">Last 30 Days</option>
            <option value="custom">Custom</option>
          </select>
        </div>

        <!-- Keyword Search -->
        <div>
          <label class="block text-sm font-medium text-gray-700 mb-1">Keyword Search</label>
          <div class="relative">
            <input
              v-model="filters.keyword"
              @keyup.enter="applyFilters"
              type="text"
              placeholder="Search in messages..."
              class="block w-full px-3 py-2 pr-10 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
            >
            <button
              @click="applyFilters"
              class="absolute inset-y-0 right-0 px-3 flex items-center text-gray-400 hover:text-gray-600"
            >
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"></path>
              </svg>
            </button>
          </div>
        </div>
      </div>

      <!-- Custom Time Range -->
      <div v-if="filters.timeRange === 'custom'" class="mt-4 grid grid-cols-2 gap-4">
        <div>
          <label class="block text-sm font-medium text-gray-700 mb-1">Start Time</label>
          <input
            v-model="filters.startTime"
            @change="applyFilters"
            type="datetime-local"
            class="block w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
          >
        </div>
        <div>
          <label class="block text-sm font-medium text-gray-700 mb-1">End Time</label>
          <input
            v-model="filters.endTime"
            @change="applyFilters"
            type="datetime-local"
            class="block w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
          >
        </div>
      </div>

      <!-- Statistics -->
      <div v-if="nodeStats && Object.keys(nodeStats).length > 0" class="mt-4 grid grid-cols-1 md:grid-cols-3 lg:grid-cols-6 gap-4">
        <div v-for="(stat, nodeId) in nodeStats" :key="nodeId" class="bg-gray-50 px-3 py-2 rounded-md">
          <div class="text-xs font-medium text-gray-500 truncate">{{ stat.node_id }}</div>
          <div class="flex items-center space-x-2 mt-1">
            <span class="text-sm font-semibold text-red-600">{{ stat.total_errors }}</span>
            <span class="text-xs text-gray-500">errors</span>
          </div>
          <div class="flex space-x-2 text-xs text-gray-400 mt-1">
            <span>H: {{ stat.hub_errors }}</span>
            <span>P: {{ stat.plugin_errors }}</span>
          </div>
        </div>
      </div>
    </div>

    <!-- Content -->
    <div class="flex-1 overflow-hidden">
      <!-- Loading State -->
      <div v-if="loading && logs.length === 0" class="flex items-center justify-center h-full">
        <div class="text-center">
          <svg class="animate-spin h-8 w-8 text-blue-500 mx-auto mb-4" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
          </svg>
          <p class="text-gray-600">Loading error logs...</p>
        </div>
      </div>

      <!-- Error State -->
      <div v-else-if="error" class="flex items-center justify-center h-full">
        <div class="text-center">
          <svg class="h-12 w-12 text-red-400 mx-auto mb-4" fill="none" viewBox="0 0 24 24">
            <path stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"></path>
          </svg>
          <h3 class="text-lg font-medium text-gray-900 mb-2">Failed to load error logs</h3>
          <p class="text-gray-600 mb-4">{{ error }}</p>
          <button
            @click="refreshLogs"
            class="inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md text-white bg-blue-600 hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500"
          >
            Try Again
          </button>
        </div>
      </div>

      <!-- No Data State -->
      <div v-else-if="logs.length === 0 && !loading" class="flex items-center justify-center h-full">
        <div class="text-center">
          <svg class="h-12 w-12 text-gray-400 mx-auto mb-4" fill="none" viewBox="0 0 24 24">
            <path stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"></path>
          </svg>
          <h3 class="text-lg font-medium text-gray-900 mb-2">No error logs found</h3>
          <p class="text-gray-600">No error logs match your current filters.</p>
        </div>
      </div>

      <!-- Log List -->
      <div v-else class="h-full overflow-auto">
        <div class="divide-y divide-gray-200">
          <div
            v-for="(log, index) in logs"
            :key="`${log.node_id}-${log.source}-${log.line}-${index}`"
            class="p-4 hover:bg-gray-50 transition-colors"
          >
            <div class="flex items-start space-x-4">
              <!-- Source Icon -->
              <div class="flex-shrink-0 mt-1">
                <div
                  class="w-8 h-8 rounded-full flex items-center justify-center text-xs font-medium"
                  :class="{
                    'bg-blue-100 text-blue-800': log.source === 'hub',
                    'bg-purple-100 text-purple-800': log.source === 'plugin'
                  }"
                >
                  {{ log.source === 'hub' ? 'H' : 'P' }}
                </div>
              </div>

              <!-- Content -->
              <div class="flex-1 min-w-0">
                <div class="flex items-center justify-between">
                  <div class="flex items-center space-x-2">
                    <span class="text-sm font-medium text-gray-900">{{ log.message }}</span>
                    <span
                      class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium"
                      :class="{
                        'bg-red-100 text-red-800': log.level === 'ERROR',
                        'bg-orange-100 text-orange-800': log.level === 'FATAL',
                        'bg-gray-100 text-gray-800': !['ERROR', 'FATAL'].includes(log.level)
                      }"
                    >
                      {{ log.level }}
                    </span>
                  </div>
                  <div class="flex items-center space-x-4 text-xs text-gray-500">
                    <span>{{ formatTimestamp(log.timestamp) }}</span>
                    <span v-if="log.node_id">Node: {{ log.node_id }}</span>
                    <span>Line: {{ log.line }}</span>
                  </div>
                </div>
                
                <!-- Context -->
                <div v-if="log.context && log.context !== log.message" class="mt-2">
                  <details class="group">
                    <summary class="cursor-pointer text-xs text-gray-500 hover:text-gray-700">
                      <span class="group-open:hidden">Show context</span>
                      <span class="hidden group-open:inline">Hide context</span>
                    </summary>
                    <div class="mt-2 p-3 bg-gray-100 rounded-md">
                      <pre class="text-xs text-gray-700 whitespace-pre-wrap break-all">{{ log.context }}</pre>
                    </div>
                  </details>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Load More -->
        <div v-if="hasMore" class="p-4 text-center border-t border-gray-200">
          <button
            @click="loadMore"
            :disabled="loading"
            class="inline-flex items-center px-4 py-2 border border-gray-300 rounded-md text-sm font-medium text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500 disabled:opacity-50"
          >
            <svg v-if="loading" class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
            </svg>
            {{ loading ? 'Loading...' : 'Load More' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, inject, computed } from 'vue'
import { hubApi } from '@/api'

// Inject global message service
const $message = inject('$message')

// Reactive state
const loading = ref(false)
const error = ref(null)
const logs = ref([])
const nodeStats = ref({})
const hasMore = ref(false)
const totalCount = ref(0)
const availableNodes = ref([])

// Filters
const filters = reactive({
  source: 'all',
  nodeId: 'all',
  timeRange: '24h',
  keyword: '',
  startTime: '',
  endTime: '',
  limit: 50,
  offset: 0
})

// Computed properties
const isClusterMode = computed(() => {
  return availableNodes.value.length > 1
})

// Helper functions
const formatTimestamp = (timestamp) => {
  return new Date(timestamp).toLocaleString()
}

const getTimeRangeParams = () => {
  const now = new Date()
  let startTime, endTime

  switch (filters.timeRange) {
    case '1h':
      startTime = new Date(now.getTime() - 60 * 60 * 1000)
      break
    case '6h':
      startTime = new Date(now.getTime() - 6 * 60 * 60 * 1000)
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
    case 'custom':
      if (filters.startTime) startTime = new Date(filters.startTime)
      if (filters.endTime) endTime = new Date(filters.endTime)
      break
  }

  const params = {}
  if (startTime) params.start_time = startTime.toISOString()
  if (endTime) params.end_time = endTime.toISOString()
  
  return params
}

const buildApiParams = () => {
  const params = {
    limit: filters.limit,
    offset: filters.offset
  }

  if (filters.source !== 'all') params.source = filters.source
  if (filters.nodeId !== 'all') params.node_id = filters.nodeId
  if (filters.keyword) params.keyword = filters.keyword

  Object.assign(params, getTimeRangeParams())

  return params
}

// API functions
const fetchErrorLogs = async (reset = true) => {
  if (reset) {
    filters.offset = 0
    logs.value = []
  }

  loading.value = true
  error.value = null

  try {
    const params = buildApiParams()
    
    // Use cluster API if available and not filtering by specific node
    let response
    if (isClusterMode.value && filters.nodeId === 'all') {
      response = await hubApi.getClusterErrorLogs(params)
      if (response.node_stats) {
        nodeStats.value = response.node_stats
      }
    } else {
      response = await hubApi.getErrorLogs(params)
      nodeStats.value = {}
    }

    if (reset) {
      logs.value = response.logs || []
    } else {
      logs.value.push(...(response.logs || []))
    }

    hasMore.value = response.has_more || false
    totalCount.value = response.total_count || logs.value.length

    // Update available nodes from logs
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

  } catch (err) {
    error.value = err.message
    $message?.error?.('Failed to fetch error logs: ' + err.message)
  } finally {
    loading.value = false
  }
}

const loadMore = async () => {
  if (loading.value || !hasMore.value) return
  
  filters.offset += filters.limit
  await fetchErrorLogs(false)
}

const applyFilters = async () => {
  await fetchErrorLogs(true)
}

const applyTimeRange = async () => {
  // Clear custom time inputs when switching away from custom
  if (filters.timeRange !== 'custom') {
    filters.startTime = ''
    filters.endTime = ''
  }
  await fetchErrorLogs(true)
}

const refreshLogs = async () => {
  await fetchErrorLogs(true)
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
  await fetchErrorLogs(true)
})
</script> 