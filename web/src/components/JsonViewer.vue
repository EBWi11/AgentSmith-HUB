<template>
  <div class="json-viewer">
    <div ref="container" class="json-container"></div>
  </div>
</template>

<script setup>
import { ref, onMounted, onBeforeUnmount, watch } from 'vue'
import * as monaco from 'monaco-editor'

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

const container = ref(null)
let editor = null

// Format value as JSON string
const formatValue = (val) => {
  if (typeof val === 'string') {
    try {
      // Try to parse and reformat
      const parsed = JSON.parse(val)
      return JSON.stringify(parsed, null, 2)
    } catch {
      // If not valid JSON, just return as is
      return val
    }
  }
  return JSON.stringify(val, null, 2)
}

const initializeEditor = () => {
  if (!container.value) return

  // Create read-only Monaco editor with JSON language
  editor = monaco.editor.create(container.value, {
    value: formatValue(props.value),
    language: 'json',
    readOnly: true,
    theme: 'vs',
    minimap: { enabled: false },
    scrollBeyondLastLine: false,
    wordWrap: 'on',
    automaticLayout: true,
    fontSize: 12,
    lineNumbers: 'off',
    folding: true,
    glyphMargin: false,
    lineDecorationsWidth: 0,
    lineNumbersMinChars: 0,
    renderLineHighlight: 'none',
    scrollbar: {
      vertical: props.height === 'auto' ? 'hidden' : 'auto',
      horizontal: 'auto',
      verticalScrollbarSize: 8,
      horizontalScrollbarSize: 8
    }
  })

  // Set height
  if (props.height === 'auto') {
    // Calculate content height and set container height accordingly
    const lineCount = editor.getModel().getLineCount()
    const lineHeight = editor.getOption(monaco.editor.EditorOption.lineHeight)
    const contentHeight = lineCount * lineHeight + 20 // Add some padding
    container.value.style.height = `${Math.max(contentHeight, 60)}px`
    editor.layout()
  } else {
    container.value.style.height = props.height
  }
}

// Watch for value changes
watch(() => props.value, (newValue) => {
  if (editor) {
    editor.setValue(formatValue(newValue))
    
    // Recalculate height for auto mode
    if (props.height === 'auto') {
      setTimeout(() => {
        const lineCount = editor.getModel().getLineCount()
        const lineHeight = editor.getOption(monaco.editor.EditorOption.lineHeight)
        const contentHeight = lineCount * lineHeight + 20 // Add some padding
        container.value.style.height = `${Math.max(contentHeight, 60)}px`
        editor.layout()
      }, 0)
    }
  }
}, { deep: true })

onMounted(() => {
  initializeEditor()
})

onBeforeUnmount(() => {
  if (editor) {
    editor.dispose()
  }
})
</script>

<style scoped>
.json-viewer {
  border: 1px solid #e5e7eb;
  border-radius: 6px;
  overflow: hidden;
}

.json-container {
  width: 100%;
  min-height: 100px;
}
</style> 