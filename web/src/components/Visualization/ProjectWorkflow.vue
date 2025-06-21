<template>
  <div class="h-full w-full bg-gray-50">
    <VueFlow
      v-model:nodes="nodes"
      v-model:edges="edges"
      :fit-view-on-init="true"
      :nodes-draggable="false"
      :edges-updatable="false"
      :prevent-scrolling="false"
      :auto-connect="false"
      :elevate-edges-on-select="false"
      @node-click="onNodeClick"
      @node-context-menu="onNodeContextMenu"
    >
      <template #node-custom="nodeProps">
        <div @click="() => handleNodeClick(nodeProps)" @contextmenu.prevent="(event) => handleNodeContextMenu(event, nodeProps)">
          <CustomNode 
            :node-type="nodeProps.data.nodeType" 
            :node-name="nodeProps.data.nodeName"
            class="cursor-pointer hover:shadow-md transition-shadow duration-200"
          />
        </div>
      </template>

      <Background :pattern-color="'#e5e7eb'" :gap="10" />
      <Controls :position="'top-right'" />
    </VueFlow>

    <!-- Right-click menu -->
    <div v-if="showContextMenu" class="context-menu" :style="contextMenuStyle">
      <div class="bg-white rounded-lg shadow-lg border border-gray-200 py-1 min-w-[160px]">
        <button 
          class="w-full px-4 py-2 text-left text-sm hover:bg-gray-100 flex items-center"
          @click="viewSampleData"
        >
          <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
          </svg>
          View Sample Data
        </button>
      </div>
    </div>

    <!-- Sample data modal -->
    <div v-if="showSampleModal" class="fixed inset-0 bg-black bg-opacity-30 flex items-center justify-center z-50">
      <div class="bg-white rounded-lg shadow-xl w-3/4 max-w-4xl">
        <div class="px-6 py-4 border-b border-gray-200 flex justify-between items-center">
          <h3 class="text-lg font-medium">Sample Data - {{ selectedNode?.data.nodeType }} ({{ selectedNode?.data.nodeName }})</h3>
          <button @click="closeSampleModal" class="text-gray-400 hover:text-gray-500">
            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
        <div class="p-6 max-h-[70vh] overflow-auto">
          <div v-if="loadingSamples" class="flex justify-center items-center py-8">
            <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
          </div>
          <div v-else-if="!sampleDataGrouped || Object.keys(sampleDataGrouped).length === 0" class="text-center text-gray-500 py-8">
            No sample data available
          </div>
          <div v-else class="space-y-6">
            <!-- Grouped by ProjectNodeSequence -->
            <div v-for="(samples, projectNodeSequence) in sampleDataGrouped" :key="projectNodeSequence" class="border border-gray-200 rounded-lg p-4">
              <div class="mb-3 flex items-center justify-between">
                <h4 class="text-sm font-medium text-gray-700">Project Node Sequence: {{ projectNodeSequence }}</h4>
                <span class="text-xs bg-blue-100 text-blue-800 px-2 py-1 rounded-full">{{ samples.length }} samples</span>
              </div>
              <div class="space-y-3">
                <div v-for="(sample, index) in samples.slice(0, 5)" :key="index" class="bg-gray-50 rounded p-3">
                  <div class="text-xs text-gray-500 mb-2 flex justify-between">
                    <span>Sample {{ index + 1 }}</span>
                    <span v-if="sample.timestamp">{{ new Date(sample.timestamp).toLocaleString() }}</span>
                  </div>
                  <div class="text-xs text-gray-500 mb-2">
                    Source: {{ sample.source }}
                  </div>
                  <pre class="text-sm overflow-x-auto">{{ JSON.stringify(sample.data, null, 2) }}</pre>
                </div>
                <div v-if="samples.length > 5" class="text-center">
                  <span class="text-xs text-gray-500">... and {{ samples.length - 5 }} more samples</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, watch, onMounted, onUnmounted, computed } from 'vue';
import { VueFlow } from '@vue-flow/core';
import { Background } from '@vue-flow/background';
import { Controls } from '@vue-flow/controls';
import { useRouter } from 'vue-router';
import dagre from 'dagre';
import yaml from 'js-yaml';
import CustomNode from './CustomNode.vue';
import { hubApi } from '../../api';

const router = useRouter();

const props = defineProps({
    projectContent: {
      type: String,
      required: true,
    },
});

const nodes = ref([]);
const edges = ref([]);

// Right-click menu related
const showContextMenu = ref(false);
const contextMenuStyle = ref({
  position: 'fixed',
  top: '0px',
  left: '0px',
});
const selectedNode = ref(null);

// Sample data related
const showSampleModal = ref(false);
const loadingSamples = ref(false);
const sampleDataRaw = ref({});

// Computed property to group sample data by ProjectNodeSequence
const sampleDataGrouped = computed(() => {
  return sampleDataRaw.value;
});

// VueFlow node click handler (keeping compatibility)
function onNodeClick(event, node) {
  handleNodeClick(node);
}

// VueFlow context menu handler (keeping compatibility)
function onNodeContextMenu(event, node) {
  handleNodeContextMenu(event, node);
}

// New node click handler
function handleNodeClick(nodeProps) {
  if (!nodeProps || !nodeProps.data) {
    console.warn('Invalid nodeProps:', nodeProps);
    return;
  }
  
  const type = nodeProps.data.nodeType?.toLowerCase();
  const id = nodeProps.data.componentId;
  
  if (!type || !id) {
    console.warn('Invalid node data:', nodeProps.data);
    return;
  }

  // Determine route based on node type
  let routePath;
  switch (type) {
    case 'input':
      routePath = `/app/inputs/${id}`;
      break;
    case 'output':
      routePath = `/app/outputs/${id}`;
      break;
    case 'ruleset':
      routePath = `/app/rulesets/${id}`;
      break;
    default:
      console.warn('Unsupported node type:', type);
      return;
  }

  // Open component details page in new tab
  const url = window.location.origin + routePath;
  window.open(url, '_blank');
}

// New context menu handler
function handleNodeContextMenu(event, nodeProps) {
  event.preventDefault();
  event.stopPropagation();
  showContextMenu.value = true;
  contextMenuStyle.value = {
    position: 'fixed',
    top: `${event.clientY}px`,
    left: `${event.clientX}px`,
  };
  selectedNode.value = nodeProps;
}

// Listen for global click events to close context menu
function onGlobalClick(event) {
  if (event.target.closest('.context-menu')) return;
  showContextMenu.value = false;
}

// Handle ESC key press
function handleEscKey(event) {
  if (event.key === 'Escape') {
    if (showSampleModal.value) {
      closeSampleModal();
    } else if (showContextMenu.value) {
      showContextMenu.value = false;
    }
  }
}

// Add global click event listener on component mount
onMounted(() => {
  document.addEventListener('click', onGlobalClick);
  document.addEventListener('keydown', handleEscKey);
});

// Remove global click event listener on component unmount
onUnmounted(() => {
  document.removeEventListener('click', onGlobalClick);
  document.removeEventListener('keydown', handleEscKey);
});

// View sample data
async function viewSampleData() {
  showContextMenu.value = false;
  showSampleModal.value = true;
  loadingSamples.value = true;
  
  try {
    const nodeType = selectedNode.value.data.nodeType.toLowerCase();
    const componentId = selectedNode.value.data.componentId;
    
    const response = await hubApi.getSamplerData({
      name: nodeType,
      projectNodeSequence: `${nodeType}.${componentId}`
    });
    
    if (response && response[nodeType]) {
      // Store the grouped sample data directly
      sampleDataRaw.value = response[nodeType];
    } else {
      sampleDataRaw.value = {};
    }
  } catch (error) {
    console.error('Failed to fetch sample data:', error);
    samples.value = [];
  } finally {
    loadingSamples.value = false;
  }
}

// Close sample data modal
function closeSampleModal() {
  showSampleModal.value = false;
  sampleDataRaw.value = {};
}

const parseAndLayoutWorkflow = (rawProjectContent) => {
  if (!rawProjectContent) {
    nodes.value = [];
    edges.value = [];
    return;
  }

  try {
    const doc = yaml.load(rawProjectContent);
    const content = doc.content || '';
    const lines = content.trim().split('\n');
    
    const tempNodes = new Map();
    const tempEdges = [];

    lines.forEach((line, index) => {
      if (!line.trim() || !line.includes('->')) return;
      const parts = line.split('->');
      if (parts.length !== 2) return;
      
      const fromId = parts[0].trim();
      const toId = parts[1].trim();
      
      const addNode = (id) => {
        if (id && !tempNodes.has(id)) {
          const [type, ...nameParts] = id.split('.');
          const name = nameParts.join('.') || type;
          tempNodes.set(id, {
            id: id,
            type: 'custom',
            data: { 
              nodeType: type.toUpperCase(), 
              nodeName: name,
              componentId: name,
              originalId: id
            }
          });
        }
      };

      addNode(fromId);
      addNode(toId);
      
      tempEdges.push({ 
        id: `e-${fromId}-${toId}-${index}`, 
        source: fromId, 
        target: toId,
        type: 'default',
        style: { stroke: '#9ca3af', strokeWidth: 1.2 },
        markerEnd: { type: 'arrowclosed', color: '#9ca3af' }
      });
    });

    const newNodes = Array.from(tempNodes.values());
    
    const g = new dagre.graphlib.Graph();
    g.setDefaultEdgeLabel(() => ({}));
    g.setGraph({ rankdir: 'TB', nodesep: 80, ranksep: 100 });

    newNodes.forEach(node => {
      g.setNode(node.id, { width: 75, height: 38 });
    });
    tempEdges.forEach(edge => {
      g.setEdge(edge.source, edge.target);
    });
    
    dagre.layout(g);

    nodes.value = newNodes.map(node => {
      const nodeWithPosition = g.node(node.id);
      return {
        ...node,
        position: { x: nodeWithPosition.x - 37.5, y: nodeWithPosition.y - 19 },
      };
    });

    edges.value = tempEdges;

  } catch (e) {
    console.error('Error parsing workflow:', e);
    nodes.value = [];
    edges.value = [];
  }
};

watch(() => props.projectContent, (newVal) => {
  parseAndLayoutWorkflow(newVal);
}, { immediate: true, deep: true });
</script> 

<style>
@import '@vue-flow/core/dist/style.css';
@import '@vue-flow/controls/dist/style.css';

.vue-flow__attribution {
    display: none;
}

.vue-flow__node {
  border: none !important;
  box-shadow: none !important;
  background-color: transparent !important;
  transition: transform 0.2s ease;
}

.vue-flow__node:hover {
  transform: scale(1.02);
}

.context-menu {
  z-index: 1000;
}

/* 限制控制按钮在预览区域内 */
.vue-flow__controls {
  position: absolute !important;
  top: 10px !important;
  right: 10px !important;
  left: auto !important;
  max-width: calc(100% - 20px) !important;
  z-index: 100 !important;
}

/* 确保控制按钮不会溢出到右侧 */
.vue-flow__controls .vue-flow__controls-button {
  display: inline-block !important;
  margin-right: 5px !important;
}
</style> 