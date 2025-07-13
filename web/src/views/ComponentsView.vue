<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useDataCacheStore } from '../stores/dataCache'

const dataCache = useDataCacheStore()
const route = useRoute()
const router = useRouter()

// 在组件挂载时获取所有组件列表
onMounted(async () => {
  const componentTypes = ['inputs', 'outputs', 'rulesets', 'plugins', 'projects']
  await Promise.all(componentTypes.map(type => dataCache.fetchComponents(type)))
})

// 当路由参数变化时，也重新获取组件列表
watch(() => route.params.type, async (newType) => {
  if (newType) {
    await dataCache.fetchComponents(newType)
  }
})
</script> 