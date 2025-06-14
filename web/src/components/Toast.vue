<template>
  <div class="fixed top-6 right-6 z-50 flex flex-col space-y-2 items-end">
    <transition-group name="toast-slide" tag="div">
      <div v-for="msg in messages" :key="msg.id"
        class="px-4 py-2.5 rounded shadow-md min-w-[280px] text-sm flex items-center mb-2"
        :class="{
          'bg-white border-l-4 border-green-500 text-gray-700': msg.type === 'success',
          'bg-white border-l-4 border-red-500 text-gray-700': msg.type === 'error',
          'bg-white border-l-4 border-yellow-500 text-gray-700': msg.type === 'warning',
          'bg-white border-l-4 border-blue-500 text-gray-700': msg.type === 'info'
        }"
      >
        <span class="mr-2.5 flex-shrink-0" :class="{
          'text-green-500': msg.type === 'success',
          'text-red-500': msg.type === 'error',
          'text-yellow-500': msg.type === 'warning',
          'text-blue-500': msg.type === 'info'
        }">
          <!-- Success Icon -->
          <svg v-if="msg.type === 'success'" xmlns="http://www.w3.org/2000/svg" class="h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
            <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd" />
          </svg>
          
          <!-- Error Icon -->
          <svg v-else-if="msg.type === 'error'" xmlns="http://www.w3.org/2000/svg" class="h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
            <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clip-rule="evenodd" />
          </svg>
          
          <!-- Warning Icon -->
          <svg v-else-if="msg.type === 'warning'" xmlns="http://www.w3.org/2000/svg" class="h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
            <path fill-rule="evenodd" d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z" clip-rule="evenodd" />
          </svg>
          
          <!-- Info Icon -->
          <svg v-else xmlns="http://www.w3.org/2000/svg" class="h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
            <path fill-rule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z" clip-rule="evenodd" />
          </svg>
        </span>
        <span class="flex-grow">{{ msg.text }}</span>
        <button @click="dismiss(msg.id)" class="ml-2 text-gray-400 hover:text-gray-600 transition-colors">
          <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>
    </transition-group>
  </div>
</template>

<script>
let toastId = 0
export default {
  data() {
    return {
      messages: []
    }
  },
  methods: {
    show(text, type = 'success') {
      const id = ++toastId
      this.messages.push({ id, text, type })
      
      // Auto dismiss after timeout
      setTimeout(() => {
        this.dismiss(id)
      }, 4000)
      
      return id
    },
    dismiss(id) {
      this.messages = this.messages.filter(m => m.id !== id)
    }
  }
}
</script>

<style>

</style> 