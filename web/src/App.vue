<template>
  <router-view />
  <Toast ref="toast" />
</template>

<script>
import Toast from './components/Toast.vue'
export default {
  name: 'App',
  components: { Toast },
  mounted() {
    window.$toast = this.$refs.toast
  }
}
</script>

<style>
/* Using a cdn for now, we can install it locally later */
@import url('https://rsms.me/inter/inter.css');
html { font-family: 'Inter', sans-serif; }
</style> 