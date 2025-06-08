<template>
  <div class="duckdb-cm-simple">
    <div ref="editor" class="duckdb-cm-simple-container" />
  </div>
</template>

<script setup>
import { ref, watch, onMounted, onBeforeUnmount } from 'vue'
import { EditorView, lineNumbers, highlightActiveLineGutter, highlightSpecialChars, drawSelection, dropCursor, rectangularSelection, highlightActiveLine, keymap } from '@codemirror/view'
import { EditorState } from '@codemirror/state'
import { xml } from '@codemirror/lang-xml'
import { json } from '@codemirror/lang-json'
import { HighlightStyle, syntaxHighlighting, indentOnInput, bracketMatching } from '@codemirror/language'
import { tags } from '@lezer/highlight'
import { history, defaultKeymap, historyKeymap } from '@codemirror/commands'
import { closeBrackets } from '@codemirror/autocomplete'

const props = defineProps({
  value: String,
  language: { type: String, default: 'json' },
  readOnly: { type: Boolean, default: true },
})

const editor = ref(null)
let view = null

const getLang = () => {
  if (props.language === 'xml') return xml()
  return json()
}

// Final, pixel-perfect theme to match DuckDB
const duckDBTheme = EditorView.theme({
  '&': {
    color: '#34495e',
    backgroundColor: '#fff',
    height: '100%',
    fontSize: '14px',
    fontFamily: '"JetBrains Mono", "Menlo", "monospace"',
    lineHeight: '1.7',
  },
  '.cm-scroller': {
    overflow: 'hidden',
    fontFamily: 'inherit',
  },
  '.cm-content, .cm-gutters': {
      paddingTop: '12px',
      paddingBottom: '12px',
  },
  '.cm-gutters': {
    backgroundColor: '#fff',
    color: '#adb5bd',
    border: 'none',
    paddingLeft: '20px',
    width: '55px',
  },
  '.cm-gutterElement': {
      textAlign: 'right',
      paddingRight: '20px',
      fontWeight: 'normal',
  },
  '.cm-activeLine, .cm-activeLineGutter': {
    backgroundColor: 'transparent'
  }
}, { dark: false });

// Final, pixel-perfect highlighting style to match DuckDB
const duckDBHighlightStyle = HighlightStyle.define([
  { tag: tags.tagName, color: '#1967d2' }, 
  { tag: tags.attributeName, color: '#008073' },
  { tag: tags.attributeValue, color: '#c41a1e' },
  { tag: tags.string, color: '#c41a1e' },
  { tag: tags.number, color: '#9b59b6' },
  { tag: tags.bool, color: '#9b59b6' },
  { tag: tags.propertyName, color: '#008073' },
  { tag: tags.comment, color: '#95a5a6', fontStyle: 'italic' },
]);

onMounted(() => {
  const startState = EditorState.create({
    doc: props.value || '',
    extensions: [
      lineNumbers(),
      highlightActiveLineGutter(),
      highlightSpecialChars(),
      history(),
      drawSelection(),
      dropCursor(),
      EditorState.readOnly.of(props.readOnly),
      indentOnInput(),
      bracketMatching(),
      closeBrackets(),
      rectangularSelection(),
      highlightActiveLine(),
      keymap.of([...defaultKeymap, ...historyKeymap]),
      getLang(),
      duckDBTheme,
      syntaxHighlighting(duckDBHighlightStyle),
      EditorView.lineWrapping,
    ],
  })

  view = new EditorView({
    state: startState,
    parent: editor.value
  })
})

watch(() => props.value, (val) => {
  if (view && val !== view.state.doc.toString()) {
    view.dispatch({
      changes: { from: 0, to: view.state.doc.length, insert: val || '' }
    })
  }
})

onBeforeUnmount(() => {
  if (view) view.destroy()
})
</script>

<style scoped>
.duckdb-cm-simple {
  border: 1.5px solid #E0E7EF;
  border-radius: 12px;
  background: #fff;
  display: flex;
  height: 100%;
}
.duckdb-cm-simple-container {
  height: auto;
  font-family: 'JetBrains Mono', 'Menlo', "monospace";
  font-size: 14px;
  line-height: 1.7;
  background: #fff;
  flex: 1;
  overflow: hidden;
}
</style> 