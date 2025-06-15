<template>
  <div class="monaco-editor-wrapper">
    <div ref="container" class="monaco-editor-container"></div>
  </div>
</template>

<script setup>
import { ref, watch, onMounted, onBeforeUnmount, getCurrentInstance, computed } from 'vue';
import { useStore } from 'vuex';
import * as monaco from 'monaco-editor';
import { onBeforeUpdate } from 'vue';

const props = defineProps({
  value: String,
  language: { type: String, default: 'yaml' },
  readOnly: { type: Boolean, default: true },
  errorLines: { type: Array, default: () => [] },
  originalValue: { type: String, default: '' }, // 用于diff模式
  diffMode: { type: Boolean, default: false }, // 是否开启diff模式
});

const container = ref(null);
let editor = null;
let diffEditor = null;
const { emit } = getCurrentInstance();
const store = useStore();



// 获取store中的数据
const availablePlugins = computed(() => store.getters.getAvailablePlugins);
const nodeTypes = computed(() => store.getters.getNodeTypes);
const logicTypes = computed(() => store.getters.getLogicTypes);
const countTypes = computed(() => store.getters.getCountTypes);
const rootTypes = computed(() => store.getters.getRootTypes);
const commonFields = computed(() => store.getters.getCommonFields);

// 获取组件列表
const inputComponents = computed(() => store.getters.getComponents('inputs'));
const outputComponents = computed(() => store.getters.getComponents('outputs'));
const rulesetComponents = computed(() => store.getters.getComponents('rulesets'));
const pluginComponents = computed(() => store.getters.getComponents('plugins'));

// 在组件挂载时获取插件列表和组件列表
onMounted(() => {
  store.dispatch('fetchAvailablePlugins');
  store.dispatch('fetchComponents', 'inputs');
  store.dispatch('fetchComponents', 'outputs');
  store.dispatch('fetchComponents', 'rulesets');
  store.dispatch('fetchComponents', 'plugins');
  
  // 设置Monaco主题
  setupMonacoTheme();
  
  // 注册语言提示
  registerLanguageProviders();
  
  // 初始化编辑器
  initializeEditor();
  
  // 添加窗口大小变化监听，确保编辑器布局正确
  window.addEventListener('resize', handleResize);
  
  // 初始布局调整
  setTimeout(() => {
    handleResize();
  }, 200);
});

// 设置Monaco主题
function setupMonacoTheme() {
  monaco.editor.defineTheme('agentsmith-theme', {
    base: 'vs',
    inherit: true,
    rules: [
      { token: 'tag', foreground: '3366ae' },
      { token: 'attribute.name', foreground: '367719' },
      { token: 'attribute.value', foreground: 'a63437' },
      { token: 'string', foreground: 'a63437' },
      { token: 'number', foreground: '17572d' },
      { token: 'keyword', foreground: '17572d' },
      { token: 'property', foreground: '008073' },
      { token: 'comment', foreground: '95a5a6', fontStyle: 'italic' },
    ],
    colors: {
      'editor.foreground': '#34495e',
      'editor.background': '#ffffff',
      'editor.lineHighlightBackground': '#f5f7fa',
      'editorLineNumber.foreground': '#adb5bd',
      'editor.selectionBackground': '#e5f3ff',
      'editorCursor.foreground': '#34495e',
      'editorError.foreground': '#e74c3c',
      'editorWarning.foreground': '#f39c12',
    }
  });
  
  monaco.editor.setTheme('agentsmith-theme');
}

// 注册语言提示
function registerLanguageProviders() {
  // YAML 语言提示
  monaco.languages.registerCompletionItemProvider('yaml', {
    provideCompletionItems: function(model, position) {
      const textUntilPosition = model.getValueInRange({
        startLineNumber: position.lineNumber,
        startColumn: 1,
        endLineNumber: position.lineNumber,
        endColumn: position.column
      });
      
      const word = model.getWordUntilPosition(position);
      const range = {
        startLineNumber: position.lineNumber,
        endLineNumber: position.lineNumber,
        startColumn: word.startColumn,
        endColumn: word.endColumn
      };
      
      const suggestions = [];
      
      // 项目流程定义提示
      if (model.getValue().includes('flow:')) {
        // 如果在输入组件引用（from或to后面）
        if (textUntilPosition.includes('from:') || textUntilPosition.includes('to:')) {
          suggestions.push(
            {
              label: 'input.',
              kind: monaco.languages.CompletionItemKind.Module,
              documentation: 'Input component',
              insertText: 'input.',
              range: range
            },
            {
              label: 'ruleset.',
              kind: monaco.languages.CompletionItemKind.Module,
              documentation: 'Ruleset component',
              insertText: 'ruleset.',
              range: range
            },
            {
              label: 'output.',
              kind: monaco.languages.CompletionItemKind.Module,
              documentation: 'Output component',
              insertText: 'output.',
              range: range
            },
            {
              label: 'plugin.',
              kind: monaco.languages.CompletionItemKind.Module,
              documentation: 'Plugin component',
              insertText: 'plugin.',
              range: range
            }
          );
          
          // 如果已经输入了组件类型前缀，提供该类型的具体组件
          if (textUntilPosition.includes('input.')) {
            inputComponents.value.forEach(comp => {
              suggestions.push({
                label: `input.${comp.id}`,
                kind: monaco.languages.CompletionItemKind.Value,
                documentation: `Input: ${comp.id}`,
                insertText: `input.${comp.id}`,
                range: range
              });
            });
          } else if (textUntilPosition.includes('ruleset.')) {
            rulesetComponents.value.forEach(comp => {
              suggestions.push({
                label: `ruleset.${comp.id}`,
                kind: monaco.languages.CompletionItemKind.Value,
                documentation: `Ruleset: ${comp.id}`,
                insertText: `ruleset.${comp.id}`,
                range: range
              });
            });
          } else if (textUntilPosition.includes('output.')) {
            outputComponents.value.forEach(comp => {
              suggestions.push({
                label: `output.${comp.id}`,
                kind: monaco.languages.CompletionItemKind.Value,
                documentation: `Output: ${comp.id}`,
                insertText: `output.${comp.id}`,
                range: range
              });
            });
          } else if (textUntilPosition.includes('plugin.')) {
            pluginComponents.value.forEach(comp => {
              suggestions.push({
                label: `plugin.${comp.id}`,
                kind: monaco.languages.CompletionItemKind.Value,
                documentation: `Plugin: ${comp.id}`,
                insertText: `plugin.${comp.id}`,
                range: range
              });
            });
          }
        }
      }
      
      return {
        suggestions: suggestions
      };
    }
  });
  
  // XML 语言提示
  monaco.languages.registerCompletionItemProvider('xml', {
    provideCompletionItems: function(model, position) {
      const textUntilPosition = model.getValueInRange({
        startLineNumber: position.lineNumber,
        startColumn: 1,
        endLineNumber: position.lineNumber,
        endColumn: position.column
      });
      
      const word = model.getWordUntilPosition(position);
      const range = {
        startLineNumber: position.lineNumber,
        endLineNumber: position.lineNumber,
        startColumn: word.startColumn,
        endColumn: word.endColumn
      };
      
      const suggestions = [];
      
      // 根标签提示
      if (textUntilPosition.trim() === '' || textUntilPosition.trim() === '<?xml version="1.0" encoding="UTF-8"?>') {
        suggestions.push({
          label: '<root>',
          kind: monaco.languages.CompletionItemKind.Snippet,
          documentation: 'Root element',
          insertText: '<root type="DETECTION">\n  $0\n</root>',
          insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
          range: range
        });
      }
      
      // root标签属性提示
      else if (textUntilPosition.includes('<root')) {
        rootTypes.value.forEach(type => {
          suggestions.push({
            label: `type="${type.value}"`,
            kind: monaco.languages.CompletionItemKind.Property,
            documentation: type.detail,
            insertText: `type="${type.value}"`,
            range: range
          });
        });
      }
      
      // rule标签及属性提示
      else if (textUntilPosition.trim() === '' || (textUntilPosition.includes('<root') && !textUntilPosition.includes('<rule'))) {
        suggestions.push({
          label: '<rule>',
          kind: monaco.languages.CompletionItemKind.Snippet,
          documentation: 'Rule element',
          insertText: '<rule id="$1" name="$2" author="$3">\n  $0\n</rule>',
          insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
          range: range
        });
      }
      
      // 更多XML提示...
      
      return {
        suggestions: suggestions
      };
    }
  });
  
  // Go 语言提示
  monaco.languages.registerCompletionItemProvider('go', {
    provideCompletionItems: function(model, position) {
      const textUntilPosition = model.getValueInRange({
        startLineNumber: position.lineNumber,
        startColumn: 1,
        endLineNumber: position.lineNumber,
        endColumn: position.column
      });
      
      const word = model.getWordUntilPosition(position);
      const range = {
        startLineNumber: position.lineNumber,
        endLineNumber: position.lineNumber,
        startColumn: word.startColumn,
        endColumn: word.endColumn
      };
      
      const suggestions = [];
      
      // 插件函数提示
      if (textUntilPosition.includes('func') && !textUntilPosition.includes('(')) {
        if (availablePlugins.value && availablePlugins.value.length > 0) {
          availablePlugins.value.forEach(plugin => {
            suggestions.push({
              label: plugin.name,
              kind: monaco.languages.CompletionItemKind.Function,
              documentation: plugin.description || 'Plugin function',
              insertText: `${plugin.name}(_$ORIDATA)`,
              range: range
            });
          });
        } else {
          suggestions.push({
            label: 'plugin_name',
            kind: monaco.languages.CompletionItemKind.Function,
            documentation: 'Example plugin function',
            insertText: 'plugin_name(_$ORIDATA)',
            range: range
          });
        }
      }
      
      return {
        suggestions: suggestions
      };
    }
  });
}

// 初始化编辑器
function initializeEditor() {
  if (!container.value) return;
  
  // Check container dimensions
  const containerRect = container.value.getBoundingClientRect();
  
  // If container has no dimensions, wait and try again
  if (containerRect.width === 0 || containerRect.height === 0) {
    setTimeout(() => initializeEditor(), 100);
    return;
  }
  
  const options = {
    value: props.value || '',
    language: getLanguage(),
    readOnly: props.readOnly,
    automaticLayout: true,
    minimap: { enabled: true },
    scrollBeyondLastLine: false,
    lineNumbers: 'on',
    renderLineHighlight: 'all',
    scrollbar: {
      verticalScrollbarSize: 10,
      horizontalScrollbarSize: 10
    },
    fontSize: 13,
    fontFamily: 'inherit',
    lineHeight: 20,
    tabSize: 2,
    wordWrap: 'on',
    contextmenu: true,
    quickSuggestions: !props.readOnly,
    snippetSuggestions: props.readOnly ? 'none' : 'inline',
    suggestOnTriggerCharacters: !props.readOnly,
    acceptSuggestionOnEnter: props.readOnly ? 'off' : 'on',
    folding: true,
    autoIndent: 'full',
    formatOnPaste: !props.readOnly,
    formatOnType: !props.readOnly,
    // Ensure consistent appearance regardless of read-only state
    renderWhitespace: 'none',
    renderControlCharacters: false,
    renderIndentGuides: true,
    cursorBlinking: props.readOnly ? 'solid' : 'blink',
    cursorStyle: 'line',
    selectOnLineNumbers: true,
    glyphMargin: true,
    lineDecorationsWidth: 10,
    lineNumbersMinChars: 3,
    overviewRulerBorder: false,
    overviewRulerLanes: 2,
    hideCursorInOverviewRuler: props.readOnly,
    // Remove all possible margins and padding
    padding: { top: 0, bottom: 0, left: 0, right: 0 },
    scrollBeyondLastColumn: 0,
    scrollBeyondLastLine: false,
    wordWrapColumn: 80,
    wrappingIndent: 'none',
  };
  
  // If diff mode, create diff editor
  if (props.diffMode && props.originalValue !== undefined) {
    diffEditor = monaco.editor.createDiffEditor(container.value, {
      ...options,
      originalEditable: false,
      renderSideBySide: true,
      ignoreTrimWhitespace: false,
      renderOverviewRuler: true,
      renderIndicators: true,
      enableSplitViewResizing: true,
      originalAriaLabel: 'Original',
      modifiedAriaLabel: 'Modified',
      diffWordWrap: 'on',
      diffAlgorithm: 'advanced',
      accessibilityVerbose: true,
      colorDecorators: true,
      scrollBeyondLastLine: false,
      // Remove margins and padding for diff editor
      padding: { top: 0, bottom: 0, left: 0, right: 0 },
      scrollBeyondLastColumn: 0,
      // Optimize diff display for new files
      renderSideBySide: props.originalValue === '' ? false : true,
      // Enable experimental features for better diff display
      experimental: {
        showMoves: true,
      },
      scrollbar: {
        useShadows: true,
        verticalHasArrows: true,
        horizontalHasArrows: true,
        vertical: 'visible',
        horizontal: 'visible',
        verticalScrollbarSize: 12,
        horizontalScrollbarSize: 12,
      }
    });
    
    // Create two models with correct language settings
    const language = getLanguage();
    const originalModel = monaco.editor.createModel(props.originalValue || '', language);
    const modifiedModel = monaco.editor.createModel(props.value || '', language);
    
    diffEditor.setModel({
      original: originalModel,
      modified: modifiedModel
    });
    
    // Get the modified editor instance
    editor = diffEditor.getModifiedEditor();
    
    // Ensure editor layout is correct
    setTimeout(() => {
      if (diffEditor) {
        diffEditor.layout();
        
        // Configure diff editor options
        const isNewFile = props.originalValue === '';
        
        diffEditor.updateOptions({
          renderSideBySide: !isNewFile, // Side-by-side for existing files, inline for new files
          renderOverviewRuler: true,
        });
        
        // Scroll to first difference if not a new file
        if (!isNewFile) {
          const nav = diffEditor.getNavigator();
          if (nav.hasNext()) {
            nav.next();
            const diff = nav.current();
            if (diff) {
              const modifiedEditor = diffEditor.getModifiedEditor();
              if (modifiedEditor) {
                modifiedEditor.revealLineInCenter(diff.modifiedLineStart);
              }
            }
          }
        }
      }
    }, 300);
  } else {
    // Create regular editor
    editor = monaco.editor.create(container.value, options);
    
    // Reset decorations array for new editor
    currentDecorations = [];
    
    // Explicitly set the value after creation
    if (props.value) {
      try {
        editor.setValue(props.value);
      } catch (error) {
        console.warn('Failed to set initial editor value:', error);
      }
    }
  }
  
  // Add save shortcut
  try {
    editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.Enter, function() {
      const content = editor.getValue();
      emit('save', content);
    });
  } catch (error) {
    console.warn('Failed to add save command:', error);
  }
  
  // Listen for content changes
  try {
    editor.onDidChangeModelContent(() => {
      const content = editor.getValue();
      emit('update:value', content);
    });
  } catch (error) {
    console.warn('Failed to add content change listener:', error);
  }
  
  // Highlight error lines
  updateErrorLines(props.errorLines);
  
  // Force layout after a short delay
  setTimeout(() => {
    try {
      if (isEditorValid(editor)) {
        editor.layout();
        const currentValue = editor.getValue();
        
        if (currentValue.length === 0 && props.value) {
          editor.setValue(props.value);
        }
        
        // Force another layout after setting value
        setTimeout(() => {
          if (isEditorValid(editor)) {
            editor.layout();
          }
        }, 50);
      }
    } catch (error) {
      console.warn('Failed to layout editor:', error);
    }
  }, 100);
}

// 获取编辑器语言
function getLanguage() {
  switch (props.language) {
    case 'xml':
      return 'xml';
    case 'yaml':
      return 'yaml';
    case 'go':
      return 'go';
    default:
      return 'json';
  }
}

// Helper function to check if editor is valid and not disposed
function isEditorValid(editorInstance) {
  if (!editorInstance) return false;
  try {
    // Try to access a basic property to check if editor is still valid
    editorInstance.getModel();
    return true;
  } catch (error) {
    return false;
  }
}

// Toggle diff view mode




// 存储当前的装饰器ID
let currentDecorations = [];

// 更新错误行高亮
function updateErrorLines(errorLines) {
  if (!isEditorValid(editor)) return;
  
  try {
    // 创建新的装饰器
    let newDecorations = [];
    
    // 如果有错误行，创建装饰器
    if (errorLines && errorLines.length > 0) {
      newDecorations = errorLines.map(error => {
        const lineNum = typeof error === 'object' ? error.line : parseInt(error);
        if (isNaN(lineNum) || lineNum <= 0) return null;
        
        return {
          range: new monaco.Range(lineNum, 1, lineNum, 1),
          options: {
            isWholeLine: true,
            linesDecorationsClassName: 'monaco-error-line-decoration',
            className: 'monaco-error-line',
            hoverMessage: {
              value: typeof error === 'object' && error.message ? error.message : 'Error in this line'
            }
          }
        };
      }).filter(Boolean);
    }
    
    // 更新装饰器：清除旧的，应用新的
    currentDecorations = editor.deltaDecorations(currentDecorations, newDecorations);
  } catch (error) {
    console.warn('Failed to update error lines:', error);
  }
}

// 监听值变化
watch(() => props.value, (newValue) => {
  if (editor && editor.getModel() && newValue !== editor.getValue()) {
    try {
      editor.setValue(newValue || '');
    } catch (error) {
      console.warn('Failed to set editor value:', error);
    }
  }
});

// 监听语言变化
watch(() => props.language, (newLanguage) => {
  if (editor && editor.getModel()) {
    try {
      const model = editor.getModel();
      if (model) {
        monaco.editor.setModelLanguage(model, getLanguage());
      }
    } catch (error) {
      console.warn('Failed to set editor language:', error);
    }
  }
});

// 监听只读状态变化
watch(() => props.readOnly, (newReadOnly) => {
  if (isEditorValid(editor)) {
    try {
      editor.updateOptions({ readOnly: newReadOnly });
    } catch (error) {
      console.warn('Failed to update editor options:', error);
    }
  }
});

// 监听错误行变化
watch(() => props.errorLines, (newErrorLines) => {
  updateErrorLines(newErrorLines);
});

// 监听diff模式变化
watch(() => [props.diffMode, props.originalValue], ([newDiffMode, newOriginalValue]) => {
  if (newDiffMode !== (diffEditor !== null)) {
    // 模式发生变化，需要重新创建编辑器
    disposeEditors();
    initializeEditor();
  } else if (isEditorValid(diffEditor) && newOriginalValue !== undefined) {
    try {
      // 只更新原始模型的内容
      const originalModel = diffEditor.getOriginalEditor().getModel();
      if (originalModel) {
        originalModel.setValue(newOriginalValue);
      }
    } catch (error) {
      console.warn('Failed to update diff editor original value:', error);
    }
  }
}, { deep: true });

// 处理窗口大小变化
function handleResize() {
  try {
    if (isEditorValid(editor)) {
      editor.layout();
    }
    if (isEditorValid(diffEditor)) {
      diffEditor.layout();
    }
  } catch (error) {
    console.warn('Failed to resize editor:', error);
  }
}

// 组件销毁前清理
onBeforeUnmount(() => {
  // 移除窗口大小变化监听
  window.removeEventListener('resize', handleResize);
  disposeEditors();
});

// 清理编辑器实例
function disposeEditors() {
  try {
    if (isEditorValid(editor)) {
      editor.dispose();
    }
  } catch (error) {
    console.warn('Failed to dispose editor:', error);
  } finally {
    editor = null;
    currentDecorations = []; // 重置装饰器数组
  }
  
  try {
    if (isEditorValid(diffEditor)) {
      diffEditor.dispose();
    }
  } catch (error) {
    console.warn('Failed to dispose diff editor:', error);
  } finally {
    diffEditor = null;
  }
}

// 暴露方法给父组件
defineExpose({
  focus: () => {
    try {
      if (isEditorValid(editor)) {
        editor.focus();
      }
    } catch (error) {
      console.warn('Failed to focus editor:', error);
    }
  },
  getValue: () => {
    try {
      return isEditorValid(editor) ? editor.getValue() : '';
    } catch (error) {
      console.warn('Failed to get editor value:', error);
      return '';
    }
  },
  setValue: (value) => {
    try {
      if (isEditorValid(editor)) {
        editor.setValue(value || '');
      }
    } catch (error) {
      console.warn('Failed to set editor value:', error);
    }
  },
  getEditor: () => editor,
  getDiffEditor: () => diffEditor
});
</script>

<style>
.monaco-editor-wrapper {
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0;
  border: none;
  overflow: hidden;
}



.monaco-editor-container {
  width: 100%;
  height: 100%;
  flex: 1;
  min-height: 300px;
  margin: 0;
  padding: 0;
  border: none;
  border-radius: 0;
  overflow: hidden;
}

/* 确保diff编辑器完全填满整个容器 */
.monaco-diff-editor {
  width: 100% !important;
  height: 100% !important;
  margin: 0 !important;
  padding: 0 !important;
  border: none !important;
}

.monaco-diff-editor .editor.original,
.monaco-diff-editor .editor.modified {
  width: 50% !important;
  margin: 0 !important;
  padding: 0 !important;
}

/* 移除所有边距和空白 */
.monaco-diff-editor .decorationsOverviewRuler {
  display: none !important;
}

.monaco-diff-editor .diffOverview {
  width: 0 !important;
  display: none !important;
}

/* 移除编辑器内部的边距 */
.monaco-editor .overflow-guard,
.monaco-diff-editor .overflow-guard {
  margin: 0 !important;
  padding: 0 !important;
  border: none !important;
}

/* 确保编辑器内容区域填满 */
.monaco-editor .monaco-scrollable-element,
.monaco-diff-editor .monaco-scrollable-element {
  margin: 0 !important;
  padding: 0 !important;
}

/* 移除编辑器周围的空白 */
.monaco-editor,
.monaco-diff-editor {
  border-radius: 0 !important;
  box-shadow: none !important;
}

/* 确保编辑器视口填满 */
.monaco-editor .view-overlays,
.monaco-diff-editor .view-overlays,
.monaco-editor .view-lines,
.monaco-diff-editor .view-lines {
  margin: 0 !important;
  padding: 0 !important;
}

/* 强制移除所有可能的边距和填充 */
.monaco-editor *,
.monaco-diff-editor * {
  box-sizing: border-box !important;
}

.monaco-editor .monaco-editor-background,
.monaco-diff-editor .monaco-editor-background {
  margin: 0 !important;
  padding: 0 !important;
}

/* 确保编辑器完全贴合容器边缘 */
.monaco-editor .lines-content,
.monaco-diff-editor .lines-content {
  margin: 0 !important;
  padding: 0 !important;
}

.monaco-editor .view-zone,
.monaco-diff-editor .view-zone {
  margin: 0 !important;
  padding: 0 !important;
}

/* 最强制性的样式 - 确保完全填满 */
.monaco-editor-wrapper,
.monaco-editor-container,
.monaco-editor,
.monaco-diff-editor {
  position: relative !important;
  top: 0 !important;
  left: 0 !important;
  right: 0 !important;
  bottom: 0 !important;
}

/* 移除任何可能的默认间距 */
.monaco-editor .monaco-editor,
.monaco-diff-editor .monaco-editor {
  margin: 0 !important;
  padding: 0 !important;
  border: none !important;
  outline: none !important;
}

/* 确保编辑器区域完全贴合 */
.monaco-editor .editor-container,
.monaco-diff-editor .editor-container {
  margin: 0 !important;
  padding: 0 !important;
  border: none !important;
}

.monaco-error-line {
  background-color: rgba(255, 0, 0, 0.1);
}

.monaco-error-line-decoration {
  background-color: #e74c3c;
  width: 4px !important;
  margin-left: 3px;
}

/* Diff编辑器样式优化 */
.monaco-diff-editor .editor-container {
  height: 100%;
}

.monaco-diff-editor .diffOverview {
  border-left: 1px solid #ddd;
}

/* 增强差异显示 */
.monaco-editor .line-insert,
.monaco-diff-editor .line-insert,
.monaco-editor-background .insertedLineBackground {
  background-color: rgba(155, 240, 155, 0.2) !important;
}

.monaco-editor .line-delete,
.monaco-diff-editor .line-delete,
.monaco-editor-background .removedLineBackground {
  background-color: rgba(255, 160, 160, 0.2) !important;
}

.monaco-editor .char-insert,
.monaco-diff-editor .char-insert,
.monaco-editor .inserted-text,
.monaco-diff-editor .inserted-text {
  background-color: rgba(155, 240, 155, 0.5) !important;
  border: none !important;
}

.monaco-editor .char-delete,
.monaco-diff-editor .char-delete,
.monaco-editor .removed-text,
.monaco-diff-editor .removed-text {
  background-color: rgba(255, 160, 160, 0.5) !important;
  border: none !important;
  text-decoration: line-through;
}

/* 修复差异编辑器分隔线 */
.monaco-diff-editor .diffViewport {
  background-color: rgba(0, 0, 255, 0.4);
}

/* 确保滚动条正确显示 */
.monaco-scrollable-element {
  visibility: visible !important;
}

/* 修复差异编辑器高度问题 */
.monaco-editor, 
.monaco-diff-editor, 
.monaco-editor .overflow-guard, 
.monaco-diff-editor .overflow-guard {
  height: 100% !important;
}

/* 确保编辑器内容可见 */
.monaco-editor .monaco-scrollable-element,
.monaco-diff-editor .monaco-scrollable-element {
  width: 100% !important;
  height: 100% !important;
}


</style> 