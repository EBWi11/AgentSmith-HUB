<template>
  <div v-if="showModal" class="fixed inset-0 bg-black bg-opacity-30 flex items-center justify-center z-50">
    <div class="bg-white rounded shadow-lg p-6 w-[1000px] max-h-[90vh] overflow-hidden flex flex-col">
      <div class="flex justify-between items-center mb-4">
        <h3 class="font-bold text-lg">Test Output: {{ outputId }}</h3>
        <button @click="closeModal" class="text-gray-400 hover:text-gray-600">
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
          </svg>
        </button>
      </div>
      
      <div class="flex flex-1 overflow-hidden" style="min-height: 500px;">
        <!-- Left panel: Input data -->
        <div class="w-1/2 pr-3 flex flex-col overflow-hidden">
          <h4 class="text-sm font-medium text-gray-700 mb-2">Input Data:</h4>
          <div class="flex-1 overflow-hidden border border-gray-200 rounded-md" style="height: 400px;">
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
        
        <!-- Right panel: Results -->
        <div class="w-1/2 pl-3 flex flex-col overflow-hidden">
          <h4 class="text-sm font-medium text-gray-700 mb-2">Results:</h4>
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
              <p>No results yet. Click "Run Test" to send data to the output.</p>
            </div>
            
            <div v-else class="space-y-4">
              <div class="bg-white border border-gray-200 rounded-md p-4">
                <h5 class="font-medium text-gray-700 mb-2">Output Type</h5>
                <div class="px-3 py-1 bg-blue-100 text-blue-800 rounded-full inline-block text-sm">
                  {{ testResults.outputType || 'Unknown' }}
                </div>
              </div>
              
              <div class="bg-white border border-gray-200 rounded-md p-4">
                <h5 class="font-medium text-gray-700 mb-2">Metrics</h5>
                <div class="grid grid-cols-1 gap-4">
                  <div class="bg-gray-50 p-3 rounded">
                    <div class="text-xs text-gray-500">Total Messages</div>
                    <div class="text-xl font-semibold">{{ testResults.metrics?.produceTotal || 0 }}</div>
                  </div>
                  <div v-if="testResults.arrayProcessed" class="bg-blue-50 p-3 rounded">
                    <div class="text-xs text-blue-600">Items Processed</div>
                    <div class="text-xl font-semibold text-blue-700">{{ testResults.itemCount || 0 }}</div>
                  </div>
                </div>
              </div>
              
              <div v-if="testResults.isTemp" class="bg-yellow-50 border-l-4 border-yellow-400 p-4">
                <div class="flex">
                  <div class="flex-shrink-0">
                    <svg class="h-5 w-5 text-yellow-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                    </svg>
                  </div>
                  <div class="ml-3">
                    <p class="text-sm text-yellow-700">This output has pending changes. Test was performed using the pending version.</p>
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
          :disabled="testLoading"
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
import { ref, watch, onMounted, onUnmounted } from 'vue';
import { hubApi } from '../api';
import MonacoEditor from './MonacoEditor.vue';

// Props
const props = defineProps({
  show: Boolean,
  outputId: String
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

watch(() => props.outputId, (newVal) => {
  // Reset state when output changes
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
    let totalMessages = 0;
    let allResults = [];
    
    if (Array.isArray(data)) {
      // Process array of JSON objects
      for (let i = 0; i < data.length; i++) {
        try {
          const response = await hubApi.testOutput(props.outputId, data[i]);
          if (response.success) {
            allResults.push(response);
            totalMessages += response.metrics?.produceTotal || 0;
          } else {
            testError.value = `Error processing item ${i + 1}: ${response.error || 'Unknown error'}`;
            return;
          }
        } catch (e) {
          testError.value = `Error processing item ${i + 1}: ${e.message}`;
          return;
        }
      }
      
      // Combine results for array processing
      testResults.value = {
        success: true,
        outputType: allResults[0]?.outputType || 'Unknown',
        metrics: {
          produceTotal: totalMessages
        },
        isTemp: allResults[0]?.isTemp || false,
        arrayProcessed: true,
        itemCount: data.length
      };
    } else {
      // Process single JSON object (existing logic)
      const response = await hubApi.testOutput(props.outputId, data);
      
      if (response.success) {
        testResults.value = response;
      } else {
        testError.value = response.error || 'Unknown error occurred';
      }
    }
  } catch (e) {
    testError.value = e.message || 'Failed to test output';
  } finally {
    testLoading.value = false;
  }
}
</script>

<style scoped>
/* Any component-specific styles can go here */
</style> 