<template>
  <div v-if="loading" class="flex items-center justify-center h-full text-gray-400 text-lg">Loading...</div>
  <div v-else-if="error" class="flex items-center justify-center h-full text-red-400 text-lg">{{ error }}</div>
  
  <!-- Create Mode -->
  <div v-else-if="props.item && props.item.isNew" class="h-full flex flex-col">
    <CodeEditor v-model:value="editorValue" :language="props.item.type === 'rulesets' ? 'xml' : (props.item.type === 'plugins' ? 'go' : 'yaml')" :read-only="false" :error-lines="errorLines" class="flex-1" @save="content => saveNew(content)" />
    <div v-if="saveError" class="text-xs text-red-500 mt-2">{{ saveError }}</div>
  </div>

  <!-- Edit Mode -->
  <div v-else-if="props.item && props.item.isEdit && detail" class="h-full flex flex-col">
    <!-- Ruleset validation status -->
    <div v-if="isRuleset && validationResult.errors.length > 0" class="validation-errors p-3 mb-3 bg-red-50 border-l-4 border-red-500 text-red-700">
      <h3 class="font-bold text-sm">Validation Errors</h3>
      <ul class="mt-1 text-xs">
        <li v-for="(error, index) in validationResult.errors" :key="index" class="mb-1">
          <span class="font-semibold">Line {{ error.line }}:</span> 
          {{ error.message }}
          <span v-if="error.detail" class="block ml-4 text-red-600 italic">{{ error.detail }}</span>
        </li>
      </ul>
    </div>

    <div v-if="isRuleset && validationResult.warnings.length > 0" class="validation-warnings p-3 mb-3 bg-yellow-50 border-l-4 border-yellow-500 text-yellow-700">
      <h3 class="font-bold text-sm">Validation Warnings</h3>
      <ul class="mt-1 text-xs">
        <li v-for="(warning, index) in validationResult.warnings" :key="index" class="mb-1">
          <span class="font-semibold">Line {{ warning.line }}:</span> 
          {{ warning.message }}
          <span v-if="warning.detail" class="block ml-4 text-yellow-600 italic">{{ warning.detail }}</span>
        </li>
      </ul>
    </div>
    
    <CodeEditor v-model:value="editorValue" :language="props.item.type === 'rulesets' ? 'xml' : (props.item.type === 'plugins' ? 'go' : 'yaml')" :read-only="false" :error-lines="errorLines" class="flex-1" @save="content => saveEdit(content)" />
    <div class="flex justify-end mt-4 px-4 space-x-3 border-t pt-4 pb-3">
      <button 
        @click="cancelEdit" 
        class="px-3 py-1.5 border border-gray-300 text-gray-700 text-sm rounded hover:bg-gray-50 transition-colors duration-150 focus:outline-none focus:ring-1 focus:ring-gray-300"
      >
        Cancel
      </button>
      <button 
        @click="() => saveEdit()" 
        class="px-3 py-1.5 bg-blue-500 text-white text-sm rounded hover:bg-blue-600 transition-colors duration-150 focus:outline-none focus:ring-1 focus:ring-blue-500 flex items-center space-x-1.5"
        :disabled="saving"
      >
        <span v-if="saving" class="w-3 h-3 border-1.5 border-white border-t-transparent rounded-full animate-spin"></span>
        <span>{{ saving ? 'Saving...' : 'Save' }}</span>
      </button>
    </div>
    <div v-if="saveError" class="text-xs text-red-500 mt-2 px-4 mb-3">{{ saveError }}</div>
  </div>

  <!-- Special layout for projects -->
  <div v-else-if="props.item && props.item.type === 'projects' && detail && detail.raw" class="flex h-full">
    <div class="w-1/2 h-full">
       <CodeEditor :value="detail.raw" :language="'yaml'" :read-only="true" class="h-full" />
    </div>
    <div class="w-1/2 h-full border-l border-gray-200">
      <ProjectWorkflow :projectContent="detail.raw" />
    </div>
  </div>

  <!-- Default layout for other components -->
  <div v-else-if="detail && detail.raw" class="h-full">
    <CodeEditor :value="detail.raw" :language="props.item.type === 'rulesets' ? 'xml' : (props.item.type === 'plugins' ? 'go' : 'yaml')" :read-only="true" class="h-full" />
  </div>

  <div v-else class="flex items-center justify-center h-full text-gray-400 text-lg">
    No content available
  </div>
</template>

<script setup>
import { ref, watch, inject, computed, onMounted } from 'vue'
import { hubApi } from '../api'
import CodeEditor from './CodeEditor.vue'
import ProjectWorkflow from './Visualization/ProjectWorkflow.vue'
import { useStore } from 'vuex'
import { validateRulesetXml } from '../utils/rulesetValidator'
import { getDefaultTemplate } from '../utils/templateGenerator'

// Props
const props = defineProps({
  item: Object
})

// Emits
const emit = defineEmits(['created', 'updated', 'cancel-edit'])

// Reactive state
const loading = ref(false)
const error = ref(null)
const detail = ref(null)
const editorValue = ref('')
const saveError = ref('')
const saving = ref(false)
const originalContent = ref('') // Save original content for restoring when canceling edit
const errorLines = ref([]) // 错误行数组
const validationResult = ref({
  isValid: true,
  errors: [],
  warnings: []
})
const isRuleset = computed(() => {
  return props.item.type === 'rulesets'
})

// Global message component
const $message = inject('$message', window?.$toast)
const store = useStore()
const router = useRouter()

// Watch for item changes
watch(
  () => props.item,
  (newVal) => {
    if (newVal && newVal.isNew) {
      detail.value = null
      editorValue.value = getTemplateForComponent(newVal.type, newVal.id)
      errorLines.value = [] // 清空错误行
    } else if (newVal && newVal.isEdit) {
      fetchDetail(newVal, true)
      errorLines.value = [] // 清空错误行
    } else {
      fetchDetail(newVal)
      errorLines.value = [] // 清空错误行
    }
  },
  { immediate: true }
)

// 从错误信息中提取行号
function extractLineNumber(errorMessage) {
  if (!errorMessage) return null;
  
  // 尝试从错误信息中提取行号
  const lineMatches = errorMessage.match(/line\s*(\d+)/i) || 
                      errorMessage.match(/line:\s*(\d+)/i) ||
                      errorMessage.match(/location:.*line\s*(\d+)/i);
  
  if (lineMatches && lineMatches[1]) {
    return parseInt(lineMatches[1]);
  }
  
  return null;
}

// Methods
async function fetchDetail(item, forEdit = false) {
  detail.value = null
  error.value = null
  if (!item || !item.id) return
  loading.value = true
  try {
    let data
    switch (item.type) {
      case 'inputs':
        data = await hubApi.getInput(item.id); break
      case 'outputs':
        data = await hubApi.getOutput(item.id); break
      case 'rulesets':
        data = await hubApi.getRuleset(item.id); break
      case 'projects':
        data = await hubApi.getProject(item.id); break
      case 'plugins':
        data = await hubApi.getPlugin(item.id); break
      default:
        data = null
    }
    detail.value = data
    if (forEdit) {
      // Initialize with empty string even if raw is empty to allow editing
      editorValue.value = data?.raw || ''
      originalContent.value = data?.raw || '' // Save original content
      
      // Check if this is already a temporary file
      const isAlreadyTemp = item.isNew || (data && data.path && data.path.endsWith('.new'));
      console.log('Component details:', {
        id: item.id,
        type: item.type,
        isNew: item.isNew,
        path: data?.path,
        isAlreadyTemp
      });
      
      // Only create a temporary file if this is not already a temporary file
      if (!isAlreadyTemp) {
        try {
          // Convert plural component type to singular for API call
          const singularType = item.type.endsWith('s') ? item.type.slice(0, -1) : item.type;
          console.log('Creating temporary file for', singularType, item.id);
          
          // Create a temporary file for editing, but don't submit changes
          const response = await hubApi.createTempFile(singularType, item.id);
          console.log('Temporary file creation response:', response);
        } catch (e) {
          // Only show error message on failure
          console.error('Failed to create temporary file:', e);
          if (e.response) {
            console.error('Error response:', e.response.status, e.response.data);
          }
          $message?.error?.('Failed to create temporary file: ' + (e?.message || 'Unknown error'))
        }
      } else {
        console.log('Skipping temporary file creation as this is already a temporary file');
      }
    }
  } catch (e) {
    error.value = 'Failed to load details'
    console.error('Failed to load details:', e);
    if (e.response) {
      console.error('Error response:', e.response.status, e.response.data);
    }
  } finally {
    loading.value = false
  }
}

// 添加验证函数
const validateRuleset = () => {
  if (isRuleset.value && editorValue.value) {
    const result = validateRulesetXml(editorValue.value)
    validationResult.value = result
    
    // 更新错误行高亮
    errorLines.value = result.errors.map(error => error.line)
    return result.isValid
  }
  return true
}

// 监听编辑内容变化，进行实时验证
watch(editorValue, (newContent) => {
  if (isRuleset.value && newContent) {
    validateRuleset()
  }
}, { deep: true })

// 在组件挂载时进行初始验证
onMounted(() => {
  if (isRuleset.value && editorValue.value) {
    validateRuleset()
  }
  
  // 如果是项目类型，获取所有组件列表
  if (props.item && props.item.type === 'projects') {
    store.dispatch('fetchAllComponents')
  }
})

async function saveEdit(content) {
  // 如果直接从CodeEditor的@save事件调用，content会有值
  // 如果从按钮点击调用，content会是undefined
  const contentToSave = content !== undefined ? content : editorValue.value
  
  // 对ruleset进行验证
  if (isRuleset.value) {
    const isValid = validateRuleset()
    if (!isValid && !confirm('Ruleset contains validation errors. Save anyway?')) {
      return
    }
  }
  
  saveError.value = ''
  saving.value = true
  
  try {
    // 保存组件
    const response = await hubApi.saveEdit(props.item.type, props.item.id, contentToSave)
    console.log('Save response:', response)
    
    // 如果是ruleset，保存后进行验证
    if (isRuleset.value) {
      try {
        const verifyRes = await verifyComponent(props.item.type, props.item.id)
        if (verifyRes.status === 200) {
          $message?.success?.('Saved and verified successfully')
        } else {
          $message?.warning?.('Saved but verification failed: ' + verifyRes.data)
          // 解析错误信息中的行号
          const errorMessage = verifyRes.data
          const lineNumber = extractLineNumber(errorMessage)
          if (lineNumber) {
            errorLines.value = [lineNumber]
          }
        }
      } catch (verifyErr) {
        $message?.warning?.('Saved but verification failed: ' + verifyErr.message)
        const lineNumber = extractLineNumber(verifyErr.message)
        if (lineNumber) {
          errorLines.value = [lineNumber]
        }
      }
    } else {
      $message?.success?.('Saved successfully')
    }
    
    // 更新组件列表
    emit('updated', props.item.id)
  } catch (err) {
    console.error('Failed to save edit:', err)
    saveError.value = err.response?.data || err.message || 'Failed to save'
    $message?.error?.('Error: ' + saveError.value)
  } finally {
    saving.value = false
  }
}

async function saveNew(content) {
  // 如果直接从CodeEditor的@save事件调用，content会有值
  // 如果从按钮点击调用，content会是undefined
  const contentToSave = content !== undefined ? content : editorValue.value
  
  // 对ruleset进行验证
  if (isRuleset.value) {
    const isValid = validateRuleset()
    if (!isValid && !confirm('Ruleset contains validation errors. Save anyway?')) {
      return
    }
  }
  
  saveError.value = ''
  saving.value = true
  
  try {
    // 保存新组件
    const response = await hubApi.saveNew(props.item.type, props.item.id, contentToSave)
    console.log('Save new response:', response)
    
    // 如果是ruleset，保存后进行验证
    if (isRuleset.value) {
      try {
        const verifyRes = await verifyComponent(props.item.type, props.item.id)
        if (verifyRes.status === 200) {
          $message?.success?.('Created and verified successfully')
        } else {
          $message?.warning?.('Created but verification failed: ' + verifyRes.data)
          // 解析错误信息中的行号
          const errorMessage = verifyRes.data
          const lineNumber = extractLineNumber(errorMessage)
          if (lineNumber) {
            errorLines.value = [lineNumber]
          }
        }
      } catch (verifyErr) {
        $message?.warning?.('Created but verification failed: ' + verifyErr.message)
        const lineNumber = extractLineNumber(verifyErr.message)
        if (lineNumber) {
          errorLines.value = [lineNumber]
        }
      }
    } else {
      $message?.success?.('Created successfully')
    }
    
    // 通知父组件创建成功
    emit('created', props.item.id)
  } catch (err) {
    console.error('Failed to create new component:', err)
    saveError.value = err.response?.data || err.message || 'Failed to create'
    $message?.error?.('Error: ' + saveError.value)
  } finally {
    saving.value = false
  }
}

function cancelEdit() {
  // Restore original content
  editorValue.value = originalContent.value
  if (detail.value) detail.value.raw = originalContent.value
  // Clear error messages
  saveError.value = ''
  errorLines.value = [] // 清空错误行
  // Exit edit mode
  emit('cancel-edit', props.item)
}

function getLanguage(type) {
  switch (type) {
    case 'rulesets':
      return 'xml'
    case 'plugins':
      return 'go'
    case 'yaml':
      return 'yaml'
    default:
      return 'yaml'
  }
}

function getTemplateForComponent(type, id) {
  // 传递store参数，特别是对于项目类型
  return getDefaultTemplate(type, id, store);
}
</script> 

<style scoped>

/* 验证错误和警告样式 */
.validation-errors, .validation-warnings {
  margin-bottom: 15px;
  padding: 10px;
  border-radius: 4px;
}

.validation-errors {
  background-color: rgba(244, 67, 54, 0.1);
  border-left: 4px solid #f44336;
}

.validation-warnings {
  background-color: rgba(255, 152, 0, 0.1);
  border-left: 4px solid #ff9800;
}

.validation-errors h3, .validation-warnings h3 {
  margin-top: 0;
  font-size: 16px;
  font-weight: bold;
}

</style> 