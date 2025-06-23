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
        <!-- Main Node Info Row -->
        <div class="px-6 py-4">
          <div class="flex items-center justify-between">
            <!-- Left: Basic Info -->
            <div class="flex items-center space-x-4 min-w-0 flex-1">
              <!-- Role Badge -->
              <span 
                class="inline-flex items-center px-3 py-1 rounded-full text-sm font-medium flex-shrink-0"
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
              <div class="min-w-0 flex-1">
                <div class="text-lg font-semibold text-gray-900 truncate">{{ node.address }}</div>
                <div class="text-sm text-gray-500">ID: {{ node.id }}</div>
              </div>
              
              <!-- Status Indicators -->
              <div class="flex items-center space-x-2 flex-shrink-0">
                <!-- Health Status -->
                <div 
                  class="w-3 h-3 rounded-full"
                  :class="{
                    'bg-green-500': node.isHealthy,
                    'bg-red-500': !node.isHealthy
                  }"
                  :title="node.isHealthy ? 'Healthy' : 'Unhealthy'"
                ></div>
                
                <!-- Status Consistency -->
                <div 
                  v-if="node.hasStatusIssue"
                  class="w-3 h-3 rounded-full bg-red-500 animate-pulse"
                  title="Status inconsistent with leader"
                ></div>
                
                <!-- Performance Warning -->
                <div 
                  v-if="node.hasPerformanceIssue"
                  class="w-3 h-3 rounded-full bg-yellow-500 animate-pulse"
                  title="Performance issue detected"
                ></div>
              </div>
            </div>

            <!-- Center: Message Metrics -->
            <div class="flex items-center space-x-6 mx-8 flex-shrink-0">
              <!-- Input Messages -->
              <div class="text-center">
                <div class="text-xs text-blue-600 font-medium mb-1">Input/h</div>
                <div class="text-xl font-bold text-blue-800">
                  {{ formatMessagesPerHour(node.metrics.inputMessages) }}
                </div>
              </div>
              
              <!-- Output Messages -->
              <div class="text-center">
                <div class="text-xs text-green-600 font-medium mb-1">Output/h</div>
                <div class="text-xl font-bold text-green-800">
                  {{ formatMessagesPerHour(node.metrics.outputMessages) }}
                </div>
              </div>
            </div>

            <!-- Right: System Resources -->
            <div class="flex items-center space-x-8 flex-shrink-0">
              <!-- CPU Usage -->
              <div class="text-center min-w-0">
                <div class="text-xs text-gray-600 font-medium mb-1">CPU</div>
                <div class="flex items-center space-x-2">
                  <div class="w-16 bg-gray-200 rounded-full h-2">
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
              <div class="text-center min-w-0">
                <div class="text-xs text-gray-600 font-medium mb-1">Memory</div>
                <div class="flex items-center space-x-2">
                  <div class="w-16 bg-gray-200 rounded-full h-2">
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
              <div class="text-center">
                <div class="text-xs text-gray-600 font-medium mb-1">Goroutines</div>
                <div class="text-lg font-semibold text-gray-800">{{ node.metrics.goroutineCount }}</div>
              </div>
            </div>

            <!-- Far Right: Last Seen -->
            <div class="text-right flex-shrink-0 ml-6">
              <span class="text-xs text-gray-500">
                {{ formatLastSeen(node.lastSeen) }}
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
    <div v-if="!loading && !error && filteredNodes.length === 0" class="text-center py-12">
      <svg class="mx-auto h-12 w-12 text-gray-400 mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
      </svg>
      <h3 class="text-lg font-medium text-gray-900 mb-2">No nodes found</h3>
      <p class="text-gray-500">{{ searchQuery ? 'No nodes match your search query.' : 'No cluster nodes available.' }}</p>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { hubApi } from '../api'

// Reactive state
const searchQuery = ref('')
const loading = ref(true)
const error = ref(null)
const clusterInfo = ref({})
const nodeMessageData = ref({})
const systemMetrics = ref({})
const refreshInterval = ref(null)

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
          lastSeen: new Date(node.last_seen),
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
    defaultMetrics.inputMessages = nodeData.input_messages || 0
    defaultMetrics.outputMessages = nodeData.output_messages || 0
  }
  
  // Get system metrics from cluster system metrics API
  const nodeSystemMetrics = systemMetrics.value[nodeId]
  if (nodeSystemMetrics) {
    defaultMetrics.cpuPercent = nodeSystemMetrics.cpu_percent || 0
    defaultMetrics.memoryUsedMB = nodeSystemMetrics.memory_used_mb || 0
    defaultMetrics.memoryPercent = nodeSystemMetrics.memory_percent || 0
    defaultMetrics.goroutineCount = nodeSystemMetrics.goroutine_count || 0
  }
  
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

function getCPUColor(cpuPercent) {
  if (cpuPercent > 80) return 'text-red-600'
  if (cpuPercent > 60) return 'text-yellow-600'
  return 'text-green-600'
}

function getCPUBarColor(cpuPercent) {
  if (cpuPercent > 80) return 'bg-red-500'
  if (cpuPercent > 60) return 'bg-yellow-500'
  return 'bg-green-500'
}

function getMemoryColor(memoryPercent) {
  if (memoryPercent > 85) return 'text-red-600'
  if (memoryPercent > 70) return 'text-yellow-600'
  return 'text-green-600'
}

function getMemoryBarColor(memoryPercent) {
  if (memoryPercent > 85) return 'bg-red-500'
  if (memoryPercent > 70) return 'bg-yellow-500'
  return 'bg-green-500'
}



function formatMessagesPerHour(messages) {
  // Format real message counts (no conversion needed)
  if (messages >= 1000000) {
    return (messages / 1000000).toFixed(1) + 'M'
  }
  if (messages >= 1000) {
    return (messages / 1000).toFixed(1) + 'K'
  }
  return messages.toString()
}

function formatLastSeen(lastSeen) {
  const now = new Date()
  const diff = now - lastSeen
  
  if (diff < 60000) { // Less than 1 minute
    return 'Just now'
  } else if (diff < 3600000) { // Less than 1 hour
    const minutes = Math.floor(diff / 60000)
    return `${minutes}m ago`
  } else if (diff < 86400000) { // Less than 1 day
    const hours = Math.floor(diff / 3600000)
    return `${hours}h ago`
  } else {
    const days = Math.floor(diff / 86400000)
    return `${days}d ago`
  }
}

async function fetchAllData() {
  try {
    loading.value = true
    error.value = null
    
    // Fetch cluster info
    const cluster = await hubApi.fetchClusterStatus()
    clusterInfo.value = cluster
    
    // Fetch node-level message data (only from leader)
    if (cluster.status === 'leader') {
      try {
        const nodeMessagesResponse = await hubApi.getAllNodeHourlyMessages()
        nodeMessageData.value = nodeMessagesResponse.data || {}
      } catch (messageError) {
        console.warn('Failed to fetch node message data:', messageError)
        nodeMessageData.value = {}
      }
    }
    
    // Fetch system metrics for all nodes
    if (cluster.status === 'leader') {
      // Leader can fetch cluster-wide system metrics
      try {
        const systemResponse = await hubApi.getClusterSystemMetrics()
        const clusterMetrics = systemResponse.metrics || {}
        
        // Store cluster metrics directly - API already returns the correct format
        systemMetrics.value = clusterMetrics
      } catch (systemError) {
        console.warn('Failed to fetch cluster system metrics:', systemError)
      }
    }
    
    // Always fetch current node's system metrics as fallback
    try {
      const metrics = await hubApi.getCurrentSystemMetrics()
      // Extract current metrics from API response
      if (metrics && metrics.current) {
        systemMetrics.value[cluster.self_id] = metrics.current
      }
    } catch (metricsError) {
      console.warn(`Failed to fetch system metrics for current node:`, metricsError)
      systemMetrics.value[cluster.self_id] = {
        cpu_percent: 0,
        memory_used_mb: 0,
        memory_percent: 0,
        goroutine_count: 0
      }
    }
    
  } catch (err) {
    console.error('Error fetching cluster data:', err)
    error.value = 'Failed to load cluster information'
  } finally {
    loading.value = false
  }
}

function startAutoRefresh() {
  // Refresh data every 15 seconds
  refreshInterval.value = setInterval(fetchAllData, 15000)
}

function stopAutoRefresh() {
  if (refreshInterval.value) {
    clearInterval(refreshInterval.value)
    refreshInterval.value = null
  }
}

// Lifecycle
onMounted(() => {
  fetchAllData()
  startAutoRefresh()
})

onUnmounted(() => {
  stopAutoRefresh()
})
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