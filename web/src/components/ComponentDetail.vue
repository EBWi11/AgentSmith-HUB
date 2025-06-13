<template>
  <div v-if="loading" class="flex items-center justify-center h-full text-gray-400 text-lg">加载中...</div>
  <div v-else-if="error" class="flex items-center justify-center h-full text-red-400 text-lg">{{ error }}</div>
  
  <!-- 新建模式 -->
  <div v-else-if="item && item.isNew" class="h-full flex flex-col">
    <CodeEditor v-model:value="editorValue" :language="item.type === 'rulesets' ? 'xml' : (item.type === 'plugins' ? 'go' : 'yaml')" :readOnly="false" class="flex-1" @save="saveNew" />
    <div v-if="saveError" class="text-xs text-red-500 mt-2">{{ saveError }}</div>
  </div>

  <!-- 编辑模式 -->
  <div v-else-if="item && item.isEdit && detail && detail.raw" class="h-full flex flex-col">
    <CodeEditor v-model:value="editorValue" :language="item.type === 'rulesets' ? 'xml' : (item.type === 'plugins' ? 'go' : 'yaml')" :readOnly="false" class="flex-1" @save="saveEdit" />
    <div v-if="saveError" class="text-xs text-red-500 mt-2">{{ saveError }}</div>
  </div>

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
    <CodeEditor :value="detail.raw" :language="item.type === 'rulesets' ? 'xml' : (item.type === 'plugins' ? 'go' : 'yaml')" class="h-full" />
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
      detail: null,
      editorValue: '',
      saveError: ''
    }
  },
  watch: {
    item: {
      immediate: true,
      handler(newVal) {
        if (newVal && newVal.isNew) {
          this.detail = null;
          this.editorValue = this.getDefaultTemplate(newVal.type, newVal.id);
        } else if (newVal && newVal.isEdit) {
          this.fetchDetail(newVal, true);
        } else {
          this.fetchDetail(newVal);
        }
      }
    }
  },
  methods: {
    async fetchDetail(item, forEdit = false) {
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
        if (forEdit && data && data.raw) {
          this.editorValue = data.raw;
        }
      } catch (e) {
        this.error = '加载详情失败';
      } finally {
        this.loading = false;
      }
    },
    getDefaultTemplate(type, id) {
      switch (type) {
        case 'inputs':
          return ``;
        case 'outputs':
          return ``;
        case 'rulesets':
          return ``;
        case 'projects':
          return ``;
        case 'plugins':
          return ``;
        default:
          return ``;
      }
    },
    async saveNew() {
      this.saveError = '';
      try {
        const { type, id } = this.item;
        const raw = '';
        let resp;
        switch (type) {
          case 'inputs':
            resp = await hubApi.createInput(id, raw); break;
          case 'outputs':
            resp = await hubApi.createOutput(id, raw); break;
          case 'rulesets':
            resp = await hubApi.createRuleset(id, raw); break;
          case 'projects':
            resp = await hubApi.createProject(id, raw); break;
          case 'plugins':
            resp = await hubApi.createPlugin ? await hubApi.createPlugin(id, raw) : null; break;
          default:
            throw new Error('不支持的类型');
        }
        this.$emit('created', { type, id });
        this.$message && this.$message.success('创建成功！');
      } catch (e) {
        this.saveError = '保存失败: ' + (e?.message || '未知错误');
      }
    },
    async saveEdit() {
      this.saveError = '';
      this.saving = true;
      try {
        const { type, id } = this.item;
        const raw = this.editorValue;
        let resp;
        switch (type) {
          case 'inputs':
            resp = await hubApi.updateInput(id, raw); break;
          case 'outputs':
            resp = await hubApi.updateOutput(id, raw); break;
          case 'rulesets':
            resp = await hubApi.updateRuleset(id, raw); break;
          case 'projects':
            resp = await hubApi.updateProject(id, raw); break;
          case 'plugins':
            resp = await hubApi.updatePlugin ? await hubApi.updatePlugin(id, raw) : null; break;
          default:
            throw new Error('不支持的类型');
        }
        this.$emit('updated', { type, id });
        this.$message && this.$message.success('保存成功！');
      } catch (e) {
        this.saveError = '保存失败: ' + (e?.message || '未知错误');
      } finally {
        this.saving = false;
      }
    }
  }
}
</script> 