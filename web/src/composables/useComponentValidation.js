import { ref, inject } from 'vue'
import { hubApi } from '../api'
import { extractLineNumber } from '../utils/common'

/**
 * Component validation composable
 * Centralizes validation logic for different component types
 */
export function useComponentValidation() {
  const validationResult = ref({
    isValid: true,
    errors: [],
    warnings: []
  })
  
  const errorLines = ref([])
  const showValidationPanel = ref(false)
  const verifyLoading = ref(false)
  
  // Global message component
  const $message = inject('$message', window?.$toast)
  
  /**
   * Clear validation state
   */
  const clearValidation = () => {
    validationResult.value = { isValid: true, errors: [], warnings: [] }
    errorLines.value = []
    showValidationPanel.value = false
  }
  
  /**
   * Process validation response and update UI state
   */
  const processValidationResponse = (response, componentType, showMessages = false) => {
    if (!response?.data) {
      clearValidation()
      return true
    }
    
    const data = response.data
    
    if (data.valid) {
      clearValidation()
      if (showMessages) {
        const typeLabel = getComponentTypeLabel(componentType)
        $message?.success?.(`${typeLabel} configuration is valid`)
      }
      return true
    }
    
    // Handle validation errors
    let errors = []
    let warnings = data.warnings || []
    
    if (data.errors && Array.isArray(data.errors)) {
      // Structured error response
      errors = data.errors
    } else if (data.error) {
      // Single error message  
      const lineNumber = extractLineNumber(data.error, componentType)
      errors = [{
        line: lineNumber || 'Unknown',
        message: data.error,
        detail: data.detail || null
      }]
    } else {
      // Generic error
      errors = [{
        line: 'Unknown',
        message: 'Validation failed',
        detail: null
      }]
    }
    
    validationResult.value = {
      isValid: false,
      errors,
      warnings
    }
    
    errorLines.value = errors.map(err => typeof err.line === 'number' ? err.line : extractLineNumber(err.message, componentType)).filter(Boolean)
    showValidationPanel.value = true
    
    if (showMessages) {
      const errorCount = errors.length
      const warningCount = warnings.length
      
      if (errorCount > 0) {
        $message?.error?.(`Verification failed: ${errorCount} error${errorCount > 1 ? 's' : ''} found`)
      } else if (warningCount > 0) {
        $message?.warning?.(`Verification completed with ${warningCount} warning${warningCount > 1 ? 's' : ''}`)
      }
    }
    
    return false
  }
  
  /**
   * Real-time validation (silent, no user messages)
   */
  const validateRealtime = async (componentType, componentId, content) => {
    if (!componentType || !componentId || !content) {
      return true
    }
    
    try {
      const response = await hubApi.verifyComponent(componentType, componentId, content)
      return processValidationResponse(response, componentType, false)
    } catch (error) {
      // Silent failure for real-time validation
      clearValidation()
      return true
    }
  }
  
  /**
   * Manual verification (with user messages)
   */
  const verifyComponent = async (componentType, componentId, content) => {
    if (!componentType || !componentId) {
      $message?.warning?.('Missing component information')
      return false
    }
    
    if (!content) {
      $message?.warning?.('No content to verify')
      return false
    }
    
    verifyLoading.value = true
    
    try {
      const response = await hubApi.verifyComponent(componentType, componentId, content)
      return processValidationResponse(response, componentType, true)
    } catch (error) {
      const errorMessage = error.response?.data?.error || error.message || 'Unknown verification error'
      $message?.error?.('Verification error: ' + errorMessage)
      
      const lineNumber = extractLineNumber(errorMessage, componentType)
      validationResult.value = {
        isValid: false,
        errors: [{
          line: lineNumber || 'Unknown',
          message: errorMessage,
          detail: error.response?.data?.detail || null
        }],
        warnings: []
      }
      
      errorLines.value = lineNumber ? [lineNumber] : []
      showValidationPanel.value = true
      
      return false
    } finally {
      verifyLoading.value = false
    }
  }
  
  /**
   * Pre-save validation with user confirmation
   */
  const validateBeforeSave = async (componentType, componentId, content, isNewComponent = false) => {
    if (!content) {
      return true // Allow saving empty content
    }
    
    const action = isNewComponent ? 'create' : 'save'
    
    try {
      const response = await hubApi.verifyComponent(componentType, componentId, content)
      
      if (response.data && !response.data.valid) {
        const errorMessage = response.data?.error || 'Unknown verification error'
        const confirmed = confirm(`Verification failed: ${errorMessage}\n\n${action.charAt(0).toUpperCase() + action.slice(1)} anyway?`)
        return confirmed
      }
      
      return true
    } catch (error) {
      const errorMessage = error.response?.data?.error || error.message || 'Unknown verification error'
      const confirmed = confirm(`Verification error: ${errorMessage}\n\n${action.charAt(0).toUpperCase() + action.slice(1)} anyway?`)
      return confirmed
    }
  }
  
  /**
   * Post-save verification (with messages)
   */
  const verifyAfterSave = async (componentType, componentId, action = 'saved') => {
    try {
      const response = await hubApi.verifyComponent(componentType, componentId)
      
      if (response.data && response.data.valid) {
        $message?.success?.(`${action.charAt(0).toUpperCase() + action.slice(1)} and verified successfully`)
      } else {
        const errorMessage = response.data?.error || 'Unknown verification error'
        $message?.warning?.(`${action.charAt(0).toUpperCase() + action.slice(1)} but verification failed: ${errorMessage}`)
        
        // Extract line number for highlighting
        const lineNumber = extractLineNumber(errorMessage, componentType)
        if (lineNumber) {
          errorLines.value = [lineNumber]
        }
      }
    } catch (error) {
      const errorMessage = error.response?.data?.error || error.message || 'Unknown verification error'
      $message?.warning?.(`${action.charAt(0).toUpperCase() + action.slice(1)} but verification failed: ${errorMessage}`)
      
      const lineNumber = extractLineNumber(errorMessage, componentType)
      if (lineNumber) {
        errorLines.value = [lineNumber]
      }
    }
  }
  
  return {
    validationResult,
    errorLines,
    showValidationPanel,
    verifyLoading,
    clearValidation,
    validateRealtime,
    verifyComponent,
    validateBeforeSave,
    verifyAfterSave
  }
}

/**
 * Get component type label for messages
 */
function getComponentTypeLabel(componentType) {
  const labels = {
    inputs: 'Input',
    outputs: 'Output', 
    rulesets: 'Ruleset',
    plugins: 'Plugin',
    projects: 'Project'
  }
  return labels[componentType] || componentType
} 