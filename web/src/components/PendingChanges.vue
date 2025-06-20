<template>
  <div class="h-full flex flex-col p-4">
    <div class="flex justify-between items-center mb-4">
      <h2 class="text-xl font-semibold">Pending Changes</h2>
      <div class="flex space-x-2">
        <button 
          @click="refreshChanges" 
          class="px-3 py-1.5 border border-gray-300 text-gray-700 text-sm rounded hover:bg-gray-50 transition-colors duration-150 focus:outline-none"
        >
          Refresh
        </button>
        <button 
          @click="verifyChanges" 
          class="px-3 py-1.5 border border-gray-300 text-gray-700 text-sm rounded hover:bg-gray-50 transition-colors duration-150 focus:outline-none"
          :disabled="verifying || !changes.length"
        >
          <span v-if="verifying" class="w-3 h-3 border-1.5 border-gray-700 border-t-transparent rounded-full animate-spin mr-1"></span>
          Verify
        </button>
        <button 
          @click="cancelAllChanges" 
          class="px-3 py-1.5 border border-red-300 text-red-700 text-sm rounded hover:bg-red-50 transition-colors duration-150 focus:outline-none"
          :disabled="cancelling || !changes.length"
        >
          <span v-if="cancelling" class="w-3 h-3 border-1.5 border-red-700 border-t-transparent rounded-full animate-spin mr-1"></span>
          Cancel All
        </button>
        <button 
          @click="applyChanges" 
          class="px-3 py-1.5 bg-blue-500 text-white text-sm rounded hover:bg-blue-600 transition-colors duration-150 focus:outline-none flex items-center space-x-1.5"
          :disabled="applying || !changes.length"
        >
          <span v-if="applying" class="w-3 h-3 border-1.5 border-white border-t-transparent rounded-full animate-spin"></span>
          <span>{{ applying ? 'Applying...' : 'Apply All Changes' }}</span>
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
      <div v-for="(change, index) in changes" :key="index" class="mb-4 border rounded-md overflow-hidden">
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
              class="px-2 py-1 text-xs border border-gray-300 text-gray-700 rounded hover:bg-gray-50 transition-colors duration-150 focus:outline-none mr-2"
              :disabled="verifying"
            >
              Verify
            </button>
            <button 
              @click="applySingleChange(change)" 
              class="px-2 py-1 text-xs bg-blue-500 text-white rounded hover:bg-blue-600 transition-colors duration-150 focus:outline-none mr-2"
              :disabled="applying || (change.verifyStatus === 'error')"
            >
              Apply
            </button>
            <button 
              @click="cancelUpgrade(change)" 
              class="px-2 py-1 text-xs bg-red-500 text-white rounded hover:bg-red-600 transition-colors duration-150 focus:outline-none"
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
import { ref, onMounted, inject, nextTick } from 'vue'
import { hubApi } from '../api'
import MonacoEditor from './MonacoEditor.vue'

// Define emits
const emit = defineEmits(['refresh-list'])

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
          console.warn('Skipping invalid change:', change)
          return false
        }
        if (!change.type || !change.id) {
          console.warn('Skipping change with missing type or id:', change)
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
            const lineMatches = verifyResult.error.match(/line\s*(\d+)/i) || 
                               verifyResult.error.match(/line:\s*(\d+)/i) ||
                               verifyResult.error.match(/location:.*line\s*(\d+)/i);
            
            if (lineMatches && lineMatches[1]) {
              change.errorLine = parseInt(lineMatches[1]);
            }
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
        $message?.error?.('Failed to verify change: ' + errorMessage)
      }
    } else {
      $message?.error?.('Failed to verify change: Unknown error')
    }
    
    // Refresh editor layout
    await nextTick()
    refreshEditorsLayout()
  } finally {
    verifying.value = false
  }
}

async function applyChanges() {
  if (!changes.value || !changes.value.length) return
  
  // Check if any changes are in applying state
  if (applying.value) {
    console.warn('Apply operation already in progress')
    return
  }
  
  applying.value = true
  
  try {
    // Validate changes before applying
    const validChanges = changes.value.filter(change => {
      if (!change || !change.type || !change.id) {
        console.warn('Skipping invalid change:', change)
        return false
      }
      return true
    })
    
    if (validChanges.length === 0) {
      $message?.warning?.('No valid changes to apply')
      return
    }
    
    // Record component types before applying for later list refresh
    const affectedTypes = new Set(validChanges.map(change => change.type))
    
    // Use enhanced API for better transaction handling
    const result = await hubApi.applyPendingChangesEnhanced()
    
    // Validate result
    if (!result || typeof result !== 'object') {
      throw new Error('Invalid response from server')
    }
    
    if (result.success_count > 0) {
      $message?.success?.(`Applied successfully! Success: ${result.success_count}, Failed: ${result.failure_count}`)
      
      // Show projects that need restart
      if (result.projects_to_restart && result.projects_to_restart.length > 0) {
        $message?.warning?.(`Projects requiring restart: ${result.projects_to_restart.join(', ')}`)
      }
    }
    
    if (result.failure_count > 0) {
      $message?.error?.(`Failed to apply ${result.failure_count} changes`)
      
      // Show detailed error information
      if (result.failed_changes && result.failed_changes.length > 0) {
        const errorDetails = result.failed_changes.map(fc => `${fc.type}:${fc.id} - ${fc.error}`).join('\n')
        console.error('Apply failures:', errorDetails)
      }
    }
    
    // Refresh the list
    await refreshChanges()
    
    // Refresh all affected component type lists
    affectedTypes.forEach(type => {
      // Notify parent component to refresh corresponding type list
      emit('refresh-list', getApiComponentType(type))
    })
    
    // 确保编辑器布局正确
    refreshEditorsLayout()
  } catch (e) {
    // Handle verification failure cases
    if (e.response?.data?.failed_changes) {
      const failedChanges = e.response.data.failed_changes
      const failedComponents = failedChanges.map(f => `${getComponentTypeLabel(f.type)} ${f.id}: ${f.error}`).join('\n');
      
      $message?.error?.(`Verification failed, unable to apply changes:\n${failedComponents}`, { timeout: 10000 });
    } else {
      $message?.error?.('Failed to apply changes: ' + (e?.message || 'Unknown error'))
    }
    
    // 即使失败，也要刷新列表以确保显示最新状态
    await refreshChanges();
  } finally {
    applying.value = false
  }
}

async function applySingleChange(change) {
  // 如果验证失败，不允许应用
  if (change.verifyStatus === 'error') {
    $message?.error?.('Cannot apply invalid change. Please fix the errors first.')
    return
  }
  
  applying.value = true
  
  try {
    // Actually apply the single change via API
    await hubApi.applySingleChange(change.type, change.id)
    
    // Check if this change requires project restart
    if (needsRestart(change)) {
      // Find affected projects
      const projectsToRestart = findAffectedProjects(change)
      if (projectsToRestart.length > 0) {
        await restartProjects(projectsToRestart)
      }
    }
    
    $message?.success?.(`Change applied successfully!`)
    
    // Refresh the list
    await refreshChanges()
    
    // Refresh affected component type list
    emit('refresh-list', getApiComponentType(change.type))
    
    // 确保编辑器布局正确
    refreshEditorsLayout()
  } catch (e) {

    // Handle verification failure cases
    if (e.isVerificationError) {
      $message?.error?.(`验证失败，无法应用更改: ${e.message}`, { timeout: 5000 });
    } else {
      $message?.error?.('Failed to apply change: ' + (e?.message || 'Unknown error'))
    }
    
    // 即使失败，也要刷新列表以确保显示最新状态
    await refreshChanges();
    emit('refresh-list', getApiComponentType(change.type))
  } finally {
    applying.value = false
  }
}

// Check if a component change requires project restart
function needsRestart(change) {
  // Rulesets support hot reload, other components require restart
  return change.type !== 'ruleset'
}

// Find projects that need to be restarted based on all pending changes
function findProjectsToRestart() {
  const projectIds = new Set()
  
  // For each non-ruleset change, find affected projects
  changes.value.forEach(change => {
    if (needsRestart(change)) {
      // For now, we'll just restart all projects when there are changes
      // In a more sophisticated implementation, we would check which projects
      // use the changed components
      projectIds.add('all')
    }
  })
  
  return Array.from(projectIds)
}

// Find projects affected by a specific change
function findAffectedProjects() {
  // For now, we'll just restart all projects when there are changes
  // In a more sophisticated implementation, we would check which projects
  // use the changed component
  return ['all']
}

// Restart the specified projects
async function restartProjects(projectIds) {
  try {
    if (projectIds.includes('all')) {
      // Restart all projects
      await hubApi.restartAllProjects()
      $message?.success?.('All projects restarted')
    } else {
      // Restart specific projects
      for (const id of projectIds) {
        await hubApi.restartProject(id)
      }
      $message?.success?.(`Projects restarted: ${projectIds.join(', ')}`)
    }
  } catch (e) {
        $message?.error?.('Failed to restart projects: ' + (e?.message || 'Unknown error'))
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
    
    // Refresh the list to remove the cancelled change
    await refreshChanges()
    
    // Refresh affected component type list
    emit('refresh-list', getApiComponentType(change.type))
    
    // Ensure editor layout is correct
    refreshEditorsLayout()
  } catch (e) {
    $message?.error?.('Failed to cancel change: ' + (e?.message || 'Unknown error'))
    
    // 即使失败，也要刷新列表以确保显示最新状态
    await refreshChanges();
    emit('refresh-list', getApiComponentType(change.type))
  } finally {
    cancelling.value = false
  }
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
    
    // 即使失败，也要刷新列表以确保显示最新状态
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
</style>