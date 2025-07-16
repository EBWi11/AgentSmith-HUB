<template>
  <div class="p-6 h-full overflow-auto">
    <div class="flex justify-between items-center mb-6">
      <h2 class="text-2xl font-bold text-gray-900">Cluster Nodes</h2>
      
      <!-- Search Bar -->
      <div class="relative max-w-md">
        <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
          <svg class="h-5 w-5 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
          </svg>
        </div>
        <input
          v-model="searchQuery"
          type="text"
          placeholder="Search by IP address..."
          class="w-full pl-10 pr-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-transparent"
        />
      </div>
    </div>

    <!-- Loading State -->
    <div v-if="loading" class="flex justify-center items-center h-64">
      <div class="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-500"></div>
    </div>

    <!-- Error State -->
    <div v-else-if="error" class="text-center text-red-500 py-8">
      <svg class="mx-auto h-12 w-12 text-red-400 mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
      </svg>
      <p class="text-lg font-medium">{{ error }}</p>
    </div>

    <!-- Nodes List -->
    <div v-else class="space-y-4">
      <div 
        v-for="node in filteredNodes" 
        :key="node.id"
        class="bg-white rounded-lg shadow-md border border-gray-200 overflow-hidden transition-all duration-200 hover:shadow-lg"
        :class="{
          'ring-2 ring-blue-500 border-blue-500': node.isLeader,
          'ring-2 ring-red-500 border-red-500': node.hasStatusIssue,
          'ring-1 ring-yellow-500 border-yellow-500': node.hasPerformanceIssue
        }"
      >
        <!-- Main Node Info Row with horizontal scroll for wide content -->
        <div class="px-6 py-4 overflow-x-auto">
          <div class="flex items-center min-w-max space-x-6">
            <!-- Left: Basic Info -->
            <div class="flex items-center space-x-4 flex-shrink-0">
              <!-- Role Badge -->
              <span 
                class="inline-flex items-center px-3 py-1 rounded-full text-sm font-medium w-20 justify-center"
                :class="{
                  'bg-blue-100 text-blue-800': node.isLeader,
                  'bg-gray-100 text-gray-800': !node.isLeader
                }"
              >
                <svg class="w-4 h-4 mr-1" fill="currentColor" viewBox="0 0 20 20">
                  <path v-if="node.isLeader" d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
                  <path v-else d="M10 12a2 2 0 100-4 2 2 0 000 4z M10 2a8 8 0 100 16 8 8 0 000-16zM8 10a2 2 0 114 0 2 2 0 01-4 0z" />
                </svg>
                {{ node.isLeader ? 'Leader' : 'Follower' }}
              </span>
              
              <!-- Node Address & ID -->
              <div class="w-36 flex-shrink-0">
                <div class="text-lg font-semibold text-gray-900 truncate" :title="node.address">{{ node.address }}</div>
                <div class="text-sm text-gray-500 truncate" :title="node.id">ID: {{ node.id }}</div>
              </div>
              
              <!-- Status Indicators -->
              <div class="flex items-center space-x-2 flex-shrink-0">
                <!-- Health Status (always rendered to reserve width) -->
                <div
                  class="w-3 h-3 rounded-full"
                  :class="node.isHealthy ? 'bg-green-500' : 'bg-red-500'"
                  :title="node.isHealthy ? 'Healthy' : 'Unhealthy'"
                ></div>

                <!-- Status Consistency -->
                <div class="w-3 h-3">
                  <div
                    v-if="node.hasStatusIssue"
                    class="w-3 h-3 rounded-full bg-red-500 animate-pulse"
                    title="Status inconsistent with leader"
                  ></div>
                </div>

                <!-- Performance Warning -->
                <div class="w-3 h-3">
                  <div
                    v-if="node.hasPerformanceIssue"
                    class="w-3 h-3 rounded-full bg-yellow-500 animate-pulse"
                    title="Performance issue detected"
                  ></div>
                </div>
              </div>
            </div>

            <!-- Center: Message Metrics -->
            <div class="flex items-center space-x-4 flex-shrink-0">
              <!-- Input Messages -->
              <div class="text-center w-16">
                <div class="text-xs text-blue-600 font-medium mb-1">Input/d</div>
                <div class="text-lg font-bold text-blue-800">
                  {{ formatMessagesPerDay(node.metrics.inputMessages) }}
                </div>
              </div>
              
              <!-- Output Messages -->
              <div class="text-center w-16">
                <div class="text-xs text-green-600 font-medium mb-1">Output/d</div>
                <div class="text-lg font-bold text-green-800">
                  {{ formatMessagesPerDay(node.metrics.outputMessages) }}
                </div>
              </div>
              
              <!-- Version -->
              <div class="text-center w-32">
                <div class="text-xs text-purple-600 font-medium mb-1">Version</div>
                <div 
                  class="text-[10px] font-mono px-1 py-1 rounded text-center break-all leading-tight"
                  :class="getVersionDisplayClass(node)"
                  :title="getVersionTooltip(node)"
                >
                  {{ formatVersion(node.version) }}
                </div>
              </div>
            </div>

            <!-- Right: System Resources -->
            <div class="flex items-center space-x-6 flex-shrink-0">
              <!-- CPU Usage -->
              <div class="text-center">
                <div class="text-xs text-gray-600 font-medium mb-1">CPU</div>
                <div class="flex items-center space-x-2">
                  <div class="w-12 bg-gray-200 rounded-full h-2">
                    <div 
                      class="h-2 rounded-full transition-all duration-300"
                      :class="getCPUBarColor(node.metrics.cpuPercent)"
                      :style="{ width: `${Math.min(node.metrics.cpuPercent, 100)}%` }"
                    ></div>
                  </div>
                  <span class="text-sm font-semibold min-w-max" :class="getCPUColor(node.metrics.cpuPercent)">
                    {{ node.metrics.cpuPercent.toFixed(1) }}%
                  </span>
                </div>
              </div>

              <!-- Memory Usage -->
              <div class="text-center">
                <div class="text-xs text-gray-600 font-medium mb-1">Memory</div>
                <div class="flex items-center space-x-2">
                  <div class="w-12 bg-gray-200 rounded-full h-2">
                    <div 
                      class="h-2 rounded-full transition-all duration-300"
                      :class="getMemoryBarColor(node.metrics.memoryPercent)"
                      :style="{ width: `${Math.min(node.metrics.memoryPercent, 100)}%` }"
                    ></div>
                  </div>
                  <span class="text-sm font-semibold min-w-max" :class="getMemoryColor(node.metrics.memoryPercent)">
                    {{ node.metrics.memoryUsedMB.toFixed(1) }}MB
                  </span>
                </div>
              </div>
              
              <!-- Goroutines -->
              <div class="text-center w-16">
                <div class="text-xs text-gray-600 font-medium mb-1">Goroutines</div>
                <div class="text-lg font-semibold text-gray-800">{{ node.metrics.goroutineCount }}</div>
              </div>
            </div>

            <!-- Far Right: Last Seen -->
            <div class="text-center flex-shrink-0 w-20">
              <div class="text-xs text-gray-600 font-medium mb-1">Last Seen</div>
              <span class="text-[10px] text-gray-500 leading-tight">
                {{ formatTimeAgo(node.lastSeen) }}
              </span>
            </div>
          </div>
        </div>

        <!-- Status Issues Alert (if any) -->
        <div v-if="node.hasStatusIssue || node.hasPerformanceIssue" class="px-6 py-3 bg-red-50 border-t border-red-100">
          <div class="flex items-start space-x-2">
            <svg class="w-4 h-4 text-red-500 mt-0.5 flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
              <path fill-rule="evenodd" d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z" clip-rule="evenodd" />
            </svg>
            <div class="text-sm">
              <div v-if="node.hasStatusIssue" class="text-red-700 font-medium">Status Inconsistency</div>
              <div v-if="node.hasPerformanceIssue" class="text-yellow-700 font-medium">Performance Issue</div>
              <div class="text-red-600 mt-1">{{ getIssueDescription(node) }}</div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Empty State -->
    <div v-if="!loading && !error && filteredNodes.length === 0" class="flex-1 flex items-center justify-center text-gray-500">
      {{ searchQuery ? 'No nodes match your search query' : 'No cluster nodes available' }}
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { hubApi } from '../api'
import { useDataCacheStore } from '../stores/dataCache'
import { formatMessagesPerDay, formatTimeAgo, getCPUColor, getCPUBarColor, getMemoryColor, getMemoryBarColor } from '../utils/common'

// Reactive state
const searchQuery = ref('')
const loading = ref(true)
const error = ref(null)
const clusterInfo = ref({})
const nodeMessageData = ref({})
const systemMetrics = ref({})
const refreshInterval = ref(null)

// Data cache store
const dataCache = useDataCacheStore()

// Computed properties
const filteredNodes = computed(() => {
  const nodes = processedNodes.value
  if (!searchQuery.value.trim()) {
    return nodes
  }
  
  const query = searchQuery.value.toLowerCase().trim()
  return nodes.filter(node => 
    node.address.toLowerCase().includes(query) ||
    node.id.toLowerCase().includes(query)
  )
})

const processedNodes = computed(() => {
  const nodes = []
  
  // Add current node (self)
  if (clusterInfo.value.self_id) {
    const selfNode = {
      id: clusterInfo.value.self_id,
      address: clusterInfo.value.self_address,
      isLeader: clusterInfo.value.status === 'leader',
      isHealthy: true,
      lastSeen: new Date(),
      version: clusterInfo.value.version || 'unknown',
      metrics: getNodeMetrics(clusterInfo.value.self_id),
      hasStatusIssue: false,
      hasPerformanceIssue: false
    }
    
    // Check for performance issues
    selfNode.hasPerformanceIssue = checkPerformanceIssues(selfNode)
    
    nodes.push(selfNode)
  }
  
  // Add other cluster nodes
  if (clusterInfo.value.nodes && Array.isArray(clusterInfo.value.nodes)) {
    clusterInfo.value.nodes.forEach(node => {
      if (node.id !== clusterInfo.value.self_id) {
        const processedNode = {
          id: node.id,
          address: node.address,
          isLeader: node.status === 'leader',
          isHealthy: node.is_healthy,
          lastSeen: new Date(node.last_seen * 1000), // Convert Unix timestamp (seconds) to milliseconds
          version: node.version || 'unknown',
          metrics: getNodeMetrics(node.id),
          hasStatusIssue: checkStatusConsistency(node),
          hasPerformanceIssue: false
        }
        
        // Check for performance issues
        processedNode.hasPerformanceIssue = checkPerformanceIssues(processedNode)
        
        nodes.push(processedNode)
      }
    })
  }
  
  // Sort nodes: leader first, then by address
  return nodes.sort((a, b) => {
    if (a.isLeader && !b.isLeader) return -1
    if (!a.isLeader && b.isLeader) return 1
    return a.address.localeCompare(b.address)
  })
})

// Methods
function getNodeMetrics(nodeId) {
  const defaultMetrics = {
    inputMessages: 0,
    outputMessages: 0,
    cpuPercent: 0,
    memoryUsedMB: 0,
    memoryPercent: 0,
    goroutineCount: 0
  }
  
  // Get real message data for this node
  if (nodeMessageData.value && nodeMessageData.value[nodeId]) {
    const nodeData = nodeMessageData.value[nodeId]
    // Handle both uppercase and lowercase formats from backend
    defaultMetrics.inputMessages = nodeData.input_messages || nodeData.INPUT_messages || 0
    defaultMetrics.outputMessages = nodeData.output_messages || nodeData.OUTPUT_messages || 0
  }
  
  // Get system metrics from cluster system metrics API
  // Only show system metrics if we have data for this specific node
  const nodeSystemMetrics = systemMetrics.value[nodeId]
  if (nodeSystemMetrics) {
    defaultMetrics.cpuPercent = nodeSystemMetrics.cpu_percent || 0
    defaultMetrics.memoryUsedMB = nodeSystemMetrics.memory_used_mb || 0
    defaultMetrics.memoryPercent = nodeSystemMetrics.memory_percent || 0
    defaultMetrics.goroutineCount = nodeSystemMetrics.goroutine_count || 0
  }
  // If we don't have system metrics for this node, keep default values (0)
  // This happens when accessing from follower nodes for other nodes
  
  return defaultMetrics
}

function checkStatusConsistency(node) {
  // Check if node's leader status is consistent with cluster state
  const expectedLeaderStatus = node.id === clusterInfo.value.leader_id
  return node.status === 'leader' !== expectedLeaderStatus
}

function checkPerformanceIssues(node) {
  const metrics = node.metrics
  // Define performance thresholds
  const CPU_WARNING_THRESHOLD = 80
  const MEMORY_WARNING_THRESHOLD = 85
  
  return metrics.cpuPercent > CPU_WARNING_THRESHOLD || 
         metrics.memoryPercent > MEMORY_WARNING_THRESHOLD
}

function getIssueDescription(node) {
  const issues = []
  
  if (node.hasStatusIssue) {
    issues.push('Node status inconsistent with cluster leader')
  }
  
  if (node.hasPerformanceIssue) {
    if (node.metrics.cpuPercent > 80) {
      issues.push(`High CPU usage: ${node.metrics.cpuPercent.toFixed(1)}%`)
    }
    if (node.metrics.memoryPercent > 85) {
      issues.push(`High memory usage: ${node.metrics.memoryPercent.toFixed(1)}%`)
    }
  }
  
  return issues.join(', ')
}

// Version-related helper functions
function formatVersion(version) {
  if (!version || version === 'unknown') {
    return 'N/A'
  }
  
  // Return full version string
  return version
}

function getVersionDisplayClass(node) {
  if (!node.version || node.version === 'unknown') {
    return 'bg-gray-100 text-gray-600'
  }
  
  // Get leader version for comparison
  const leaderVersion = getLeaderVersion()
  if (!leaderVersion) {
    return 'bg-gray-100 text-gray-700'
  }
  
  // If this is the leader node or versions match, show normal style
  if (node.isLeader || node.version === leaderVersion) {
    return 'bg-green-100 text-green-800'
  }
  
  // Version mismatch - show red background
  return 'bg-red-100 text-red-800'
}

function getVersionTooltip(node) {
  if (!node.version || node.version === 'unknown') {
    return 'Version information not available'
  }
  
  const leaderVersion = getLeaderVersion()
  if (node.isLeader) {
    return `Leader version: ${node.version}`
  }
  
  if (!leaderVersion) {
    return `Node version: ${node.version}`
  }
  
  if (node.version === leaderVersion) {
    return `Version: ${node.version} (up to date)`
  }
  
  return `Version: ${node.version}\nLeader version: ${leaderVersion}\n⚠️ Configuration out of sync`
}

function getLeaderVersion() {
  // Find leader node and return its version
  const leaderNode = processedNodes.value.find(node => node.isLeader)
  return leaderNode?.version || clusterInfo.value.version
}

async function fetchAllData() {
  try {
    loading.value = true
    error.value = null
    
    // Fetch cluster info using dataCache
    const cluster = await dataCache.fetchClusterInfo(true) // Force refresh for real-time updates
    clusterInfo.value = cluster
    
    // Fetch node-level message data (only available from leader)
    try {
      const nodeMessagesResponse = await hubApi.getAllNodeDailyMessages()
      // Response shape: { data: { nodeId: {...} }, ... }
      nodeMessageData.value = (nodeMessagesResponse?.data) || {}
    } catch (messageError) {
      // console.warn('Failed to fetch node message data:', messageError)
      // Node message data is only available from leader node
      // console.info('Node message data is only available from leader node')
    }
    
    // Initialize system metrics object
    systemMetrics.value = {}
    
    // Fetch system metrics for all nodes (leader returns full data, follower may get 400)
    try {
      const systemResponse = await dataCache.fetchSystemMetrics(true) // Force refresh
      if (systemResponse && systemResponse.metrics) {
        // Leader path: aggregated metrics for all nodes
        systemMetrics.value = systemResponse.metrics
      }
    } catch (systemError) {
      // console.warn('Failed to fetch cluster system metrics:', systemError)
      // This is expected for follower nodes - they can't access cluster system metrics
    }
    
    // Always fetch current node's system metrics as fallback
    try {
      const metrics = await hubApi.getCurrentSystemMetrics()
      // Extract current metrics from API response
      if (metrics && metrics.current && cluster.self_id) {
        systemMetrics.value[cluster.self_id] = metrics.current
      }
    } catch (metricsError) {
      // console.warn(`Failed to fetch system metrics for current node:`, metricsError)
    }
    
  } catch (err) {
    console.error('Error fetching cluster data:', err)
    error.value = 'Failed to load cluster information'
  } finally {
    loading.value = false
  }
}

// Use smart refresh system instead of fixed intervals
// Lifecycle
onMounted(() => {
  fetchAllData()
})

// Smart refresh will handle automatic updates
</script>

<style scoped>
/* Add any custom styles here if needed */
.animate-pulse {
  animation: pulse 2s cubic-bezier(0.4, 0, 0.6, 1) infinite;
}

@keyframes pulse {
  0%, 100% {
    opacity: 1;
  }
  50% {
    opacity: .5;
  }
}
</style> 