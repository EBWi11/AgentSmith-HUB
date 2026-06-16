<template>
  <div class="h-full flex flex-col bg-white">
    <div class="flex items-center justify-between p-4 border-b border-gray-200">
      <h1 class="text-xl font-semibold text-gray-900">Agent Logs</h1>
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
      <div class="grid grid-cols-1 md:grid-cols-4 lg:grid-cols-7 gap-4">
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

        <!-- Project Filter -->
        <div>
          <label class="block text-sm font-medium text-gray-700 mb-1">Project</label>
          <input
            v-model="filters.project"
            @input="debouncedSearch"
            type="text"
            placeholder="Filter by project node..."
            class="filter-input w-full"
          >
        </div>

        <!-- Agent Filter -->
        <div>
          <label class="block text-sm font-medium text-gray-700 mb-1">Agent</label>
          <input
            v-model="filters.agent"
            @input="debouncedSearch"
            type="text"
            placeholder="Filter by agent..."
            class="filter-input w-full"
          >
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

        <!-- Search -->
        <div class="lg:col-span-3 md:col-span-2">
          <label class="block text-sm font-medium text-gray-700 mb-1">Search</label>
          <input 
            v-model="filters.keyword" 
            @input="debouncedSearch"
            type="text" 
            placeholder="Search args, result, or error..."
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
        <div class="text-gray-500">Loading agent logs...</div>
      </div>
      
      <div v-else-if="error" class="p-4 bg-red-50 border border-red-200 text-red-700 text-sm">
        {{ error }}
      </div>
      
      <div v-else-if="!logs.length" class="flex-1 flex items-center justify-center text-gray-500">
        No agent logs found
      </div>
      
      <div v-else class="space-y-2 p-4">
        <div
          v-for="(log, index) in logs"
          :key="`${log.node_id}-${log.timestamp}-${index}`"
          class="border border-gray-200 rounded-lg overflow-hidden hover:border-gray-300 transition-colors"
        >
          <div class="flex items-start justify-between gap-3 p-3 bg-gray-50 border-b border-gray-200 cursor-pointer"
               @click="toggleLogDetail(index)">
            <div class="flex items-start space-x-3 min-w-0 flex-1">
              <div class="flex items-center justify-center w-8 h-8 rounded-full bg-green-500 shrink-0 mt-0.5">
                <span class="text-white text-xs font-medium">A</span>
              </div>
              <div class="min-w-0 flex-1">
                <h3 class="font-medium text-gray-900 break-words">
                  {{ log._displayAgentId }}
                </h3>
                <div class="flex flex-wrap items-center gap-x-2 gap-y-1 mt-1 text-sm text-gray-500">
                  <span class="shrink-0">{{ log._formattedTimestamp }}</span>
                  <span v-if="log.node_id" class="text-xs text-blue-600 bg-blue-100 px-2 py-1 rounded font-medium break-all">
                    {{ log.node_id }}
                  </span>
                  <span
                    v-if="log.is_test"
                    class="text-xs text-amber-700 bg-amber-100 px-2 py-1 rounded font-semibold uppercase tracking-wide"
                  >
                    TEST
                  </span>
                  <span
                    v-if="log._displayProjectId"
                    class="text-xs text-indigo-700 bg-indigo-100 px-2 py-1 rounded font-medium break-all max-w-full"
                  >
                    Project: {{ log._displayProjectId }}
                  </span>
                  <span
                    v-if="log._hasMemoryCommitted"
                    class="text-xs text-emerald-800 bg-emerald-50 px-2 py-1 rounded font-medium break-all max-w-full ring-1 ring-emerald-100"
                  >
                    Memory Done
                  </span>
                  <span
                    v-if="log.error"
                    class="text-red-600 text-sm break-words whitespace-pre-wrap w-full"
                  >
                    {{ log.error }}
                  </span>
                </div>
              </div>
            </div>
            
            <div class="flex items-center space-x-2 shrink-0">
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
          
          <!-- Log Details: Input / Output / Trace (stacked) -->
          <div v-if="expandedLogs.has(index)" class="p-4 bg-white space-y-4 min-w-0">
            <!-- Top: Input -->
            <details class="rounded-lg border border-gray-200 overflow-hidden min-w-0 log-io-section">
              <summary
                class="cursor-pointer list-none px-4 py-3 bg-gray-100 border-b border-gray-200 flex items-center justify-between gap-3
                       [&::-webkit-details-marker]:hidden"
              >
                <div class="min-w-0">
                  <h4 class="text-sm font-semibold text-gray-900">Input</h4>
                  <p class="text-xs text-gray-500 mt-0.5">Raw event JSON (full inbound payload)</p>
                  <p class="text-xs text-gray-500 mt-1 leading-relaxed">
                    In the Trace below, LLM input is stored as <span class="font-mono bg-gray-200/80 px-1 rounded">[omitted]</span> (to avoid repeating full context).
                    Full inbound payload is shown here.
                  </p>
                </div>
                <svg class="log-io-section-chevron w-4 h-4 text-gray-500 shrink-0 transition-transform duration-200" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
                </svg>
              </summary>

              <div class="p-4 bg-white">
                <template v-if="log.raw_input">
                  <p
                    v-if="log._isRawInputTruncated"
                    class="text-xs text-amber-900 bg-amber-50 border border-amber-200 rounded-md px-3 py-2 mb-2"
                  >
                    This row contains a truncated JSON snapshot (older hub or oversized event). Upgrade / redeploy hub for larger agent log payloads, or export raw from upstream.
                  </p>
                  <JsonViewer
                    v-if="log._parsedRawInput != null"
                    :value="log._parsedRawInput"
                    height="auto"
                    expand-vertical
                    class="min-w-0"
                  />
                  <div v-else class="bg-gray-50 border border-gray-200 rounded-md p-3 min-w-0">
                    <pre class="text-sm text-gray-800 whitespace-pre-wrap break-words font-mono leading-relaxed">{{ log.raw_input }}</pre>
                  </div>
                </template>
                <p v-else class="text-sm text-gray-500">No input payload recorded.</p>
              </div>
            </details>

            <!-- Middle: Output (LLM only) -->
            <details class="rounded-lg border border-gray-200 overflow-hidden min-w-0 log-io-section" open>
              <summary
                class="cursor-pointer list-none px-4 py-3 bg-gray-100 border-b border-gray-200 flex items-center justify-between gap-3
                       [&::-webkit-details-marker]:hidden"
              >
                <div class="min-w-0">
                  <h4 class="text-sm font-semibold text-gray-900">Output</h4>
                  <p class="text-xs text-gray-500 mt-0.5">LLM block only (not full forwarded message)</p>
                </div>
                <svg class="log-io-section-chevron w-4 h-4 text-gray-500 shrink-0 transition-transform duration-200" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
                </svg>
              </summary>

              <div class="p-4 bg-white">
                <template v-if="log._hasLlmOutput">
                  <p
                    v-if="log._isRawOutputTruncated"
                    class="text-xs text-amber-900 bg-amber-50 border border-amber-200 rounded-md px-3 py-2 mb-2"
                  >
                    Raw output was truncated when stored; the LLM block below may be incomplete or missing.
                  </p>
                  <JsonViewer
                    :value="log._llmOutput"
                    height="auto"
                    expand-vertical
                    class="min-w-0"
                  />
                </template>
                <p v-else class="text-sm text-gray-500">No LLM output block for this run.</p>
              </div>
            </details>

            <!-- Bottom: Trace (collapsed by default) -->
            <details class="log-trace-shell rounded-lg border border-gray-200 overflow-hidden min-w-0">
              <summary
                class="cursor-pointer list-none px-4 py-3 bg-gray-100 border-b border-gray-200 flex items-center justify-between gap-3 hover:bg-gray-200/80 select-none [&::-webkit-details-marker]:hidden"
              >
                <div>
                  <h4 class="text-sm font-semibold text-gray-900">Trace</h4>
                  <p class="text-xs text-gray-500 mt-0.5">Each LLM / tool step; LLM input is [omitted] (see Input above)</p>
                </div>
                <svg
                  class="log-trace-shell-chevron w-4 h-4 text-gray-500 shrink-0 transition-transform duration-200"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
                </svg>
              </summary>
              <div class="p-4 bg-white border-t border-gray-100">
                <AgentTraceViewer v-if="log.trace" :trace="log.trace" :default-open="false" />
                <p v-else class="text-sm text-gray-500">No trace for this run (e.g. timeout, no LLM call reached). See Input above.</p>
              </div>
            </details>

            <!-- Comments -->
            <div class="mt-4 border-t border-gray-200 pt-5">
              <div class="flex items-baseline justify-between gap-2 mb-3">
                <h4 class="text-sm font-semibold text-gray-900">Comments</h4>
                <span v-if="log.comments && log.comments.length" class="text-xs text-gray-500 tabular-nums">
                  {{ log.comments.length }} total
                </span>
              </div>

              <div v-if="(log.comments && log.comments.length)" class="space-y-3 mb-4">
                <div
                  v-for="(c, ci) in log.comments"
                  :key="ci"
                  class="text-sm rounded-lg border border-gray-200 bg-white shadow-sm overflow-hidden"
                  :class="c.type === 'memory_summary' ? 'border-l-4 border-l-emerald-500' : 'border-l-4 border-l-blue-500'"
                >
                  <div class="px-3 py-2.5">
                    <div class="flex flex-wrap items-center justify-between gap-2 mb-2">
                      <span class="font-medium text-gray-900">
                        {{ c.author || 'user' }}
                      </span>
                      <span class="text-xs text-gray-500 shrink-0">
                        {{ formatTimestamp(c.created_at) }}
                      </span>
                    </div>
                    <div class="flex flex-wrap gap-1.5 mb-2">
                      <span
                        v-if="c.type === 'memory_summary'"
                        class="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-emerald-50 text-emerald-800 ring-1 ring-emerald-100"
                      >
                        Memory Summary
                      </span>
                      <span
                        v-if="c.tag"
                        class="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-blue-50 text-blue-800 ring-1 ring-blue-100"
                      >
                        {{ c.tag }}
                      </span>
                      <span
                        v-if="c.status"
                        class="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-gray-50 text-gray-700 ring-1 ring-gray-200"
                      >
                        {{ c.status }}
                      </span>
                    </div>
                    <p class="text-gray-800 whitespace-pre-wrap break-words leading-relaxed text-[13px]">
                      {{ c.comment }}
                    </p>
                  </div>
                </div>
              </div>

              <!-- Composer (one-shot: comment + memory; locked after success) -->
              <div
                v-if="log._hasMemoryCommitted"
                class="rounded-xl border border-emerald-100 bg-emerald-50/60 px-4 py-3 text-sm text-emerald-900"
              >
                <div class="mb-2 flex flex-wrap items-center gap-2">
                  <span
                    class="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-emerald-100 text-emerald-900 ring-1 ring-emerald-200"
                  >
                    Feedback + Memory Done
                  </span>
                </div>
                Memory has been applied for this log from your feedback. Further comments are disabled.
              </div>
              <div v-else class="rounded-xl border border-gray-200 bg-gradient-to-b from-gray-50 to-white p-4 shadow-sm">
                <label :for="'agent-log-comment-' + index" class="block text-xs font-medium text-gray-600 mb-1.5">Feedback</label>
                <textarea
                  :id="'agent-log-comment-' + index"
                  v-model="getCommentDraft(log.id).text"
                  rows="3"
                  class="filter-input w-full resize-y min-h-[5rem] text-sm"
                  placeholder="Describe what to remember (false positive, prompt tweak, etc.). This will be saved and merged into agent memory."
                ></textarea>

                <div class="mt-3 flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
                  <div class="w-full sm:max-w-xs">
                    <label :for="'agent-log-tag-' + index" class="block text-xs font-medium text-gray-600 mb-1.5">Tag</label>
                    <select :id="'agent-log-tag-' + index" v-model="getCommentDraft(log.id).tag" class="filter-select w-full text-sm">
                      <option value="fp">False Positive</option>
                      <option value="tp">True Positive</option>
                      <option value="improve_prompt">Improve Prompt</option>
                    </select>
                  </div>

                  <div class="flex flex-col sm:flex-row sm:flex-wrap gap-2 sm:justify-end sm:pb-0.5 w-full sm:w-auto">
                    <button
                      type="button"
                      @click.stop="submitFeedbackAndMemory(log)"
                      :disabled="!getCommentDraft(log.id).text.trim() || submittingFeedback"
                      class="btn btn-primary btn-sm w-full sm:w-auto justify-center min-w-[10rem]"
                      title="Save your note and update agent memory in one step"
                    >
                      <span v-if="submittingFeedback">Working…</span>
                      <span v-else>Submit &amp; update memory</span>
                    </button>
                  </div>
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
import { ref, shallowRef, reactive, onMounted, inject, computed, defineAsyncComponent } from 'vue'
import { hubApi } from '@/api'
import { debounce } from '../utils/performance'
import { useDataCacheStore } from '../stores/dataCache'
const AgentTraceViewer = defineAsyncComponent(() => import('../components/AgentTraceViewer.vue'))
const JsonViewer = defineAsyncComponent(() => import('../components/JsonViewer.vue'))

const $message = inject('$message')
const dataCache = useDataCacheStore()

const loading = ref(false)
const error = ref(null)
const logs = shallowRef([])
const totalCount = ref(0)

// Filters
const filters = reactive({
  nodeId: 'all',
  project: '',
  agent: '',
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
const commentDrafts = reactive({})
const submittingFeedback = ref(false)

function getCommentDraft(logId) {
  if (!logId) return { text: '', tag: 'fp' }
  if (!commentDrafts[logId]) {
    commentDrafts[logId] = { text: '', tag: 'fp' }
  }
  return commentDrafts[logId]
}

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
    level: 'all',
    limit: pageSize.value,
    offset: (currentPage.value - 1) * pageSize.value
  }

  if (filters.nodeId && filters.nodeId !== 'all') {
    params.node_id = filters.nodeId
  }

  if (filters.keyword) {
    params.keyword = filters.keyword
  }

  if (filters.project) {
    params.project = filters.project
  }
  if (filters.agent) {
    params.agent = filters.agent
  }

  Object.assign(params, getTimeRangeParams())
  return params
}

const fetchLogs = async () => {
  loading.value = true
  error.value = null

  try {
    const params = buildApiParams()
    const response = await hubApi.getAgentLogs(params)

    logs.value = (response.logs || []).map(enrichAgentLog)
    totalCount.value = response.total_count || 0
  } catch (err) {
    error.value = err.message
    $message?.error?.('Failed to fetch agent logs: ' + err.message)
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

const TRUNCATED_AGENT_LOG_SUFFIX = /\.\.\. \(truncated, \d+ bytes total\)\s*$/

function stripAgentLogTruncationSuffix(text) {
  return String(text).replace(TRUNCATED_AGENT_LOG_SUFFIX, '')
}

function isTruncatedPayload(s) {
  if (s === null || s === undefined) return false
  return TRUNCATED_AGENT_LOG_SUFFIX.test(String(s))
}

/** Parsed value for JsonViewer, or null if not valid JSON. */
function parseForJsonViewer(raw) {
  if (raw === null || raw === undefined || raw === '') return null
  if (typeof raw === 'object') return raw
  const text = String(raw)
  const tryParse = (t) => {
    try {
      return JSON.parse(t)
    } catch {
      return null
    }
  }
  let v = tryParse(text)
  if (v !== null) return v
  const stripped = stripAgentLogTruncationSuffix(text)
  if (stripped !== text) {
    v = tryParse(stripped)
    if (v !== null) return v
  }
  return null
}

/**
 * Agent raw_output is the full forwarded message (original fields + llm block).
 * UI should show only the LLM portion here; Trace still holds tool/LLM steps.
 */
function extractLlmOnlyFromRawOutput(log) {
  const raw = log?.raw_output
  if (raw === null || raw === undefined || raw === '') return null

  let parsed
  try {
    parsed = typeof raw === 'object' ? raw : JSON.parse(stripAgentLogTruncationSuffix(String(raw)))
  } catch {
    return null
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return null

  const llm = parsed.llm
  if (llm === null || llm === undefined) return null
  if (typeof llm !== 'object' || Array.isArray(llm)) return llm

  const agentId = log?.agent_id || extractAgentFromContext(log)
  if (agentId && Object.prototype.hasOwnProperty.call(llm, agentId)) {
    return llm[agentId]
  }
  return llm
}

function hasLlmOutput(log) {
  const v = extractLlmOnlyFromRawOutput(log)
  return v !== null && v !== undefined
}

function hasLogMemoryCommitted(log) {
  const list = log?.comments || []
  return list.some((c) => c.type === 'memory_summary' && c.status === 'committed')
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
    ['Timestamp', 'Node', 'Agent', 'ProjectNode', 'Error', 'RawInput', 'RawOutput', 'Trace'].join(','),
    ...logs.value.map(log => [
      log.timestamp,
      log.node_id || '',
      `"${(log.agent_id || '').replace(/"/g, '""')}"`,
      `"${(log.project_node_sequence || '').replace(/"/g, '""')}"`,
      `"${(log.error || '').replace(/"/g, '""')}"`,
      `"${(log.raw_input || '').replace(/"/g, '""')}"`,
      `"${(log.raw_output || '').replace(/"/g, '""')}"`,
      `"${(log.trace || '').replace(/"/g, '""')}"`
    ].join(','))
  ].join('\n')

  const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' })
  const link = document.createElement('a')
  const url = URL.createObjectURL(blob)
  link.setAttribute('href', url)
  link.setAttribute('download', `agent-logs-${new Date().toISOString().slice(0, 19)}.csv`)
  link.style.visibility = 'hidden'
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  URL.revokeObjectURL(url)
}

async function submitFeedbackAndMemory(log) {
  const draft = getCommentDraft(log.id)
  const text = (draft.text || '').trim()
  if (!text || submittingFeedback.value) return
  submittingFeedback.value = true
  try {
    const resp = await hubApi.generateAgentMemoryFromLog(log.agent_id, log.id, {
      comment: text,
      tag: draft.tag
    })
    draft.text = ''
    draft.tag = 'fp'
    await fetchLogs()
    if (resp?.warning) {
      $message?.warning?.(resp.warning)
    } else {
      $message?.success?.('Feedback saved and memory updated')
    }
  } catch (err) {
    if (err?.status === 409) {
      await fetchLogs()
      $message?.warning?.(
        err.message ||
          'Agent memory was updated elsewhere. Logs refreshed — review and submit again if needed.'
      )
    } else {
      $message?.error?.('Failed to submit: ' + err.message)
    }
  } finally {
    submittingFeedback.value = false
  }
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

function extractProjectId(log) {
  if (log?.project_id) return log.project_id
  const ctx = safeParseContext(log)
  return ctx.project || ''
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

function enrichAgentLog(log) {
  const parsedContext = safeParseContext(log)
  const displayAgentId = log.agent_id || parsedContext.agent || 'Unknown Agent'
  const displayProjectId = log.project_id || parsedContext.project || ''
  const parsedRawInput = parseForJsonViewer(log.raw_input)
  const llmOutput = extractLlmOnlyFromRawOutput({
    ...log,
    context: Object.keys(parsedContext).length ? JSON.stringify(parsedContext) : log.context
  })

  return {
    ...log,
    _formattedTimestamp: formatTimestamp(log.timestamp),
    _displayAgentId: displayAgentId,
    _displayProjectId: displayProjectId,
    _parsedRawInput: parsedRawInput,
    _isRawInputTruncated: isTruncatedPayload(log.raw_input),
    _llmOutput: llmOutput,
    _hasLlmOutput: llmOutput !== null && llmOutput !== undefined,
    _isRawOutputTruncated: Boolean(log.raw_output) && isTruncatedPayload(log.raw_output),
    _hasMemoryCommitted: hasLogMemoryCommitted(log)
  }
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

.btn-primary {
  @apply border-transparent text-white bg-blue-600 hover:bg-blue-700 focus:ring-blue-500 disabled:opacity-50 disabled:cursor-not-allowed disabled:hover:bg-blue-600;
}

.btn-ghost {
  @apply border-gray-200 text-gray-600 bg-white hover:bg-gray-50 hover:text-gray-900 focus:ring-gray-400 disabled:opacity-50 disabled:cursor-not-allowed;
}

.btn-sm {
  @apply px-3 py-1.5 text-xs;
}

/* Trace panel: chevron reflects open state (Tailwind 3.3 has no group-open on <details>) */
.log-trace-shell[open] > summary .log-trace-shell-chevron {
  transform: rotate(180deg);
}

/* Input/Output panels: chevron reflects open state */
.log-io-section[open] > summary .log-io-section-chevron {
  transform: rotate(180deg);
}
</style>
