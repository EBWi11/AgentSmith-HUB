<template>
  <div class="h-full flex flex-col bg-white">
    <!-- Header -->
    <div class="flex items-center justify-between p-4 border-b border-gray-200">
      <h1 class="text-xl font-semibold text-gray-900">Load Local Components</h1>
      <div class="flex items-center space-x-2">
        <button 
          @click="refreshChanges" 
          :disabled="loading"
          class="px-3 py-1.5 text-sm border border-gray-300 rounded-md hover:bg-gray-50 disabled:opacity-50"
        >
          Refresh
        </button>
        <button 
          @click="verifyChanges" 
          :disabled="!changes.length || verifying"
          class="px-3 py-1.5 text-sm border border-gray-300 rounded-md hover:bg-gray-50 disabled:opacity-50"
        >
          {{ verifying ? 'Verifying...' : 'Verify' }}
        </button>
        <button 
          @click="loadAllChanges" 
          :disabled="!changes.length || loading"
          class="px-3 py-1.5 text-sm bg-blue-600 text-white rounded-md hover:bg-blue-700 disabled:opacity-50"
        >
          {{ loading ? 'Loading...' : 'Load All Changes' }}
        </button>
      </div>
    </div>

    <!-- Content -->
    <div class="flex-1 overflow-y-auto">
      <div v-if="loading && !changes.length" class="flex items-center justify-center h-64">
        <div class="text-gray-500">Loading local changes...</div>
      </div>
      
      <div v-else-if="error" class="p-4 bg-red-50 border border-red-200 text-red-700 text-sm">
        {{ error }}
      </div>
      
      <div v-else-if="!changes.length" class="flex items-center justify-center h-64">
        <div class="text-gray-500">No local changes detected</div>
      </div>
      
      <div v-else class="space-y-4 p-4">
        <div v-for="change in changes" :key="`${change.type}-${change.id}`" class="border border-gray-200 rounded-lg overflow-hidden">
          <div class="flex items-center justify-between p-3 bg-gray-50 border-b border-gray-200">
            <div class="flex items-center space-x-3">
              <h3 class="font-medium text-gray-900">
                {{ getComponentTypeLabel(change.type) }}: {{ change.id }}
                <span class="ml-2 text-xs px-2 py-1 rounded" :class="getChangeStatusClass(change)">
                  {{ getChangeStatusLabel(change) }}
                </span>
              </h3>
              <div v-if="change.verifyStatus === 'success'" class="text-xs text-green-600">
                ✓ Verified
              </div>
              <div v-else-if="change.verifyStatus === 'error'" class="text-xs text-red-600">
                ✗ Verification Failed
              </div>
            </div>
            <div class="flex items-center space-x-2">
              <button 
                v-if="change.has_local"
                @click="verifySingleChange(change)" 
                :disabled="verifying"
                class="px-2 py-1 text-xs border border-gray-300 rounded hover:bg-gray-50 disabled:opacity-50"
              >
                Verify
              </button>
              <button 
                @click="loadSingleChange(change)" 
                :disabled="change.verifyStatus === 'error' || loading"
                :class="getLoadButtonClass(change)"
              >
                {{ getLoadButtonText(change) }}
              </button>
            </div>
          </div>
          
          <div class="bg-gray-100" style="padding: 0; margin: 0;">
            <div v-if="change.verifyError" class="p-2 bg-red-50 border border-red-200 text-red-700 text-xs" style="margin: 0 0 8px 0;">
              {{ change.verifyError }}
            </div>
            
            <div class="bg-white" style="margin: 0; padding: 0; border: none; border-radius: 0; overflow: hidden;">
              <!-- New local file: show full content -->
              <div v-if="change.has_local && !change.has_memory" style="height: 400px; margin: 0; padding: 0; border: none;">
                <MonacoEditor 
                  :key="`full-${change.type}-${change.id}`"
                  :value="change.local_content || ''" 
                  :language="getEditorLanguage(change.type)" 
                  :read-only="true" 
                  :error-lines="change.errorLine ? [{ line: change.errorLine }] : []"
                  :diff-mode="false"
                  style="height: 100%; width: 100%; margin: 0; padding: 0; border: none;"
                />
              </div>
              <!-- Modified file or deleted file: use diff mode -->
              <div v-else style="height: 400px; margin: 0; padding: 0; border: none;">
                <MonacoEditor 
                  :key="`diff-${change.type}-${change.id}`"
                  :value="change.local_content || ''" 
                  :original-value="change.memory_content || ''"
                  :language="getEditorLanguage(change.type)" 
                  :read-only="true" 
                  :error-lines="change.errorLine ? [{ line: change.errorLine }] : []"
                  :diff-mode="true"
                  style="height: 100%; width: 100%; margin: 0; padding: 0; border: none;"
                />
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, inject, nextTick } from 'vue'
import { hubApi } from '../api'
import MonacoEditor from './MonacoEditor.vue'

// Define emits
const emit = defineEmits(['refresh-list'])

// State
const changes = ref([])
const loading = ref(false)
const error = ref(null)
const verifying = ref(false)

// Global message component
const $message = inject('$message', window?.$toast)

// Lifecycle hooks
onMounted(() => {
  refreshChanges()
})

// Methods
async function refreshChanges() {
  loading.value = true
  error.value = null
  
  try {
    const data = await hubApi.fetchLocalChanges()
    changes.value = data.map(change => ({
      ...change,
      verifyStatus: null,
      verifyError: null,
      errorLine: null
    })) || []
    
    // Wait for DOM update then refresh editor layout
    await nextTick()
    refreshEditorsLayout()
  } catch (e) {
    error.value = 'Failed to fetch local changes: ' + (e?.message || 'Unknown error')
  } finally {
    loading.value = false
  }
}

// Refresh all editor layouts
function refreshEditorsLayout() {
  // Give editors some time to render
  setTimeout(() => {
    // Find all Monaco editor instances on the page and refresh layout
    const editorElements = document.querySelectorAll('.monaco-editor-container')
    editorElements.forEach(el => {
      const editor = el.__vue__?.exposed
      if (editor) {
        const monacoEditor = editor.getEditor()
        const diffEditor = editor.getDiffEditor()
        
        if (monacoEditor) {
          monacoEditor.layout()
        }
        
        if (diffEditor) {
          diffEditor.layout()
        }
      }
    })
  }, 300)
}

function getEditorLanguage(type) {
  switch (type) {
    case 'rulesets':
      return 'xml'
    case 'plugins':
      return 'go'
    default:
      return 'yaml'
  }
}

// Convert singular component type to plural form (for API calls)
function getApiComponentType(type) {
  switch (type) {
    case 'input':
      return 'inputs'
    case 'output':
      return 'outputs'
    case 'ruleset':
      return 'rulesets'
    case 'project':
      return 'projects'
    case 'plugin':
      return 'plugins'
    default:
      return type + 's' // Default: add 's'
  }
}

async function verifyChanges() {
  if (!changes.value.length) return
  
  verifying.value = true
  let allValid = true
  
  try {
    // Verify each change
    for (const change of changes.value) {
      try {
        // Call verification API with plural component type
        const result = await hubApi.verifyComponent(getApiComponentType(change.type), change.id, change.local_content)
        
        // API now returns consistent format: {data: {valid: boolean, error: string|null}}
        const isValid = result.data?.valid === true;
        const errorMessage = result.data?.error || '';
        
        if (isValid) {
          change.verifyStatus = 'success'
          change.verifyError = null
        } else {
          change.verifyStatus = 'error'
          change.verifyError = errorMessage || 'Unknown verification error'
          allValid = false
          
          // Try to extract line number from error message
          if (errorMessage && typeof errorMessage === 'string') {
            const lineMatches = errorMessage.match(/line\s*(\d+)/i) || 
                               errorMessage.match(/line:\s*(\d+)/i) ||
                               errorMessage.match(/location:.*line\s*(\d+)/i);
            
            if (lineMatches && lineMatches[1]) {
              change.errorLine = parseInt(lineMatches[1]);
            }
          }
        }
      } catch (e) {
        change.verifyStatus = 'error'
        change.verifyError = e.message || 'Verification failed'
        allValid = false
        
        // Try to extract line number from error message
        const errorMessage = e.message || ''
        if (errorMessage && typeof errorMessage === 'string') {
          const lineMatches = errorMessage.match(/line\s*(\d+)/i) || 
                             errorMessage.match(/line:\s*(\d+)/i) ||
                             errorMessage.match(/location:.*line\s*(\d+)/i);
          
          if (lineMatches && lineMatches[1]) {
            change.errorLine = parseInt(lineMatches[1]);
          }
        }
      }
    }
    
    // Refresh editor layout to ensure error line highlighting displays correctly
    await nextTick()
    refreshEditorsLayout()
    
    if (allValid) {
      $message?.success?.('All changes verified successfully!')
    } else {
      $message?.error?.('Some changes failed verification. Please fix the errors before loading.')
    }
  } catch (e) {
    $message?.error?.('Failed to verify changes: ' + (e?.message || 'Unknown error'))
  } finally {
    verifying.value = false
  }
}

async function verifySingleChange(change) {
  verifying.value = true
  
  try {
    // Call verification API with plural component type
    const result = await hubApi.verifyComponent(getApiComponentType(change.type), change.id, change.local_content)
    
    // API now returns consistent format: {data: {valid: boolean, error: string|null}}
    const isValid = result.data?.valid === true;
    const errorMessage = result.data?.error || '';
    
    if (isValid) {
      change.verifyStatus = 'success'
      change.verifyError = null
      change.errorLine = null
      $message?.success?.('Verification successful!')
    } else {
      change.verifyStatus = 'error'
      change.verifyError = errorMessage || 'Unknown verification error'
      
      // Try to extract line number from error message
      if (errorMessage && typeof errorMessage === 'string') {
        const lineMatches = errorMessage.match(/line\s*(\d+)/i) || 
                           errorMessage.match(/line:\s*(\d+)/i) ||
                           errorMessage.match(/location:.*line\s*(\d+)/i);
        
        if (lineMatches && lineMatches[1]) {
          const lineNum = parseInt(lineMatches[1]);
          change.errorLine = lineNum;
          $message?.error?.(`Verification failed at line ${lineNum}: ${errorMessage}`)
        } else {
          $message?.error?.(`Verification failed: ${errorMessage}`)
        }
      } else {
        $message?.error?.(`Verification failed: ${errorMessage || 'Unknown error'}`)
      }
    }
    
    // Refresh editor layout to ensure error line highlighting displays correctly
    await nextTick()
    refreshEditorsLayout()
  } catch (e) {
    change.verifyStatus = 'error'
    change.verifyError = e.message || 'Verification failed'
    
    // Try to extract line number from error message
    const errorMessage = e.message || ''
    if (errorMessage && typeof errorMessage === 'string') {
      const lineMatches = errorMessage.match(/line\s*(\d+)/i) || 
                         errorMessage.match(/line:\s*(\d+)/i) ||
                         errorMessage.match(/location:.*line\s*(\d+)/i);
      
      if (lineMatches && lineMatches[1]) {
        const lineNum = parseInt(lineMatches[1]);
        change.errorLine = lineNum;
        $message?.error?.(`Verification failed at line ${lineNum}: ${errorMessage}`)
      } else {
        $message?.error?.(`Verification failed: ${errorMessage}`)
      }
    } else {
      $message?.error?.(`Verification failed: ${errorMessage || 'Unknown error'}`)
    }
  } finally {
    verifying.value = false
  }
}

async function loadAllChanges() {
  // Check if any changes failed verification
  const hasErrors = changes.value.some(change => change.verifyStatus === 'error')
  if (hasErrors) {
    $message?.error?.('Cannot load changes with verification errors. Please fix the errors first.')
    return
  }
  
  loading.value = true
  
  try {
    // Load all changes via API
    await hubApi.loadLocalChanges()
    
    $message?.success?.('All local changes loaded successfully!')
    
    // Refresh the list
    await refreshChanges()
    
    // Refresh all potentially affected component type lists
    ['inputs', 'outputs', 'rulesets', 'projects', 'plugins'].forEach(type => {
      emit('refresh-list', type)
    })
  } catch (e) {
    $message?.error?.('Failed to load changes: ' + (e?.message || 'Unknown error'))
  } finally {
    loading.value = false
  }
}

async function loadSingleChange(change) {
  // If verification failed, don't allow loading
  if (change.verifyStatus === 'error') {
    $message?.error?.('Cannot load invalid change. Please fix the errors first.')
    return
  }
  
  loading.value = true
  
  try {
    // If local file doesn't exist but memory does, delete from memory
    if (!change.has_local && change.has_memory) {
      await hubApi.deleteComponent(getApiComponentType(change.type), change.id)
      $message?.success?.(`Component deleted from memory successfully!`)
    } else {
      // Load the single change via API
      await hubApi.loadSingleLocalChange(change.type, change.id)
      $message?.success?.(`Local change loaded successfully!`)
    }
    
    // Refresh the list
    await refreshChanges()
    
    // Refresh affected component type list
    emit('refresh-list', change.type)
    
    // Ensure editor layout is correct
    refreshEditorsLayout()
  } catch (e) {
    $message?.error?.('Failed to process change: ' + (e?.message || 'Unknown error'))
  } finally {
    loading.value = false
  }
}

function getComponentTypeLabel(type) {
  const labels = {
    'input': 'Input',
    'output': 'Output',
    'ruleset': 'Ruleset',
    'project': 'Project',
    'plugin': 'Plugin'
  }
  
  return labels[type] || type
}

function getChangeStatusClass(change) {
  if (!change.has_local && change.has_memory) {
    return 'bg-red-100 text-red-800'  // Deleted locally
  } else if (change.has_local && !change.has_memory) {
    return 'bg-green-100 text-green-800'  // New local file
  } else {
    return 'bg-orange-100 text-orange-800'  // Modified
  }
}

function getChangeStatusLabel(change) {
  if (!change.has_local && change.has_memory) {
    return 'Deleted Locally'
  } else if (change.has_local && !change.has_memory) {
    return 'New Local File'
  } else {
    return 'Local Modified'
  }
}

function getLoadButtonClass(change) {
  if (!change.has_local && change.has_memory) {
    return 'px-2 py-1 text-xs bg-red-600 text-white rounded hover:bg-red-700 disabled:opacity-50'
  } else {
    return 'px-2 py-1 text-xs bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50'
  }
}

function getLoadButtonText(change) {
  if (!change.has_local && change.has_memory) {
    return 'Delete from Memory'
  } else {
    return 'Load'
  }
}
</script>

<style scoped>
pre {
  white-space: pre-wrap;
  word-wrap: break-word;
}
</style> 