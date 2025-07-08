/**
 * Common utility functions
 * Used to reduce repetitive formatting and processing logic
 */

// Component type configuration
export const COMPONENT_TYPES = {
  inputs: {
    label: 'Input',
    icon: '📥',
    language: 'yaml',
    supportsConnectCheck: true
  },
  outputs: {
    label: 'Output', 
    icon: '📤',
    language: 'yaml',
    supportsConnectCheck: false  // Let individual components decide based on type
  },
  rulesets: {
    label: 'Ruleset',
    icon: '📋',
    language: 'xml',
    supportsConnectCheck: false
  },
  plugins: {
    label: 'Plugin',
    icon: '🔌',
    language: 'go',
    supportsConnectCheck: false
  },
  projects: {
    label: 'Project',
    icon: '📁',
    language: 'yaml',
    supportsConnectCheck: false
  }
}

/**
 * Get component type label
 */
export function getComponentTypeLabel(type) {
  return COMPONENT_TYPES[type]?.label || type
}

// Note: getComponentTypeIcon was removed as it was unused

/**
 * Get editor language
 */
export function getEditorLanguage(type) {
  return COMPONENT_TYPES[type]?.language || 'yaml'
}

/**
 * Check if component supports connection check
 */
export function supportsConnectCheck(type) {
  return COMPONENT_TYPES[type]?.supportsConnectCheck || false
}

/**
 * Convert API component type (singular to plural)
 */
export function getApiComponentType(type) {
  const mapping = {
    input: 'inputs',
    output: 'outputs', 
    ruleset: 'rulesets',
    project: 'projects',
    plugin: 'plugins'
  }
  return mapping[type] || (type.endsWith('s') ? type : type + 's')
}

/**
 * Format numbers
 */
export function formatNumber(num) {
  if (num >= 1000000) {
    return (num / 1000000).toFixed(1) + 'M'
  }
  if (num >= 1000) {
    return (num / 1000).toFixed(1) + 'K'
  }
  return num.toString()
}

/**
 * Format percentage
 */
export function formatPercent(num) {
  if (typeof num !== 'number' || isNaN(num)) {
    return '0.0'
  }
  return num.toFixed(1)
}

/**
 * Format daily message count
 */
export function formatMessagesPerDay(messages) {
  return formatNumber(messages || 0)
}

/**
 * Format time difference
 */
export function formatTimeAgo(date) {
  if (!date) return 'Unknown'
  
  const now = new Date()
  const diff = now - new Date(date)
  
  if (diff < 60000) { // Less than 1 minute
    return 'Just now'
  } else if (diff < 3600000) { // Less than 1 hour
    const minutes = Math.floor(diff / 60000)
    return `${minutes}m ago`
  } else if (diff < 86400000) { // Less than 1 day
    const hours = Math.floor(diff / 3600000)
    return `${hours}h ago`
  } else {
    const days = Math.floor(diff / 86400000)
    return `${days}d ago`
  }
}

/**
 * Get project status label
 */
export function getStatusLabel(status) {
  const mapping = {
    running: 'R',
    stopped: 'S',
    starting: '◐',  // Use half-circle symbol for starting
    stopping: '●',  // Use dot symbol for stopping
    error: 'E'
  }
  return mapping[status] || '?'
}

/**
 * Get status title
 */
export function getStatusTitle(item) {
  if (!item.status) return 'Unknown Status'
  
  const statusMap = {
    running: 'Running',
    stopped: 'Stopped',
    starting: 'Starting',
    stopping: 'Stopping',
    error: item.error ? `Error: ${item.error}` : 'Error'
  }
  
  return statusMap[item.status] || item.status
}

/**
 * Extract line number from error message
 * Supports project-specific line number adjustment for YAML content structure
 */
export function extractLineNumber(errorMessage, componentType = null, editorContent = null) {
  if (!errorMessage || typeof errorMessage !== 'string') {
    return null
  }
  
  const lineMatches = errorMessage.match(/yaml[:-]?line\s+(\d+)/i) ||
                      errorMessage.match(/at\s+line\s+(\d+)/i) ||
                      errorMessage.match(/line\s+(\d+)/i) || 
                      errorMessage.match(/line:\s*(\d+)/i) ||
                      errorMessage.match(/location:.*line\s*(\d+)/i) ||
                      errorMessage.match(/\(line:\s*(\d+)\)/i)
  
  if (lineMatches && lineMatches[1]) {
    let lineNumber = parseInt(lineMatches[1])
    
    // For project validation errors, adjust line number to account for YAML structure
    // Backend parses only the content part (after 'content: |'), but frontend shows full YAML
    if (componentType === 'projects' && editorContent) {
      // Check if this is a YAML file with 'content: |' structure
      const lines = editorContent.split('\n')
      for (let i = 0; i < Math.min(5, lines.length); i++) {
        if (lines[i].trim().startsWith('content:')) {
          // Found 'content:' line, backend line numbers need to be offset
          lineNumber += i + 1 // +1 for the content line itself
          break
        }
      }
    }
    
    return lineNumber
  }
  
  return null
}

/**
 * Copy text to clipboard
 */
export async function copyToClipboard(text) {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text)
    } else {
      // Fallback for older browsers
      const textArea = document.createElement('textarea')
      textArea.value = text
      textArea.style.position = 'fixed'
      textArea.style.left = '-999999px'
      textArea.style.top = '-999999px'
      document.body.appendChild(textArea)
      textArea.focus()
      textArea.select()
      document.execCommand('copy')
      textArea.remove()
    }
    return true
  } catch (err) {
    console.error('Failed to copy text: ', err)
    return false
  }
}

/**
 * Debounce function
 */
export function debounce(func, wait, immediate) {
  let timeout
  return function executedFunction(...args) {
    const later = () => {
      timeout = null
      if (!immediate) func(...args)
    }
    const callNow = immediate && !timeout
    clearTimeout(timeout)
    timeout = setTimeout(later, wait)
    if (callNow) func(...args)
  }
}

// Note: throttle function was removed as it was unused

// Note: deepClone function was removed as it was unused

/**
 * Check if component change requires restart
 */
export function needsRestart(change) {
  // Check if it's a project component, or a component used by projects
  return change.type === 'projects' || 
         (change.requires_restart === true) ||
         (change.affected_projects && change.affected_projects.length > 0)
}

/**
 * Get CPU color class
 */
export function getCPUColor(cpuPercent) {
  if (cpuPercent > 80) return 'text-red-600'
  if (cpuPercent > 60) return 'text-yellow-600'
  return 'text-green-600'
}

/**
 * Get CPU progress bar color class
 */
export function getCPUBarColor(cpuPercent) {
  if (cpuPercent > 80) return 'bg-red-500'
  if (cpuPercent > 60) return 'bg-yellow-500'
  return 'bg-green-500'
}

/**
 * Get memory color class
 */
export function getMemoryColor(memoryPercent) {
  if (memoryPercent > 85) return 'text-red-600'
  if (memoryPercent > 70) return 'text-yellow-600'
  return 'text-green-600'
}

/**
 * Get memory progress bar color class
 */
export function getMemoryBarColor(memoryPercent) {
  if (memoryPercent > 85) return 'bg-red-500'
  if (memoryPercent > 70) return 'bg-yellow-500'
  return 'bg-green-500'
} 