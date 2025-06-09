<template>
  <div class="h-full w-full bg-gray-50">
    <VueFlow
      v-model:nodes="nodes"
      v-model:edges="edges"
      :fit-view-on-init="true"
      :nodes-draggable="false"
      :edges-updatable="false"
    >
      <template #node-custom="props">
        <CustomNode :node-type="props.data.nodeType" :node-name="props.data.nodeName" />
      </template>

      <Background :pattern-color="'#e5e7eb'" :gap="10" />
      <MiniMap />
      <Controls />
    </VueFlow>
  </div>
</template>

<script setup>
import { ref, watch } from 'vue';
import { VueFlow } from '@vue-flow/core';
import { Background } from '@vue-flow/background';
import { Controls } from '@vue-flow/controls';
import { MiniMap } from '@vue-flow/minimap';
import dagre from 'dagre';
import yaml from 'js-yaml';
import CustomNode from './CustomNode.vue';

const props = defineProps({
    projectContent: {
      type: String,
      required: true,
    },
});

const nodes = ref([]);
const edges = ref([]);

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
            id,
            type: 'custom',
            data: { nodeType: type.toUpperCase(), nodeName: name }
          });
          }
      };

      addNode(fromId);
      addNode(toId);
      
      tempEdges.push({ 
        id: `e-${fromId}-${toId}-${index}`, 
        source: fromId, 
        target: toId,
        type: 'smoothstep',
        style: { stroke: '#a1a1aa', strokeWidth: 1.5 },
        markerEnd: { type: 'arrowclosed', color: '#a1a1aa' }
      });
        });

    const newNodes = Array.from(tempNodes.values());
    
    const g = new dagre.graphlib.Graph();
    g.setDefaultEdgeLabel(() => ({}));
    g.setGraph({ rankdir: 'TB', nodesep: 15, ranksep: 20 });

    newNodes.forEach(node => {
      g.setNode(node.id, { width: 90, height: 45 });
    });
    tempEdges.forEach(edge => {
      g.setEdge(edge.source, edge.target);
    });
    
    dagre.layout(g);

    nodes.value = newNodes.map(node => {
      const nodeWithPosition = g.node(node.id);
          return {
        ...node,
        position: { x: nodeWithPosition.x - 45, y: nodeWithPosition.y - 22.5 },
          };
        });

    edges.value = tempEdges;

      } catch (e) {
        console.error("Error parsing project workflow:", e);
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
@import '@vue-flow/minimap/dist/style.css';

.vue-flow__attribution {
    display: none;
}

.vue-flow__node {
  border: none !important;
  box-shadow: none !important;
  background-color: transparent !important;
}
</style> 