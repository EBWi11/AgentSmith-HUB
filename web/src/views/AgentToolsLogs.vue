<template>
  <div class="h-full flex flex-col bg-white">
    <div class="flex items-center justify-between p-4 border-b border-gray-200">
      <h1 class="text-xl font-semibold text-gray-900">Agent Tools Logs</h1>
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

        <!-- Search (agent / project / args / result) -->
        <div class="lg:col-span-3">
          <label class="block text-sm font-medium text-gray-700 mb-1">Search</label>
          <input 
            v-model="filters.keyword" 
            @input="debouncedSearch"
            type="text" 
            placeholder="Search agent, project, args, or result..."
            class="filter-input w-full"
          >
        </div>
      </div>

      <!-- Custom Date Range -->
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
    </div>

    <!-- Content -->
    <div class="flex-1 overflow-y-auto">
      <div v-if="loading && !logs.length" class="flex items-center justify-center h-64">
        <div class="text-gray-500">Loading agent tools logs...</div>
      </div>
      
      <div v-else-if="error" class="p-4 bg-red-50 border border-red-200 text-red-700 text-sm">
        {{ error }}
      </div>
      
      <div v-else-if="!logs.length" class="flex-1 flex items-center justify-center text-gray-500">
        No agent tools logs found
      </div>
      
      <div v-else class="space-y-2 p-4">
        <div
          v-for="(log, index) in logs"
          :key="`${log.node_id}-${log.timestamp}-${index}`"
          class="border border-gray-200 rounded-lg overflow-hidden hover:border-gray-300 transition-colors"
        >
          <div class="flex items-center justify-between p-3 bg-gray-50 border-b border-gray-200 cursor-pointer"
               @click="toggleLogDetail(index)">
            <div class="flex items-center space-x-3">
              <div class="flex items-center space-x-2">
                <!-- Source Icon (always Agent here) -->
                <div class="flex items-center justify-center w-8 h-8 rounded-full bg-green-500">
                  <span class="text-white text-xs font-medium">A</span>
                </div>
                
                <!-- Log Info -->
                <div>
                  <h3 class="font-medium text-gray-900">
                    {{ extractAgentFromContext(log) }} · {{ extractToolNameFromContext(log) }}
                  </h3>
                  <div class="flex items-center space-x-2 text-sm text-gray-500">
                    <span>{{ formatTimestamp(log.timestamp) }}</span>
                    <span v-if="log.node_id" class="text-xs text-blue-600 bg-blue-100 px-2 py-1 rounded font-medium">
                      {{ log.node_id }}
                    </span>
                    <span v-if="extractProjectFromContext(log)" class="text-xs text-purple-700 bg-purple-100 px-2 py-1 rounded font-medium">
                      {{ extractProjectFromContext(log) }}
                    </span>
                    <span v-if="log.error" class="text-red-600 truncate max-w-xs" :title="log.error">{{ log.error }}</span>
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
          
          <!-- Log Details -->
          <div v-if="expandedLogs.has(index)" class="p-4 bg-white">
            <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
              <!-- Basic Info -->
              <div>
                <h4 class="text-sm font-medium text-gray-900 mb-2">Call Details</h4>
                <dl class="space-y-1 text-sm">
                  <div class="grid grid-cols-3 gap-1">
                    <dt class="text-gray-500">Agent:</dt>
                    <dd class="col-span-2 text-gray-900">{{ extractAgentFromContext(log) || '-' }}</dd>
                  </div>
                  <div class="grid grid-cols-3 gap-1">
                    <dt class="text-gray-500">Project Node:</dt>
                    <dd class="col-span-2 text-gray-900">{{ extractProjectFromContext(log) || '-' }}</dd>
                  </div>
                  <div class="grid grid-cols-3 gap-1">
                    <dt class="text-gray-500">Tool:</dt>
                    <dd class="col-span-2 text-gray-900">{{ extractToolNameFromContext(log) || '-' }}</dd>
                  </div>
                  <div class="grid grid-cols-3 gap-1">
                    <dt class="text-gray-500">Timestamp:</dt>
                    <dd class="col-span-2 text-gray-900">{{ formatFullTimestamp(log.timestamp) }}</dd>
                  </div>
                  <div v-if="log.error" class="grid grid-cols-3 gap-1">
                    <dt class="text-gray-500">Error:</dt>
                    <dd class="col-span-2 text-red-700 font-medium whitespace-pre-wrap break-words">{{ log.error }}</dd>
                  </div>
                </dl>
              </div>

              <!-- Raw Context (includes args/result/duration/etc) -->
              <div v-if="log.context">
                <h4 class="text-sm font-medium text-gray-900 mb-2">Raw Context</h4>
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

const $message = inject('$message')
const dataCache = useDataCacheStore()

const loading = ref(false)
const error = ref(null)
const logs = ref([])
const totalCount = ref(0)

// Filters
const filters = reactive({
  nodeId: 'all',
  timeRange: '1h',
  startDate: '',
  endDate: '',
  keyword: ''
})

const availableNodes = ref([])

// Pagination
const currentPage = ref(1)
const pageSize = ref(50)
const expandedLogs = ref(new Set())

const totalPages = computed(() => {
  return Math.max(1, Math.ceil(totalCount.value / pageSize.value))
})

function setDefaultTimeRange() {
  filters.timeRange = '1h'
  filters.startDate = ''
  filters.endDate = ''
}

function getTimeRangeParams() {
  if (filters.timeRange === 'custom' && filters.startDate && filters.endDate) {
    return {
      start_time: new Date(filters.startDate).toISOString(),
      end_time: new Date(filters.endDate).toISOString()
    }
  }

  const end = new Date()
  let start = new Date(end)

  switch (filters.timeRange) {
    case '1h':
      start.setHours(end.getHours() - 1)
      break
    case '6h':
      start.setHours(end.getHours() - 6)
      break
    case '12h':
      start.setHours(end.getHours() - 12)
      break
    case '24h':
      start.setDate(end.getDate() - 1)
      break
    case '7d':
      start.setDate(end.getDate() - 7)
      break
    case '30d':
      start.setDate(end.getDate() - 30)
      break
    default:
      start.setHours(end.getHours() - 1)
  }

  return {
    start_time: start.toISOString(),
    end_time: end.toISOString()
  }
}

function buildApiParams() {
  const params = {
    source: 'agent',
    limit: pageSize.value,
    offset: (currentPage.value - 1) * pageSize.value
  }

  if (filters.nodeId && filters.nodeId !== 'all') {
    params.node_id = filters.nodeId
  }

  if (filters.keyword) {
    params.keyword = filters.keyword
  }

  Object.assign(params, getTimeRangeParams())
  return params
}

const fetchLogs = async () => {
  loading.value = true
  error.value = null

  try {
    const params = buildApiParams()
    const response = await hubApi.getErrorLogs(params) // reuse unified endpoint

    logs.value = response.logs || []
    totalCount.value = response.total_count || 0
  } catch (err) {
    error.value = err.message
    $message?.error?.('Failed to fetch agent tools logs: ' + err.message)
  } finally {
    loading.value = false
  }
}

const fetchAvailableNodes = async () => {
  try {
    const response = await hubApi.getErrorLogNodes()
    availableNodes.value = response.nodes.map(nodeId => ({
      id: nodeId,
      name: nodeId
    }))
  } catch (err) {
    console.warn('Failed to fetch error log nodes for agent tools logs:', err)
    try {
      const clusterInfo = await dataCache.fetchClusterInfo()
      const nodes = new Set()
      if (clusterInfo.self_id) nodes.add(clusterInfo.self_id)
      if (clusterInfo.nodes && Array.isArray(clusterInfo.nodes)) {
        clusterInfo.nodes.forEach(node => node.id && nodes.add(node.id))
      }
      availableNodes.value = Array.from(nodes).map(id => ({ id, name: id }))
    } catch {}
  }
}

const applyFilters = async () => {
  currentPage.value = 1
  expandedLogs.value.clear()
  await fetchLogs()
}

const debouncedSearch = debounce(() => {
  applyFilters()
}, 500)

const refreshLogs = async () => {
  await fetchLogs()
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
    fetchLogs()
  }
}

function nextPage() {
  if (currentPage.value < totalPages.value) {
    currentPage.value++
    fetchLogs()
  }
}

function formatTimestamp(ts) {
  if (!ts) return ''
  return new Date(ts).toLocaleString()
}

function formatFullTimestamp(ts) {
  if (!ts) return ''
  return new Date(ts).toISOString()
}

function getLevelClass(level) {
  const classes = {
    'ERROR': 'bg-red-100 text-red-800',
    'FATAL': 'bg-red-100 text-red-800',
  }
  return classes[level] || 'bg-gray-100 text-gray-800'
}

function exportLogs() {
  if (logs.value.length === 0) return

  const csvContent = [
    ['Timestamp', 'Node', 'Level', 'Message', 'Error', 'Context'].join(','),
    ...logs.value.map(log => [
      log.timestamp,
      log.node_id || '',
      log.level,
      `"${(log.message || '').replace(/"/g, '""')}"`,
      `"${(log.error || '').replace(/"/g, '""')}"`,
      `"${(log.context || '').replace(/"/g, '""')}"`
    ].join(','))
  ].join('\n')

  const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' })
  const link = document.createElement('a')
  const url = URL.createObjectURL(blob)
  link.setAttribute('href', url)
  link.setAttribute('download', `agent-tools-logs-${new Date().toISOString().slice(0, 19)}.csv`)
  link.style.visibility = 'hidden'
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
}

// Helpers to extract agent / project / tool from context JSON string (Details)
function safeParseContext(log) {
  if (!log.context) return {}
  try {
    return JSON.parse(log.context)
  } catch {
    return {}
  }
}

function extractAgentFromContext(log) {
  const ctx = safeParseContext(log)
  return ctx.agent || ''
}

function extractProjectFromContext(log) {
  const ctx = safeParseContext(log)
  return ctx.project_node_sequence || ''
}

function extractToolNameFromContext(log) {
  const ctx = safeParseContext(log)
  if (ctx.kind === 'plugin') {
    return ctx.plugin || ''
  }
  if (ctx.kind === 'skill') {
    if (ctx.skill && ctx.function) return `${ctx.skill}.${ctx.function}`
    return ctx.skill || ctx.function || ''
  }
  return ''
}

function handleTimeRangeChange() {
  if (filters.timeRange !== 'custom') {
    filters.startDate = ''
    filters.endDate = ''
    applyFilters()
  }
}

onMounted(async () => {
  setDefaultTimeRange()
  try {
    await fetchAvailableNodes()
  } catch {}
  await fetchLogs()
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

