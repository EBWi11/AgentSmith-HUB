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
      fetchDetail
    } = options
    
    if (!componentType || !componentId) {
      saveError.value = 'Invalid component information'
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
        response = await hubApi.saveNew(componentType, componentId, content)
      } else {
        response = await hubApi.saveEdit(componentType, componentId, content)
      }
      
      // Add a small delay to ensure backend has processed the save
      await new Promise(resolve => setTimeout(resolve, 200))
      
      // Handle post-save operations
      if (fetchDetail) {
        // For edit operations, refresh the content while staying in edit mode
        if (!isNewComponent) {
          await fetchDetail({ type: componentType, id: componentId }, true)
        }
      }
      
      // Post-save verification with messages
      if (verifyAfterSave) {
        const action = isNewComponent ? 'created' : 'saved'
        await verifyAfterSave(componentType, componentId, action)
      }
      
      // Clear all cache since component save can affect multiple data types
      const action = isNewComponent ? 'create' : 'save'
      setTimeout(() => {
        dataCache.clearAll(`${action} component: ${componentType}:${componentId}`)
      }, 1500)
      
      // Call success callback if provided
      if (onSuccess) {
        // Use setTimeout to avoid re-render issues
        setTimeout(() => {
          onSuccess({ type: componentType, id: componentId })
          // Clear the prevent refetch flag after a delay  
          setTimeout(() => {
            preventRefetch.value = false
          }, 500)
        }, 100)
      }
      
      return true
      
    } catch (error) {
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