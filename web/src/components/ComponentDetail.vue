<template>
  <div v-if="loading" class="flex items-center justify-center h-full text-gray-400 text-lg">加载中...</div>
  <div v-else-if="error" class="flex items-center justify-center h-full text-red-400 text-lg">{{ error }}</div>
  
  <!-- Special layout for projects -->
  <div v-else-if="item && item.type === 'projects' && detail && detail.raw" class="flex h-full">
    <div class="w-1/2 h-full">
       <CodeEditor :value="detail.raw" :language="'yaml'" class="h-full" />
    </div>
    <div class="w-1/2 h-full border-l border-gray-200">
      <ProjectWorkflow :projectContent="detail.raw" />
    </div>
  </div>

  <!-- Default layout for other components -->
  <div v-else-if="detail && detail.raw" class="h-full">
    <CodeEditor :value="detail.raw" :language="item.type === 'rulesets' ? 'xml' : (item.type === 'plugin' ? 'go' : 'yaml')" class="h-full" />
  </div>

  <div v-else class="flex items-center justify-center h-full text-gray-400 text-lg">
    暂无内容
  </div>
</template>

<script>
import { hubApi } from '../api/index.js';
import CodeEditor from './CodeEditor.vue';
import ProjectWorkflow from './Visualization/ProjectWorkflow.vue';

export default {
  name: 'ComponentDetail',
  components: { 
    CodeEditor,
    ProjectWorkflow 
  },
  props: {
    item: Object
  },
  data() {
    return {
      loading: false,
      error: null,
      detail: null
    }
  },
  watch: {
    item: {
      immediate: true,
      handler(newVal) {
        this.fetchDetail(newVal);
      }
    }
  },
  methods: {
    async fetchDetail(item) {
      this.detail = null;
      this.error = null;
      if (!item || !item.id) return;
      this.loading = true;
      try {
        let data;
        switch (item.type) {
          case 'inputs':
            data = await hubApi.getInput(item.id); break;
          case 'outputs':
            data = await hubApi.getOutput(item.id); break;
          case 'rulesets':
            data = await hubApi.getRuleset(item.id); break;
          case 'projects':
            data = await hubApi.getProject(item.id); break;
          case 'plugins':
            data = await hubApi.getPlugin(item.id); break;
          default:
            data = null;
        }
        this.detail = data;
      } catch (e) {
        this.error = '加载详情失败';
      } finally {
        this.loading = false;
      }
    }
  }
}
</script> 