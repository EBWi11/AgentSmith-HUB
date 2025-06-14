<template>
  <div v-if="showModal" class="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
    <div class="bg-white rounded-lg shadow-xl w-4/5 max-w-4xl h-4/5 flex flex-col">
      <div class="flex justify-between items-center px-6 py-4 border-b">
        <h2 class="text-xl font-semibold text-gray-800">Test Plugin</h2>
        <button @click="closeModal" class="text-gray-500 hover:text-gray-700">
          <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
          </svg>
        </button>
      </div>
      
      <div class="flex flex-1 overflow-hidden">
        <!-- Input Panel -->
        <div class="w-1/2 flex flex-col border-r">
          <div class="px-6 py-3 bg-gray-50 border-b">
            <h3 class="text-sm font-medium text-gray-700">Input Data</h3>
          </div>
          <div class="flex-1 p-4 overflow-auto">
            <CodeEditor 
              v-model:value="inputData" 
              language="json" 
              :read-only="false" 
              class="h-full" 
            />
          </div>
        </div>
        
        <!-- Output Panel -->
        <div class="w-1/2 flex flex-col">
          <div class="px-6 py-3 bg-gray-50 border-b">
            <h3 class="text-sm font-medium text-gray-700">Output Results</h3>
          </div>
          <div class="flex-1 p-4 overflow-auto">
            <div v-if="testLoading" class="flex items-center justify-center h-full">
              <div class="animate-spin rounded-full h-10 w-10 border-b-2 border-blue-500"></div>
            </div>
            <div v-else-if="testError" class="bg-red-50 border-l-4 border-red-500 p-4 h-full overflow-auto">
              <div class="text-red-700 font-medium">Error</div>
              <pre class="text-red-600 text-sm mt-2 whitespace-pre-wrap">{{ testError }}</pre>
            </div>
            <div v-else-if="testExecuted && Object.keys(testResults).length > 0" class="h-full overflow-auto">
              <CodeEditor 
                :value="JSON.stringify(testResults, null, 2)" 
                language="json" 
                :read-only="true" 
                class="h-full" 
              />
            </div>
            <div v-else-if="testExecuted" class="flex items-center justify-center h-full text-gray-400">
              No results returned
            </div>
            <div v-else class="flex items-center justify-center h-full text-gray-400">
              Run test to see results
            </div>
          </div>
        </div>
      </div>
      
      <div class="px-6 py-4 border-t flex justify-end space-x-3">
        <button 
          @click="runTest" 
          class="px-5 py-2.5 bg-blue-500 hover:bg-blue-600 text-white rounded transition-colors text-base focus:outline-none focus:ring-2 focus:ring-blue-300 flex items-center"
          :disabled="testLoading"
        >
          <span v-if="testLoading" class="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin mr-2"></span>
          <span>{{ testLoading ? 'Running...' : 'Run Test' }}</span>
        </button>
        <button @click="closeModal" class="px-5 py-2.5 bg-gray-100 hover:bg-gray-200 rounded transition-colors text-base">
          Close
        </button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, watch, onMounted, onUnmounted } from 'vue';
import { hubApi } from '../api';
import CodeEditor from './CodeEditor.vue';

// Props
const props = defineProps({
  show: Boolean,
  pluginId: String
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

// Debug info on mount
onMounted(() => {
  console.log('PluginTestModal mounted with props:', props);
});

// Watch for prop changes
watch(() => props.show, (newVal) => {
  console.log('PluginTestModal: show prop changed to', newVal);
  showModal.value = newVal;
  
  // 添加或移除ESC键监听
  if (newVal) {
    document.addEventListener('keydown', handleEscKey);
  } else {
    document.removeEventListener('keydown', handleEscKey);
  }
}, { immediate: true });

watch(() => props.pluginId, (newVal) => {
  console.log('PluginTestModal: pluginId prop changed to', newVal);
  // Reset state when plugin changes
  testResults.value = {};
  testError.value = null;
  testExecuted.value = false;
});

// 在组件卸载时移除事件监听
onUnmounted(() => {
  document.removeEventListener('keydown', handleEscKey);
});

// Methods
function closeModal() {
  emit('close');
}

// 处理ESC键按下
function handleEscKey(event) {
  if (event.key === 'Escape') {
    closeModal();
  }
}

async function runTest() {
  testLoading.value = true;
  testError.value = null;
  testResults.value = {};
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
    const response = await hubApi.testPlugin(props.pluginId, data);
    
    if (response.success) {
      testResults.value = response.result || {};
    } else {
      testError.value = response.error || 'Unknown error occurred';
    }
  } catch (e) {
    testError.value = e.message || 'Failed to test plugin';
    console.error('Test plugin error:', e);
  } finally {
    testLoading.value = false;
  }
}
</script>

<style scoped>
/* Any component-specific styles can go here */
</style> 