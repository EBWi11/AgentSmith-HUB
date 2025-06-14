<template>
  <router-view />
  <Toast ref="toast" />
</template>

<script setup>
import { ref, onMounted, provide } from 'vue'
import Toast from './components/Toast.vue'
import { useStore } from 'vuex'

const toast = ref(null)
const store = useStore()

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
  store.dispatch('fetchAvailablePlugins')
})
</script>

<style>
/* Using a cdn for now, we can install it locally later */
@import url('https://rsms.me/inter/inter.css');
html { font-family: 'Inter', sans-serif; }
</style> 