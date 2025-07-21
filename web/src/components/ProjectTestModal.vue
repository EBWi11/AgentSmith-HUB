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
        <div class="w-1/2 pr-3 flex flex-col overflow-hidden">
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
          <div class="flex-1 overflow-hidden border border-gray-200 rounded-md" style="height: 520px;">
            <MonacoEditor 
              v-model:value="inputData" 
              :language="'json'" 
              :read-only="false" 
              class="h-full" 
              :error-lines="jsonError ? [{ line: jsonErrorLine }] : []"
              style="height: 100%; min-height: 500px;"
              @update:value="onInputDataChange"
            />
          </div>
        </div>
        
        <!-- Right panel: Output Results -->
        <div class="w-1/2 pl-3 flex flex-col overflow-hidden">
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
            
            <div v-else class="space-y-6">
              <div v-for="(results, outputName) in outputResults" :key="outputName">
                <div class="flex justify-between items-center mb-3">
                  <h5 class="font-medium text-gray-700 text-base">Output: {{ outputName }}</h5>
                  <span class="px-2 py-1 bg-green-100 text-green-800 text-sm rounded">
                    {{ results.length }} message(s)
                  </span>
                </div>
                
                <div v-if="results.length === 0" class="text-sm text-gray-500 italic bg-gray-50 p-3 rounded">
                  No messages received yet. Output components send data to external systems in test mode.
                </div>
                
                <div v-else class="space-y-3">
                  <div v-for="(result, index) in results" :key="index" class="bg-gray-50 rounded p-3">
                    <div v-if="results.length > 1" class="text-xs text-gray-500 mb-2">Message {{ index + 1 }}</div>
                    <JsonViewer :value="cleanResult(result)" height="auto" />
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
          class="btn btn-test-project btn-md"
          :disabled="testLoading || !selectedInputNode"
        >
          <span v-if="testLoading" class="w-4 h-4 border-2 border-current border-t-transparent rounded-full animate-spin mr-2"></span>
          Run Test
        </button>
        <button @click="closeModal" class="btn btn-secondary btn-md">
          Close
        </button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, watch, onMounted, computed, onUnmounted } from 'vue';
import { hubApi } from '../api';
import MonacoEditor from './MonacoEditor.vue';
import JsonViewer from './JsonViewer.vue';
import { useDataCacheStore } from '../stores/dataCache';

// Props
const props = defineProps({
  show: Boolean,
  projectId: String,
  projectContent: String  // Optional: if provided, test this content instead of saved project
});

// Emits
const emit = defineEmits(['close']);

// Data cache store
const dataCache = useDataCacheStore();

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
const outputResults = ref({});
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
    // Fetch project input nodes
    fetchProjectInputs();
  } else {
    // Remove ESC key listener
    document.removeEventListener('keydown', handleEscKey);
  }
}, { immediate: true });

watch(() => props.projectId, (newVal, oldVal) => {
    // Reset state when project changes
  if (newVal !== oldVal) {
    resetState();
    // Fetch project input nodes for the new project
    if (showModal.value) {
      fetchProjectInputs();
    }
  }
});

// Save test data when it changes
function onInputDataChange(newValue) {
  if (props.projectId && newValue && newValue.trim() !== '') {
    const cachedData = dataCache.getTestCache('projects', props.projectId) || {};
    cachedData.inputData = newValue;
    dataCache.setTestCache('projects', props.projectId, cachedData);
  }
}

// Watch for input data changes and save to cache using unified cache
watch(inputData, (newVal) => {
  if (props.projectId && newVal) {
    const cachedData = dataCache.getTestCache('projects', props.projectId) || {};
    cachedData.inputData = newVal;
    dataCache.setTestCache('projects', props.projectId, cachedData);
  }
});

// Watch for selected input node changes and save to cache using unified cache
watch(selectedInputNode, async (newVal, oldVal) => {
  if (props.projectId && newVal && newVal !== oldVal) {
    const cachedData = dataCache.getTestCache('projects', props.projectId) || {};
    cachedData.selectedInputNode = newVal;
    dataCache.setTestCache('projects', props.projectId, cachedData);
    
    // Only load sample data if we don't have cached input data for this input node
    const hasCachedInputData = cachedData.inputData && cachedData.inputData.trim() !== '';
    if (!hasCachedInputData) {
      // Try to get sample data for the selected input
      try {
        // Ensure we don't duplicate the 'input.' prefix
        const projectNodeSequence = newVal.startsWith('input.') ? newVal : `input.${newVal}`;
        const sampleDataResponse = await hubApi.getSamplerData('input', projectNodeSequence);
        if (sampleDataResponse && sampleDataResponse.input && Object.keys(sampleDataResponse.input).length > 0) {
          // Extract the first sample data from the response
          let firstSampleData = null;
          for (const [flowPath, samples] of Object.entries(sampleDataResponse.input)) {
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
            // Convert sample data to JSON string for the editor
            const sampleJson = JSON.stringify(firstSampleData, null, 2);
            inputData.value = sampleJson;
            
            // Update cache with the sample data
            cachedData.inputData = sampleJson;
            dataCache.setTestCache('projects', props.projectId, cachedData);
          }
        }
      } catch (error) {
        // If sample data fetch fails, provide default sample data
        
        // Provide default sample data based on input type
        const defaultSampleData = {
          "timestamp": Math.floor(Date.now() / 1000),
          "event_type": "test_event",
          "user_id": "test_user",
          "source_ip": "192.168.1.100",
          "success": true,
          "device_info": {
            "os": "Windows",
            "browser": "Chrome",
            "version": "96.0.4664.110"
          },
          "message": "Default test data for project testing"
        };
        
        const sampleJson = JSON.stringify(defaultSampleData, null, 2);
        inputData.value = sampleJson;
        
        // Update cache with the default sample data
        cachedData.inputData = sampleJson;
        dataCache.setTestCache('projects', props.projectId, cachedData);
      }
    }
  }
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

// Methods
function resetState() {
  testResults.value = {};
  testError.value = null;
  testExecuted.value = false;
  outputResults.value = {};
  jsonError.value = null;
  jsonErrorLine.value = null;
  
  // Clear input nodes first
  inputNodes.value = [];
  selectedInputNode.value = '';
  
  // Reset input data to default
  inputData.value = '{\n  "timestamp": 1698765432,\n  "event_type": "login",\n  "user_id": "user123",\n  "source_ip": "192.168.1.100",\n  "success": true,\n  "device_info": {\n    "os": "Windows",\n    "browser": "Chrome",\n    "version": "96.0.4664.110"\n  }\n}';
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
  // Don't reset jsonError and jsonErrorLine here - let them be reset when needed
  
  try {
    // Parse input data
    let data;
    try {
      data = JSON.parse(inputData.value);
      // Clear JSON errors on successful parse
      jsonError.value = null;
      jsonErrorLine.value = null;
    } catch (e) {
      testError.value = `Invalid JSON: ${e.message}`;
      
      // Extract line number from JSON parse error
      const errorLine = extractErrorLine(e.message, inputData.value);
      if (errorLine) {
        jsonErrorLine.value = errorLine;
        jsonError.value = e.message;
      }
      
      testLoading.value = false;
      return;
    }
    
    // Check if data is array or single object
    if (Array.isArray(data)) {
      // Process array of JSON objects
      let allOutputResults = {};
      let totalProcessed = 0;
      
      for (let i = 0; i < data.length; i++) {
        try {
          let response;
          if (props.projectContent) {
            // Test with editor content
            response = await hubApi.testProjectContent(props.projectContent, selectedInputNode.value, data[i]);
          } else {
            // Test with saved project
            response = await hubApi.testProject(props.projectId, selectedInputNode.value, data[i]);
          }
          
          if (response.success) {
            totalProcessed++;
            
            // Merge output results
            if (response.outputs) {
              Object.keys(response.outputs).forEach(outputId => {
                if (!allOutputResults[outputId]) {
                  allOutputResults[outputId] = [];
                }
                if (response.outputs[outputId]) {
                  allOutputResults[outputId].push(...response.outputs[outputId]);
                }
              });
            }
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
        success: true,
        arrayProcessed: true,
        itemCount: data.length,
        totalProcessed: totalProcessed
      };
      outputResults.value = allOutputResults;
    } else {
      // Process single JSON object
      let response;
      if (props.projectContent) {
        // Test with editor content
        response = await hubApi.testProjectContent(props.projectContent, selectedInputNode.value, data);
      } else {
        // Test with saved project
        response = await hubApi.testProject(props.projectId, selectedInputNode.value, data);
      }
      
      if (response.success) {
        testResults.value = response;
        outputResults.value = response.outputs || {};
      } else {
        const backendError = response.error || 'Unknown error occurred';
        testError.value = backendError;
        
        // Extract line number from backend YAML error
        const errorLine = extractErrorLine(backendError, props.projectContent || '');
        if (errorLine) {
          jsonErrorLine.value = errorLine;
          jsonError.value = backendError;
        }
      }
    }
  } catch (e) {
    testError.value = e.message || 'Failed to test project';
  } finally {
    testLoading.value = false;
  }
}

// Extract line number from error message (JSON or YAML)
function extractErrorLine(errorMessage, sourceContent = '') {
  if (!errorMessage) return null;
  
  // Handle YAML errors from backend: "yaml-line X:", "yaml: line X:"
  const yamlLineMatch = errorMessage.match(/yaml[:-]?line\s+(\d+)/i);
  if (yamlLineMatch) {
    return parseInt(yamlLineMatch[1]);
  }
  
  // Handle general line format: "line X", "at line X"
  const lineMatch = errorMessage.match(/(?:at\s+)?line\s+(\d+)/i);
  if (lineMatch) {
    return parseInt(lineMatch[1]);
  }
  
  // Handle position-based errors: "position X"
  const posMatch = errorMessage.match(/position\s+(\d+)/i);
  if (posMatch && sourceContent) {
    const position = parseInt(posMatch[1]);
    const lines = sourceContent.substring(0, position).split('\n');
    return lines.length;
  }
  
  return null;
}

// Clean result data for better display
function cleanResult(result) {
  if (!result || typeof result !== 'object') {
    return result;
  }
  
  const cleaned = { ...result };
  
  // Remove or simplify internal technical fields
  delete cleaned._hub_output_timestamp;
  
  // Simplify project node sequence to just show the flow path
  if (cleaned._hub_project_node_sequence) {
    const pns = cleaned._hub_project_node_sequence;
    // Extract meaningful parts: INPUT.name -> RULESET.name -> OUTPUT.name
    const parts = pns.split('.');
    const meaningfulParts = [];
    
    for (let i = 0; i < parts.length; i++) {
      if (parts[i] === 'INPUT' && i + 1 < parts.length) {
        meaningfulParts.push(`Input: ${parts[i + 1]}`);
        i++; // Skip the next part as we already used it
      } else if (parts[i] === 'RULESET' && i + 1 < parts.length) {
        meaningfulParts.push(`Ruleset: ${parts[i + 1]}`);
        i++; // Skip the next part as we already used it
      } else if (parts[i] === 'OUTPUT' && i + 1 < parts.length) {
        meaningfulParts.push(`Output: ${parts[i + 1]}`);
        i++; // Skip the next part as we already used it
      }
    }
    
    if (meaningfulParts.length > 0) {
      cleaned._flow_path = meaningfulParts.join(' → ');
    }
    delete cleaned._hub_project_node_sequence;
  }
  
  // Clean up rule hit information
  if (cleaned._hub_hit_rule_id) {
    cleaned._matched_rule = cleaned._hub_hit_rule_id;
    delete cleaned._hub_hit_rule_id;
  }
  
  // Clean up input information
  if (cleaned._hub_input) {
    cleaned._input_source = cleaned._hub_input;
    delete cleaned._hub_input;
  }
  
  return cleaned;
}

// Fetch project input nodes
async function fetchProjectInputs() {
  if (!props.projectId) return;
  
  inputNodesLoading.value = true;
  try {
    let response;
    if (props.projectContent) {
      // For editor content, we need to parse the project content to get input nodes
      // This is a simplified approach - in a real implementation, you might want to
      // create a separate API endpoint that accepts project content
      response = await hubApi.getProjectInputs(props.projectId);
    } else {
      response = await hubApi.getProjectInputs(props.projectId);
    }
    
    if (response.success && response.inputs) {
      inputNodes.value = response.inputs;
      
      // If there are input nodes, auto-select the first one or restore cached selection
      if (inputNodes.value.length > 0) {
        // Try to restore cached input node selection
        const cachedData = dataCache.getTestCache('projects', props.projectId);
        if (cachedData && cachedData.selectedInputNode) {
          // Check if the cached input node still exists in the current project
          const cachedNodeExists = inputNodes.value.some(node => node.id === cachedData.selectedInputNode);
          if (cachedNodeExists) {
            selectedInputNode.value = cachedData.selectedInputNode;
            // Restore cached input data if available
            if (cachedData.inputData && cachedData.inputData.trim() !== '') {
              inputData.value = cachedData.inputData;
              return;
            }
          }
        }
        
        // If no cached selection or cached node doesn't exist, select the first one
        selectedInputNode.value = inputNodes.value[0].id;
        
        // Try to get sample data for the first input node
        try {
          // Ensure we don't duplicate the 'input.' prefix
          const projectNodeSequence = inputNodes.value[0].id.startsWith('input.') ? inputNodes.value[0].id : `input.${inputNodes.value[0].id}`;
          const sampleDataResponse = await hubApi.getSamplerData('input', projectNodeSequence);
          if (sampleDataResponse && sampleDataResponse.input && Object.keys(sampleDataResponse.input).length > 0) {
            // Extract the first sample data from the response
            let firstSampleData = null;
            for (const [flowPath, samples] of Object.entries(sampleDataResponse.input)) {
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
              // Convert sample data to JSON string for the editor
              const sampleJson = JSON.stringify(firstSampleData, null, 2);
              inputData.value = sampleJson;
              
              // Update cache with the sample data
              const cachedData = dataCache.getTestCache('projects', props.projectId) || {};
              cachedData.inputData = sampleJson;
              dataCache.setTestCache('projects', props.projectId, cachedData);
            }
          }
        } catch (error) {
          // If sample data fetch fails, provide default sample data
          
          // Provide default sample data based on input type
          const defaultSampleData = {
            "timestamp": Math.floor(Date.now() / 1000),
            "event_type": "test_event",
            "user_id": "test_user",
            "source_ip": "192.168.1.100",
            "success": true,
            "device_info": {
              "os": "Windows",
              "browser": "Chrome",
              "version": "96.0.4664.110"
            },
            "message": "Default test data for project testing"
          };
          
          const sampleJson = JSON.stringify(defaultSampleData, null, 2);
          inputData.value = sampleJson;
          
          // Update cache with the default sample data
          const cachedData = dataCache.getTestCache('projects', props.projectId) || {};
          cachedData.inputData = sampleJson;
          dataCache.setTestCache('projects', props.projectId, cachedData);
        }
      }
    } else {
      // Handle error case
    }
  } catch (error) {
    // Handle error case
  } finally {
    inputNodesLoading.value = false;
  }
}
</script>

<style scoped>
/* Any component-specific styles can go here */
</style> 