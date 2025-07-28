<template>
  <div class="json-viewer">
    <pre class="json-content" v-html="highlightedJson"></pre>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import Prism from 'prismjs'
import 'prismjs/components/prism-json'
import 'prismjs/themes/prism.css'

const props = defineProps({
  value: {
    type: [String, Object, Array],
    required: true
  },
  height: {
    type: String,
    default: '200px'
  }
})

// Format and highlight JSON using Prism
const highlightedJson = computed(() => {
  let jsonString = ''
  
  if (typeof props.value === 'string') {
    try {
      // Try to parse and reformat
      const parsed = JSON.parse(props.value)
      jsonString = JSON.stringify(parsed, null, 2)
    } catch {
      // If not valid JSON, just return as is
      return props.value
    }
  } else {
    jsonString = JSON.stringify(props.value, null, 2)
  }
  
  // Use Prism to highlight the JSON
  return Prism.highlight(jsonString, Prism.languages.json, 'json')
})
</script>

<style scoped>
.json-viewer {
  border: 1px solid #e5e7eb;
  border-radius: 6px;
  overflow: hidden;
  background-color: #f9fafb;
}

.json-content {
  margin: 0;
  padding: 12px;
  font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
  font-size: 12px;
  line-height: 1.4;
  color: #374151;
  white-space: pre-wrap;
  word-wrap: break-word;
  overflow-x: auto;
  background-color: transparent;
  border: none;
  outline: none;
  resize: none;
  width: 100%;
  min-height: 60px;
  max-height: 400px;
  overflow-y: auto;
}

/* Override Prism theme colors for better visibility */
.json-content :deep(.token.property) {
  color: #059669 !important;
  font-weight: 600;
}

.json-content :deep(.token.string) {
  color: #dc2626 !important;
}

.json-content :deep(.token.number) {
  color: #2563eb !important;
  font-weight: 500;
}

.json-content :deep(.token.boolean) {
  color: #7c3aed !important;
  font-weight: 500;
}

.json-content :deep(.token.null) {
  color: #7c3aed !important;
  font-weight: 500;
}

/* Selection styles */
.json-content::selection {
  background-color: #add6ff;
  color: #000000;
}

.json-content::-moz-selection {
  background-color: #add6ff;
  color: #000000;
}
</style> 