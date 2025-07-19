<template>
  <div v-if="showModal" class="fixed inset-0 bg-black bg-opacity-30 flex items-center justify-center z-50">
    <div class="bg-white rounded shadow-lg p-6 w-[1200px] max-h-[90vh] overflow-hidden flex flex-col">
      <div class="flex justify-between items-center mb-4">
        <h3 class="font-bold text-lg">Test Ruleset: {{ rulesetId }}</h3>
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
              @update:value="onInputDataChange"
            />
          </div>
        </div>
        
        <!-- Right panel: Results -->
        <div class="w-1/2 pl-3 flex flex-col overflow-hidden">
          <h4 class="text-sm font-medium text-gray-700 mb-2">Results:</h4>
          <div class="flex-1 overflow-auto border border-gray-200 rounded-md bg-gray-50" style="max-height: 400px;">
            <div v-if="testLoading" class="flex justify-center items-center h-full">
              <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
            </div>
            
            <div v-else-if="testError" class="bg-red-50 border-l-4 border-red-500 p-4 m-3">
              <div class="flex">
                <div class="flex-shrink-0">
                  <svg class="h-5 w-5 text-red-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                  </svg>
                </div>
                <div class="ml-3 flex-1">
                  <p class="text-sm text-red-700 whitespace-pre-wrap break-all">{{ testError }}</p>
                </div>
              </div>
            </div>
            
            <div v-else-if="testResults.length === 0" class="text-center py-8 px-3 text-gray-500">
              <p>No results yet. Click "Run Test" to execute the ruleset.</p>
              <p v-if="testExecuted" class="mt-2 text-sm text-yellow-600">
                No output was generated. The ruleset may not have matched any rules.
              </p>
            </div>
            
            <div v-else>
              <div v-for="(result, index) in testResults" :key="index" class="p-3">
                <div class="bg-white border border-gray-200 rounded-md">
                  <div class="px-3 py-2 border-b border-gray-100 flex justify-between items-center">
                    <span class="text-sm font-medium text-gray-700">
                      Result {{ index + 1 }}
                    </span>
                    <span v-if="result._HUB_HIT_RULE_ID" class="px-2 py-0.5 bg-green-100 text-green-800 text-xs rounded-full">
                      Rule: {{ result._HUB_HIT_RULE_ID }}
                    </span>
                  </div>
                  <div class="p-3">
                    <pre class="text-xs font-mono text-gray-700 whitespace-pre-wrap break-all">{{ formatJson(result) }}</pre>
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
          class="btn btn-test-ruleset btn-lg"
          :class="{ 'btn-loading': testLoading }"
          :disabled="testLoading"
        >
          <span v-if="testLoading" class="w-5 h-5 border-2 border-white border-t-transparent rounded-full animate-spin"></span>
          <span>Run Test</span>
        </button>
        <button @click="closeModal" class="btn btn-secondary btn-lg">
          Close
        </button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, watch, onMounted, onUnmounted, computed } from 'vue';
import { hubApi } from '../api';
import MonacoEditor from './MonacoEditor.vue';
import JsonViewer from './JsonViewer.vue';
import { useDataCacheStore } from '../stores/dataCache';

// Props
const props = defineProps({
  show: Boolean,
  rulesetId: String,
  rulesetContent: String  // Optional: if provided, test this content instead of saved ruleset
});

// Emits
const emit = defineEmits(['close']);

// Data cache store
const dataCache = useDataCacheStore();

// Reactive state
const showModal = ref(false);
const inputData = ref('');
const testResults = ref([]);
const testLoading = ref(false);
const testError = ref(null);
const testExecuted = ref(false);
const jsonError = ref(null);
const jsonErrorLine = ref(null);

// Default test data
const defaultTestData = '{\n  "data": "test data",\n  "data_type": "59",\n  "exe": "/bin/bash",\n  "pid": "1234",\n  "dip": "192.168.1.1",\n  "sip": "192.168.1.2",\n  "dport": "80",\n  "sport": "12345"\n}';

// Debug info on mount
onMounted(() => {
});

// Watch for prop changes
watch(() => props.show, async (newVal) => {
  showModal.value = newVal;
  if (newVal) {
    // Reset state when opening modal but preserve test data
    await resetState();
    // Add ESC key listener
    document.addEventListener('keydown', handleEscKey);
  } else {
    // Remove ESC key listener
    document.removeEventListener('keydown', handleEscKey);
  }
}, { immediate: true });

watch(() => props.rulesetId, async (newVal) => {
  if (newVal) {
    // Load cached test data for this ruleset using unified cache
    const cachedData = dataCache.getTestCache('rulesets', newVal);
    if (cachedData) {
      inputData.value = cachedData;
    } else {
      // Try to get sample data from backend
      try {
        // Ensure we don't duplicate the 'ruleset.' prefix
        const projectNodeSequence = newVal.startsWith('ruleset.') ? newVal : `ruleset.${newVal}`;
        const sampleDataResponse = await hubApi.getSamplerData('ruleset', projectNodeSequence);
        if (sampleDataResponse && sampleDataResponse.ruleset && Object.keys(sampleDataResponse.ruleset).length > 0) {
          // Extract the first sample data from the response
          let firstSampleData = null;
          for (const [flowPath, samples] of Object.entries(sampleDataResponse.ruleset)) {
            if (Array.isArray(samples) && samples.length > 0) {
              // Take only the first sample from the first flow path that has data
              const firstSample = samples[0];
              if (firstSample && firstSample.data) {
                firstSampleData = firstSample.data;
                break; // Stop after finding the first sample
              }
            }
          }
          
          if (firstSampleData) {
            // Use the first sample data as test data
            inputData.value = JSON.stringify(firstSampleData, null, 2);
            console.log(`Loaded 1 sample for ruleset ${newVal}`);
          } else {
            inputData.value = defaultTestData;
          }
        } else {
          inputData.value = defaultTestData;
        }
      } catch (error) {
        console.warn(`Failed to load sample data for ruleset ${newVal}:`, error);
        inputData.value = defaultTestData;
      }
    }
  }
  // Reset other state when ruleset changes
  testResults.value = [];
  testError.value = null;
  testExecuted.value = false;
});

// Remove event listener on component unmount
onUnmounted(() => {
  document.removeEventListener('keydown', handleEscKey);
});

// Methods
function closeModal() {
  emit('close');
}

// Handle ESC key press
function handleEscKey(event) {
  if (event.key === 'Escape') {
    closeModal();
  }
}

async function runTest() {
  testLoading.value = true;
  testError.value = null;
  testResults.value = [];
  testExecuted.value = true;
  jsonError.value = null;
  jsonErrorLine.value = null;
  
  // Debug: Check parameters
  console.log('DEBUG: runTest called with:', {
    rulesetId: props.rulesetId,
    rulesetContent: props.rulesetContent,
    rulesetContentType: typeof props.rulesetContent,
    rulesetContentLength: props.rulesetContent?.length || 0,
    willUseContentAPI: props.rulesetContent !== undefined
  });
  
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
            let response;
            // Use editor content if available and not empty, otherwise fall back to saved file
            if (props.rulesetContent !== undefined && props.rulesetContent.trim() !== '') {
              response = await hubApi.testRulesetContent(props.rulesetContent, data[i]);
            } else {
              response = await hubApi.testRuleset(props.rulesetId, data[i]);
            }
          
          if (response.success) {
            // Add results from this item
            const itemResults = response.results || [];
            allResults.push(...itemResults);
          } else {
            testError.value = `Error processing item ${i + 1}: ${response.error || 'Unknown error'}`;
            return;
          }
        } catch (e) {
          testError.value = `Error processing item ${i + 1}: ${e.message}`;
          return;
        }
      }
      
      testResults.value = allResults;
    } else {
      // Process single JSON object (existing logic)
      let response;
      // Use editor content if available and not empty, otherwise fall back to saved file
      if (props.rulesetContent !== undefined && props.rulesetContent.trim() !== '') {
        response = await hubApi.testRulesetContent(props.rulesetContent, data);
      } else {
        response = await hubApi.testRuleset(props.rulesetId, data);
      }
      
      if (response.success) {
        testResults.value = response.results || [];
      } else {
        testError.value = response.error || 'Unknown error occurred';
      }
    }
  } catch (e) {
    testError.value = e.message || 'Failed to test ruleset';
  } finally {
    testLoading.value = false;
  }
}

function formatTestResult() {
  if (testError.value) {
    return testError.value;
  } else if (testResults.value.length === 0) {
    return 'No results yet. Click "Run Test" to execute the ruleset.';
  } else {
    return testResults.value.map(result => JSON.stringify(result, null, 2)).join('\n');
  }
}

// Save test data when it changes
function onInputDataChange(newValue) {
  if (props.rulesetId) {
    dataCache.setTestCache('rulesets', props.rulesetId, newValue);
  }
}

// Format JSON for display
function formatJson(obj) {
  try {
    return JSON.stringify(obj, null, 2);
  } catch (e) {
    return String(obj);
  }
}

async function resetState() {
  // Load cached test data or use default using unified cache
  if (props.rulesetId) {
    const cachedData = dataCache.getTestCache('rulesets', props.rulesetId);
    if (cachedData) {
      inputData.value = cachedData;
    } else {
      // Try to get sample data from backend
      try {
        // Ensure we don't duplicate the 'ruleset.' prefix
        const projectNodeSequence = props.rulesetId.startsWith('ruleset.') ? props.rulesetId : `ruleset.${props.rulesetId}`;
        const sampleDataResponse = await hubApi.getSamplerData('ruleset', projectNodeSequence);
        if (sampleDataResponse && sampleDataResponse.ruleset && Object.keys(sampleDataResponse.ruleset).length > 0) {
          // Extract the first sample data from the response
          let firstSampleData = null;
          for (const [flowPath, samples] of Object.entries(sampleDataResponse.ruleset)) {
            if (Array.isArray(samples) && samples.length > 0) {
              // Take only the first sample from the first flow path that has data
              const firstSample = samples[0];
              if (firstSample && firstSample.data) {
                firstSampleData = firstSample.data;
                break; // Stop after finding the first sample
              }
            }
          }
          
          if (firstSampleData) {
            // Use the first sample data as test data
            inputData.value = JSON.stringify(firstSampleData, null, 2);
            console.log(`Loaded 1 sample for ruleset ${props.rulesetId}`);
          } else {
            inputData.value = defaultTestData;
          }
        } else {
          inputData.value = defaultTestData;
        }
      } catch (error) {
        console.warn(`Failed to load sample data for ruleset ${props.rulesetId}:`, error);
        inputData.value = defaultTestData;
      }
    }
  } else {
    inputData.value = defaultTestData;
  }
  
  testResults.value = [];
  testError.value = null;
  testExecuted.value = false;
  jsonError.value = null;
  jsonErrorLine.value = null;
}
</script>

<style scoped>
/* Any component-specific styles can go here */
</style> 