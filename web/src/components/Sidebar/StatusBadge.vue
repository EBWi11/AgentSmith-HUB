<template>
  <span 
    class="text-xs w-5 h-5 flex items-center justify-center rounded-full cursor-help transition-colors"
    :class="badgeClass"
    :title="displayTooltip"
    @mouseenter="showTooltip"
    @mouseleave="hideTooltip"
  >
    {{ text }}
  </span>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  text: {
    type: String,
    required: true
  },
  type: {
    type: String,
    required: true,
    validator: (value) => ['local', 'temp', 'running', 'stopped', 'starting', 'stopping', 'error', 'detection', 'whitelist'].includes(value)
  },
  tooltip: {
    type: String,
    default: ''
  },
  error: {
    type: String,
    default: ''
  }
})

const badgeClass = computed(() => {
  const classes = {
    local: 'bg-gray-100 text-gray-800',
    temp: 'bg-blue-100 text-blue-800',
    running: 'bg-green-100 text-green-800',
    stopped: 'bg-gray-100 text-gray-800',
    starting: 'bg-blue-100 text-blue-800',
    stopping: 'bg-orange-100 text-orange-800',
    error: 'bg-red-100 text-red-800',
    detection: 'bg-purple-100 text-purple-800',
    whitelist: 'bg-emerald-100 text-emerald-800'
  }
  return classes[props.type] || 'bg-gray-100 text-gray-800'
})

const displayTooltip = computed(() => {
  if (props.type === 'error' && props.error) {
    return `Error: ${props.error}`
  }
  return props.tooltip
})

function showTooltip(event) {
  // Could implement custom tooltip here if needed
}

function hideTooltip() {
  // Could implement custom tooltip here if needed
}
</script> 