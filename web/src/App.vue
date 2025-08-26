<template>
  <router-view />
  <Toast ref="toast" />
</template>

<script setup>
import { ref, onMounted, provide } from 'vue'
import Toast from './components/Toast.vue'
import { useStore } from 'vuex'
import { useDataCacheStore } from './stores/dataCache'

const toast = ref(null)
const store = useStore()
const dataCache = useDataCacheStore()

// Provide global toast service
provide('$message', {
  success: (message) => toast.value?.show(message),
  error: (message) => toast.value?.show(message, 'error'),
  warning: (message) => toast.value?.show(message, 'warning'),
  info: (message) => toast.value?.show(message, 'info')
})

onMounted(() => {
  // Keep global variable for compatibility
  window.$toast = toast.value
  // Fetch available plugins using unified cache
  // dataCache.fetchAvailablePlugins()
})
</script>

<style>
/* Using local fonts to avoid network timeouts */
@import url('./assets/fonts/inter-local.css');
html { 
  font-family: 'Inter', sans-serif; 
}
@supports (font-variation-settings: normal) {
  html { font-family: 'InterVariable', sans-serif; }
}
</style> 