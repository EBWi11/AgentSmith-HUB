<template>
  <div class="relative flex items-center justify-between py-1 hover:bg-gray-100 rounded-md cursor-pointer group"
       :class="{ 'bg-blue-50': isSelected }"
       @click="$emit('select', item)">
    <!-- Tree lines -->
    <div class="absolute left-5 top-1/2 bottom-0 w-px bg-gray-300" v-if="!isLast"></div>
    <div class="absolute left-5 top-1/2 w-2 h-px bg-gray-300"></div>
    <div class="absolute left-5 top-0 h-1/2 w-px bg-gray-300"></div>
    
    <div class="flex items-center min-w-0 flex-1 pl-8 pr-3">
      <span class="text-sm truncate">{{ item.id || item.name }}</span>
      
      <!-- Status badges -->
      <div class="flex items-center ml-2 space-x-1">
        <!-- Plugin type badge -->
        <StatusBadge 
          v-if="type === 'plugins' && item.type === 'local'"
          text="L"
          type="local"
          tooltip="Built-in Plugin"
        />
        
        <!-- Temporary file badge -->
        <StatusBadge 
          v-if="item.hasTemp"
          text="T"
          type="temp"
          tooltip="Temporary Version"
        />
        
        <!-- Project status badge -->
        <StatusBadge 
          v-if="type === 'projects' && item.status"
          :text="getStatusLabel(item.status)"
          :type="item.status"
          :tooltip="getStatusTitle(item)"
          :error="item.error || ''"
        />
      </div>
    </div>
    
    <!-- Actions menu -->
    <div class="relative mr-3">
      <button 
        class="p-1 rounded-full text-gray-400 hover:text-gray-600 hover:bg-gray-200 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity w-6 h-6 flex items-center justify-center"
        @click.stop="toggleMenu"
      >
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 5v.01M12 12v.01M12 19v.01M12 6a1 1 0 110-2 1 1 0 010 2zm0 7a1 1 0 110-2 1 1 0 010 2zm0 7a1 1 0 110-2 1 1 0 010 2z"/>
        </svg>
      </button>
      
      <!-- Dropdown menu -->
      <ItemMenu 
        v-if="showMenu"
        :item="item"
        :type="type"
        @action="handleMenuAction"
        @close="showMenu = false"
      />
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import StatusBadge from './StatusBadge.vue'
import ItemMenu from './ItemMenu.vue'

const props = defineProps({
  item: {
    type: Object,
    required: true
  },
  type: {
    type: String,
    required: true
  },
  index: {
    type: Number,
    required: true
  },
  isLast: {
    type: Boolean,
    default: false
  },
  isSelected: {
    type: Boolean,
    default: false
  }
})

const emit = defineEmits(['select', 'action'])

const showMenu = ref(false)

function toggleMenu() {
  showMenu.value = !showMenu.value
}

function handleMenuAction(actionType, payload = null) {
  showMenu.value = false
  emit('action', actionType, props.item, payload)
}

function getStatusLabel(status) {
  const labels = {
    running: 'R',
    stopped: 'S',
    starting: '◐',  // 使用半圆符号表示启动中
    stopping: '●',  // 使用圆点符号表示正在停止中
    error: 'E'
  }
  return labels[status] || '?'
}

function getStatusTitle(item) {
  const titles = {
    running: `Project is running (${item.id})`,
    stopped: `Project is stopped (${item.id})`,
    starting: `Project is starting (${item.id})`,
    stopping: `Project is stopping (${item.id})`,
    error: item.error ? `Project has errors: ${item.error}` : `Project has errors (${item.id})`
  }
  
  return titles[item.status] || `Unknown status (${item.id})`
}
</script>