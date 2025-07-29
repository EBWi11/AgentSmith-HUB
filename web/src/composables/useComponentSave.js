import { ref, inject } from 'vue'
import { hubApi } from '../api'
import { useDataCacheStore } from '../stores/dataCache'

/**
 * Component save operations composable
 * Centralizes save logic for creating and updating components
 */
export function useComponentSave() {
  const saving = ref(false)
  const saveError = ref('')
  const preventRefetch = ref(false)
  const dataCache = useDataCacheStore()
  
  // Global message component
  const $message = inject('$message', window?.$toast)
  
  /**
   * Generic save operation for both new and existing components
   */
  const saveComponent = async (
    componentType,
    componentId, 
    content,
    isNewComponent = false,
    options = {}
  ) => {
    const {
      validateBeforeSave,
      verifyAfterSave,
      onSuccess,
      fetchDetail,
      exitToViewMode = true // New option to control whether to exit to view mode
    } = options
    
    if (!componentType || !componentId) {
      saveError.value = 'Invalid component information'
      return false
    }

    // Special handling for new components: ensure content is not empty
    if (isNewComponent && (!content || content.trim() === '')) {
      saveError.value = 'Component content cannot be empty'
      return false
    }

    saveError.value = ''
    saving.value = true
    
    try {
      // Pre-save validation with user confirmation
      if (validateBeforeSave) {
        const shouldProceed = await validateBeforeSave(componentType, componentId, content, isNewComponent)
        if (!shouldProceed) {
          saving.value = false
          return false
        }
      }
      
      // Set flag to prevent unnecessary re-fetching during save
      preventRefetch.value = true
      
      // Perform the save operation
      let response
      if (isNewComponent) {
        console.log('useComponentSave: Calling hubApi.saveNew', { componentType, componentId, contentLength: content.length })
        response = await hubApi.saveNew(componentType, componentId, content)
        console.log('useComponentSave: hubApi.saveNew response', response)
      } else {
        response = await hubApi.saveEdit(componentType, componentId, content)
      }
      
      // Call success callback immediately to switch to view mode
      if (onSuccess) {
        onSuccess({ type: componentType, id: componentId, exitToViewMode })
      }
      
      // Add a delay to ensure backend has processed the save and we can read the saved value
      await new Promise(resolve => setTimeout(resolve, 500))
      
      // Handle post-save operations after switching to view mode
      if (fetchDetail) {
        // For edit operations, refresh the content in view mode
        if (!isNewComponent) {
          await fetchDetail({ type: componentType, id: componentId }, false)
        }
        // For new components, we don't need to fetch detail since the component
        // is still in temporary state and will be available after deployment
      }
      
      // Post-save verification with messages (after view mode switch)
      if (verifyAfterSave) {
        const action = isNewComponent ? 'created' : 'saved'
        await verifyAfterSave(componentType, componentId, action)
      }
      
      // Clear all cache since component save can affect multiple data types
      const cacheAction = isNewComponent ? 'create' : 'save'
      setTimeout(() => {
        dataCache.clearAll(`${cacheAction} component: ${componentType}:${componentId}`)
      }, 1500)
      
      // Clear the prevent refetch flag after a delay
      setTimeout(() => {
        preventRefetch.value = false
      }, 500)
      
      return true
      
    } catch (error) {
      // Handle save errors
      saveError.value = error.response?.data?.error || error.message || `Failed to ${isNewComponent ? 'create' : 'save'}`
      $message?.error?.('Error: ' + saveError.value)
      return false
    } finally {
      saving.value = false
    }
  }
  
  /**
   * Save existing component (edit mode)
   */
  const saveEdit = async (componentType, componentId, content, options = {}) => {
    return await saveComponent(componentType, componentId, content, false, options)
  }
  
  /**
   * Save new component (create mode)
   */
  const saveNew = async (componentType, componentId, content, options = {}) => {
    return await saveComponent(componentType, componentId, content, true, options)
  }
  
  return {
    saving,
    saveError,
    preventRefetch,
    saveComponent,
    saveEdit,
    saveNew
  }
} 