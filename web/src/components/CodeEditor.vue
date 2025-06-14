<template>
  <div class="duckdb-cm-simple">
    <div ref="editor" class="duckdb-cm-simple-container" />
  </div>
</template>

<script setup>
import { ref, watch, onMounted, onBeforeUnmount, getCurrentInstance, computed } from 'vue'
import { EditorView, lineNumbers, highlightActiveLineGutter, highlightSpecialChars, drawSelection, dropCursor, rectangularSelection, highlightActiveLine, keymap, Decoration } from '@codemirror/view'
import { EditorState, StateField, StateEffect } from '@codemirror/state'
import { xml } from '@codemirror/lang-xml'
import { json } from '@codemirror/lang-json'
import { yaml } from '@codemirror/lang-yaml'
import { HighlightStyle, syntaxHighlighting, indentOnInput, bracketMatching } from '@codemirror/language'
import { tags } from '@lezer/highlight'
import { history, defaultKeymap, historyKeymap } from '@codemirror/commands'
import { closeBrackets, autocompletion, completionKeymap } from '@codemirror/autocomplete'
import { useStore } from 'vuex'

const props = defineProps({
  value: String,
  language: { type: String, default: 'yaml' },
  readOnly: { type: Boolean, default: true },
  errorLines: { type: Array, default: () => [] }, // 错误行数组，每个元素包含 {line, message}
})

const editor = ref(null)
let view = null
const { emit } = getCurrentInstance();

// 定义错误行高亮效果
const addErrorLine = StateEffect.define();
const clearErrorLines = StateEffect.define();

// 创建错误行装饰器
const errorLineField = StateField.define({
  create() {
    return Decoration.none;
  },
  update(decorations, tr) {
    decorations = decorations.map(tr.changes);
    
    for (let effect of tr.effects) {
      if (effect.is(addErrorLine)) {
        const line = effect.value;
        const linePos = tr.state.doc.line(line);
        const deco = Decoration.line({
          attributes: { class: "cm-error-line" }
        });
        decorations = decorations.update({
          add: [deco.range(linePos.from)]
        });
      } else if (effect.is(clearErrorLines)) {
        decorations = Decoration.none;
      }
    }
    
    return decorations;
  },
  provide: f => EditorView.decorations.from(f)
});

// 添加store
const store = useStore()

// 获取store中的数据
const availablePlugins = computed(() => store.getters.getAvailablePlugins)
const nodeTypes = computed(() => store.getters.getNodeTypes)
const logicTypes = computed(() => store.getters.getLogicTypes)
const countTypes = computed(() => store.getters.getCountTypes)
const rootTypes = computed(() => store.getters.getRootTypes)
const commonFields = computed(() => store.getters.getCommonFields)

// 获取组件列表
const inputComponents = computed(() => store.getters.getComponents('inputs'))
const outputComponents = computed(() => store.getters.getComponents('outputs'))
const rulesetComponents = computed(() => store.getters.getComponents('rulesets'))
const pluginComponents = computed(() => store.getters.getComponents('plugins'))

// 在组件挂载时获取插件列表和组件列表
onMounted(() => {
  store.dispatch('fetchAvailablePlugins')
  store.dispatch('fetchComponents', 'inputs')
  store.dispatch('fetchComponents', 'outputs')
  store.dispatch('fetchComponents', 'rulesets')
  store.dispatch('fetchComponents', 'plugins')
})

// Define custom completions for different languages
function getCompletions(context) {
  const word = context.matchBefore(/\w*/)
  if (!word || word.from === word.to && !context.explicit) return null

  // Get the current line up to the cursor position
  const line = context.state.doc.lineAt(context.pos)
  const lineText = line.text.slice(0, context.pos - line.from)
  
  let completions = []
  
  // Project completions
  if (props.language === 'yaml' && context.state.doc.toString().includes('flow:')) {
    // 如果在输入组件引用（from或to后面）
    if (lineText.includes('from:') || lineText.includes('to:')) {
      completions = [
        { label: 'input.', detail: 'Input component', apply: 'input.' },
        { label: 'ruleset.', detail: 'Ruleset component', apply: 'ruleset.' },
        { label: 'output.', detail: 'Output component', apply: 'output.' },
        { label: 'plugin.', detail: 'Plugin component', apply: 'plugin.' }
      ]
      
      // 如果已经输入了组件类型前缀，提供该类型的具体组件
      if (lineText.includes('input.')) {
        completions = inputComponents.value.map(comp => ({
          label: `input.${comp.id}`,
          detail: `Input: ${comp.id}`,
          apply: `input.${comp.id}`
        }))
      } else if (lineText.includes('ruleset.')) {
        completions = rulesetComponents.value.map(comp => ({
          label: `ruleset.${comp.id}`,
          detail: `Ruleset: ${comp.id}`,
          apply: `ruleset.${comp.id}`
        }))
      } else if (lineText.includes('output.')) {
        completions = outputComponents.value.map(comp => ({
          label: `output.${comp.id}`,
          detail: `Output: ${comp.id}`,
          apply: `output.${comp.id}`
        }))
      } else if (lineText.includes('plugin.')) {
        completions = pluginComponents.value.map(comp => ({
          label: `plugin.${comp.id}`,
          detail: `Plugin: ${comp.id}`,
          apply: `plugin.${comp.id}`
        }))
      }
    } else if (lineText.trim().startsWith('-')) {
      completions = [
        { label: 'from:', detail: 'Source component', apply: 'from: "' },
        { label: 'to:', detail: 'Destination component', apply: 'to: "' }
      ]
    } else if (lineText.trim() === '' || lineText.trim() === 'flow:') {
      completions = [
        { label: '- from:', detail: 'New flow connection', apply: '- from: "' }
      ]
    } else if (lineText.includes('name:')) {
      completions = [
        { label: 'flow:', detail: 'Define flow connections', apply: 'flow:\n  ' }
      ]
    }
  } 
  // Input completions
  else if (props.language === 'yaml' && lineText.includes('type:')) {
    completions = [
      { label: 'file', detail: 'File input type', apply: 'file' },
      { label: 'kafka', detail: 'Kafka input type', apply: 'kafka' },
      { label: 'http', detail: 'HTTP input type', apply: 'http' }
    ]
  }
  // Output completions
  else if (props.language === 'yaml' && lineText.includes('type:') && !context.state.doc.toString().includes('input')) {
    completions = [
      { label: 'kafka', detail: 'Kafka output type', apply: 'kafka' },
      { label: 'http', detail: 'HTTP output type', apply: 'http' },
      { label: 'file', detail: 'File output type', apply: 'file' }
    ]
  }
  // Ruleset XML completions - 增强版
  else if (props.language === 'xml') {
    // 根标签提示
    if (lineText.trim() === '' || lineText.trim() === '<?xml version="1.0" encoding="UTF-8"?>') {
      completions = [
        { label: '<root>', detail: 'Root element', apply: '<root type="DETECTION">\n  ' },
      ]
    }
    // root标签属性提示
    else if (lineText.includes('<root')) {
      completions = rootTypes.value.map(type => ({
        label: `type="${type.value}"`, 
        detail: type.detail, 
        apply: `type="${type.value}"`
      }))
    }
    // rule标签及属性提示
    else if (lineText.trim() === '' || (lineText.includes('<root') && !lineText.includes('<rule'))) {
      completions = [
        { label: '<rule>', detail: 'Rule element', apply: '<rule id="" name="" author="">\n    ' },
      ]
    }
    else if (lineText.includes('<rule')) {
      completions = [
        { label: 'id=', detail: 'Rule ID attribute', apply: 'id="rule_id"' },
        { label: 'name=', detail: 'Rule name attribute', apply: 'name="Rule Name"' },
        { label: 'author=', detail: 'Rule author attribute', apply: 'author="Author Name"' },
      ]
    }
    // rule子元素提示
    else if (lineText.includes('<rule') && !lineText.includes('</rule>')) {
      const indent = lineText.match(/^\s*/)?.[0] || '';
      completions = [
        { label: '<filter>', detail: 'Filter element', apply: '<filter field="">' },
        { label: '<checklist>', detail: 'Checklist element', apply: '<checklist condition="">\n' + indent + '    ' },
        { label: '<threshold>', detail: 'Threshold element', apply: '<threshold group_by="" range="" local_cache="true" count_type="SUM" count_field="">' },
        { label: '<append>', detail: 'Append element', apply: '<append field_name="">' },
        { label: '<plugin>', detail: 'Plugin element', apply: '<plugin>plugin_name()</plugin>' },
        { label: '<del>', detail: 'Delete fields element', apply: '<del>field1,field2</del>' },
      ]
    }
    // filter标签属性提示
    else if (lineText.includes('<filter')) {
      completions = [
        { label: 'field=', detail: 'Field to filter on', apply: 'field="data_type"' },
      ]
    }
    // checklist标签属性提示
    else if (lineText.includes('<checklist')) {
      completions = [
        { label: 'condition=', detail: 'Logical condition', apply: 'condition="a and (b or c)"' },
      ]
    }
    // checklist子元素提示
    else if (lineText.includes('<checklist') && !lineText.includes('</checklist>')) {
      completions = [
        { label: '<node>', detail: 'Check node element', apply: '<node id="" type="" field="">' },
      ]
    }
    // node标签属性提示
    else if (lineText.includes('<node')) {
      completions = [
        { label: 'id=', detail: 'Node ID for condition reference', apply: 'id="a"' },
        { label: 'type=', detail: 'Check type', apply: 'type="' },
        { label: 'field=', detail: 'Field to check', apply: 'field="data"' },
        { label: 'logic=', detail: 'Logic for multiple values', apply: 'logic="or"' },
        { label: 'delimiter=', detail: 'Delimiter for multiple values', apply: 'delimiter="|"' },
      ]
    }
    // node类型提示
    else if (lineText.includes('type="') || lineText.includes('type=\'')) {
      completions = nodeTypes.value.map(type => ({
        label: type.value, 
        detail: type.detail, 
        apply: type.value
      }))
    }
    // logic提示
    else if (lineText.includes('logic="') || lineText.includes('logic=\'')) {
      completions = logicTypes.value.map(type => ({
        label: type.value, 
        detail: type.detail, 
        apply: type.value
      }))
    }
    // threshold标签属性提示
    else if (lineText.includes('<threshold')) {
      completions = [
        { label: 'group_by=', detail: 'Fields to group by', apply: 'group_by="exe,data_type"' },
        { label: 'range=', detail: 'Time range', apply: 'range="30s"' },
        { label: 'local_cache=', detail: 'Use local cache', apply: 'local_cache="true"' },
        { label: 'count_type=', detail: 'Count type', apply: 'count_type="' },
        { label: 'count_field=', detail: 'Field to count', apply: 'count_field="dip"' },
      ]
    }
    // count_type提示
    else if (lineText.includes('count_type="') || lineText.includes('count_type=\'')) {
      completions = countTypes.value.map(type => ({
        label: type.value, 
        detail: type.detail, 
        apply: type.value
      }))
    }
    // append标签属性提示
    else if (lineText.includes('<append')) {
      completions = [
        { label: 'field_name=', detail: 'Field name to append', apply: 'field_name="data_type"' },
        { label: 'type=', detail: 'Append type', apply: 'type="PLUGIN"' },
      ]
    }
    // 常用字段名提示
    else if (lineText.includes('field="') || lineText.includes('field=\'') || 
             lineText.includes('field_name="') || lineText.includes('field_name=\'') || 
             lineText.includes('count_field="') || lineText.includes('count_field=\'')) {
      completions = commonFields.value.map(field => ({
        label: field.value, 
        detail: field.detail, 
        apply: field.value
      }))
    }
    // 插件函数提示
    else if ((lineText.includes('<node type="PLUGIN"') || 
              lineText.includes('<append type="PLUGIN"') || 
              lineText.includes('<plugin>')) && 
             !lineText.includes('(')) {
      // 如果有可用的插件列表，则提供插件名称的自动完成
      if (availablePlugins.value && availablePlugins.value.length > 0) {
        completions = availablePlugins.value.map(plugin => ({
          label: plugin.name,
          detail: plugin.description || 'Plugin function',
          apply: `${plugin.name}(_$ORIDATA)`
        }))
      } else {
        // 默认插件示例
        completions = [
          { label: 'plugin_name', detail: 'Example plugin function', apply: 'plugin_name(_$ORIDATA)' }
        ]
      }
    }
    // 原始数据引用提示
    else if (lineText.includes('<node') || lineText.includes('<append') || lineText.includes('<plugin')) {
      if (!completions.length) {
        completions = [
          { label: '_$', detail: 'Reference raw data field', apply: '_$' },
          { label: '_$ORIDATA', detail: 'Reference entire raw data', apply: '_$ORIDATA' },
        ]
      }
    }
  }
  
  return {
    from: word.from,
    options: completions,
    filter: false
  }
}

const getLang = () => {
  if (props.language === 'xml') return xml()
  if (props.language === 'yaml') return yaml()
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
  },
  // 错误行高亮样式
  '.cm-error-line': {
    backgroundColor: 'rgba(255, 0, 0, 0.1)',
  },
  // Autocomplete styling
  '.cm-tooltip': {
    border: '1px solid #ddd',
    backgroundColor: 'white',
    borderRadius: '4px',
    boxShadow: '0 2px 8px rgba(0, 0, 0, 0.15)',
  },
  '.cm-tooltip.cm-tooltip-autocomplete': {
    '& > ul': {
      maxHeight: '200px',
      fontFamily: 'inherit',
      fontSize: '13px',
    },
    '& > ul > li': {
      padding: '4px 8px',
    },
    '& > ul > li[aria-selected]': {
      backgroundColor: '#e5f3ff',
      color: '#34495e',
    }
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
  const saveKeymap = keymap.of([{
    key: 'Mod-Enter',
    run() {
      const content = view.state.doc.toString();
      console.log('Save triggered with content:', {
        content,
        type: typeof content,
        length: content.length
      });
      emit('save', content);
      return true;
    }
  }]);
  
  // Custom completion source
  const customCompletions = autocompletion({
    override: [getCompletions],
    defaultKeymap: true,
    activateOnTyping: true,
    icons: false
  })
  
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
      keymap.of([...defaultKeymap, ...historyKeymap, ...completionKeymap]),
      getLang(),
      duckDBTheme,
      syntaxHighlighting(duckDBHighlightStyle),
      EditorView.lineWrapping,
      saveKeymap,
      // 错误行高亮
      errorLineField,
      // Only enable autocompletion when not in read-only mode
      !props.readOnly ? customCompletions : [],
      EditorView.updateListener.of(update => {
        if (update.docChanged) {
          const content = update.state.doc.toString();
          console.log('Editor content updated:', {
            content,
            type: typeof content,
            length: content.length
          });
          emit('update:value', content);
        }
      })
    ],
  })

  view = new EditorView({
    state: startState,
    parent: editor.value
  })
  
  // 初始化错误行高亮
  updateErrorLines(props.errorLines);
})

// 更新错误行高亮
function updateErrorLines(errorLines) {
  if (!view) return;
  
  // 清除所有错误行高亮
  view.dispatch({
    effects: clearErrorLines.of(null)
  });
  
  // 添加新的错误行高亮
  if (errorLines && errorLines.length > 0) {
    const effects = errorLines.map(error => {
      // 确保行号是有效的
      const lineNum = typeof error === 'object' ? error.line : parseInt(error);
      if (isNaN(lineNum) || lineNum <= 0 || lineNum > view.state.doc.lines) {
        return null;
      }
      return addErrorLine.of(lineNum);
    }).filter(Boolean);
    
    if (effects.length > 0) {
      view.dispatch({ effects });
    }
  }
}

watch(() => props.value, (val) => {
  if (view && val !== view.state.doc.toString()) {
    view.dispatch({
      changes: { from: 0, to: view.state.doc.length, insert: val || '' }
    })
  }
})

// 监听错误行变化
watch(() => props.errorLines, (val) => {
  updateErrorLines(val);
})

onBeforeUnmount(() => {
  if (view) view.destroy()
})
</script>

<style>
.duckdb-cm-simple {
  height: 100%;
  width: 100%;
  position: relative;
}
.duckdb-cm-simple-container {
  height: 100%;
  width: 100%;
  overflow: auto;
}
</style>