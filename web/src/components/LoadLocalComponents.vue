<template>
  <div class="h-full flex flex-col p-4">
    <div class="flex items-center justify-between mb-4">
      <h1 class="text-xl font-semibold text-gray-900">Load Local Components</h1>
      <div class="flex items-center space-x-2">
        <button 
          @click="refreshChanges" 
          :disabled="loading"
          class="btn btn-secondary btn-sm"
        >
          Refresh
        </button>
        <button 
          @click="verifyChanges" 
          :disabled="!changes.length || verifying"
          class="btn btn-verify btn-sm"
        >
          {{ verifying ? 'Verifying...' : 'Verify' }}
        </button>
      </div>
    </div>

    <div v-if="loading && !changes.length" class="flex-1 flex items-center justify-center">
      <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500"></div>
    </div>
    
    <div v-else-if="error" class="flex-1 flex items-center justify-center text-red-500">
      {{ error }}
    </div>
    
    <div v-else-if="!changes.length" class="flex-1 flex items-center justify-center text-gray-500">
      No local changes detected
    </div>
    
    <div v-else class="flex-1 overflow-auto">
      <div v-for="change in sortedChanges" :key="`${change.type}-${change.id}`" class="mb-4 border rounded-md overflow-hidden">
        <div class="bg-gray-50 p-3 flex justify-between items-center border-b">
          <div class="font-medium">
            <span class="text-gray-700">{{ getComponentTypeLabel(change.type) }}:</span>
            <span class="ml-1">{{ change.id }}</span>
            <span class="ml-2 text-xs px-2 py-1 rounded" :class="getChangeStatusClass(change)">
              {{ getChangeStatusLabel(change) }}
            </span>
            <span v-if="change.verifyStatus === 'success'" class="ml-2 px-1.5 py-0.5 bg-green-100 text-green-800 text-xs rounded">Verified</span>
            <span v-if="change.verifyStatus === 'error'" class="ml-2 px-1.5 py-0.5 bg-red-100 text-red-800 text-xs rounded">Invalid</span>
          </div>
          <div class="flex items-center">
            <button 
              v-if="change.has_local"
              @click="verifySingleChange(change)" 
              :disabled="verifying"
              class="btn btn-verify btn-xs mr-2"
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
          
          <div style="margin: 0; padding: 0; border: none; border-radius: 0; overflow: hidden;">
            <!-- New local file: display content directly -->
            <div v-if="change.has_local && !change.has_memory" style="height: 400px; margin: 0; padding: 0; border: none;">
              <MonacoEditor 
                :key="`new-local-${change.type}-${change.id}`"
                :value="change.local_content || ''" 
                :language="getEditorLanguage(change.type)" 
                :read-only="true" 
                :error-lines="change.errorLine ? [{ line: change.errorLine }] : []"
                :diff-mode="false"
                style="height: 100%; width: 100%; margin: 0; padding: 0; border: none;"
              />
            </div>
            <!-- File deleted locally: show memory content -->
            <div v-else-if="!change.has_local && change.has_memory" style="height: 400px; margin: 0; padding: 0; border: none;">
              <MonacoEditor 
                :key="`deleted-${change.type}-${change.id}`"
                :value="change.memory_content || ''" 
                :language="getEditorLanguage(change.type)" 
                :read-only="true" 
                :error-lines="change.errorLine ? [{ line: change.errorLine }] : []"
                :diff-mode="false"
                style="height: 100%; width: 100%; margin: 0; padding: 0; border: none;"
              />
            </div>
            <!-- File changed: show diff mode -->
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
</template>

<script setup>
import { ref, computed, onMounted, inject, nextTick } from 'vue'
import { hubApi } from '../api'
import MonacoEditor from './MonacoEditor.vue'
import { getComponentTypeLabel, getEditorLanguage, getApiComponentType } from '../utils/common'

// Define emits
const emit = defineEmits(['refresh-list'])

// State
const changes = ref([])
const loading = ref(false)
const error = ref(null)
const verifying = ref(false)

// Global message component
const $message = inject('$message', window?.$toast)

// Computed properties
const sortedChanges = computed(() => {
  return [...changes.value].sort((a, b) => {
    // Define component type priority (lower number = higher priority)
    const getTypePriority = (type) => {
      switch (type) {
        case 'input': return 1
        case 'output': return 2
        case 'ruleset': return 3
        case 'plugin': return 4
        case 'project': return 5  // project goes last
        default: return 6
      }
    }
    
    const priorityA = getTypePriority(a.type)
    const priorityB = getTypePriority(b.type)
    
    // If same type, sort by id
    if (priorityA === priorityB) {
      return a.id.localeCompare(b.id)
    }
    
    return priorityA - priorityB
  })
})

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
    
    // Dispatch global event for any component changes
    if (changes.value.length > 0) {
      const affectedTypes = [...new Set(changes.value.map(change => change.type))]
      window.dispatchEvent(new CustomEvent('localChangesLoaded', { 
        detail: { types: affectedTypes, timestamp: Date.now() }
      }))
    }
  } catch (e) {
    $message?.error?.('Failed to load changes: ' + (e?.message || 'Unknown error'))
    
    // 即使失败，也要刷新列表以确保显示最新状态
    await refreshChanges();
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
    
    // Dispatch global event for the component change
    window.dispatchEvent(new CustomEvent('localChangesLoaded', { 
      detail: { type: change.type, id: change.id, timestamp: Date.now() }
    }))
    
    // Ensure editor layout is correct
    refreshEditorsLayout()
  } catch (e) {
    $message?.error?.('Failed to process change: ' + (e?.message || 'Unknown error'))
    
    // Even if it fails, refresh the list to ensure the latest status is displayed
    await refreshChanges();
    
    // Even if it fails, try refreshing the corresponding component type list
    emit('refresh-list', change.type)
  } finally {
    loading.value = false
  }
}



function getChangeStatusClass(change) {
  if (!change.has_local && change.has_memory) {
    return 'bg-red-100 text-red-800'  // File deleted locally but exists in memory
  } else if (change.has_local && !change.has_memory) {
    return 'bg-blue-100 text-blue-800'  // New local file not yet loaded
  } else {
    return 'bg-yellow-100 text-yellow-800'  // File exists both locally and in memory (needs sync)
  }
}

function getChangeStatusLabel(change) {
  if (!change.has_local && change.has_memory) {
    return 'File Deleted'
  } else if (change.has_local && !change.has_memory) {
    return 'New Local File'
  } else {
    return 'File Changed'
  }
}

function getLoadButtonClass(change) {
  if (!change.has_local && change.has_memory) {
    return 'btn btn-danger btn-xs'
  } else {
    return 'btn btn-primary btn-xs'
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

/* Button Styles - Minimal Design to match other components */
.btn.btn-secondary {
  background: transparent !important;
  border: 1px solid #d1d5db !important;
  color: #6b7280 !important;
  transition: all 0.15s ease !important;
  box-shadow: none !important;
  transform: none !important;
}

.btn.btn-secondary:hover:not(:disabled) {
  border-color: #9ca3af !important;
  color: #4b5563 !important;
  background: rgba(0, 0, 0, 0.05) !important;
  box-shadow: none !important;
  transform: none !important;
}

.btn.btn-verify {
  background: transparent !important;
  border: 1px solid #d1d5db !important;
  color: #6b7280 !important;
  transition: all 0.15s ease !important;
  box-shadow: none !important;
  transform: none !important;
}

.btn.btn-verify:hover:not(:disabled) {
  border-color: #059669 !important;
  color: #059669 !important;
  background: rgba(236, 253, 245, 0.3) !important;
  box-shadow: none !important;
  transform: none !important;
}

.btn.btn-primary {
  background: transparent !important;
  border: 1px solid #3b82f6 !important;
  color: #3b82f6 !important;
  transition: all 0.15s ease !important;
  box-shadow: none !important;
  transform: none !important;
}

.btn.btn-primary:hover:not(:disabled) {
  border-color: #2563eb !important;
  color: #2563eb !important;
  background: rgba(59, 130, 246, 0.05) !important;
  box-shadow: none !important;
  transform: none !important;
}

.btn.btn-danger {
  background: transparent !important;
  border: 1px solid #dc2626 !important;
  color: #dc2626 !important;
  transition: all 0.15s ease !important;
  box-shadow: none !important;
  transform: none !important;
}

.btn.btn-danger:hover:not(:disabled) {
  border-color: #b91c1c !important;
  color: #b91c1c !important;
  background: rgba(220, 38, 38, 0.05) !important;
  box-shadow: none !important;
  transform: none !important;
}
</style> 