<template>
  <div class="cluster-heatmap">
    <!-- Header -->
    <div class="flex items-center justify-between mb-3">
      <h3 class="text-lg font-medium text-gray-900">
        Cluster Nodes 
        <span class="text-sm text-gray-500 font-normal ml-2">
          ({{ allNodes.length }})
        </span>
      </h3>
      <div class="flex items-center space-x-4 text-xs text-gray-500">
        <div class="flex items-center space-x-1">
          <div class="w-3 h-3 rounded-sm bg-gray-300"></div>
          <span>Unknown</span>
        </div>
        <div class="flex items-center space-x-1">
          <div class="w-3 h-3 rounded-sm bg-yellow-400"></div>
          <span>Mismatch</span>
        </div>
        <div class="flex items-center space-x-1">
          <div class="w-3 h-3 rounded-sm bg-green-400"></div>
          <span>Match</span>
        </div>
        <div class="flex items-center space-x-1">
          <div class="w-3 h-3 rounded-sm bg-blue-500 border border-blue-300"></div>
          <span>Leader</span>
        </div>
      </div>
    </div>

    <!-- GitHub-style Heatmap -->
    <div class="github-heatmap">
      <div 
        v-for="node in allNodes" 
        :key="node.id"
        class="contribution-square"
        :class="[
          getNodeClass(node),
          node.isLeader ? 'leader-square' : 'follower-square'
        ]"
        role="button"
        tabindex="0"
        :aria-label="`Copy IP ${getNodeIP(node)}`"
        @mouseenter="showNodeTooltip(node, $event)"
        @mousemove="moveNodeTooltip($event)"
        @mouseleave="hideNodeTooltip"
        @focus="showNodeTooltip(node, $event)"
        @blur="hideNodeTooltip"
        @click="copyIP(getNodeIP(node))"
        @keydown.enter.prevent="copyIP(getNodeIP(node))"
        @keydown.space.prevent="copyIP(getNodeIP(node))"
      ></div>
    </div>

    <!-- Immediate Tooltip -->
    <div
      v-if="hoveredNode"
      class="node-tooltip"
      :style="{
        left: `${tooltipPosition.x}px`,
        top: `${tooltipPosition.y}px`
      }"
    >
      <div v-for="line in getNodeTooltipLines(hoveredNode)" :key="line" class="tooltip-line">
        {{ line }}
      </div>
    </div>

    <!-- Empty State -->
    <div v-if="!leaderNode && followerNodes.length === 0" class="text-center py-8 text-gray-500">
      <svg class="w-12 h-12 mx-auto mb-4 text-gray-300" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0zm6 3a2 2 0 11-4 0 2 2 0 014 0zM7 10a2 2 0 11-4 0 2 2 0 014 0z" />
      </svg>
      <p>No cluster nodes available</p>
    </div>

    <!-- Toast Notification -->
    <div 
      v-if="showToast" 
      class="fixed bottom-4 right-4 text-white px-4 py-2 rounded-lg shadow-lg z-50 transition-all duration-300"
      :class="[
        toastIsError ? 'bg-red-500' : 'bg-green-500',
        { 'opacity-0': !showToast }
      ]"
    >
      <div class="flex items-center space-x-2">
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
        </svg>
        <span>{{ toastMessage }}</span>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'

const props = defineProps({
  nodes: {
    type: Array,
    default: () => []
  },
  leaderVersion: {
    type: String,
    default: ''
  }
})

const showToast = ref(false)
const toastMessage = ref('IP copied to clipboard!')
const toastIsError = ref(false)
const hoveredNode = ref(null)
const tooltipPosition = ref({ x: 0, y: 0 })

// Computed properties
const leaderNode = computed(() => {
  return props.nodes.find(node => node.isLeader)
})

const followerNodes = computed(() => {
  return props.nodes.filter(node => !node.isLeader)
})

const allNodes = computed(() => {
  // Sort: leader first, then followers
  return [...props.nodes].sort((a, b) => {
    if (a.isLeader && !b.isLeader) return -1
    if (!a.isLeader && b.isLeader) return 1
    return getNodeIP(a).localeCompare(getNodeIP(b))
  })
})

// Methods
function getVersionStatus(node) {
  // Leader node is always considered as 'match' since it defines the version
  if (node.isLeader) {
    return 'match'
  }
  
  // Check for unknown version
  if (!node.version || node.version === 'unknown' || node.version === 'follower') {
    return 'unknown'
  }
  
  if (!props.leaderVersion || props.leaderVersion === 'unknown') {
    return 'unknown'
  }
  
  // Compare with leader version
  if (node.version === props.leaderVersion) {
    return 'match'
  }
  
  return 'mismatch'
}

function getNodeClass(node) {
  const status = getVersionStatus(node)
  
  switch (status) {
    case 'match':
      return 'bg-green-400 hover:bg-green-500'
    case 'mismatch':
      return 'bg-yellow-400 hover:bg-yellow-500'
    case 'unknown':
    default:
      return 'bg-gray-300 hover:bg-gray-400'
  }
}

function getNodeIP(node) {
  return node.address || node.id || ''
}

function getNodeTooltipLines(node) {
  const ip = getNodeIP(node) || 'unknown'
  const version = node.version || 'unknown'
  const role = node.isLeader ? 'Leader' : 'Follower'
  const status = getVersionStatus(node)
  
  let statusText = ''
  switch (status) {
    case 'match':
      statusText = '✓ Version matches'
      break
    case 'mismatch':
      statusText = '⚠ Version mismatch'
      break
    case 'unknown':
      statusText = '? Version unknown'
      break
  }
  
  return [role, `IP: ${ip}`, `Version: ${version}`, statusText, 'Click to copy IP']
}

function setTooltipPosition(event) {
  if ('clientX' in event && 'clientY' in event) {
    tooltipPosition.value = {
      x: event.clientX + 12,
      y: event.clientY + 12
    }
    return
  }

  const rect = event.currentTarget?.getBoundingClientRect()
  if (rect) {
    tooltipPosition.value = {
      x: rect.left + rect.width + 8,
      y: rect.top + rect.height + 8
    }
  }
}

function showNodeTooltip(node, event) {
  hoveredNode.value = node
  setTooltipPosition(event)
}

function moveNodeTooltip(event) {
  setTooltipPosition(event)
}

function hideNodeTooltip() {
  hoveredNode.value = null
}

function showCopyToast(message, isError = false) {
  toastMessage.value = message
  toastIsError.value = isError
  showToast.value = true
  setTimeout(() => {
    showToast.value = false
    if (!isError) {
      toastMessage.value = 'IP copied to clipboard!'
    }
    toastIsError.value = false
  }, 2000)
}

function copyTextFallback(text) {
  const textarea = document.createElement('textarea')
  textarea.value = text
  textarea.setAttribute('readonly', '')
  textarea.style.position = 'fixed'
  textarea.style.left = '-9999px'
  textarea.style.top = '0'
  document.body.appendChild(textarea)
  textarea.select()

  try {
    return document.execCommand('copy')
  } finally {
    document.body.removeChild(textarea)
  }
}

async function copyIP(ip) {
  if (!ip) {
    showCopyToast('No IP available to copy', true)
    return
  }

  try {
    if (navigator.clipboard?.writeText && window.isSecureContext) {
      await navigator.clipboard.writeText(ip)
    } else if (!copyTextFallback(ip)) {
      throw new Error('Fallback copy command failed')
    }
    showCopyToast('IP copied to clipboard!')
  } catch (err) {
    try {
      if (copyTextFallback(ip)) {
        showCopyToast('IP copied to clipboard!')
        return
      }
    } catch (fallbackErr) {
      console.error('Failed to copy IP:', fallbackErr)
    }
    console.error('Failed to copy IP:', err)
    showCopyToast('Failed to copy IP', true)
  }
}
</script>

<style scoped>
.cluster-heatmap {
  @apply relative;
}

.github-heatmap {
  @apply flex flex-wrap gap-1;
  @apply justify-start;
  max-height: 200px;
  overflow-y: auto;
  padding: 8px 0;
}

.contribution-square {
  @apply transition-all duration-200 cursor-pointer;
  @apply hover:scale-125 hover:shadow-sm;
  @apply border border-gray-200;
  @apply rounded-sm;
}

.contribution-square:focus-visible {
  @apply outline-none ring-2 ring-blue-500 ring-offset-2;
}

.node-tooltip {
  @apply fixed z-50 px-3 py-2 rounded-md bg-gray-900 text-white text-xs shadow-lg;
  pointer-events: none;
  min-width: 180px;
  max-width: 280px;
  white-space: nowrap;
  transform: translateY(-4px);
}

.tooltip-line {
  line-height: 1.35;
}

.tooltip-line:first-child {
  @apply font-semibold;
}

.tooltip-line:last-child {
  @apply mt-1 text-gray-300;
}

.leader-square {
  @apply w-4 h-4;
  @apply bg-blue-500;
}

.follower-square {
  @apply w-4 h-4;
}

/* Custom scrollbar for followers grid */
.followers-heatmap::-webkit-scrollbar {
  width: 6px;
}

.followers-heatmap::-webkit-scrollbar-track {
  @apply bg-gray-100 rounded;
}

.followers-heatmap::-webkit-scrollbar-thumb {
  @apply bg-gray-300 rounded;
}

.followers-heatmap::-webkit-scrollbar-thumb:hover {
  @apply bg-gray-400;
}

/* Responsive adjustments */
@media (max-width: 640px) {
  .github-heatmap {
    gap: 0.5px;
  }
  
  .leader-square {
    @apply w-3 h-3;
  }
  
  .follower-square {
    @apply w-3 h-3;
  }
}

/* Animation for toast */
@keyframes slideIn {
  from {
    transform: translateX(100%);
    opacity: 0;
  }
  to {
    transform: translateX(0);
    opacity: 1;
  }
}

.fixed {
  animation: slideIn 0.3s ease-out;
}
</style>
