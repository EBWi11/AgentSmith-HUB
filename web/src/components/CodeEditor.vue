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
  language: { type: String, default: 'yaml' },
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
    fontSize: '13px',
    fontFamily: 'inherit',
    lineHeight: 'normal',
  },
  '.cm-scroller': {
    overflow: 'hidden',
    fontFamily: 'inherit',
  },
  '.cm-content, .cm-gutters': {
    paddingTop: '0',
    paddingBottom: '0',
  },
  '.cm-gutters': {
    backgroundColor: '#fff',
    color: '#adb5bd',
    border: 'none',
    paddingLeft: '10px',
    width: '38px',
    minWidth: 'unset',
    fontFamily: 'inherit',
  },
  '.cm-gutterElement': {
    textAlign: 'right',
    paddingRight: '6px',
    marginLeft: '0',
    fontFamily: 'inherit',
    lineHeight: '20px',
    height: '20px',
  },
  '.cm-line': {
    paddingLeft: '10px',
    lineHeight: '20px',
    height: '20px',
    fontFamily: 'inherit',
  }
}, { dark: false });

// Final, pixel-perfect highlighting style to match DuckDB
const duckDBHighlightStyle = HighlightStyle.define([
  { tag: tags.tagName, color: '#3366ae' },
  { tag: tags.attributeName, color: '#367719' },
  { tag: tags.attributeValue, color: '#a63437' },
  { tag: tags.string, color: '#a63437' },
  { tag: tags.number, color: '#17572d' },
  { tag: tags.bool, color: '#17572d' },
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