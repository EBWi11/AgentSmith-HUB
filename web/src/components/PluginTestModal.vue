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
        <div class="w-1/2 flex flex-col">
          <div class="px-6 py-3 bg-gray-50 border-b">
            <h3 class="text-sm font-medium text-gray-700">Input Data</h3>
          </div>
          <div class="flex-1 p-4 overflow-hidden" style="height: 400px;">
            <MonacoEditor 
              v-model:value="inputData" 
              :language="'json'" 
              :read-only="false" 
              class="h-full" 
              :error-lines="jsonError ? [{ line: jsonErrorLine }] : []"
              style="height: 100%; min-height: 380px;"
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
              <div class="flex-1 overflow-hidden border border-gray-200 rounded-md" style="height: 400px;">
                <MonacoEditor 
                  :value="JSON.stringify(testResults, null, 2)" 
                  :language="'json'" 
                  :read-only="true" 
                  class="h-full" 
                  style="height: 100%; min-height: 380px;"
                />
              </div>
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
import MonacoEditor from './MonacoEditor.vue';

// Props
const props = defineProps({
  show: Boolean,
  pluginId: String
});

// Emits
const emit = defineEmits(['close']);

// Reactive state
const showModal = ref(false);
const inputData = ref('[\n  {\n    "timestamp": 1698765432,\n    "event_type": "login",\n    "user_id": "user123",\n    "source_ip": "192.168.1.100",\n    "success": true,\n    "device_info": {\n      "os": "Windows",\n      "browser": "Chrome",\n      "version": "96.0.4664.110"\n    }\n  },\n  {\n    "timestamp": 1698765433,\n    "event_type": "logout",\n    "user_id": "user123",\n    "source_ip": "192.168.1.100",\n    "success": true\n  }\n]');
const testResults = ref({});
const testLoading = ref(false);
const testError = ref(null);
const testExecuted = ref(false);
const jsonError = ref(null);
const jsonErrorLine = ref(null);

// Debug info on mount
onMounted(() => {
});

// Watch for prop changes
watch(() => props.show, (newVal) => {
  showModal.value = newVal;
  if (newVal) {
    // Reset state when opening modal
    resetState();
    // Add ESC key listener
    document.addEventListener('keydown', handleEscKey);
  } else {
    // Remove ESC key listener
    document.removeEventListener('keydown', handleEscKey);
  }
}, { immediate: true });

watch(() => props.pluginId, (newVal) => {
  // Reset state when plugin changes
  testResults.value = {};
  testError.value = null;
  testExecuted.value = false;
});

// Remove event listener on component unmount
onUnmounted(() => {
  document.removeEventListener('keydown', handleEscKey);
});

// Handle ESC key press
function handleEscKey(event) {
  if (event.key === 'Escape') {
    closeModal();
  }
}

function resetState() {
  // Reset state when opening modal
  inputData.value = '[\n  {\n    "timestamp": 1698765432,\n    "event_type": "login",\n    "user_id": "user123",\n    "source_ip": "192.168.1.100",\n    "success": true,\n    "device_info": {\n      "os": "Windows",\n      "browser": "Chrome",\n      "version": "96.0.4664.110"\n    }\n  },\n  {\n    "timestamp": 1698765433,\n    "event_type": "logout",\n    "user_id": "user123",\n    "source_ip": "192.168.1.100",\n    "success": true\n  }\n]';
  testResults.value = {};
  testError.value = null;
  testExecuted.value = false;
  jsonError.value = null;
  jsonErrorLine.value = null;
}

// Methods
function closeModal() {
  emit('close');
}

async function runTest() {
  testLoading.value = true;
  testError.value = null;
  testResults.value = {};
  testExecuted.value = true;
  jsonError.value = null;
  jsonErrorLine.value = null;
  
  try {
    // Parse input data
    let data;
    try {
      data = JSON.parse(inputData.value);
    } catch (e) {
      testError.value = `Invalid JSON: ${e.message}`;
      
      // Try to extract line number from JSON parse error
      const match = e.message.match(/line (\d+)/i) || e.message.match(/position (\d+)/i);
      if (match) {
        jsonErrorLine.value = parseInt(match[1]);
        jsonError.value = e.message;
      }
      
      testLoading.value = false;
      return;
    }
    
    // Check if data is array or single object
    if (Array.isArray(data)) {
      // Process array of JSON objects
      let allResults = [];
      
      for (let i = 0; i < data.length; i++) {
        try {
          const response = await hubApi.testPlugin(props.pluginId, data[i]);
          if (response.success) {
            allResults.push({
              index: i + 1,
              input: data[i],
              output: response.result || {}
            });
          } else {
            testError.value = `Error processing item ${i + 1}: ${response.error || 'Unknown error'}`;
            return;
          }
        } catch (e) {
          testError.value = `Error processing item ${i + 1}: ${e.message}`;
          return;
        }
      }
      
      // Set combined results
      testResults.value = {
        arrayProcessed: true,
        itemCount: data.length,
        results: allResults
      };
    } else {
      // Process single JSON object (existing logic)
      const response = await hubApi.testPlugin(props.pluginId, data);
      
      if (response.success) {
        testResults.value = response.result || {};
      } else {
        testError.value = response.error || 'Unknown error occurred';
      }
    }
  } catch (e) {
    testError.value = e.message || 'Failed to test plugin';
  } finally {
    testLoading.value = false;
  }
}
</script>

<style scoped>
/* Any component-specific styles can go here */
</style> 