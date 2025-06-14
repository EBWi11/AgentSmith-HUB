<template>
  <div v-if="showModal" class="fixed inset-0 bg-black bg-opacity-30 flex items-center justify-center z-50">
    <div class="bg-white rounded shadow-lg p-6 w-[1200px] max-h-[90vh] overflow-hidden flex flex-col">
      <div class="flex justify-between items-center mb-4">
        <h3 class="font-bold text-lg">Test Project: {{ projectId }}</h3>
        <button @click="closeModal" class="text-gray-400 hover:text-gray-600">
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
          </svg>
        </button>
      </div>
      
      <div class="flex flex-1 overflow-hidden" style="min-height: 600px;">
        <!-- Left panel: Input data -->
        <div class="w-1/3 pr-3 flex flex-col overflow-hidden">
          <div class="mb-3">
            <h4 class="text-sm font-medium text-gray-700 mb-2">Select Input Node:</h4>
            <div class="relative">
              <select 
                v-model="selectedInputNode" 
                class="w-full border border-gray-300 rounded-md shadow-sm py-2 pl-3 pr-10 text-sm focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
              >
                <option v-for="node in inputNodes" :key="node.id" :value="node.id">
                  {{ node.name }}
                </option>
              </select>
              <div v-if="inputNodesLoading" class="text-sm text-gray-500 mt-2 flex items-center">
                <div class="w-4 h-4 border-2 border-gray-300 border-t-transparent rounded-full animate-spin mr-2"></div>
                Loading input nodes...
              </div>
              <div v-else-if="inputNodes.length === 0" class="text-sm text-gray-500 mt-2">
                No input nodes available
              </div>
            </div>
          </div>
          
          <h4 class="text-sm font-medium text-gray-700 mb-2">Input Data:</h4>
          <div class="flex-1 overflow-hidden border border-gray-200 rounded-md">
            <CodeEditor 
              v-model:value="inputData" 
              :language="'json'" 
              :read-only="false" 
              class="h-full" 
            />
          </div>
        </div>
        
        <!-- Middle panel: Project Structure -->
        <div class="w-1/3 px-3 flex flex-col overflow-hidden">
          <h4 class="text-sm font-medium text-gray-700 mb-2">Project Structure:</h4>
          <div class="flex-1 overflow-auto border border-gray-200 rounded-md bg-gray-50 p-3">
            <div v-if="testLoading" class="flex justify-center items-center h-full">
              <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
            </div>
            
            <div v-else class="flex flex-col h-full">
              <!-- Project structure visualization -->
              <div class="bg-white border border-gray-200 rounded-md p-3 mb-3">
                <div v-if="!projectStructure" class="text-center text-gray-500 py-4">
                  Run test to view project structure
                </div>
                <div v-else class="flex flex-col items-center">
                  <!-- Simple visualization of project structure -->
                  <div class="w-full overflow-auto">
                    <div v-for="(node, index) in projectStructure.nodes" :key="node.id" class="mb-2">
                      <div class="px-3 py-2 rounded-md text-sm" 
                        :class="{
                          'bg-blue-100 border border-blue-200': node.type === 'input',
                          'bg-purple-100 border border-purple-200': node.type === 'ruleset',
                          'bg-green-100 border border-green-200': node.type === 'output',
                          'font-semibold': node.id === selectedInputNode
                        }">
                        <span class="mr-1">{{ node.type }}:</span>
                        <span>{{ node.name }}</span>
                      </div>
                      
                      <!-- Show connections -->
                      <div v-for="edge in getNodeEdges(node.id)" :key="edge.from + '->' + edge.to" class="ml-4 text-xs text-gray-500 flex items-center">
                        <svg class="w-4 h-4 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 8l4 4m0 0l-4 4m4-4H3"></path>
                        </svg>
                        {{ getNodeName(edge.to) }}
                      </div>
                    </div>
                  </div>
                </div>
              </div>
              
              <!-- Flow path highlighting -->
              <div v-if="projectStructure && selectedInputNode" class="bg-white border border-gray-200 rounded-md p-3">
                <h5 class="font-medium text-sm text-gray-700 mb-2">Data Flow Path:</h5>
                <div class="space-y-2">
                  <div v-for="(path, index) in getDataFlowPaths()" :key="index" class="text-xs">
                    <div class="flex items-center">
                      <span v-for="(node, nodeIndex) in path" :key="nodeIndex" class="flex items-center">
                        <span class="px-2 py-1 rounded" 
                          :class="{
                            'bg-blue-100': getNodeType(node) === 'input',
                            'bg-purple-100': getNodeType(node) === 'ruleset',
                            'bg-green-100': getNodeType(node) === 'output'
                          }">
                          {{ getNodeName(node) }}
                        </span>
                        <svg v-if="nodeIndex < path.length - 1" class="w-4 h-4 mx-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"></path>
                        </svg>
                      </span>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
        
        <!-- Right panel: Results -->
        <div class="w-1/3 pl-3 flex flex-col overflow-hidden">
          <h4 class="text-sm font-medium text-gray-700 mb-2">Output Results:</h4>
          <div class="flex-1 overflow-auto border border-gray-200 rounded-md bg-gray-50 p-3">
            <div v-if="testLoading" class="flex justify-center items-center h-full">
              <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
            </div>
            
            <div v-else-if="testError" class="bg-red-50 border-l-4 border-red-500 p-4 mb-4">
              <div class="flex">
                <div class="flex-shrink-0">
                  <svg class="h-5 w-5 text-red-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                  </svg>
                </div>
                <div class="ml-3">
                  <p class="text-sm text-red-700">{{ testError }}</p>
                </div>
              </div>
            </div>
            
            <div v-else-if="!testExecuted" class="text-center py-8 text-gray-500">
              <p>No results yet. Click "Run Test" to send data to the project.</p>
            </div>
            
            <div v-else>
              <!-- No outputs -->
              <div v-if="Object.keys(outputResults).length === 0" class="bg-yellow-50 border-l-4 border-yellow-400 p-4">
                <div class="flex">
                  <div class="flex-shrink-0">
                    <svg class="h-5 w-5 text-yellow-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                    </svg>
                  </div>
                  <div class="ml-3">
                    <p class="text-sm text-yellow-700">No output results were generated. The data may not have reached any output nodes.</p>
                  </div>
                </div>
              </div>
              
              <!-- Output results -->
              <div v-else class="space-y-4">
                <div v-for="(results, outputName) in outputResults" :key="outputName" class="bg-white border border-gray-200 rounded-md p-3">
                  <div class="flex justify-between items-center mb-2">
                    <h5 class="font-medium text-gray-700">Output: {{ outputName }}</h5>
                    <span class="px-2 py-0.5 bg-green-100 text-green-800 text-xs rounded-full">
                      {{ results.length }} message(s)
                    </span>
                  </div>
                  
                  <div v-if="results.length === 0" class="text-sm text-gray-500 italic">
                    No messages received
                  </div>
                  
                  <div v-else class="space-y-2">
                    <div v-for="(result, index) in results" :key="index" class="border border-gray-100 rounded p-2">
                      <pre class="text-xs bg-gray-50 p-2 rounded overflow-auto max-h-60">{{ JSON.stringify(result, null, 2) }}</pre>
                    </div>
                  </div>
                </div>
              </div>
              
              <div v-if="testResults.isTemp" class="bg-yellow-50 border-l-4 border-yellow-400 p-4 mt-4">
                <div class="flex">
                  <div class="flex-shrink-0">
                    <svg class="h-5 w-5 text-yellow-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                    </svg>
                  </div>
                  <div class="ml-3">
                    <p class="text-sm text-yellow-700">This project has pending changes. Test was performed using the pending version.</p>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
      
      <div class="flex justify-end mt-6 space-x-3">
        <button 
          @click="runTest" 
          class="px-5 py-2.5 bg-primary text-white rounded hover:bg-primary-dark transition-colors flex items-center space-x-2 text-base"
          :disabled="testLoading || !selectedInputNode"
        >
          <span v-if="testLoading" class="w-5 h-5 border-2 border-white border-t-transparent rounded-full animate-spin"></span>
          <span>Run Test</span>
        </button>
        <button @click="closeModal" class="px-5 py-2.5 bg-gray-100 hover:bg-gray-200 rounded transition-colors text-base">
          Close
        </button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, watch, onMounted, computed, onUnmounted } from 'vue';
import { hubApi } from '../api';
import CodeEditor from './CodeEditor.vue';

// Props
const props = defineProps({
  show: Boolean,
  projectId: String
});

// Emits
const emit = defineEmits(['close']);

// Reactive state
const showModal = ref(false);
const inputData = ref('{\n  "timestamp": 1698765432,\n  "event_type": "login",\n  "user_id": "user123",\n  "source_ip": "192.168.1.100",\n  "success": true,\n  "device_info": {\n    "os": "Windows",\n    "browser": "Chrome",\n    "version": "96.0.4664.110"\n  }\n}');
const testResults = ref({});
const testLoading = ref(false);
const testError = ref(null);
const testExecuted = ref(false);
const selectedInputNode = ref('');
const inputNodes = ref([]);
const inputNodesLoading = ref(false);
const projectStructure = ref(null);
const outputResults = ref({});

// Debug info on mount
onMounted(() => {
  console.log('ProjectTestModal mounted with props:', props);
});

// Watch for prop changes
watch(() => props.show, (newVal) => {
  console.log('ProjectTestModal: show prop changed to', newVal);
  showModal.value = newVal;
  if (newVal) {
    // Reset state when opening modal
    resetState();
    // 添加ESC键监听
    document.addEventListener('keydown', handleEscKey);
    // 获取项目输入节点
    fetchProjectInputs();
  } else {
    // 移除ESC键监听
    document.removeEventListener('keydown', handleEscKey);
  }
}, { immediate: true });

watch(() => props.projectId, (newVal) => {
  console.log('ProjectTestModal: projectId prop changed to', newVal);
  // Reset state when project changes
  resetState();
});

// 在组件卸载时移除事件监听
onUnmounted(() => {
  document.removeEventListener('keydown', handleEscKey);
});

// 处理ESC键按下
function handleEscKey(event) {
  if (event.key === 'Escape') {
    closeModal();
  }
}

// Methods
function resetState() {
  testResults.value = {};
  testError.value = null;
  testExecuted.value = false;
  selectedInputNode.value = '';
  inputNodes.value = [];
  projectStructure.value = null;
  outputResults.value = {};
}

function closeModal() {
  emit('close');
}

async function runTest() {
  if (!selectedInputNode.value) {
    testError.value = 'Please select an input node';
    return;
  }
  
  testLoading.value = true;
  testError.value = null;
  testResults.value = {};
  outputResults.value = {};
  testExecuted.value = true;
  
  try {
    // Parse input data
    let data;
    try {
      data = JSON.parse(inputData.value);
    } catch (e) {
      testError.value = `Invalid JSON: ${e.message}`;
      testLoading.value = false;
      return;
    }
    
    // Call API
    const response = await hubApi.testProject(props.projectId, selectedInputNode.value, data);
    
    if (response.success) {
      testResults.value = response;
      outputResults.value = response.outputs || {};
      projectStructure.value = response.structure || null;
      
      // If we have a structure but no input nodes, extract them
      if (projectStructure.value && inputNodes.value.length === 0) {
        extractInputNodes();
      }
    } else {
      testError.value = response.error || 'Unknown error occurred';
    }
  } catch (e) {
    testError.value = e.message || 'Failed to test project';
    console.error('Test project error:', e);
  } finally {
    testLoading.value = false;
  }
}

function extractInputNodes() {
  if (!projectStructure.value || !projectStructure.value.nodes) {
    return;
  }
  
  const inputs = projectStructure.value.nodes.filter(node => node.type === 'input');
  inputNodes.value = inputs.map(node => ({
    id: node.id,
    name: node.name
  }));
  
  // Auto-select the first input node if none is selected
  if (inputNodes.value.length > 0 && !selectedInputNode.value) {
    selectedInputNode.value = inputNodes.value[0].id;
  }
}

// Helper functions for project structure visualization
function getNodeEdges(nodeId) {
  if (!projectStructure.value || !projectStructure.value.edges) {
    return [];
  }
  return projectStructure.value.edges.filter(edge => edge.from === nodeId);
}

function getNodeName(nodeId) {
  if (!projectStructure.value || !projectStructure.value.nodes) {
    return nodeId;
  }
  const node = projectStructure.value.nodes.find(n => n.id === nodeId);
  return node ? node.name : nodeId;
}

function getNodeType(nodeId) {
  if (!projectStructure.value || !projectStructure.value.nodes) {
    return '';
  }
  const node = projectStructure.value.nodes.find(n => n.id === nodeId);
  return node ? node.type : '';
}

function getDataFlowPaths() {
  if (!projectStructure.value || !selectedInputNode.value) {
    return [];
  }
  
  const paths = [];
  const visited = new Set();
  
  function findPaths(currentNode, currentPath) {
    // Add current node to path
    currentPath.push(currentNode);
    
    // Check if this is an output node
    if (getNodeType(currentNode) === 'output') {
      // We found a complete path
      paths.push([...currentPath]);
    } else {
      // Find all edges from this node
      const edges = getNodeEdges(currentNode);
      for (const edge of edges) {
        if (!visited.has(edge.to)) {
          visited.add(edge.to);
          findPaths(edge.to, currentPath);
          visited.delete(edge.to);
        }
      }
    }
    
    // Remove current node from path (backtrack)
    currentPath.pop();
  }
  
  // Start DFS from selected input node
  visited.add(selectedInputNode.value);
  findPaths(selectedInputNode.value, []);
  
  return paths;
}

// 获取项目输入节点
async function fetchProjectInputs() {
  if (!props.projectId) return;
  
  inputNodesLoading.value = true;
  try {
    const response = await hubApi.getProjectInputs(props.projectId);
    
    if (response.success && response.inputs) {
      inputNodes.value = response.inputs;
      
      // 如果有输入节点，自动选择第一个
      if (inputNodes.value.length > 0) {
        selectedInputNode.value = inputNodes.value[0].id;
      }
    } else {
      console.error('Failed to fetch project inputs:', response.error);
    }
  } catch (error) {
    console.error('Error fetching project inputs:', error);
  } finally {
    inputNodesLoading.value = false;
  }
}
</script>

<style scoped>
/* Any component-specific styles can go here */
</style> 