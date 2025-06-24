<template>
  <div class="bg-gray-50 min-h-full max-h-screen overflow-y-auto">
    <!-- Header -->
    <div class="px-6 pt-6 pb-2">
      <h1 class="text-3xl font-bold text-gray-900">AgentSmith Hub Dashboard</h1>
      <p class="text-gray-600 mt-2">Real-time overview of your hub cluster and projects</p>
      <p class="text-sm text-blue-600 mt-1">📊 All message statistics show aggregated data from all cluster nodes</p>
    </div>

    <!-- Main Content with consistent padding -->
    <div class="px-6 pb-6 space-y-4">

    <!-- Quick Stats Cards -->
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
      <!-- Projects Card -->
      <div class="bg-white rounded-lg shadow-sm p-6">
        <div class="flex items-center">
          <div class="flex-shrink-0">
            <div class="w-8 h-8 bg-blue-100 rounded-lg flex items-center justify-center">
              <svg class="w-5 h-5 text-blue-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z" />
              </svg>
            </div>
          </div>
          <div class="ml-4">
            <p class="text-sm font-medium text-gray-500">Projects</p>
            <div class="flex items-baseline">
              <p class="text-2xl font-semibold text-gray-900 transition-all duration-300" :class="{ 'opacity-75': loading.stats }">{{ projectStats.total }}</p>
              <p class="ml-2 text-sm text-green-600 transition-all duration-300" :class="{ 'opacity-75': loading.stats }" v-if="projectStats.running > 0">
                {{ projectStats.running }} running
              </p>
            </div>
          </div>
        </div>
      </div>

      <!-- Cluster Nodes Card -->
      <div class="bg-white rounded-lg shadow-sm p-6">
        <div class="flex items-center">
          <div class="flex-shrink-0">
            <div class="w-8 h-8 bg-green-100 rounded-lg flex items-center justify-center">
              <svg class="w-5 h-5 text-green-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 12a7 7 0 1114 0 7 7 0 01-14 0zM12 8v4l3 3" />
              </svg>
            </div>
          </div>
          <div class="ml-4">
            <p class="text-sm font-medium text-gray-500">Cluster Nodes</p>
            <div class="flex items-baseline">
              <p class="text-2xl font-semibold text-gray-900 transition-all duration-300" :class="{ 'opacity-75': loading.stats }">{{ clusterStats.total }}</p>
              <p class="ml-2 text-sm text-green-600 transition-all duration-300" :class="{ 'opacity-75': loading.stats }" v-if="clusterStats.active > 0">
                {{ clusterStats.active }} active
              </p>
            </div>
          </div>
        </div>
      </div>

      <!-- Avg CPU Card -->
      <div class="bg-white rounded-lg shadow-sm p-6">
        <div class="flex items-center">
          <div class="flex-shrink-0">
            <div class="w-8 h-8 bg-purple-100 rounded-lg flex items-center justify-center">
              <svg class="w-5 h-5 text-purple-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
              </svg>
            </div>
          </div>
          <div class="ml-4">
            <p class="text-sm font-medium text-gray-500">Avg CPU</p>
            <div class="flex items-baseline">
              <p class="text-2xl font-semibold text-gray-900 transition-all duration-300" :class="{ 'opacity-75': loading.stats }">{{ formatPercent(systemStats.avgCPU) }}</p>
              <p class="ml-2 text-sm text-gray-500">%</p>
            </div>
          </div>
        </div>
      </div>

      <!-- Avg Memory Card -->
      <div class="bg-white rounded-lg shadow-sm p-6">
        <div class="flex items-center">
          <div class="flex-shrink-0">
            <div class="w-8 h-8 bg-orange-100 rounded-lg flex items-center justify-center">
              <svg class="w-5 h-5 text-orange-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 7v10c0 2.21 3.582 4 8 4s8-1.79 8-4V7M4 7c0 2.21 3.582 4 8 4s8-1.79 8-4M4 7c0-2.21 3.582-4 8-4s8 1.79 8 4" />
              </svg>
            </div>
          </div>
          <div class="ml-4">
            <p class="text-sm font-medium text-gray-500">Avg Memory</p>
            <div class="flex items-baseline">
              <p class="text-2xl font-semibold text-gray-900 transition-all duration-300" :class="{ 'opacity-75': loading.stats }">{{ formatPercent(systemStats.avgMemory) }}</p>
              <p class="ml-2 text-sm text-gray-500">%</p>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Second Row: Hub Total Statistics and Development Status -->
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-4 items-start">
      <!-- Hub Total Statistics -->
      <div class="bg-white rounded-lg shadow-sm p-4 flex flex-col">
        <h3 class="text-lg font-medium text-gray-900 mb-3 flex-shrink-0">Hub Total Message Statistics <span class="text-sm text-gray-500 font-normal">(All Nodes)</span></h3>
        <div v-if="loading.messages && Object.keys(messageData).length === 0" class="flex justify-center items-center py-4">
          <div class="animate-spin rounded-full h-6 w-6 border-primary"></div>
        </div>
        <div v-else class="flex flex-col space-y-3">
          <!-- Total Input -->
          <div class="text-center p-4 bg-blue-50 rounded-lg flex flex-col justify-center">
            <div class="text-xs text-blue-600 font-medium mb-1">Total Hub Input</div>
            <div class="text-2xl font-bold text-blue-800 mb-1 transition-all duration-300" :class="{ 'opacity-75': loading.stats }">
              {{ formatMessagesPerHour(hubTotalStats.input) }}
            </div>
            <div class="text-xs text-blue-600">messages/hour (all nodes)</div>
          </div>
          
          <!-- Total Output -->
          <div class="text-center p-4 bg-green-50 rounded-lg flex flex-col justify-center">
            <div class="text-xs text-green-600 font-medium mb-1">Total Hub Output</div>
            <div class="text-2xl font-bold text-green-800 mb-1 transition-all duration-300" :class="{ 'opacity-75': loading.stats }">
              {{ formatMessagesPerHour(hubTotalStats.output) }}
            </div>
            <div class="text-xs text-green-600">messages/hour (all nodes)</div>
          </div>
        </div>
      </div>

      <!-- Pending Changes & Local Changes -->
      <div class="bg-white rounded-lg shadow-sm p-4 flex flex-col">
        <h3 class="text-lg font-medium text-gray-900 mb-3 flex-shrink-0">Development Status</h3>
        <div v-if="loading.changes && pendingChanges.length === 0 && localChanges.length === 0" class="flex justify-center items-center py-4">
          <div class="animate-spin rounded-full h-6 w-6 border-primary"></div>
        </div>
        <div v-else class="flex flex-col space-y-3">
          <!-- Pending Changes -->
          <div class="text-center p-4 bg-orange-50 rounded-lg hover:bg-orange-100 cursor-pointer transition-colors flex flex-col justify-center"
               @click="navigateToPendingChanges">
            <div class="text-xs text-orange-600 font-medium mb-1">Components to Push</div>
            <div class="text-2xl font-bold text-orange-800 mb-1 transition-all duration-300" :class="{ 'opacity-75': loading.stats }">
              {{ pendingChangesStats.total }}
            </div>
            <div class="text-xs text-orange-600">changes ready to apply</div>
          </div>

          <!-- Local Changes -->
          <div class="text-center p-4 bg-purple-50 rounded-lg hover:bg-purple-100 cursor-pointer transition-colors flex flex-col justify-center"
               @click="navigateToLocalChanges">
            <div class="text-xs text-purple-600 font-medium mb-1">Components to Load</div>
            <div class="text-2xl font-bold text-purple-800 mb-1 transition-all duration-300" :class="{ 'opacity-75': loading.stats }">
              {{ localChangesStats.total }}
            </div>
            <div class="text-xs text-purple-600">local changes available</div>
          </div>
        </div>
      </div>
    </div>

    <!-- Third Row: Project Status Overview and Cluster Nodes -->
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-4 items-start">
      <!-- Project Status Chart -->
      <div class="bg-white rounded-lg shadow-sm p-6">
        <h3 class="text-lg font-medium text-gray-900 mb-4">Project Status Overview</h3>
        <div v-if="loading.projects && projectList.length === 0" class="flex justify-center items-center py-8">
          <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
        </div>
        <div v-else class="space-y-4">
          <div v-for="project in sortedProjects" :key="project.id" 
               class="flex items-center justify-between p-3 bg-gray-50 rounded-lg hover:bg-gray-100 cursor-pointer transition-colors"
               @click="navigateToProject(project.id)">
            <div class="flex items-center">
              <span class="w-3 h-3 rounded-full mr-3" 
                    :class="{
                      'bg-green-500': project.status === 'running',
                      'bg-gray-400': project.status === 'stopped',
                      'bg-red-500': project.status === 'error'
                    }"></span>
              <div>
                <p class="font-medium text-gray-900">{{ project.id }}</p>
                <p class="text-sm text-gray-500 capitalize">{{ project.status }}</p>
              </div>
            </div>
            <div class="text-right">
              <div class="flex items-center space-x-4">
                <!-- Input Messages -->
                <div class="text-center">
                  <p class="text-xs text-blue-600 font-medium">Input/h</p>
                  <p class="text-sm font-bold text-blue-800 transition-all duration-300" :class="{ 'opacity-75': loading.stats }">{{ formatMessagesPerHour(getProjectMessageStats(project.id).input) }}</p>
                </div>
                <!-- Output Messages -->
                <div class="text-center">
                  <p class="text-xs text-green-600 font-medium">Output/h</p>
                  <p class="text-sm font-bold text-green-800 transition-all duration-300" :class="{ 'opacity-75': loading.stats }">{{ formatMessagesPerHour(getProjectMessageStats(project.id).output) }}</p>
                </div>
                <!-- Components Count -->
                <div class="text-center">
                  <p class="text-xs text-gray-500">Components</p>
                  <p class="text-sm font-medium text-gray-900 transition-all duration-300" :class="{ 'opacity-75': loading.stats }">{{ project.components || 0 }}</p>
                </div>
              </div>
            </div>
          </div>
          <div v-if="projectList.length === 0" class="text-center text-gray-500 py-4">
            No projects available
          </div>
        </div>
      </div>

      <!-- Cluster Nodes Status -->
      <div class="bg-white rounded-lg shadow-sm p-6">
        <h3 class="text-lg font-medium text-gray-900 mb-4">Cluster Nodes</h3>
        
        <!-- Leader Node Section -->
        <div v-if="leaderNode" class="mb-6">
          <h4 class="text-sm font-semibold text-blue-700 mb-2 flex items-center">
            <svg class="w-4 h-4 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 3l14 0 0 14-14 0z" />
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="m9 12 2 2 4-4" />
            </svg>
            Leader Node
          </h4>
          <div class="p-3 bg-blue-50 rounded-lg border border-blue-200">
            <div class="flex items-center justify-between">
              <div class="flex items-center">
                <span class="w-3 h-3 rounded-full mr-3 bg-blue-500"></span>
                <div>
                  <p class="font-medium text-blue-900">{{ leaderNode.address }}</p>
                  <p class="text-sm text-blue-600">{{ leaderNode.role }} - {{ leaderNode.status }}</p>
                </div>
              </div>
              <div class="text-right">
                <p class="text-sm font-medium text-blue-900 transition-all duration-300" :class="{ 'opacity-75': loading.stats }">{{ formatPercent(leaderNode.cpu_usage || 0) }}% CPU</p>
                <p class="text-xs text-blue-600 transition-all duration-300" :class="{ 'opacity-75': loading.stats }">{{ formatPercent(leaderNode.memory_usage || 0) }}% Memory</p>
              </div>
            </div>
          </div>
        </div>

        <!-- Follower Nodes Section -->
        <div v-if="followerNodes.length > 0">
          <h4 class="text-sm font-semibold text-gray-700 mb-2 flex items-center">
            <svg class="w-4 h-4 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0zm6 3a2 2 0 11-4 0 2 2 0 014 0zM7 10a2 2 0 11-4 0 2 2 0 014 0z" />
            </svg>
            Follower Nodes ({{ followerNodes.length }})
          </h4>
          <div class="space-y-2">
            <div v-for="node in followerNodes" :key="node.node_id" class="flex items-center justify-between p-3 bg-gray-50 rounded-lg">
              <div class="flex items-center">
                <span class="w-3 h-3 rounded-full mr-3"
                      :class="{
                        'bg-green-500': node.status === 'active',
                        'bg-gray-400': node.status !== 'active'
                      }"></span>
                <div>
                  <p class="font-medium text-gray-900">{{ node.address }}</p>
                  <p class="text-sm text-gray-500">{{ node.role }} - {{ node.status }}</p>
                </div>
              </div>
              <div class="text-right">
                <p class="text-sm font-medium text-gray-900 transition-all duration-300" :class="{ 'opacity-75': loading.stats }">{{ formatPercent(node.cpu_usage || 0) }}% CPU</p>
                <p class="text-xs text-gray-500 transition-all duration-300" :class="{ 'opacity-75': loading.stats }">{{ formatPercent(node.memory_usage || 0) }}% Memory</p>
              </div>
            </div>
          </div>
        </div>

        <!-- No Nodes Available -->
        <div v-if="loading.cluster && clusterNodes.length === 0" class="flex justify-center items-center py-8">
          <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
        </div>
        <div v-else-if="clusterNodes.length === 0" class="text-center text-gray-500 py-4">
          No cluster nodes available
        </div>
      </div>
    </div>

    <!-- Last Updated -->
    <div class="text-center text-sm text-gray-500 flex items-center justify-center space-x-2">
      <span>Last updated: {{ lastUpdated }}</span>
      <div v-if="loading.stats" class="flex items-center">
        <div class="w-3 h-3 border border-gray-400 border-t-transparent rounded-full animate-spin"></div>
      </div>
    </div>

    </div>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, onUnmounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import { hubApi } from '../api'
import { formatNumber, formatPercent, formatMessagesPerHour, formatTimeAgo } from '../utils/common'

// Router
const router = useRouter()

// Reactive state
const loading = reactive({
  projects: false,
  cluster: false,
  messages: false,
  system: false,
  changes: false,
  stats: false // New loading state for stats refresh
})

const projectList = ref([])
const clusterInfo = ref({}) // Store the full cluster info response
const componentData = ref({})
const messageData = ref({})
const systemData = ref({})
const pendingChanges = ref([])
const localChanges = ref([])
const lastUpdated = ref('')
const refreshInterval = ref(null)
const statsRefreshInterval = ref(null) // New interval for frequent stats updates

// Process cluster nodes similar to ClusterStatus.vue
const clusterNodes = computed(() => {
  const nodes = []
  
  // Add current node (self)
  if (clusterInfo.value.self_id) {
    const selfNode = {
      id: clusterInfo.value.self_id,
      address: clusterInfo.value.self_address,
      role: clusterInfo.value.status === 'leader' ? 'leader' : 'follower',
      status: 'active',
      cpu_usage: getNodeSystemMetrics(clusterInfo.value.self_id).cpu_percent,
      memory_usage: getNodeSystemMetrics(clusterInfo.value.self_id).memory_percent,
      isLeader: clusterInfo.value.status === 'leader',
      isHealthy: true,
      lastSeen: new Date()
    }
    
    nodes.push(selfNode)
  }
  
  // Add other cluster nodes
  if (clusterInfo.value.nodes && Array.isArray(clusterInfo.value.nodes)) {
    clusterInfo.value.nodes.forEach(node => {
      if (node.id !== clusterInfo.value.self_id) {
        const processedNode = {
          id: node.id,
          address: node.address,
          role: node.status === 'leader' ? 'leader' : 'follower',
          status: node.is_healthy ? 'active' : 'inactive',
          cpu_usage: getNodeSystemMetrics(node.id).cpu_percent,
          memory_usage: getNodeSystemMetrics(node.id).memory_percent,
          isLeader: node.status === 'leader',
          isHealthy: node.is_healthy,
          lastSeen: new Date(node.last_seen)
        }
        
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

// Helper function to get system metrics for a node
function getNodeSystemMetrics(nodeId) {
  const defaultMetrics = {
    cpu_percent: 0,
    memory_percent: 0,
    memory_used_mb: 0,
    goroutine_count: 0
  }
  
  // Debug logging
  console.log('Getting system metrics for node:', nodeId)
  console.log('Available systemData:', systemData.value)
  
  // Get system metrics from cluster system metrics API
  if (systemData.value && systemData.value[nodeId]) {
    const nodeSystemMetrics = systemData.value[nodeId]
    console.log('Found system metrics for node:', nodeId, nodeSystemMetrics)
    return {
      cpu_percent: nodeSystemMetrics.cpu_percent || 0,
      memory_percent: nodeSystemMetrics.memory_percent || 0,
      memory_used_mb: nodeSystemMetrics.memory_used_mb || 0,
      goroutine_count: nodeSystemMetrics.goroutine_count || 0
    }
  }
  
  console.log('No system metrics found for node:', nodeId, 'using defaults')
  return defaultMetrics
}

// Computed stats
const projectStats = computed(() => {
  const total = projectList.value.length
  const running = projectList.value.filter(p => p.status === 'running').length
  const stopped = projectList.value.filter(p => p.status === 'stopped').length
  const error = projectList.value.filter(p => p.status === 'error').length
  return { total, running, stopped, error }
})

const clusterStats = computed(() => {
  const total = clusterNodes.value.length
  const active = clusterNodes.value.filter(n => n.status === 'active').length
  return { total, active }
})

// Leader and follower nodes
const leaderNode = computed(() => {
  return clusterNodes.value.find(node => node.role === 'leader') || null
})

const followerNodes = computed(() => {
  return clusterNodes.value.filter(node => node.role === 'follower')
})

// Sorted projects list (running first, then others)
const sortedProjects = computed(() => {
  return [...projectList.value].sort((a, b) => {
    // Running projects first
    if (a.status === 'running' && b.status !== 'running') return -1
    if (a.status !== 'running' && b.status === 'running') return 1
    
    // Then by status: running, stopped, error
    const statusOrder = { 'running': 0, 'stopped': 1, 'error': 2 }
    const statusDiff = (statusOrder[a.status] || 3) - (statusOrder[b.status] || 3)
    if (statusDiff !== 0) return statusDiff
    
    // Finally by project id alphabetically
    return a.id.localeCompare(b.id)
  })
})

// Hub total statistics (all projects, not just running) - uses aggregated cluster data
const hubTotalStats = computed(() => {
  let input = 0
  let output = 0
  
  // Use aggregated message data for total hub statistics
  // Data format: { project_breakdown: { projectID: { input: count, output: count, ruleset: count } } }
  if (messageData.value.project_breakdown) {
    Object.values(messageData.value.project_breakdown).forEach(projectData => {
      input += projectData.input || 0
      output += projectData.output || 0
    })
  }
  
  return {
    input,
    output,
    total: input + output
  }
})

// Get message statistics for a specific project from aggregated cluster data
function getProjectMessageStats(projectId) {
  // First try to get data from aggregated project breakdown (this is the most reliable)
  if (messageData.value.project_breakdown && messageData.value.project_breakdown[projectId]) {
    const projectData = messageData.value.project_breakdown[projectId]
    return {
      input: projectData.input || 0,
      output: projectData.output || 0,
      ruleset: projectData.ruleset || 0 // Now include ruleset processing statistics
    }
  }
  
  // UPDATED: Fallback logic now handles new ProjectNodeSequence format
  // This might happen if we're using project-specific message API endpoints
  let input = 0
  let output = 0
  let ruleset = 0
  
  // Check if messageData.value contains ProjectNodeSequence keys directly
  if (messageData.value && typeof messageData.value === 'object') {
    for (const [key, componentData] of Object.entries(messageData.value)) {
      if (componentData && typeof componentData === 'object' && componentData.component_type) {
        const totalMessages = componentData.total_messages || componentData.current_total || 0
        
        // Apply updated matching logic for new ProjectNodeSequence format
        const keyParts = key.split('.')
        
        if (componentData.component_type === 'input') {
          // Only count input if ProjectNodeSequence starts with "INPUT.componentId"
          // This matches patterns like "INPUT.api_sec" (not "INPUT.api_sec.RULESET.test")
          if (keyParts.length === 2 && keyParts[0].toUpperCase() === 'INPUT') {
            input += totalMessages
          }
        } else if (componentData.component_type === 'output') {
          // Only count output if ProjectNodeSequence ends with "OUTPUT.componentId"
          // This matches patterns like "INPUT.api_sec.RULESET.test.OUTPUT.print_demo"
          if (keyParts.length >= 2 && 
              keyParts[keyParts.length - 2].toUpperCase() === 'OUTPUT') {
            output += totalMessages
          }
        } else if (componentData.component_type === 'ruleset') {
          // Count ruleset processing load - matches patterns like "INPUT.api_sec.RULESET.test"
          // but ONLY count the RULESET's own processing, not downstream components
          for (let i = 0; i < keyParts.length - 1; i++) {
            if (keyParts[i].toUpperCase() === 'RULESET') {
              // Only count if this is the RULESET's own ProjectNodeSequence
              // Avoid counting downstream components like "INPUT.api_sec.RULESET.test.OUTPUT.print_demo"
              
              // Check if there are more components after this RULESET in the sequence
              const hasDownstream = (i + 2) < keyParts.length;
              
              if (!hasDownstream) {
                // This is the RULESET's own ProjectNodeSequence (ends with RULESET.componentId)
                ruleset += totalMessages;
              }
              // If hasDownstream is true, this means it's a downstream component's sequence
              // that happens to contain this RULESET in its path - we don't count it
              
              break;
            }
          }
        }
      }
    }
  }
  
  return { input, output, ruleset }
}

const systemStats = computed(() => {
  // Use aggregated system metrics directly from API
  if (!systemData.value || Object.keys(systemData.value).length === 0) {
    return { avgCPU: 0, avgMemory: 0, totalGoroutines: 0 }
  }

  return {
    avgCPU: systemData.value.avg_cpu_percent || 0,
    avgMemory: systemData.value.avg_memory_percent || 0,
    totalGoroutines: systemData.value.total_goroutines || 0
  }
})

// Pending changes statistics
const pendingChangesStats = computed(() => {
  const stats = {
    total: 0,
    projects: 0,
    inputs: 0,
    outputs: 0,
    rulesets: 0,
    plugins: 0
  }

  pendingChanges.value.forEach(change => {
    stats.total++
    switch (change.type) {
      case 'project':
        stats.projects++
        break
      case 'input':
        stats.inputs++
        break
      case 'output':
        stats.outputs++
        break
      case 'ruleset':
        stats.rulesets++
        break
      case 'plugin':
        stats.plugins++
        break
    }
  })

  return stats
})

// Local changes statistics
const localChangesStats = computed(() => {
  const stats = {
    total: 0,
    projects: 0,
    inputs: 0,
    outputs: 0,
    rulesets: 0,
    plugins: 0
  }

  localChanges.value.forEach(change => {
    stats.total++
    switch (change.type) {
      case 'project':
        stats.projects++
        break
      case 'input':
        stats.inputs++
        break
      case 'output':
        stats.outputs++
        break
      case 'ruleset':
        stats.rulesets++
        break
      case 'plugin':
        stats.plugins++
        break
    }
  })

  return stats
})

// Methods - 格式化函数现在从 utils/common.js 导入

function navigateToProject(projectId) {
  router.push(`/app/projects/${projectId}`)
}

function navigateToPendingChanges() {
  router.push('/app/pending-changes')
}

function navigateToLocalChanges() {
  router.push('/app/load-local-components')
}

// Fast refresh for stats and numbers only
async function refreshStats() {
  try {
    loading.stats = true
    
    // Fetch frequently changing data and component data for accurate counting
    const [messageResponse, systemResponse, componentResponse] = await Promise.all([
      hubApi.getAggregatedHourlyMessages(),
      hubApi.getAggregatedSystemMetrics(),
      hubApi.getQPSData() // Also refresh component data to ensure accurate counting
    ])

    messageData.value = messageResponse.data || {}
    systemData.value = systemResponse || {}
    componentData.value = componentResponse.data || {}

    // Fetch cluster system metrics for node display (if current node is leader)
    if (clusterInfo.value.status === 'leader') {
      try {
        const clusterSystemResponse = await hubApi.getClusterSystemMetrics()
        if (clusterSystemResponse && clusterSystemResponse.metrics) {
          // Merge cluster system metrics into systemData for node display
          Object.assign(systemData.value, clusterSystemResponse.metrics)
        }
      } catch (clusterSystemError) {
        console.warn('Failed to fetch cluster system metrics:', clusterSystemError)
      }
    }
    
    // Always fetch current node's system metrics as fallback (like ClusterStatus.vue)
    try {
      const currentMetrics = await hubApi.getCurrentSystemMetrics()
      // Extract current metrics from API response
      if (currentMetrics && currentMetrics.current && clusterInfo.value.self_id) {
        systemData.value[clusterInfo.value.self_id] = currentMetrics.current
      }
    } catch (metricsError) {
      console.warn(`Failed to fetch system metrics for current node:`, metricsError)
      if (clusterInfo.value.self_id) {
        systemData.value[clusterInfo.value.self_id] = {
          cpu_percent: 0,
          memory_used_mb: 0,
          memory_percent: 0,
          goroutine_count: 0
        }
      }
    }

    // Update component counts for all projects (including stopped ones)
    // Use project configuration data instead of QPS data to get accurate component counts
    const componentCountPromises = projectList.value.map(async (project) => {
      try {
        const componentInfo = await hubApi.getProjectComponents(project.id)
        if (componentInfo.success) {
          project.components = componentInfo.totalComponents || 0
        } else {
          console.warn(`Failed to get components for project ${project.id}:`, componentInfo.error)
          project.components = 0
        }
      } catch (error) {
        console.error(`Error fetching components for project ${project.id}:`, error)
        project.components = 0
      }
    })
    
    // Wait for all component count updates to complete
    await Promise.all(componentCountPromises)

    // Update last updated time
    lastUpdated.value = new Date().toLocaleTimeString()

  } catch (error) {
    console.error('Failed to refresh stats:', error)
  } finally {
    loading.stats = false
  }
}

// Comprehensive refresh for all data (used on initial load and less frequently)
async function fetchDashboardData() {
  try {
    // Fetch projects and cluster data (structural data that changes less frequently)
    loading.projects = true
    loading.cluster = true
    loading.changes = true
    
    const [projectsResponse, clusterResponse] = await Promise.all([
      hubApi.fetchComponentsWithTempInfo('projects'),
      hubApi.fetchClusterStatus()
    ])

    projectList.value = projectsResponse.map(project => ({
      ...project,
      messages: 0, // Will be calculated from message data
      components: 0 // Will be calculated from project details
    }))

    clusterInfo.value = clusterResponse // Store full cluster info

    // Fetch component data for component count calculation
    loading.messages = true
    const componentResponse = await hubApi.getQPSData()
    componentData.value = componentResponse.data || {}

    // Fetch cluster system metrics for initial load (if current node is leader)
    if (clusterResponse.status === 'leader') {
      try {
        const clusterSystemResponse = await hubApi.getClusterSystemMetrics()
        if (clusterSystemResponse && clusterSystemResponse.metrics) {
          // Initialize systemData with cluster system metrics
          systemData.value = { ...systemData.value, ...clusterSystemResponse.metrics }
        }
      } catch (clusterSystemError) {
        console.warn('Failed to fetch cluster system metrics on initial load:', clusterSystemError)
      }
    }
    
    // Always fetch current node's system metrics as fallback (like ClusterStatus.vue)
    try {
      const currentMetrics = await hubApi.getCurrentSystemMetrics()
      // Extract current metrics from API response
      if (currentMetrics && currentMetrics.current && clusterResponse.self_id) {
        systemData.value[clusterResponse.self_id] = currentMetrics.current
      }
    } catch (metricsError) {
      console.warn(`Failed to fetch system metrics for current node on initial load:`, metricsError)
      if (clusterResponse.self_id) {
        systemData.value[clusterResponse.self_id] = {
          cpu_percent: 0,
          memory_used_mb: 0,
          memory_percent: 0,
          goroutine_count: 0
        }
      }
    }

    // Now refresh stats (this will also update message and system data)
    await refreshStats()

    // Fetch pending changes and local changes
    try {
      const [pendingResponse, localResponse] = await Promise.all([
        hubApi.fetchEnhancedPendingChanges(),
        hubApi.fetchLocalChanges()
      ])
      
      pendingChanges.value = pendingResponse || []
      localChanges.value = localResponse || []
    } catch (error) {
      console.error('Failed to fetch changes:', error)
      pendingChanges.value = []
      localChanges.value = []
    }

  } catch (error) {
    console.error('Failed to fetch dashboard data:', error)
  } finally {
    loading.projects = false
    loading.cluster = false
    loading.messages = false
    loading.system = false
    loading.changes = false
  }
}

function startAutoRefresh() {
  // Fast refresh for stats every 10 seconds
  statsRefreshInterval.value = setInterval(() => {
    refreshStats()
  }, 10000)
  
  // Comprehensive refresh every 2 minutes for structural changes
  refreshInterval.value = setInterval(() => {
    fetchDashboardData()
  }, 120000)
}

function stopAutoRefresh() {
  if (refreshInterval.value) {
    clearInterval(refreshInterval.value)
    refreshInterval.value = null
  }
  
  if (statsRefreshInterval.value) {
    clearInterval(statsRefreshInterval.value)
    statsRefreshInterval.value = null
  }
}

// Keyboard shortcuts
function handleKeyDown(event) {
  // Press 'R' to refresh stats
  if (event.key === 'r' || event.key === 'R') {
    if (!loading.stats) {
      refreshStats()
    }
    event.preventDefault()
  }
  // Press 'Shift+R' to full refresh
  if ((event.key === 'r' || event.key === 'R') && event.shiftKey) {
    if (!loading.projects && !loading.cluster && !loading.messages && !loading.changes) {
      fetchDashboardData()
    }
    event.preventDefault()
  }
}

// Lifecycle
onMounted(() => {
  fetchDashboardData()
  startAutoRefresh()
  
  // Add keyboard event listener
  window.addEventListener('keydown', handleKeyDown)
})

onUnmounted(() => {
  stopAutoRefresh()
  
  // Remove keyboard event listener
  window.removeEventListener('keydown', handleKeyDown)
})
</script>

<style scoped>
/* 自定义样式 */
</style> 