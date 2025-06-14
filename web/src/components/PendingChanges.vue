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
              class="px-2 py-1 text-xs bg-blue-500 text-white rounded hover:bg-blue-600 transition-colors duration-150 focus:outline-none"
              :disabled="applying || (change.verifyStatus === 'error')"
            >
              Apply
            </button>
          </div>
        </div>
        
        <div class="p-3 bg-gray-100">
          <div v-if="change.verifyError" class="mb-3 p-2 bg-red-50 border border-red-200 text-red-700 text-xs rounded">
            {{ change.verifyError }}
          </div>
          <div class="grid grid-cols-2 gap-4">
            <div class="bg-white rounded border p-2">
              <div class="text-xs text-gray-500 mb-1">{{ change.is_new ? 'Empty file' : 'Original content' }}</div>
              <pre class="text-xs overflow-auto max-h-60 p-2 bg-gray-50 rounded">{{ change.old_content || '(empty)' }}</pre>
            </div>
            <div class="bg-white rounded border p-2">
              <div class="text-xs text-gray-500 mb-1">New content</div>
              <div class="h-60">
                <CodeEditor 
                  :value="change.new_content || ''" 
                  :language="getEditorLanguage(change.type)" 
                  :read-only="true" 
                  :error-lines="change.errorLine ? [{ line: change.errorLine }] : []" 
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
import { ref, onMounted, inject } from 'vue'
import { hubApi } from '../api'
import CodeEditor from './CodeEditor.vue'

// State
const changes = ref([])
const loading = ref(false)
const error = ref(null)
const applying = ref(false)
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
    const data = await hubApi.fetchPendingChanges()
    changes.value = data.map(change => ({
      ...change,
      verifyStatus: null,
      verifyError: null,
      errorLine: null
    })) || []
  } catch (e) {
    console.error('Failed to fetch pending changes:', e)
    error.value = 'Failed to fetch pending changes: ' + (e?.message || 'Unknown error')
  } finally {
    loading.value = false
  }
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

async function verifyChanges() {
  if (!changes.value.length) return
  
  verifying.value = true
  let allValid = true
  
  try {
    // 验证每个更改
    for (const change of changes.value) {
      try {
        // 调用验证API
        const result = await hubApi.verifyComponent(change.type, change.id, change.new_content)
        
        if (result.valid) {
          change.verifyStatus = 'success'
          change.verifyError = null
        } else {
          change.verifyStatus = 'error'
          change.verifyError = result.error
          allValid = false
          
          // 尝试从错误信息中提取行号
          const lineMatches = result.error.match(/line\s*(\d+)/i) || 
                             result.error.match(/line:\s*(\d+)/i) ||
                             result.error.match(/location:.*line\s*(\d+)/i);
          
          if (lineMatches && lineMatches[1]) {
            change.errorLine = parseInt(lineMatches[1]);
          }
        }
      } catch (e) {
        change.verifyStatus = 'error'
        change.verifyError = e.message || 'Verification failed'
        allValid = false
        
        // 尝试从错误信息中提取行号
        if (e.message) {
          const lineMatches = e.message.match(/line\s*(\d+)/i) || 
                             e.message.match(/line:\s*(\d+)/i) ||
                             e.message.match(/location:.*line\s*(\d+)/i);
          
          if (lineMatches && lineMatches[1]) {
            change.errorLine = parseInt(lineMatches[1]);
          }
        }
      }
    }
    
    if (allValid) {
      $message?.success?.('All changes verified successfully!')
    } else {
      $message?.error?.('Some changes failed verification. Please fix the errors before applying.')
    }
  } catch (e) {
    console.error('Failed to verify changes:', e)
    $message?.error?.('Failed to verify changes: ' + (e?.message || 'Unknown error'))
  } finally {
    verifying.value = false
  }
}

async function verifySingleChange(change) {
  verifying.value = true
  
  try {
    // 调用验证API
    const result = await hubApi.verifyComponent(change.type, change.id, change.new_content)
    
    if (result.valid) {
      change.verifyStatus = 'success'
      change.verifyError = null
      change.errorLine = null
      $message?.success?.('Verification successful!')
    } else {
      change.verifyStatus = 'error'
      change.verifyError = result.error
      
      // 尝试从错误信息中提取行号
      const lineMatches = result.error.match(/line\s*(\d+)/i) || 
                         result.error.match(/line:\s*(\d+)/i) ||
                         result.error.match(/location:.*line\s*(\d+)/i);
      
      if (lineMatches && lineMatches[1]) {
        const lineNum = parseInt(lineMatches[1]);
        change.errorLine = lineNum;
        $message?.error?.(`Verification failed at line ${lineNum}: ${result.error}`)
      } else {
        $message?.error?.(`Verification failed: ${result.error}`)
      }
    }
  } catch (e) {
    console.error('Failed to verify change:', e)
    change.verifyStatus = 'error'
    change.verifyError = e.message || 'Verification failed'
    
    // 尝试从错误信息中提取行号
    if (e.message) {
      const lineMatches = e.message.match(/line\s*(\d+)/i) || 
                         e.message.match(/line:\s*(\d+)/i) ||
                         e.message.match(/location:.*line\s*(\d+)/i);
      
      if (lineMatches && lineMatches[1]) {
        const lineNum = parseInt(lineMatches[1]);
        change.errorLine = lineNum;
        $message?.error?.(`Verification failed at line ${lineNum}: ${e.message}`)
      } else {
        $message?.error?.('Failed to verify change: ' + (e?.message || 'Unknown error'))
      }
    } else {
      $message?.error?.('Failed to verify change: ' + (e?.message || 'Unknown error'))
    }
  } finally {
    verifying.value = false
  }
}

async function applyChanges() {
  if (!changes.value.length) return
  
  applying.value = true
  
  try {
    const result = await hubApi.applyPendingChanges()
    
    // Check if any projects need to be restarted
    const projectsToRestart = findProjectsToRestart()
    if (projectsToRestart.length > 0) {
      await restartProjects(projectsToRestart)
    }
    
    $message?.success?.(`Applied successfully! Success: ${result.success_count}, Failed: ${result.failure_count}`)
    
    // Refresh the list
    await refreshChanges()
  } catch (e) {
    console.error('Failed to apply changes:', e)
    
    // 处理验证失败的情况
    if (e.verifyFailures && Array.isArray(e.verifyFailures)) {
      // 显示验证失败的详细信息
      const failedComponents = e.verifyFailures.map(f => `${getComponentTypeLabel(f.type)} ${f.id}: ${f.error}`).join('\n');
      
      $message?.error?.(`验证失败，无法应用更改:\n${failedComponents}`, { timeout: 10000 });
      
      // 如果有部分成功的更改，刷新列表
      if (e.successCount > 0) {
        await refreshChanges();
      }
    } else {
      $message?.error?.('Failed to apply changes: ' + (e?.message || 'Unknown error'))
    }
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
  } catch (e) {
    console.error('Failed to apply change:', e)
    
    // 处理验证失败的情况
    if (e.isVerificationError) {
      $message?.error?.(`验证失败，无法应用更改: ${e.message}`, { timeout: 5000 });
    } else {
      $message?.error?.('Failed to apply change: ' + (e?.message || 'Unknown error'))
    }
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
    console.error('Failed to restart projects:', e)
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
</script>

<style scoped>
pre {
  white-space: pre-wrap;
  word-wrap: break-word;
}
</style>