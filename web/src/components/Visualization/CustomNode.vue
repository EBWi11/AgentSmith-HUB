<template>
  <div 
    class="compact-node"
  >
    <div class="node-header" :style="{ backgroundColor: headerColor, borderColor: borderColor, color: textColor, fontWeight: isBold ? 'bold' : 'normal' }">
      <span class="node-title">{{ nodeType }}</span>
    </div>
    <div class="node-content">
      {{ nodeName }}
    </div>
    <!-- QPS Display -->
    <div v-if="hasQPSData" class="node-qps" :style="{ backgroundColor: qpsBackgroundColor, color: qpsTextColor }">
      <span class="qps-label">QPS:</span>
      <span class="qps-value">{{ formattedQPS }}</span>
      <span v-if="nodeCount > 1" class="qps-nodes">({{ nodeCount }} nodes)</span>
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
  },
  qps: {
    type: Number,
    default: 0,
  },
  nodeCount: {
    type: Number,
    default: 0,
  },
  hasQPSData: {
    type: Boolean,
    default: false,
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
    case 'CHECK':
      return { header: '#fef2f2', border: '#fecaca', text: '#7f1d1d', bold: true };
    case 'APPEND':
      return { header: '#fef2f2', border: '#fecaca', text: '#7f1d1d', bold: true };
    default:
      return { header: '#e2e8f0', border: '#cbd5e1', text: '#1e293b' };
  }
});

const headerColor = computed(() => colors.value.header);
const borderColor = computed(() => colors.value.border);
const textColor = computed(() => colors.value.text);
const isBold = computed(() => colors.value.bold || false);

// QPS related computed properties
const formattedQPS = computed(() => {
  if (props.qps >= 1000) {
    return (props.qps / 1000).toFixed(1) + 'k';
  }
  return props.qps.toString();
});

const qpsBackgroundColor = computed(() => {
  if (props.qps === 0) return '#f3f4f6'; // Gray for no activity
  if (props.qps < 10) return '#ecfdf5'; // Light green for low QPS
  if (props.qps < 100) return '#fef3c7'; // Light yellow for medium QPS
  return '#fef2f2'; // Light red for high QPS
});

const qpsTextColor = computed(() => {
  if (props.qps === 0) return '#6b7280'; // Gray text
  if (props.qps < 10) return '#065f46'; // Dark green text
  if (props.qps < 100) return '#92400e'; // Dark yellow text
  return '#991b1b'; // Dark red text
});
</script>

<style>
.compact-node {
  background-color: white;
  border-radius: 4px;
  border: 1px solid #e2e8f0;
  box-shadow: 0 1px 2px 0 rgb(0 0 0 / 0.05);
  width: 75px;
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
  padding: 2px 4px;
  text-align: center;
  font-size: 8px;
  line-height: 10px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  border-bottom: 1px solid;
}

.node-content {
  padding: 3px 2px;
  text-align: center;
  font-size: 9px;
  line-height: 11px;
  color: #374151;
  font-weight: 500;
  word-break: break-word;
  hyphens: auto;
}

.node-qps {
  padding: 2px 3px;
  text-align: center;
  font-size: 7px;
  line-height: 8px;
  font-weight: 600;
  border-top: 1px solid #e5e7eb;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 1px;
}

.qps-label {
  font-size: 6px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  opacity: 0.8;
}

.qps-value {
  font-size: 8px;
  font-weight: 700;
}

.qps-nodes {
  font-size: 6px;
  opacity: 0.7;
  font-weight: 400;
}

/* Handle positioning adjustments for taller nodes */
.vue-flow__handle {
  width: 6px !important;
  height: 6px !important;
}

.vue-flow__handle.vue-flow__handle-top {
  top: -3px !important;
}

.vue-flow__handle.vue-flow__handle-bottom {
  bottom: -3px !important;
}
</style> 