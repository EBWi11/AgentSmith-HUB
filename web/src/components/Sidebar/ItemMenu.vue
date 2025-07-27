<template>
  <div class="absolute right-0 mt-1 w-48 bg-white rounded-md shadow-lg z-10 border border-gray-200"
       @click.stop>
    <div class="py-1">
      <!-- Edit action -->
      <MenuItem 
        v-if="showEdit"
        icon="edit"
        text="Edit"
        @click="$emit('action', 'edit')"
      />
      
      <!-- Project specific actions -->
      <template v-if="type === 'projects'">
        <!-- Start action -->
        <MenuItem 
          v-if="(item.status === 'stopped' || item.status === 'error') && !item.hasTemp"
          icon="start"
          text="Start"
          @click="$emit('action', 'start-project')"
        />
        
        <!-- Stop action -->
        <MenuItem 
          v-if="item.status === 'running' && !item.hasTemp"
          icon="stop"
          text="Stop"
          @click="$emit('action', 'stop-project')"
        />
        
        <!-- Restart action for running projects -->
        <MenuItem 
          v-if="item.status === 'running' && !item.hasTemp"
          icon="restart"
          text="Restart"
          @click="$emit('action', 'restart-project')"
        />
        
        <!-- Starting status display in menu -->
        <div v-if="item.status === 'starting'" class="flex items-center px-4 py-2 text-sm text-blue-600">
          <div class="w-3 h-3 rounded-full bg-current animate-pulse mr-2"></div>
          Starting...
        </div>
        
        <!-- Stopping status display in menu -->
        <div v-if="item.status === 'stopping'" class="flex items-center px-4 py-2 text-sm text-orange-600">
          <div class="w-3 h-3 rounded-full bg-current animate-pulse mr-2"></div>
          Stopping...
        </div>

      </template>
      
      <!-- Connect Check -->
      <MenuItem 
        v-if="showConnectCheck"
        icon="connect"
        text="Connect Check"
        @click="$emit('action', 'connect-check')"
      />
      
      <!-- View Sample Data -->
      <MenuItem 
        v-if="showSampleData"
        icon="view"
        text="View Sample Data"
        @click="$emit('action', 'view-sample-data')"
      />
      
      <!-- View Usage -->
      <MenuItem 
        v-if="showUsage"
        icon="usage"
        text="View Usage"
        @click="$emit('action', 'view-usage')"
      />
      
      <!-- Test actions -->
      <MenuItem 
        v-if="type === 'plugins'"
        icon="test"
        text="Test Plugin"
        @click="$emit('action', 'test-plugin')"
      />
      
      <!-- Plugin Stats -->
      <MenuItem 
        v-if="type === 'plugins'"
        icon="stats"
        text="View Stats"
        @click="$emit('action', 'view-plugin-stats')"
      />
      
      <MenuItem 
        v-if="type === 'rulesets'"
        icon="test"
        text="Test Ruleset"
        @click="$emit('action', 'test-ruleset', { type: 'rulesets', id: item.id || item.name })"
      />
      
      <MenuItem 
        v-if="type === 'outputs'"
        icon="test"
        text="Test Output"
        @click="$emit('action', 'test-output', { type: 'outputs', id: item.id || item.name })"
      />
      
      <!-- Test Project -->
      <MenuItem 
        v-if="type === 'projects'"
        icon="test"
        text="Test Project"
        @click="$emit('action', 'test-project', { type: 'projects', id: item.id || item.name })"
      />
      
      <!-- Cluster Status for projects -->
      <MenuItem 
        v-if="type === 'projects'"
        icon="cluster"
        text="Cluster Status"
        @click="$emit('action', 'cluster-status')"
      />
      
      <!-- Copy name action -->
      <MenuItem 
        icon="copy"
        text="Copy Name"
        @click="$emit('action', 'copy')"
      />
      
      <!-- Separator and Delete -->
      <div v-if="showDelete" class="border-t border-gray-100 my-1"></div>
      <MenuItem 
        v-if="showDelete"
        icon="delete"
        text="Delete"
        variant="danger"
        @click="$emit('action', 'delete')"
      />
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import MenuItem from './MenuItem.vue'

const props = defineProps({
  item: {
    type: Object,
    required: true
  },
  type: {
    type: String,
    required: true
  }
})

defineEmits(['action', 'close'])

// Computed properties for menu visibility
const showEdit = computed(() => {
  return !(props.type === 'plugins' && props.item.type === 'local')
})

const showDelete = computed(() => {
  return !(props.type === 'plugins' && props.item.type === 'local')
})

const showConnectCheck = computed(() => {
  return (props.type === 'inputs' || props.type === 'outputs') && 
         !(props.type === 'plugins' && props.item.type === 'local')
})

const showSampleData = computed(() => {
  return ['inputs', 'outputs', 'rulesets'].includes(props.type)
})

const showUsage = computed(() => {
  return ['inputs', 'outputs', 'rulesets'].includes(props.type)
})
</script>

<style scoped>
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