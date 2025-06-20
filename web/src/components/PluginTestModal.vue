<template>
  <div v-if="showModal" class="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
    <div class="bg-white rounded-lg shadow-xl w-[800px] max-h-[80vh] flex flex-col">
      <div class="flex justify-between items-center px-6 py-4 border-b">
        <h2 class="text-xl font-semibold text-gray-800">Test Plugin: {{ pluginId }}</h2>
        <button @click="closeModal" class="text-gray-500 hover:text-gray-700">
          <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
          </svg>
        </button>
      </div>
      
      <div class="flex-1 p-6 overflow-y-auto">
        <!-- Plugin Arguments Input -->
        <div class="mb-6">
          <h3 class="text-lg font-medium text-gray-800 mb-4">Plugin Arguments</h3>
          <div class="space-y-3">
            <div v-for="(arg, index) in pluginArgs" :key="index" class="flex items-center space-x-3">
              <div class="flex-1">
                <label class="block text-sm font-medium text-gray-700 mb-1">
                  Argument {{ index + 1 }}
                </label>
                <input 
                  v-model="arg.value" 
                  :placeholder="`Enter argument ${index + 1} value...`"
                  class="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                />
                <div class="text-xs text-gray-500 mt-1">
                  String, number, or boolean value
                </div>
              </div>
              <button 
                @click="removePluginArg(index)" 
                class="btn btn-icon btn-danger-ghost"
                :disabled="pluginArgs.length === 1"
              >
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"></path>
                </svg>
              </button>
            </div>
          </div>
          
          <button @click="addPluginArg" class="btn btn-secondary-ghost btn-sm mt-3">
            <svg class="w-4 h-4 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6"></path>
            </svg>
            Add Argument
          </button>
        </div>

        <!-- Test Results -->
        <div v-if="testExecuted" class="mb-6">
          <h3 class="text-lg font-medium text-gray-800 mb-4">Test Results</h3>
          
          <div v-if="testLoading" class="flex items-center justify-center py-8">
            <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500"></div>
            <span class="ml-3 text-gray-600">Running test...</span>
          </div>
          
          <div v-else-if="testError" class="bg-red-50 border-l-4 border-red-500 p-4 rounded-md">
            <div class="text-red-700 font-medium mb-2">Error</div>
            <pre class="text-red-600 text-sm whitespace-pre-wrap">{{ testError }}</pre>
          </div>
          
          <div v-else class="bg-gray-50 border border-gray-200 rounded-md p-4">
            <div class="mb-3">
              <div class="text-sm font-medium text-gray-700 mb-2">Status:</div>
              <span :class="testResults.success ? 'text-green-600 bg-green-100' : 'text-red-600 bg-red-100'" 
                    class="px-2 py-1 rounded text-sm font-medium">
                {{ testResults.success ? 'Success' : 'Failed' }}
              </span>
            </div>
            
            <div v-if="testResults.result !== null && testResults.result !== undefined">
              <div class="text-sm font-medium text-gray-700 mb-2">Result:</div>
              <div class="bg-white border border-gray-200 rounded p-3">
                <pre class="text-gray-800 text-sm whitespace-pre-wrap">{{ formatResult(testResults.result) }}</pre>
              </div>
            </div>
            
            <div v-else class="text-gray-500 italic text-sm">
              No result value returned
            </div>
          </div>
        </div>
        
        <div v-else class="mb-6">
          <div class="text-center py-8 text-gray-400">
            Configure arguments and run test to see results
          </div>
        </div>
      </div>
      
      <div class="px-6 py-4 border-t flex justify-end space-x-3">
        <button 
          @click="runTest" 
          class="btn btn-test-plugin btn-md"
          :disabled="testLoading"
        >
          <span v-if="testLoading" class="w-4 h-4 border-2 border-current border-t-transparent rounded-full animate-spin mr-2"></span>
          {{ testLoading ? 'Running...' : 'Run Test' }}
        </button>
        <button @click="closeModal" class="btn btn-secondary btn-md">
          Close
        </button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, watch, onMounted, onUnmounted } from 'vue';
import { hubApi } from '../api';

// Props
const props = defineProps({
  show: Boolean,
  pluginId: String
});

// Emits
const emit = defineEmits(['close']);

// Reactive state
const showModal = ref(false);
const pluginArgs = ref([{ value: '' }]);
const testResults = ref({});
const testLoading = ref(false);
const testError = ref(null);
const testExecuted = ref(false);

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
  resetState();
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
  pluginArgs.value = [{ value: '' }];
  testResults.value = {};
  testError.value = null;
  testExecuted.value = false;
  testLoading.value = false;
}

// Methods
function closeModal() {
  emit('close');
}

// Add plugin argument
function addPluginArg() {
  pluginArgs.value.push({ value: '' });
}

// Remove plugin argument
function removePluginArg(index) {
  if (pluginArgs.value.length > 1) {
    pluginArgs.value.splice(index, 1);
  }
}

// Format result for display
function formatResult(result) {
  if (typeof result === 'object') {
    return JSON.stringify(result, null, 2);
  }
  return String(result);
}

async function runTest() {
  testLoading.value = true;
  testError.value = null;
  testResults.value = {};
  testExecuted.value = true;
  
  try {
    // Process parameter values, try to convert to appropriate types
    const args = pluginArgs.value.map(arg => {
      const value = arg.value.trim();
      if (value === '') return null;
      if (value === 'true') return true;
      if (value === 'false') return false;
      if (!isNaN(value)) return Number(value);
      return value;
    });
    
    const result = await hubApi.testPlugin(props.pluginId, args);
    testResults.value = result;
    
    // Handle error message
    if (result.error) {
      testError.value = result.error;
    }
  } catch (error) {
    testError.value = error.message || 'Failed to test plugin';
    testResults.value = { 
      success: false, 
      result: null,
      error: error.message || 'Unknown error occurred'
    };
  } finally {
    testLoading.value = false;
  }
}
</script>

<style scoped>
/* Any component-specific styles can go here */
</style> 