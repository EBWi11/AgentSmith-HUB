<template>
  <div class="h-full flex flex-col p-4">
    <div class="flex justify-between items-center mb-4">
      <h2 class="text-xl font-semibold">Pending Changes</h2>
      <div class="flex space-x-2">
        <button 
          @click="refreshChanges" 
          class="btn btn-secondary btn-sm"
        >
          Refresh
        </button>
      </div>
    </div>

    <div v-if="loading" class="flex-1 flex items-center justify-center">
      <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500"></div>
    </div>
    
    <div v-else-if="error" class="flex-1 flex items-center justify-center text-red-500">
      {{ error }}
    </div>
    
    <div v-else-if="!changes.length" class="flex-1 flex items-center justify-center text-gray-500">
      No pending changes
    </div>
    
    <div v-else class="flex-1 overflow-auto">
      <div v-for="(change, index) in sortedChanges" :key="index" class="mb-4 border rounded-md overflow-hidden">
        <div class="bg-gray-50 p-3 flex justify-between items-center border-b">
          <div class="font-medium">
            <span class="text-gray-700">{{ getComponentTypeLabel(change.type) }}:</span>
            <span class="ml-1">{{ change.id }}</span>
            <span v-if="change.is_new" class="ml-2 px-1.5 py-0.5 bg-green-100 text-green-800 text-xs rounded">New</span>
            <span v-else class="ml-2 px-1.5 py-0.5 bg-blue-100 text-blue-800 text-xs rounded">Modified</span>
            <span v-if="change.verifyStatus === 'success'" class="ml-2 px-1.5 py-0.5 bg-green-100 text-green-800 text-xs rounded">Verified</span>
            <span v-if="change.verifyStatus === 'error'" class="ml-2 px-1.5 py-0.5 bg-red-100 text-red-800 text-xs rounded">Invalid</span>
          </div>
          <div class="flex items-center">
            <div v-if="needsRestart(change)" class="mr-3 text-xs text-amber-600 flex items-center">
              <svg class="w-4 h-4 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"></path>
              </svg>
              Requires restart
            </div>
            <button 
              @click="verifySingleChange(change)" 
              class="btn btn-verify btn-xs mr-2"
              :disabled="verifying"
            >
              Verify
            </button>
            <button 
              @click="applySingleChange(change)" 
              class="btn btn-primary btn-xs mr-2"
              :disabled="applying || (change.verifyStatus === 'error')"
            >
              Apply
            </button>
            <button 
              @click="cancelUpgrade(change)" 
              class="btn btn-danger btn-xs"
              :disabled="applying || cancelling"
              title="Cancel upgrade and delete .new file"
            >
              Cancel
            </button>
          </div>
        </div>
        
        <div class="bg-gray-100" style="padding: 0; margin: 0;">
          <div v-if="change.verifyError" class="p-2 bg-red-50 border border-red-200 text-red-700 text-xs" style="margin: 0 0 8px 0;">
            {{ change.verifyError }}
          </div>
          
          <div style="margin: 0; padding: 0; border: none; border-radius: 0; overflow: hidden;">
            <!-- New file: display content directly -->
            <div v-if="change.is_new" style="height: 400px; margin: 0; padding: 0; border: none;">
              <MonacoEditor 
                :key="`new-${change.type}-${change.id}`"
                :value="change.new_content || ''" 
                :language="getEditorLanguage(change.type)" 
                :read-only="true" 
                :error-lines="change.errorLine ? [{ line: change.errorLine }] : []"
                :diff-mode="false"
                style="height: 100%; width: 100%; margin: 0; padding: 0; border: none;"
              />
            </div>
            <!-- Modified file: use diff mode -->
            <div v-else style="height: 400px; margin: 0; padding: 0; border: none;">
              <MonacoEditor 
                :key="`diff-${change.type}-${change.id}`"
                :value="change.new_content || ''" 
                :original-value="change.old_content || ''"
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
import { useApiOperations } from '../composables/useApi'
import { getEditorLanguage, getComponentTypeLabel, getApiComponentType, extractLineNumber, needsRestart } from '../utils/common'
import { debounce, throttle } from '../utils/performance'
import { useDataCacheStore } from '../stores/dataCache'
// Cache management integrated into DataCache

// Define emits
const emit = defineEmits(['refresh-list'])

// Use composables
const { loading: apiLoading, error: apiError } = useApiOperations()

// State
const changes = ref([])
const loading = ref(false)
const error = ref(null)
const applying = ref(false)
const verifying = ref(false)
const cancelling = ref(false)
const editorRefs = ref([]) // Store editor references

// Global message component
const $message = inject('$message', window?.$toast)

// Data cache store
const dataCache = useDataCacheStore()

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
    // Use enhanced API to get changes with status information
    const data = await hubApi.fetchEnhancedPendingChanges()
    
    // Validate and filter data
    if (!Array.isArray(data)) {
      throw new Error('Invalid response format: expected array')
    }
    
    changes.value = data
      .filter(change => {
        // Filter out invalid changes
        if (!change || typeof change !== 'object') {
          return false
        }
        if (!change.type || !change.id) {
          return false
        }
        return true
      })
      .map(change => ({
        ...change,
        verifyStatus: getVerifyStatusFromChange(change),
        verifyError: change.error_message || null,
        errorLine: null,
        // Ensure required fields have default values
        new_content: change.new_content || '',
        old_content: change.old_content || '',
        is_new: Boolean(change.is_new)
      }))
    
    // Wait for DOM update then refresh editor layout
    await nextTick()
    refreshEditorsLayout()
  } catch (e) {
    console.error('Error fetching pending changes:', e)
    error.value = 'Failed to fetch pending changes: ' + (e?.message || 'Unknown error')
    changes.value = [] // Reset to empty array on error
  } finally {
    loading.value = false
  }
}

// Helper function to convert enhanced change status to verify status
function getVerifyStatusFromChange(change) {
  switch (change.status) {
    case 'verified':
      return 'success'
    case 'invalid':
      return 'error'
    case 'applied':
      return 'success'
    case 'failed':
      return 'error'
    default:
      return null
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

// These functions are now imported from utils/common.js

async function verifyChanges() {
  if (!changes.value.length) return
  
  verifying.value = true
  
  try {
    // Use enhanced batch verification API
    const result = await hubApi.verifyPendingChanges()
    
    if (result.valid_changes === result.total_changes) {
      $message?.success?.(`All ${result.total_changes} changes verified successfully!`)
    } else {
      $message?.warning?.(`${result.valid_changes} valid, ${result.invalid_changes} invalid out of ${result.total_changes} changes`)
    }
    
    // Update individual change status based on verification results
    if (result.results) {
      for (const verifyResult of result.results) {
        const change = changes.value.find(c => c.type === verifyResult.type && c.id === verifyResult.id)
        if (change) {
          change.verifyStatus = verifyResult.valid ? 'success' : 'error'
          change.verifyError = verifyResult.error || null
          
          // Try to extract line number from error message
          if (!verifyResult.valid && verifyResult.error) {
            change.errorLine = extractLineNumber(verifyResult.error)
          } else {
            change.errorLine = null;
          }
        }
      }
    }
    
    // Refresh editor layout to ensure error line highlighting displays correctly
    await nextTick()
    refreshEditorsLayout()
    
    // Refresh the changes list to get updated status from server
    await refreshChanges()
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
    const result = await hubApi.verifyComponent(getApiComponentType(change.type), change.id, change.new_content)
    
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
      const lineNum = extractLineNumber(errorMessage)
      if (lineNum) {
        change.errorLine = lineNum
        $message?.error?.(`Verification failed at line ${lineNum}: ${errorMessage}`)
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
    const lineNum = extractLineNumber(errorMessage)
    if (lineNum) {
      change.errorLine = lineNum
      $message?.error?.(`Verification failed at line ${lineNum}: ${errorMessage}`)
    } else {
      $message?.error?.(`Failed to verify change: ${errorMessage || 'Unknown error'}`)
    }
    
    // Refresh editor layout
    await nextTick()
    refreshEditorsLayout()
  } finally {
    verifying.value = false
  }
}

// Apply a single change
async function applySingleChange(change) {
  applying.value = true
  
  try {
    // Call apply API
    await hubApi.applySingleChange(change.type, change.id)
    
    $message?.success?.(`Change applied successfully for ${getComponentTypeLabel(change.type)} "${change.id}"`)
    
    // Immediately clear pending changes cache to ensure fresh data
    dataCache.clearCache('pendingChanges')
    // Also clear the affected component type cache for immediate UI update
    dataCache.clearComponentCache(change.type)
    
    // Force refresh all component lists to ensure hasTemp is updated
    await Promise.all([
      dataCache.fetchComponents('inputs', true),
      dataCache.fetchComponents('outputs', true),
      dataCache.fetchComponents('rulesets', true),
      dataCache.fetchComponents('projects', true),
      dataCache.fetchComponents('plugins', true)
    ])
    
    // Refresh the list to remove the applied change
    await refreshChanges()
    
    // Refresh affected component type list
    emit('refresh-list', getApiComponentType(change.type))
    
    // Ensure editor layout is correct
    refreshEditorsLayout()
  } catch (e) {
    $message?.error?.('Failed to apply change: ' + (e?.message || 'Unknown error'))
    
    // Even if failed, clear cache and refresh list to ensure latest status is displayed
    dataCache.clearCache('pendingChanges')
    await refreshChanges();
    emit('refresh-list', getApiComponentType(change.type))
  } finally {
    applying.value = false
  }
}

// Cancel upgrade for a single change
async function cancelUpgrade(change) {
  // Confirm the action
  const confirmed = confirm(`Are you sure you want to cancel the upgrade for ${getComponentTypeLabel(change.type)} "${change.id}"?\n\nThis will delete the .new file and all pending changes will be lost.`)
  if (!confirmed) {
    return
  }
  
  cancelling.value = true
  
  try {
    // Use enhanced cancel API
    await hubApi.cancelPendingChange(change.type, change.id)
    
    $message?.success?.(`Change cancelled for ${getComponentTypeLabel(change.type)} "${change.id}"`)
    
    // Immediately clear pending changes cache to ensure fresh data
    dataCache.clearCache('pendingChanges')
    // Also clear the affected component type cache for immediate UI update
    dataCache.clearComponentCache(change.type)
    
    // Force refresh all component lists to ensure hasTemp is updated
    await Promise.all([
      dataCache.fetchComponents('inputs', true),
      dataCache.fetchComponents('outputs', true),
      dataCache.fetchComponents('rulesets', true),
      dataCache.fetchComponents('projects', true),
      dataCache.fetchComponents('plugins', true)
    ])
    
    // Refresh the list to remove the cancelled change
    await refreshChanges()
    
    // Refresh affected component type list
    emit('refresh-list', getApiComponentType(change.type))
    
    // Ensure editor layout is correct
    refreshEditorsLayout()
  } catch (e) {
    $message?.error?.('Failed to cancel change: ' + (e?.message || 'Unknown error'))
    
    // Even if failed, clear cache and refresh list to ensure latest status is displayed
    dataCache.clearCache('pendingChanges')
    await refreshChanges();
    emit('refresh-list', getApiComponentType(change.type))
  } finally {
    cancelling.value = false
  }
}

// Apply all pending changes
async function applyAllChanges() {
  if (!changes.value.length) return
  
  // Confirm the action
  const confirmed = confirm(`Are you sure you want to apply ALL pending changes?\n\nThis will make all changes active and restart affected projects.`)
  if (!confirmed) {
    return
  }
  
  applying.value = true
  
  try {
    // Get current project states before applying changes
    const currentProjects = await dataCache.fetchComponents('projects')
    const runningProjects = currentProjects
      .filter(p => p.status === 'running')
      .map(p => p.id)
    
    // Call apply all changes API
    const result = await hubApi.applyAllChanges()
    
    $message?.success?.(result.message || 'All changes applied successfully!')
    
    // Get projects that need to restart from the API response
    const projectsToRestart = result.projects_to_restart || []
    
    // Immediately set running projects that will be restarted to 'stopping' status
    if (projectsToRestart.length > 0) {
      // Find projects that were running and will be restarted
      const restartingProjects = projectsToRestart.filter(projectId => 
        runningProjects.includes(projectId)
      )
      
      if (restartingProjects.length > 0) {
        // Update UI immediately to show stopping status
        const updatedProjects = currentProjects.map(project => {
          if (restartingProjects.includes(project.id)) {
            return { ...project, status: 'stopping' }
          }
          return project
        })
        
        // Update cache with new status for immediate UI feedback
        dataCache.updateComponentCache('projects', updatedProjects)
        
        // Emit refresh event to update sidebar
        emit('refresh-list', 'projects')
        
        $message?.info?.(`${restartingProjects.length} running projects are restarting...`)
        
        // Start accelerated polling for the restarting projects
        startAcceleratedProjectPolling(restartingProjects)
      }
    }
    
    // Show failed changes if any
    if (result.failed_changes > 0) {
      const failedDetails = result.failed_change_details || []
      let errorMsg = `${result.failed_changes} changes failed to apply:`
      failedDetails.forEach(failed => {
        errorMsg += `\n- ${failed.type}/${failed.id}: ${failed.error}`
      })
      $message?.warning?.(errorMsg)
    }
    
    // Clear all caches to ensure fresh data
    dataCache.clearCache('pendingChanges')
    ['inputs', 'outputs', 'rulesets', 'projects', 'plugins'].forEach(type => {
      dataCache.clearComponentCache(type)
    })
    
    // Force refresh all component lists
    await Promise.all([
      dataCache.fetchComponents('inputs', true),
      dataCache.fetchComponents('outputs', true),
      dataCache.fetchComponents('rulesets', true),
      dataCache.fetchComponents('projects', true),
      dataCache.fetchComponents('plugins', true)
    ])
    
    // Refresh the pending changes list
    await refreshChanges()
    
    // Emit refresh events for all component types
    ['inputs', 'outputs', 'rulesets', 'projects', 'plugins'].forEach(type => {
      emit('refresh-list', type)
    })
    
    // Ensure editor layout is correct
    refreshEditorsLayout()
  } catch (e) {
    $message?.error?.('Failed to apply all changes: ' + (e?.message || 'Unknown error'))
    
    // Even if failed, clear cache and refresh to ensure latest status
    dataCache.clearCache('pendingChanges')
    await refreshChanges()
  } finally {
    applying.value = false
  }
}

// Start accelerated polling for restarting projects
function startAcceleratedProjectPolling(projectIds) {
  if (!projectIds || projectIds.length === 0) return
  
  const pollInterval = 1000 // Poll every 1 second for faster updates
  const maxPollTime = 30000 // Stop polling after 30 seconds
  const startTime = Date.now()
  
  const poll = async () => {
    try {
      // Check if we've exceeded max poll time
      if (Date.now() - startTime > maxPollTime) {
        console.log('Accelerated project polling timeout')
        return
      }
      
      // Fetch latest project data
      const projects = await dataCache.fetchComponents('projects', true)
      
      // Check if any projects are still in transition
      const stillTransitioning = projectIds.some(projectId => {
        const project = projects.find(p => p.id === projectId)
        return project && (project.status === 'stopping' || project.status === 'starting')
      })
      
      // Update UI with latest status
      emit('refresh-list', 'projects')
      
      // Continue polling if projects are still transitioning
      if (stillTransitioning) {
        setTimeout(poll, pollInterval)
      } else {
        console.log('All projects finished transitioning')
      }
    } catch (error) {
      console.error('Error during accelerated project polling:', error)
      // Continue polling on error, but log it
      setTimeout(poll, pollInterval)
    }
  }
  
  // Start polling
  setTimeout(poll, pollInterval)
}

// Cancel all pending changes
async function cancelAllChanges() {
  if (!changes.value.length) return
  
  // Confirm the action
  const confirmed = confirm(`Are you sure you want to cancel ALL pending changes?\n\nThis will delete all .new files and all pending changes will be lost.`)
  if (!confirmed) {
    return
  }
  
  cancelling.value = true
  
  try {
    const result = await hubApi.cancelAllPendingChanges()
    
    $message?.success?.(`${result.cancelled_count} changes cancelled successfully`)
    
    // Immediately clear pending changes cache to ensure fresh data
    dataCache.clearCache('pendingChanges')
    // Also clear all component type caches for immediate UI update
    ['inputs', 'outputs', 'rulesets', 'projects', 'plugins'].forEach(type => {
      dataCache.clearComponentCache(type)
    })
    
    // Force refresh all component lists to ensure hasTemp is updated
    await Promise.all([
      dataCache.fetchComponents('inputs', true),
      dataCache.fetchComponents('outputs', true),
      dataCache.fetchComponents('rulesets', true),
      dataCache.fetchComponents('projects', true),
      dataCache.fetchComponents('plugins', true)
    ])
    
    // Refresh the list to remove all cancelled changes
    await refreshChanges()
    
    // Refresh all component type lists
    ['inputs', 'outputs', 'rulesets', 'projects', 'plugins'].forEach(type => {
      emit('refresh-list', type)
    })
    
    // Ensure editor layout is correct
    refreshEditorsLayout()
  } catch (e) {
    $message?.error?.('Failed to cancel all changes: ' + (e?.message || 'Unknown error'))
    
    // Even if failed, clear cache and refresh list to ensure latest status is displayed
    dataCache.clearCache('pendingChanges')
    await refreshChanges()
  } finally {
    cancelling.value = false
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