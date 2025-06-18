<template>
  <div 
    class="compact-node"
  >
    <div class="node-header" :style="{ backgroundColor: headerColor, borderColor: borderColor, color: textColor }">
      <span class="node-title">{{ nodeType }}</span>
    </div>
    <div class="node-content">
      {{ nodeName }}
    </div>
    <Handle type="target" :position="Position.Top" />
    <Handle type="source" :position="Position.Bottom" />
  </div>
</template>

<script setup>
import { computed } from 'vue';
import { Handle, Position } from '@vue-flow/core';

const props = defineProps({
  nodeType: {
    type: String,
    required: true,
  },
  nodeName: {
    type: String,
    required: true,
  }
});

const colors = computed(() => {
  switch (props.nodeType.toUpperCase()) {
    case 'INPUT':
      return { header: '#e0f2fe', border: '#bae6fd', text: '#0c4a6e' };
    case 'OUTPUT':
      return { header: '#dcfce7', border: '#bbf7d0', text: '#166534' };
    case 'RULESET':
      return { header: '#f3e8ff', border: '#e9d5ff', text: '#581c87' };
    default:
      return { header: '#e2e8f0', border: '#cbd5e1', text: '#1e293b' };
  }
});

const headerColor = computed(() => colors.value.header);
const borderColor = computed(() => colors.value.border);
const textColor = computed(() => colors.value.text);
</script>

<style>
.compact-node {
  background-color: white;
  border-radius: 4px;
  border: 1px solid #e2e8f0;
  box-shadow: 0 1px 2px 0 rgb(0 0 0 / 0.05);
  width: 90px;
  font-family: 'Inter', sans-serif;
  overflow: hidden;
  cursor: pointer;
  transition: box-shadow 0.2s ease-in-out;
  user-select: none;
}

.compact-node:hover {
  box-shadow: 0 4px 6px -1px rgb(0 0 0 / 0.1), 0 2px 4px -2px rgb(0 0 0 / 0.1);
}

.node-header {
  padding: 1px 5px;
  font-weight: 400;
  font-size: 9px;
  border-bottom: 1px solid;
  text-transform: uppercase;
}

.node-content {
  padding: 4px 5px;
  font-size: 11px;
  color: #334155;
  min-height: 15px;
  word-wrap: break-word;
}
</style> 