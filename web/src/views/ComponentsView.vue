<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useStore } from 'vuex'

const store = useStore()
const route = useRoute()
const router = useRouter()

// 在组件挂载时获取所有组件列表
onMounted(() => {
  store.dispatch('fetchAllComponents')
})

// 当路由参数变化时，也重新获取组件列表
watch(() => route.params.type, (newType) => {
  if (newType) {
    store.dispatch('fetchComponents', newType)
  }
})
</script> 