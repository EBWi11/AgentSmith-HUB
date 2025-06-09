<template>
  <div class="flex flex-col h-screen bg-white">
    <Header />
    <div class="flex flex-1 overflow-hidden">
      <Sidebar :selected="selected" @select-item="onSelectItem" />
      <main class="flex-1 bg-gray-50">
        <ComponentDetail v-if="selected && selected.type !== 'cluster'" :item="selected" />
        <ClusterStatus v-else-if="selected && selected.type === 'cluster'" />
      </main>
    </div>
  </div>
</template>

<script>
import Header from '../components/Header.vue'
import Sidebar from '../components/Sidebar/Sidebar.vue'
import ComponentDetail from '../components/ComponentDetail.vue'
import ClusterStatus from '../components/ClusterStatus.vue'

export default {
  name: 'MainLayout',
  components: {
    Header,
    Sidebar,
    ComponentDetail,
    ClusterStatus
  },
  data() {
    return {
      selected: null
    }
  },
  methods: {
    onSelectItem(item) {
      this.selected = item;
    }
  }
}
</script> 