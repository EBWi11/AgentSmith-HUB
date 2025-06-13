<template>
  <div class="fixed top-6 right-6 z-50 flex flex-col space-y-2 items-end">
    <transition-group name="fade" tag="div">
      <div v-for="msg in messages" :key="msg.id"
        class="bg-red-600 text-white px-4 py-2 rounded shadow-lg min-w-[200px] text-sm flex items-center mb-2">
        <span class="mr-2">❗</span>
        <span>{{ msg.text }}</span>
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
    show(text) {
      const id = ++toastId
      this.messages.push({ id, text })
      setTimeout(() => {
        this.messages = this.messages.filter(m => m.id !== id)
      }, 4000)
    }
  }
}
</script>

<style>
.fade-enter-active, .fade-leave-active { transition: opacity 0.3s; }
.fade-enter, .fade-leave-to { opacity: 0; }
</style> 