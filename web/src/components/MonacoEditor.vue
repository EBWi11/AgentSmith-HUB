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
  
  // 完全禁用Monaco内置的YAML语言支持
  try {
    // 取消注册所有现有的YAML completion providers
    const yamlProviders = monaco.languages.getLanguages().find(lang => lang.id === 'yaml');
    if (yamlProviders) {
      // 重新定义YAML语言，移除所有内置功能
      monaco.languages.setLanguageConfiguration('yaml', {
        wordPattern: /[\w\d_$\-\.]+/g,
        brackets: [],
        autoClosingPairs: [],
        surroundingPairs: [],
        comments: {
          lineComment: '#'
        }
      });
    }
  } catch (e) {
    console.warn('Failed to disable built-in YAML support:', e);
  }
  
  // 注册语言提示
  registerLanguageProviders();
  
  // 注册编辑器动作和快捷键
  registerEditorActions();
  
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
      { token: 'tag', foreground: '005cc5' },
      { token: 'attribute.name', foreground: '6f42c1' },
      { token: 'attribute.value', foreground: 'd73a49' },
      { token: 'string', foreground: 'd73a49' },
      { token: 'number', foreground: '005cc5' },
      { token: 'keyword', foreground: 'd73a49' },
      { token: 'property', foreground: '005cc5' },
      { token: 'comment', foreground: '6a737d', fontStyle: 'italic' },
      { token: 'variable', foreground: 'e36209' },
      { token: 'type', foreground: '6f42c1' },
    ],
    colors: {
      'editor.foreground': '#24292e',
      'editor.background': '#ffffff',
      'editor.lineHighlightBackground': '#f6f8fa',
      'editorLineNumber.foreground': '#d1d9e0',
      'editorActiveLineNumber.foreground': '#24292e',
      'editor.selectionBackground': '#c8e1ff',
      'editor.inactiveSelectionBackground': '#e5f0ff',
      'editorCursor.foreground': '#24292e',
      'editorError.foreground': '#d73a49',
      'editorWarning.foreground': '#f66a0a',
      'editorGutter.background': '#ffffff',
      'editorGutter.addedBackground': '#28a745',
      'editorGutter.deletedBackground': '#d73a49',
      'editorGutter.modifiedBackground': '#2188ff',
      'scrollbarSlider.background': '#959da533',
      'scrollbarSlider.hoverBackground': '#959da544',
      'scrollbarSlider.activeBackground': '#959da588',
    }
  });
  
  monaco.editor.setTheme('agentsmith-theme');
}

// 全局级别的provider注册标记 - 确保整个应用只注册一次
window.monacoProvidersRegistered = window.monacoProvidersRegistered || false;

// 注册语言提示
function registerLanguageProviders() {
  // 防止重复注册 - 全局级别检查
  if (window.monacoProvidersRegistered) {
    
    return;
  }
  

  


  // YAML 语言提示 - 针对Input/Output/Project组件
  monaco.languages.registerCompletionItemProvider('yaml', {
    provideCompletionItems: function(model, position) {
      try {
        const currentLine = model.getLineContent(position.lineNumber);
        const textUntilPosition = model.getValueInRange({
          startLineNumber: 1,
          startColumn: 1,
          endLineNumber: position.lineNumber,
          endColumn: position.column
        });
        
        const lineUntilPosition = currentLine.substring(0, position.column - 1);
        
        const word = model.getWordUntilPosition(position);
        const range = {
          startLineNumber: position.lineNumber,
          endLineNumber: position.lineNumber,
          startColumn: word.startColumn,
          endColumn: word.endColumn
        };
        
        let result;
        
        // 根据上下文检测组件类型
        const componentType = detectYamlComponentType(textUntilPosition, currentLine);
        
        if (componentType === 'input') {
          result = getInputCompletions(textUntilPosition, lineUntilPosition, range, position);
        } else if (componentType === 'output') {
          result = getOutputCompletions(textUntilPosition, lineUntilPosition, range, position);
        } else if (componentType === 'project') {
          // 检测是否是项目流程定义（在content内容区域）
          if (textUntilPosition.includes('content:') || lineUntilPosition.includes('->') || 
              lineUntilPosition.includes('INPUT.') || lineUntilPosition.includes('OUTPUT.') || 
              lineUntilPosition.includes('RULESET.')) {
            result = getProjectFlowCompletions(textUntilPosition, lineUntilPosition, range, position);
          } else {
            result = getProjectCompletions(textUntilPosition, lineUntilPosition, range, position);
          }
        } else {
          // 默认基础YAML补全
          result = getBaseYamlCompletions(textUntilPosition, lineUntilPosition, range, position);
        }
        
        // 简单去重
        if (result && result.suggestions && Array.isArray(result.suggestions)) {
          const uniqueSuggestions = [];
          const seenLabels = new Set();
          
          result.suggestions.forEach((suggestion, index) => {
            if (suggestion && suggestion.label) {
              const label = suggestion.label.toString().trim();
              
              if (!seenLabels.has(label)) {
                seenLabels.add(label);
                uniqueSuggestions.push({
                  label: label,
                  kind: suggestion.kind || monaco.languages.CompletionItemKind.Text,
                  insertText: suggestion.insertText || label,
                  range: suggestion.range || range,
                  documentation: suggestion.documentation || '',
                  sortText: `yaml_${String(index).padStart(3, '0')}_${label}`,
                  detail: `YAML: ${label}`
                });
              }
            }
          });
          
          return {
            suggestions: uniqueSuggestions,
            incomplete: false
          };
        }
        
        return { suggestions: [], incomplete: false };
      } catch (error) {
        console.error('YAML completion error:', error);
        return { suggestions: [], incomplete: false };
      }
    },
    
    triggerCharacters: [' ', ':', '\n', '\t', '-', '|', '.']
  });
  


  // XML 语言提示 - 针对Ruleset组件
  monaco.languages.registerCompletionItemProvider('xml', {
    provideCompletionItems: function(model, position) {
      try {
        const currentLine = model.getLineContent(position.lineNumber);
        const textUntilPosition = model.getValueInRange({
          startLineNumber: 1,
          startColumn: 1,
          endLineNumber: position.lineNumber,
          endColumn: position.column
        });
        
        const lineUntilPosition = currentLine.substring(0, position.column - 1);
        
        const word = model.getWordUntilPosition(position);
        const range = {
          startLineNumber: position.lineNumber,
          endLineNumber: position.lineNumber,
          startColumn: word.startColumn,
          endColumn: word.endColumn
        };
        
        const result = getRulesetXmlCompletions(textUntilPosition, lineUntilPosition, range, position);
        
        // 简单去重
        if (result && result.suggestions && Array.isArray(result.suggestions)) {
          const uniqueSuggestions = [];
          const seenLabels = new Set();
          
          result.suggestions.forEach((suggestion, index) => {
            if (suggestion && suggestion.label) {
              const label = suggestion.label.toString();
              
              if (!seenLabels.has(label)) {
                seenLabels.add(label);
                uniqueSuggestions.push({
                  label: label,
                  kind: suggestion.kind || monaco.languages.CompletionItemKind.Text,
                  insertText: suggestion.insertText || label,
                  range: suggestion.range || range,
                  documentation: suggestion.documentation || '',
                  sortText: `xml_${String(index).padStart(3, '0')}_${label}`,
                  detail: `Ruleset XML: ${label}`
                });
              }
            }
          });
          
          return {
            suggestions: uniqueSuggestions,
            incomplete: false
          };
        }
        
        return { suggestions: [], incomplete: false };
      } catch (error) {
        console.error('XML completion error:', error);
        return { suggestions: [], incomplete: false };
      }
    },
    
    triggerCharacters: ['<', ' ', '=', '"', '\n', '\t']
  });
  


  // Go 语言提示 - 针对Plugin组件
  monaco.languages.registerCompletionItemProvider('go', {
    provideCompletionItems: function(model, position) {
      try {
        const currentLine = model.getLineContent(position.lineNumber);
        const textUntilPosition = model.getValueInRange({
          startLineNumber: 1,
          startColumn: 1,
          endLineNumber: position.lineNumber,
          endColumn: position.column
        });
        
        const lineUntilPosition = currentLine.substring(0, position.column - 1);
        
        const word = model.getWordUntilPosition(position);
        const range = {
          startLineNumber: position.lineNumber,
          endLineNumber: position.lineNumber,
          startColumn: word.startColumn,
          endColumn: word.endColumn
        };
        
        const result = getPluginGoCompletions(textUntilPosition, lineUntilPosition, range, position);
        
        // 简单去重
        if (result && result.suggestions && Array.isArray(result.suggestions)) {
          const uniqueSuggestions = [];
          const seenLabels = new Set();
          
          result.suggestions.forEach((suggestion, index) => {
            if (suggestion && suggestion.label) {
              const label = suggestion.label.toString();
              
              if (!seenLabels.has(label)) {
                seenLabels.add(label);
                uniqueSuggestions.push({
                  label: label,
                  kind: suggestion.kind || monaco.languages.CompletionItemKind.Text,
                  insertText: suggestion.insertText || label,
                  range: suggestion.range || range,
                  documentation: suggestion.documentation || '',
                  sortText: `go_${String(index).padStart(3, '0')}_${label}`,
                  detail: `Go Plugin: ${label}`
                });
              }
            }
          });
          
          return {
            suggestions: uniqueSuggestions,
            incomplete: false
          };
        }
        
        return { suggestions: [], incomplete: false };
      } catch (error) {
        console.error('Go completion error:', error);
        return { suggestions: [], incomplete: false };
      }
    },
    
    triggerCharacters: ['.', '(', ' ', '\n', '\t']
  });
  
  // 标记providers已注册 - 全局级别
  window.monacoProvidersRegistered = true;
  

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
    fontSize: 14,
    fontFamily: '"JetBrains Mono", "Fira Code", "Cascadia Code", "SF Mono", Monaco, Menlo, "Ubuntu Mono", Consolas, "Liberation Mono", "DejaVu Sans Mono", "Courier New", monospace',
    lineHeight: 21,
    tabSize: 2,
    wordWrap: 'on',
    contextmenu: true,
    // 根据语言类型和只读状态配置补全
    quickSuggestions: props.readOnly ? false : true,
    snippetSuggestions: props.readOnly ? 'none' : 'inline',
    suggestOnTriggerCharacters: !props.readOnly,
    acceptSuggestionOnEnter: props.readOnly ? 'off' : 'on',
    tabCompletion: props.readOnly ? 'off' : 'on',
    suggestSelection: 'first',
    acceptSuggestionOnCommitCharacter: !props.readOnly,
    quickSuggestionsDelay: 100,
    // 禁用内置的单词补全，保留自定义补全
    wordBasedSuggestions: 'off',
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

// 注册编辑器动作和快捷键
function registerEditorActions() {
  // 注册智能代码格式化动作
  monaco.editor.addEditorAction({
    id: 'smart-format',
    label: 'Smart Format Document',
    keybindings: [
      monaco.KeyMod.CtrlCmd | monaco.KeyMod.Shift | monaco.KeyCode.KeyF
    ],
    contextMenuGroupId: 'navigation',
    contextMenuOrder: 1.5,
    run: function(editor) {
      // 根据语言类型进行智能格式化
      const model = editor.getModel();
      if (!model) return;
      
      const language = model.getLanguageId();
      const fullText = model.getValue();
      
      if (language === 'yaml') {
        // YAML格式化逻辑
        formatYamlDocument(editor, fullText);
      } else if (language === 'xml') {
        // XML格式化逻辑
        formatXmlDocument(editor, fullText);
      } else if (language === 'go') {
        // Go代码格式化逻辑
        formatGoDocument(editor, fullText);
      }
    }
  });
  
  // 注册快速插入模板动作
  monaco.editor.addEditorAction({
    id: 'insert-template',
    label: 'Insert Component Template',
    keybindings: [
      monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyT
    ],
    contextMenuGroupId: 'navigation',
    contextMenuOrder: 2.5,
    run: function(editor) {
      const model = editor.getModel();
      if (!model) return;
      
      const language = model.getLanguageId();
      insertComponentTemplate(editor, language);
    }
  });
  
  // 注册智能注释切换
  monaco.editor.addEditorAction({
    id: 'toggle-smart-comment',
    label: 'Toggle Smart Comment',
    keybindings: [
      monaco.KeyMod.CtrlCmd | monaco.KeyCode.Slash
    ],
    contextMenuGroupId: 'navigation',
    contextMenuOrder: 3.5,
    run: function(editor) {
      const model = editor.getModel();
      if (!model) return;
      
      const language = model.getLanguageId();
      toggleSmartComment(editor, language);
    }
  });
  
  // 注册快速补全建议动作
  monaco.editor.addEditorAction({
    id: 'trigger-suggest',
    label: 'Trigger Suggest',
    keybindings: [
      monaco.KeyMod.CtrlCmd | monaco.KeyCode.Space
    ],
    run: function(editor) {
      editor.trigger('keyboard', 'editor.action.triggerSuggest', {});
    }
  });
}

// 格式化YAML文档
function formatYamlDocument(editor, content) {
  try {
    // 基本的YAML格式化逻辑
    const lines = content.split('\n');
    const formattedLines = lines.map(line => {
      // 移除尾随空格
      line = line.trimEnd();
      
      // 规范化缩进（2个空格）
      const match = line.match(/^(\s*)(.*)/);
      if (match) {
        const indent = match[1];
        const content = match[2];
        const indentLevel = Math.floor(indent.length / 2);
        return '  '.repeat(indentLevel) + content;
      }
      
      return line;
    });
    
    const formattedContent = formattedLines.join('\n');
    const model = editor.getModel();
    const fullRange = model.getFullModelRange();
    
    editor.executeEdits('format-yaml', [{
      range: fullRange,
      text: formattedContent
    }]);
  } catch (error) {
    console.warn('YAML formatting error:', error);
  }
}

// 格式化XML文档
function formatXmlDocument(editor, content) {
  try {
    // 基本的XML格式化逻辑
    let formatted = content
      .replace(/></g, '>\n<')
      .replace(/^\s*\n/gm, '')
      .trim();
    
    const lines = formatted.split('\n');
    let indentLevel = 0;
    const formattedLines = lines.map(line => {
      const trimmed = line.trim();
      
      if (trimmed.startsWith('</')) {
        indentLevel = Math.max(0, indentLevel - 1);
      }
      
      const indentedLine = '    '.repeat(indentLevel) + trimmed;
      
      if (trimmed.startsWith('<') && !trimmed.startsWith('</') && !trimmed.endsWith('/>')) {
        indentLevel++;
      }
      
      return indentedLine;
    });
    
    const formattedContent = formattedLines.join('\n');
    const model = editor.getModel();
    const fullRange = model.getFullModelRange();
    
    editor.executeEdits('format-xml', [{
      range: fullRange,
      text: formattedContent
    }]);
  } catch (error) {
    console.warn('XML formatting error:', error);
  }
}

// 格式化Go文档
function formatGoDocument(editor, content) {
  try {
    // 基本的Go代码格式化逻辑
    const lines = content.split('\n');
    let indentLevel = 0;
    let inString = false;
    
    const formattedLines = lines.map(line => {
      const trimmed = line.trim();
      
      if (trimmed.includes('{') && !inString) {
        const indentedLine = '\t'.repeat(indentLevel) + trimmed;
        indentLevel++;
        return indentedLine;
      } else if (trimmed.includes('}') && !inString) {
        indentLevel = Math.max(0, indentLevel - 1);
        return '\t'.repeat(indentLevel) + trimmed;
      } else {
        return '\t'.repeat(indentLevel) + trimmed;
      }
    });
    
    const formattedContent = formattedLines.join('\n');
    const model = editor.getModel();
    const fullRange = model.getFullModelRange();
    
    editor.executeEdits('format-go', [{
      range: fullRange,
      text: formattedContent
    }]);
  } catch (error) {
    console.warn('Go formatting error:', error);
  }
}

// 插入组件模板
function insertComponentTemplate(editor, language) {
  const position = editor.getPosition();
  if (!position) return;
  
  let template = '';
  
  switch (language) {
    case 'yaml':
      template = 'type: ${1|kafka,aliyun_sls,elasticsearch,print|}\n${2}';
      break;
    case 'xml':
      template = '<root name="${1:ruleset-name}" type="${2|DETECTION,CLASSIFICATION|}">\n    <rule id="${3:rule-id}" name="${4:rule-name}" author="${5:author}">\n        ${6}\n    </rule>\n</root>';
      break;
    case 'go':
      template = 'func Eval(${1|data string,oriData map[string]interface{}|}) (${2|bool,map[string]interface{}|}, error) {\n    ${3:// Your plugin logic here}\n    \n    return ${4|true,oriData|}, nil\n}';
      break;
    default:
      return;
  }
  
  editor.trigger('keyboard', 'type', {
    text: template
  });
  
  // 触发snippet插入
  setTimeout(() => {
    editor.trigger('keyboard', 'editor.action.insertSnippet', {
      snippet: template
    });
  }, 100);
}

// 智能注释切换
function toggleSmartComment(editor, language) {
  const selection = editor.getSelection();
  if (!selection) return;
  
  const model = editor.getModel();
  if (!model) return;
  
  let commentPrefix = '';
  switch (language) {
    case 'yaml':
      commentPrefix = '# ';
      break;
    case 'xml':
      // XML注释比较复杂，这里简化处理
      editor.trigger('keyboard', 'editor.action.blockComment', {});
      return;
    case 'go':
      commentPrefix = '// ';
      break;
    default:
      return;
  }
  
  const startLine = selection.startLineNumber;
  const endLine = selection.endLineNumber;
  
  const edits = [];
  let isCommenting = false;
  
  // 检查是否需要添加注释或移除注释
  for (let i = startLine; i <= endLine; i++) {
    const line = model.getLineContent(i);
    const trimmed = line.trim();
    if (trimmed && !trimmed.startsWith(commentPrefix.trim())) {
      isCommenting = true;
      break;
    }
  }
  
  // 执行注释或取消注释
  for (let i = startLine; i <= endLine; i++) {
    const line = model.getLineContent(i);
    const trimmed = line.trim();
    
    if (trimmed) {
      if (isCommenting) {
        // 添加注释
        const firstNonWhitespace = line.search(/\S/);
        if (firstNonWhitespace >= 0) {
          edits.push({
            range: {
              startLineNumber: i,
              startColumn: firstNonWhitespace + 1,
              endLineNumber: i,
              endColumn: firstNonWhitespace + 1
            },
            text: commentPrefix
          });
        }
      } else {
        // 移除注释
        const commentIndex = line.indexOf(commentPrefix);
        if (commentIndex >= 0) {
          edits.push({
            range: {
              startLineNumber: i,
              startColumn: commentIndex + 1,
              endLineNumber: i,
              endColumn: commentIndex + 1 + commentPrefix.length
            },
            text: ''
          });
        }
      }
    }
  }
  
  if (edits.length > 0) {
    editor.executeEdits('toggle-comment', edits);
  }
}

// 检测YAML组件类型
function detectYamlComponentType(fullText, currentLine) {
  // 优先检测project类型（检查更多的project特征）
  if (fullText.includes('content:') || 
      fullText.includes('->') || 
      fullText.includes('INPUT.') || 
      fullText.includes('OUTPUT.') || 
      fullText.includes('RULESET.') ||
      currentLine.includes('INPUT.') || 
      currentLine.includes('OUTPUT.') || 
      currentLine.includes('RULESET.')) {
    return 'project';
  }
  
  const typeMatch = fullText.match(/type:\s*(kafka|aliyun_sls|elasticsearch|print)/);
  if (typeMatch) {
    const type = typeMatch[1];
    if (['kafka', 'aliyun_sls'].includes(type)) {
      return 'input';
    } else if (['kafka', 'elasticsearch', 'print'].includes(type)) {
      return 'output';
    }
  }
  
  // 检查是否有input/output特有的配置段
  if (fullText.includes('consumer_group') || fullText.includes('cursor_position')) {
    return 'input';
  }
  if (fullText.includes('index:') || fullText.includes('batch_size:')) {
    return 'output';
  }
  
  return 'unknown';
}

// Input组件智能补全
function getInputCompletions(fullText, lineText, range, position) {
  const suggestions = [];
  
  // 特殊处理：检查是否是INPUT.后的补全
  const currentWord = getCurrentWord(lineText, position.column);
  if (currentWord.includes('.')) {
    const [prefix, partial] = currentWord.split('.');
    const partialLower = (partial || '').toLowerCase();
    
    if (prefix === 'INPUT') {
      
      if (inputComponents.value.length > 0) {
        // 提示所有INPUT组件
        inputComponents.value.forEach(input => {
          if ((!partial || input.id.toLowerCase().includes(partialLower)) && 
              !suggestions.some(s => s.label === input.id)) {
            suggestions.push({
              label: input.id,
              kind: monaco.languages.CompletionItemKind.Reference,
              documentation: `Input component: ${input.id}`,
              insertText: input.id,
              range: range
            });
          }
        });
      } else {
        // 如果没有input组件，添加一个提示
        suggestions.push({
          label: 'No input components available',
          kind: monaco.languages.CompletionItemKind.Text,
          documentation: 'No input components found. Please create input components first.',
          insertText: '',
          range: range
        });
      }
      
      return { suggestions };
    }
  }
  
  // 解析当前YAML上下文
  const context = parseYamlContext(fullText, lineText, position);
  
  // 根据不同的上下文提供精确的补全
  let result;
  if (context.isInValue) {
    result = getInputValueCompletions(context, range, fullText);
  } else if (context.isInKey) {
    result = getInputKeyCompletions(context, range, fullText);
  } else {
    // 默认情况 - 根据当前层级和已有配置提供建议
    result = getDefaultInputCompletions(fullText, context, range);
  }
  
  return result;
}

// 解析YAML上下文
function parseYamlContext(fullText, lineText, position) {
  const lines = fullText.split('\n');
  const currentLineIndex = position.lineNumber - 1;
  const beforeCursor = lineText.substring(0, position.column - 1);
  const afterCursor = lineText.substring(position.column - 1);
  
  const context = {
    currentLine: lineText,
    beforeCursor,
    afterCursor,
    indentLevel: getIndentLevel(lineText),
    isInKey: false,
    isInValue: false,
    currentKey: '',
    currentSection: '',
    parentSections: [],
    lineIndex: currentLineIndex
  };
  
  // 检测是否在值位置（冒号后面）
  const colonIndex = beforeCursor.lastIndexOf(':');
  if (colonIndex !== -1) {
    const afterColon = beforeCursor.substring(colonIndex + 1).trim();
    if (afterColon === '' || afterColon.startsWith(' ')) {
      context.isInValue = true;
      // 提取键名
      const beforeColon = beforeCursor.substring(0, colonIndex).trim();
      context.currentKey = beforeColon.split(/\s+/).pop() || '';
    }
  } else {
    // 在键位置
    context.isInKey = true;
  }
  
  // 解析当前所在的配置段
  context.parentSections = getYamlSections(lines, currentLineIndex);
  if (context.parentSections.length > 0) {
    context.currentSection = context.parentSections[context.parentSections.length - 1];
  }
  
  return context;
}

// 获取YAML配置段层级
function getYamlSections(lines, currentLineIndex) {
  const sections = [];
  const currentIndent = getIndentLevel(lines[currentLineIndex] || '');
  
  // 向上查找父级配置段
  for (let i = currentLineIndex - 1; i >= 0; i--) {
    const line = lines[i];
    if (line.trim() === '') continue;
    
    const lineIndent = getIndentLevel(line);
    if (lineIndent < currentIndent) {
      const match = line.match(/^\s*([^:]+):/);
      if (match) {
        sections.unshift(match[1].trim());
        if (lineIndent === 0) break;
      }
    }
  }
  
  return sections;
}

// Input值补全
function getInputValueCompletions(context, range, fullText) {
  const suggestions = [];
  
  // type属性值补全
  if (context.currentKey === 'type') {
    const inputTypes = [
      { value: 'kafka', description: 'Apache Kafka input source' },
      { value: 'aliyun_sls', description: 'Alibaba Cloud SLS input source' }
    ];
    
    inputTypes.forEach(type => {
      if (!suggestions.some(s => s.label === type.value)) {
        suggestions.push({
          label: type.value,
          kind: monaco.languages.CompletionItemKind.EnumMember,
          documentation: type.description,
          insertText: type.value,
          range: range
        });
      }
    });
  }
  
  // compression属性值补全
  else if (context.currentKey === 'compression') {
    const compressionTypes = ['none', 'gzip', 'snappy', 'lz4', 'zstd'];
    compressionTypes.forEach(comp => {
      if (!suggestions.some(s => s.label === comp)) {
        suggestions.push({
          label: comp,
          kind: monaco.languages.CompletionItemKind.EnumMember,
          documentation: `${comp} compression`,
          insertText: comp,
          range: range
        });
      }
    });
  }
  
  // enable属性值补全
  else if (context.currentKey === 'enable') {
    suggestions.push(
      { label: 'true', kind: monaco.languages.CompletionItemKind.EnumMember, documentation: 'Enable feature', insertText: 'true', range: range },
      { label: 'false', kind: monaco.languages.CompletionItemKind.EnumMember, documentation: 'Disable feature', insertText: 'false', range: range }
    );
  }
  
  // mechanism属性值补全
  else if (context.currentKey === 'mechanism') {
    const mechanisms = ['plain', 'scram-sha-256', 'scram-sha-512'];
    mechanisms.forEach(mech => {
      if (!suggestions.some(s => s.label === mech)) {
        suggestions.push({
          label: mech,
          kind: monaco.languages.CompletionItemKind.EnumMember,
          documentation: `SASL ${mech} mechanism`,
          insertText: mech,
          range: range
        });
      }
    });
  }
  
  // cursor_position属性值补全
  else if (context.currentKey === 'cursor_position') {
    suggestions.push(
      { label: 'BEGIN_CURSOR', kind: monaco.languages.CompletionItemKind.EnumMember, documentation: 'Start from beginning', insertText: 'BEGIN_CURSOR', range: range },
      { label: 'END_CURSOR', kind: monaco.languages.CompletionItemKind.EnumMember, documentation: 'Start from end', insertText: 'END_CURSOR', range: range }
    );
  }
  
  // endpoint格式建议
  else if (context.currentKey === 'endpoint') {
    suggestions.push({
      label: 'region.log.aliyuncs.com',
      kind: monaco.languages.CompletionItemKind.Snippet,
      documentation: 'Aliyun SLS endpoint format',
      insertText: '${1:cn-beijing}.log.aliyuncs.com',
      insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
      range: range
    });
  }
  
  // 数组项建议 - 只提供格式提示
  else if (context.currentKey === 'brokers' || context.beforeCursor.includes('- ')) {
    suggestions.push({
      label: 'broker-address:port',
      kind: monaco.languages.CompletionItemKind.Snippet,
      documentation: 'Kafka broker address format',
      insertText: '${1:broker-host}:${2:9092}',
      insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
      range: range
    });
  }
  
  return { suggestions };
}

// Input键补全
function getInputKeyCompletions(context, range, fullText) {
  const suggestions = [];
  
  // 根级别配置
  if (context.indentLevel === 0) {
    if (!fullText.includes('type:')) {
      suggestions.push({
        label: 'type',
        kind: monaco.languages.CompletionItemKind.Property,
        documentation: 'Input source type',
        insertText: 'type: ${1|kafka,aliyun_sls|}',
        insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
        range: range
      });
    }
    
    // 根据type提供相应的配置段
    const typeMatch = fullText.match(/type:\s*(kafka|aliyun_sls)/);
    if (typeMatch) {
      const inputType = typeMatch[1];
      
      if (inputType === 'kafka' && !fullText.includes('kafka:')) {
        suggestions.push({
          label: 'kafka',
          kind: monaco.languages.CompletionItemKind.Module,
          documentation: 'Kafka input configuration section',
          insertText: 'kafka:\n  ${1}',
          insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
          range: range
        });
      }
      
      if (inputType === 'aliyun_sls' && !fullText.includes('aliyun_sls:')) {
        suggestions.push({
          label: 'aliyun_sls',
          kind: monaco.languages.CompletionItemKind.Module,
          documentation: 'Aliyun SLS input configuration section',
          insertText: 'aliyun_sls:\n  ${1}',
          insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
          range: range
        });
      }
    }
  }
  
  // Kafka配置段内部
  else if (context.currentSection === 'kafka') {
    const kafkaKeys = [
      { key: 'brokers', desc: 'Kafka broker addresses', template: 'brokers:\n  - "${1:broker-host}:${2:9092}"' },
      { key: 'topic', desc: 'Kafka topic name', template: 'topic: "${1:topic-name}"' },
      { key: 'group', desc: 'Consumer group name', template: 'group: "${1:consumer-group}"' },
      { key: 'compression', desc: 'Message compression type', template: 'compression: "${1|none,gzip,snappy,lz4,zstd|}"' },
      { key: 'sasl', desc: 'SASL authentication configuration', template: 'sasl:\n  enable: ${1|true,false|}\n  mechanism: "${2|plain,scram-sha-256,scram-sha-512|}"\n  username: "${3:username}"\n  password: "${4:password}"' }
    ];
    
    kafkaKeys.forEach(item => {
      if (!fullText.includes(`${item.key}:`) && !suggestions.some(s => s.label === item.key)) {
        suggestions.push({
          label: item.key,
          kind: monaco.languages.CompletionItemKind.Property,
          documentation: item.desc,
          insertText: item.template,
          insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
          range: range
        });
      }
    });
  }
  
  // SASL配置段内部
  else if (context.currentSection === 'sasl') {
    const saslKeys = [
      { key: 'enable', desc: 'Enable SASL authentication', template: 'enable: ${1|true,false|}' },
      { key: 'mechanism', desc: 'SASL mechanism', template: 'mechanism: "${1|plain,scram-sha-256,scram-sha-512|}"' },
      { key: 'username', desc: 'SASL username', template: 'username: "${1:username}"' },
      { key: 'password', desc: 'SASL password', template: 'password: "${1:password}"' }
    ];
    
    saslKeys.forEach(item => {
      if (!fullText.includes(`${item.key}:`) && !suggestions.some(s => s.label === item.key)) {
        suggestions.push({
          label: item.key,
          kind: monaco.languages.CompletionItemKind.Property,
          documentation: item.desc,
          insertText: item.template,
          insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
          range: range
        });
      }
    });
  }
  
  // Aliyun SLS配置段内部
  else if (context.currentSection === 'aliyun_sls') {
    const slsKeys = [
      { key: 'endpoint', desc: 'SLS service endpoint', template: 'endpoint: "${1:region}.log.aliyuncs.com"' },
      { key: 'access_key_id', desc: 'Access key ID', template: 'access_key_id: "${1:your-access-key-id}"' },
      { key: 'access_key_secret', desc: 'Access key secret', template: 'access_key_secret: "${1:your-access-key-secret}"' },
      { key: 'project', desc: 'SLS project name', template: 'project: "${1:your-project}"' },
      { key: 'logstore', desc: 'SLS logstore name', template: 'logstore: "${1:your-logstore}"' },
      { key: 'consumer_group_name', desc: 'Consumer group name', template: 'consumer_group_name: "${1:consumer-group}"' },
      { key: 'consumer_name', desc: 'Consumer name', template: 'consumer_name: "${1:consumer-name}"' },
      { key: 'cursor_position', desc: 'Cursor start position', template: 'cursor_position: "${1|BEGIN_CURSOR,END_CURSOR|}"' },
      { key: 'query', desc: 'Log query filter', template: 'query: "${1:*}"' }
    ];
    
    slsKeys.forEach(item => {
      if (!fullText.includes(`${item.key}:`) && !suggestions.some(s => s.label === item.key)) {
        suggestions.push({
          label: item.key,
          kind: monaco.languages.CompletionItemKind.Property,
          documentation: item.desc,
          insertText: item.template,
          insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
          range: range
        });
      }
    });
  }
  
  return { suggestions };
}

// 默认Input补全
function getDefaultInputCompletions(fullText, context, range) {
  const suggestions = [];
  
  // 完整配置模板
  if (!fullText.includes('type:')) {
    suggestions.push(
      {
        label: 'Kafka Input Template',
        kind: monaco.languages.CompletionItemKind.Snippet,
        documentation: 'Complete Kafka input configuration',
        insertText: [
          'type: kafka',
          'kafka:',
          '  brokers:',
          '    - "${1:broker-host}:${2:9092}"',
          '  topic: "${3:topic-name}"',
          '  group: "${4:consumer-group}"',
          '  compression: "${5|none,gzip,snappy,lz4,zstd|}"'
        ].join('\n'),
        insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
        range: range
      },
      {
        label: 'Aliyun SLS Input Template',
        kind: monaco.languages.CompletionItemKind.Snippet,
        documentation: 'Complete Aliyun SLS input configuration',
        insertText: [
          'type: aliyun_sls',
          'aliyun_sls:',
          '  endpoint: "${1:region}.log.aliyuncs.com"',
          '  access_key_id: "${2:your-access-key-id}"',
          '  access_key_secret: "${3:your-access-key-secret}"',
          '  project: "${4:your-project}"',
          '  logstore: "${5:your-logstore}"',
          '  consumer_group_name: "${6:consumer-group}"',
          '  consumer_name: "${7:consumer-name}"'
        ].join('\n'),
        insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
        range: range
      }
    );
  }
  
  return { suggestions };
}

// Output组件智能补全
function getOutputCompletions(fullText, lineText, range, position) {
  const suggestions = [];
  
  // 特殊处理：检查是否是OUTPUT.后的补全
  const currentWord = getCurrentWord(lineText, position.column);
  if (currentWord.includes('.')) {
    const [prefix, partial] = currentWord.split('.');
    const partialLower = (partial || '').toLowerCase();
    
    if (prefix === 'OUTPUT' && outputComponents.value.length > 0) {
      // 提示所有OUTPUT组件
      outputComponents.value.forEach(output => {
        if ((!partial || output.id.toLowerCase().includes(partialLower)) && 
            !suggestions.some(s => s.label === output.id)) {
          suggestions.push({
            label: output.id,
            kind: monaco.languages.CompletionItemKind.Reference,
            documentation: `Output component: ${output.id}`,
            insertText: output.id,
            range: range
          });
        }
      });
      
      return { suggestions };
    }
  }
  
  // 解析当前YAML上下文
  const context = parseYamlContext(fullText, lineText, position);
  
  // 根据不同的上下文提供精确的补全
  let result;
  if (context.isInValue) {
    result = getOutputValueCompletions(context, range, fullText);
  } else if (context.isInKey) {
    result = getOutputKeyCompletions(context, range, fullText);
  } else {
    // 默认情况 - 根据当前层级和已有配置提供建议
    result = getDefaultOutputCompletions(fullText, context, range);
  }
  
  return result;
}

// Output值补全
function getOutputValueCompletions(context, range, fullText) {
  const suggestions = [];
  
  // type属性值补全
  if (context.currentKey === 'type') {
    const outputTypes = [
      { value: 'kafka', description: 'Apache Kafka output destination' },
      { value: 'elasticsearch', description: 'Elasticsearch output destination' },
      { value: 'aliyun_sls', description: 'Alibaba Cloud SLS output destination' },
      { value: 'print', description: 'Console print output for debugging' }
    ];
    
    outputTypes.forEach(type => {
      if (!suggestions.some(s => s.label === type.value)) {
        suggestions.push({
          label: type.value,
          kind: monaco.languages.CompletionItemKind.EnumMember,
          documentation: type.description,
          insertText: type.value,
          range: range
        });
      }
    });
  }
  
  // compression属性值补全
  else if (context.currentKey === 'compression') {
    const compressionTypes = ['none', 'gzip', 'snappy', 'lz4', 'zstd'];
    compressionTypes.forEach(comp => {
      if (!suggestions.some(s => s.label === comp)) {
        suggestions.push({
          label: comp,
          kind: monaco.languages.CompletionItemKind.EnumMember,
          documentation: `${comp} compression`,
          insertText: comp,
          range: range
        });
      }
    });
  }
  
  // endpoint格式建议
  else if (context.currentKey === 'endpoint') {
    suggestions.push({
      label: 'region.log.aliyuncs.com',
      kind: monaco.languages.CompletionItemKind.Snippet,
      documentation: 'Aliyun SLS endpoint format',
      insertText: '${1:cn-beijing}.log.aliyuncs.com',
      insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
      range: range
    });
  }
  
  // 数组项格式建议
  else if (context.currentKey === 'brokers' || context.currentKey === 'hosts' || context.beforeCursor.includes('- ')) {
    if (context.currentSection === 'kafka' || context.beforeCursor.includes('brokers')) {
      suggestions.push({
        label: 'broker-host:port',
        kind: monaco.languages.CompletionItemKind.Snippet,
        documentation: 'Kafka broker address format',
        insertText: '${1:broker-host}:${2:9092}',
        insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
        range: range
      });
    } else if (context.currentSection === 'elasticsearch' || context.beforeCursor.includes('hosts')) {
      suggestions.push({
        label: 'http://host:port',
        kind: monaco.languages.CompletionItemKind.Snippet,
        documentation: 'Elasticsearch host URL format',
        insertText: '${1|http,https|}://${2:elasticsearch-host}:${3:9200}',
        insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
        range: range
      });
    }
  }
  
  // 时间间隔建议
  else if (context.currentKey === 'flush_dur') {
    const durations = ['1s', '5s', '10s', '30s', '1m', '5m'];
    durations.forEach(dur => {
      if (!suggestions.some(s => s.label === dur)) {
        suggestions.push({
          label: dur,
          kind: monaco.languages.CompletionItemKind.Value,
          documentation: `Flush duration: ${dur}`,
          insertText: dur,
          range: range
        });
      }
    });
  }
  
  // 数值建议
  else if (context.currentKey === 'batch_size') {
    const sizes = ['100', '500', '1000', '5000', '10000'];
    sizes.forEach(size => {
      if (!suggestions.some(s => s.label === size)) {
        suggestions.push({
          label: size,
          kind: monaco.languages.CompletionItemKind.Value,
          documentation: `Batch size: ${size} documents`,
          insertText: size,
          range: range
        });
      }
    });
  }
  
  return { suggestions };
}

// Output键补全
function getOutputKeyCompletions(context, range, fullText) {
  const suggestions = [];
  
  // 根级别配置
  if (context.indentLevel === 0) {
    if (!fullText.includes('type:')) {
      suggestions.push({
        label: 'type',
        kind: monaco.languages.CompletionItemKind.Property,
        documentation: 'Output destination type',
        insertText: 'type: ${1|kafka,elasticsearch,aliyun_sls,print|}',
        insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
        range: range
      });
    }
    
    if (!fullText.includes('name:')) {
      suggestions.push({
        label: 'name',
        kind: monaco.languages.CompletionItemKind.Property,
        documentation: 'Output component name',
        insertText: 'name: "${1:output-name}"',
        insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
        range: range
      });
    }
    
    // 根据type提供相应的配置段
    const typeMatch = fullText.match(/type:\s*(kafka|elasticsearch|aliyun_sls|print)/);
    if (typeMatch) {
      const outputType = typeMatch[1];
      
      if (outputType === 'kafka' && !fullText.includes('kafka:')) {
        suggestions.push({
          label: 'kafka',
          kind: monaco.languages.CompletionItemKind.Module,
          documentation: 'Kafka output configuration section',
          insertText: 'kafka:\n  ${1}',
          insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
          range: range
        });
      }
      
      if (outputType === 'elasticsearch' && !fullText.includes('elasticsearch:')) {
        suggestions.push({
          label: 'elasticsearch',
          kind: monaco.languages.CompletionItemKind.Module,
          documentation: 'Elasticsearch output configuration section',
          insertText: 'elasticsearch:\n  ${1}',
          insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
          range: range
        });
      }
      
      if (outputType === 'aliyun_sls' && !fullText.includes('aliyun_sls:')) {
        suggestions.push({
          label: 'aliyun_sls',
          kind: monaco.languages.CompletionItemKind.Module,
          documentation: 'Aliyun SLS output configuration section',
          insertText: 'aliyun_sls:\n  ${1}',
          insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
          range: range
        });
      }
    }
  }
  
  // Kafka配置段内部
  else if (context.currentSection === 'kafka') {
    const kafkaKeys = [
      { key: 'brokers', desc: 'Kafka broker addresses', template: 'brokers:\n  - "${1:broker-host}:${2:9092}"' },
      { key: 'topic', desc: 'Kafka topic name', template: 'topic: "${1:topic-name}"' },
      { key: 'compression', desc: 'Message compression type', template: 'compression: "${1|none,gzip,snappy,lz4,zstd|}"' }
    ];
    
    kafkaKeys.forEach(item => {
      if (!fullText.includes(`${item.key}:`) && !suggestions.some(s => s.label === item.key)) {
        suggestions.push({
          label: item.key,
          kind: monaco.languages.CompletionItemKind.Property,
          documentation: item.desc,
          insertText: item.template,
          insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
          range: range
        });
      }
    });
  }
  
  // Elasticsearch配置段内部
  else if (context.currentSection === 'elasticsearch') {
    const esKeys = [
      { key: 'hosts', desc: 'Elasticsearch cluster hosts', template: 'hosts:\n  - "${1|http,https|}://${2:elasticsearch-host}:${3:9200}"' },
      { key: 'index', desc: 'Elasticsearch index name', template: 'index: "${1:index-name}"' },
      { key: 'batch_size', desc: 'Batch size for bulk operations', template: 'batch_size: ${1:1000}' },
      { key: 'flush_dur', desc: 'Flush duration for batching', template: 'flush_dur: "${1:5s}"' }
    ];
    
    esKeys.forEach(item => {
      if (!fullText.includes(`${item.key}:`) && !suggestions.some(s => s.label === item.key)) {
        suggestions.push({
          label: item.key,
          kind: monaco.languages.CompletionItemKind.Property,
          documentation: item.desc,
          insertText: item.template,
          insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
          range: range
        });
      }
    });
  }
  
  // Aliyun SLS配置段内部
  else if (context.currentSection === 'aliyun_sls') {
    const slsKeys = [
      { key: 'endpoint', desc: 'SLS service endpoint', template: 'endpoint: "${1:region}.log.aliyuncs.com"' },
      { key: 'access_key_id', desc: 'Access key ID', template: 'access_key_id: "${1:your-access-key-id}"' },
      { key: 'access_key_secret', desc: 'Access key secret', template: 'access_key_secret: "${1:your-access-key-secret}"' },
      { key: 'project', desc: 'SLS project name', template: 'project: "${1:your-project}"' },
      { key: 'logstore', desc: 'SLS logstore name', template: 'logstore: "${1:your-logstore}"' }
    ];
    
    slsKeys.forEach(item => {
      if (!fullText.includes(`${item.key}:`) && !suggestions.some(s => s.label === item.key)) {
        suggestions.push({
          label: item.key,
          kind: monaco.languages.CompletionItemKind.Property,
          documentation: item.desc,
          insertText: item.template,
          insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
          range: range
        });
      }
    });
  }
  
  return { suggestions };
}

// 默认Output补全
function getDefaultOutputCompletions(fullText, context, range) {
  const suggestions = [];
  
  // 完整配置模板
  if (!fullText.includes('type:')) {
    suggestions.push(
      {
        label: 'Kafka Output Template',
        kind: monaco.languages.CompletionItemKind.Snippet,
        documentation: 'Complete Kafka output configuration',
        insertText: [
          'type: kafka',
          'name: "${1:kafka-output}"',
          'kafka:',
          '  brokers:',
          '    - "${2:broker-host}:${3:9092}"',
          '  topic: "${4:topic-name}"',
          '  compression: "${5|none,gzip,snappy,lz4,zstd|}"'
        ].join('\n'),
        insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
        range: range
      },
      {
        label: 'Elasticsearch Output Template',
        kind: monaco.languages.CompletionItemKind.Snippet,
        documentation: 'Complete Elasticsearch output configuration',
        insertText: [
          'type: elasticsearch',
          'name: "${1:es-output}"',
          'elasticsearch:',
          '  hosts:',
          '    - "${2|http,https|}://${3:elasticsearch-host}:${4:9200}"',
          '  index: "${5:index-name}"',
          '  batch_size: ${6:1000}',
          '  flush_dur: "${7:5s}"'
        ].join('\n'),
        insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
        range: range
      },
      {
        label: 'Aliyun SLS Output Template',
        kind: monaco.languages.CompletionItemKind.Snippet,
        documentation: 'Complete Aliyun SLS output configuration',
        insertText: [
          'type: aliyun_sls',
          'name: "${1:sls-output}"',
          'aliyun_sls:',
          '  endpoint: "${2:region}.log.aliyuncs.com"',
          '  access_key_id: "${3:your-access-key-id}"',
          '  access_key_secret: "${4:your-access-key-secret}"',
          '  project: "${5:your-project}"',
          '  logstore: "${6:your-logstore}"'
        ].join('\n'),
        insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
        range: range
      },
      {
        label: 'Print Output Template',
        kind: monaco.languages.CompletionItemKind.Snippet,
        documentation: 'Simple print output for debugging',
        insertText: [
          'type: print',
          'name: "${1:debug-output}"'
        ].join('\n'),
        insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
        range: range
      }
    );
  }
  
  return { suggestions };
}

// Project组件自动补全
function getProjectCompletions(fullText, lineText, range, position) {
  const suggestions = [];
  
  if (!fullText.includes('content:')) {
    suggestions.push({
      label: 'content',
      kind: monaco.languages.CompletionItemKind.Property,
      documentation: 'Project data flow definition',
      insertText: [
        'content: |',
        '  INPUT.${1:input-name} -> ${2|RULESET,OUTPUT|}.${3:component-name}'
      ].join('\n'),
      insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
      range: range
    });
  }
  
  const result = { suggestions };
  return result;
}



// Project流程补全
function getProjectFlowCompletions(fullText, lineText, range, position) {
  
  const suggestions = [];
  
  // 获取当前光标位置的单词
  const currentWord = getCurrentWord(lineText, position.column);
  

  
  // 检测当前输入的上下文
  if (currentWord.includes('.')) {
    // 用户已经输入了前缀，如 "INPUT.", "OUTPUT.", "RULESET."
    const [prefix, partial] = currentWord.split('.');
    const partialLower = (partial || '').toLowerCase();
    
    // 当检测到具体前缀时，只处理该前缀的组件建议，不添加其他前缀建议
    

    
                if (prefix === 'INPUT') {
        // 计算正确的range，只替换点号后面的部分
        const dotIndex = currentWord.indexOf('.');
        const replaceRange = {
          startLineNumber: position.lineNumber,
          endLineNumber: position.lineNumber,
          startColumn: position.column - (currentWord.length - dotIndex - 1),
          endColumn: position.column
        };
        
        if (inputComponents.value.length > 0) {
          // 提示所有INPUT组件
          inputComponents.value.forEach(input => {
            if ((!partial || input.id.toLowerCase().includes(partialLower)) && 
                !suggestions.some(s => s.label === input.id)) {
              suggestions.push({
                label: input.id,
                kind: monaco.languages.CompletionItemKind.Reference,
                documentation: `Input component: ${input.id}`,
                insertText: input.id,
                range: replaceRange
              });
            }
          });
        } else {
          // 如果没有input组件，添加一个提示
          suggestions.push({
            label: 'No input components available',
            kind: monaco.languages.CompletionItemKind.Text,
            documentation: 'No input components found. Please create input components first.',
            insertText: '',
            range: replaceRange
          });
        }
        
        // 处理完INPUT组件后直接返回，不继续处理其他逻辑
        return { suggestions };
        
      } else if (prefix === 'RULESET') {
        // 计算正确的range，只替换点号后面的部分
        const dotIndex = currentWord.indexOf('.');
        const replaceRange = {
          startLineNumber: position.lineNumber,
          endLineNumber: position.lineNumber,
          startColumn: position.column - (currentWord.length - dotIndex - 1),
          endColumn: position.column
        };
        
        console.log('RULESET prefix detected, rulesetComponents:', rulesetComponents.value);
        
        if (rulesetComponents.value.length > 0) {
          // 提示所有RULESET组件
          rulesetComponents.value.forEach(ruleset => {
            if ((!partial || ruleset.id.toLowerCase().includes(partialLower)) && 
                !suggestions.some(s => s.label === ruleset.id)) {
              suggestions.push({
                label: ruleset.id,
                kind: monaco.languages.CompletionItemKind.Reference,
                documentation: `Ruleset component: ${ruleset.id}`,
                insertText: ruleset.id,
                range: replaceRange
              });
            }
          });
        } else {
          // 如果没有ruleset组件，添加一个提示
          suggestions.push({
            label: 'No ruleset components available',
            kind: monaco.languages.CompletionItemKind.Text,
            documentation: 'No ruleset components found. Please create ruleset components first.',
            insertText: '',
            range: replaceRange
          });
        }
        
        // 处理完RULESET组件后直接返回
        return { suggestions };
      
    } else if (prefix === 'OUTPUT' && outputComponents.value.length > 0) {
      // 计算正确的range，只替换点号后面的部分
      const dotIndex = currentWord.indexOf('.');
      const replaceRange = {
        startLineNumber: position.lineNumber,
        endLineNumber: position.lineNumber,
        startColumn: position.column - (currentWord.length - dotIndex - 1),
        endColumn: position.column
      };
      
      // 提示所有OUTPUT组件
      outputComponents.value.forEach(output => {
        const matches = !partial || output.id.toLowerCase().includes(partialLower);
        const alreadyExists = suggestions.some(s => s.label === output.id);
        
        if (matches && !alreadyExists) {
          suggestions.push({
            label: output.id,
            kind: monaco.languages.CompletionItemKind.Reference,
            documentation: `Output component: ${output.id}`,
            insertText: output.id,
            range: replaceRange
          });
        }
      });
      
      // 处理完OUTPUT组件后直接返回
      return { suggestions };
    }
    
    // 如果没有匹配的前缀，返回空建议
    return { suggestions: [] };
    
  } else {
    // 用户还没有输入前缀，需要根据上下文提供前缀建议
    const suggestionsMap = new Map();

    // 统一管理所有前缀建议，确保不重复
    const addSuggestion = (label, kind, doc, insertText) => {
      if (!suggestionsMap.has(label)) {
        suggestionsMap.set(label, {
          label,
          kind,
          documentation: doc,
          insertText,
          range
        });
      }
    };

    // 根据上下文判断应该提供哪些建议
    const arrowIndex = lineText.lastIndexOf('->');
    const isAfterArrow = arrowIndex !== -1 && position.column > arrowIndex + 2;

    if (isAfterArrow) {
      // 箭头后：只能是 RULESET 或 OUTPUT
      addSuggestion('RULESET', monaco.languages.CompletionItemKind.Module, 'Ruleset component reference', 'RULESET');
      addSuggestion('OUTPUT', monaco.languages.CompletionItemKind.Module, 'Output component reference', 'OUTPUT');
    } else {
      // 箭头前或新行：可以是 INPUT, RULESET, OUTPUT
      addSuggestion('INPUT', monaco.languages.CompletionItemKind.Module, 'Input component reference', 'INPUT');
      addSuggestion('RULESET', monaco.languages.CompletionItemKind.Module, 'Ruleset component reference', 'RULESET');
      addSuggestion('OUTPUT', monaco.languages.CompletionItemKind.Module, 'Output component reference', 'OUTPUT');
    }

    // 将Map中的建议转换成数组
    suggestions.push(...Array.from(suggestionsMap.values()));
    
    // 检查是否应该提示箭头操作符
    // 只有当行中有完整的组件引用（如 INPUT.demo, RULESET.test）且不在箭头后时才提示
    const hasCompleteComponentRef = /\b(INPUT|RULESET|OUTPUT)\.\w+/.test(lineText);
    if (!isAfterArrow && hasCompleteComponentRef) {
      suggestions.push({
        label: '->',
        kind: monaco.languages.CompletionItemKind.Operator,
        documentation: 'Flow operator',
        insertText: ' -> ',
        range: range
      });
    }
    
    // Flow Template已移除 - 用户不需要
  }
  
  // 最终去重
  const finalSuggestions = [];
  const seenLabels = new Set();
  
  suggestions.forEach(suggestion => {
    if (suggestion && suggestion.label) {
      const label = suggestion.label.toString().trim();
      if (!seenLabels.has(label)) {
        seenLabels.add(label);
        finalSuggestions.push(suggestion);
      }
    }
  });
  
  return { suggestions: finalSuggestions };
}

// 获取当前光标位置的单词
function getCurrentWord(lineText, column) {
  const beforeCursor = lineText.substring(0, column - 1);
  const afterCursor = lineText.substring(column - 1);
  
  // 查找单词边界，特别处理组件引用格式（如 INPUT.component_name）
  const wordStart = Math.max(
    beforeCursor.lastIndexOf(' '),
    beforeCursor.lastIndexOf('\t'),
    beforeCursor.lastIndexOf('|'),
    beforeCursor.lastIndexOf('>'),
    beforeCursor.lastIndexOf('-'),
    0  // 确保不会是负数
  ) + 1;
  
  // 对于afterCursor，需要找到下一个分隔符，但要保留完整的组件引用
  const wordEnd = afterCursor.search(/[\s\t|>-]/) === -1 ? afterCursor.length : afterCursor.search(/[\s\t|>-]/);
  
  const word = beforeCursor.substring(wordStart) + afterCursor.substring(0, wordEnd);
  
  return word;
}

// Ruleset XML智能补全
function getRulesetXmlCompletions(fullText, lineText, range, position) {
  // 解析当前XML上下文
  const context = parseXmlContext(fullText, position.lineNumber, position.column);
  
  // 根据不同的上下文提供精确的补全，避免重复
  let result;
  if (context.isInAttributeValue) {
    result = getXmlAttributeValueCompletions(context, range);
  } else if (context.isInAttributeName) {
    result = getXmlAttributeNameCompletions(context, range);
  } else if (context.isInTagName) {
    result = getXmlTagNameCompletions(context, range, fullText);
  } else if (context.isInTagContent) {
    result = getXmlTagContentCompletions(context, range, fullText);
  } else {
    // 默认情况 - 只有在没有其他匹配时才提供默认建议
    result = getDefaultXmlCompletions(fullText, range);
  }
  
  return result;
}

// 解析XML上下文
function parseXmlContext(fullText, lineNumber, column) {
  const lines = fullText.split('\n');
  const currentLine = lines[lineNumber - 1] || '';
  const beforeCursor = currentLine.substring(0, column - 1);
  const afterCursor = currentLine.substring(column - 1);
  
  // 检测当前位置的上下文
  const context = {
    currentLine,
    beforeCursor,
    afterCursor,
    isInTag: false,
    isInAttributeName: false,
    isInAttributeValue: false,
    isInTagName: false,
    isInTagContent: false,
    currentTag: '',
    currentAttribute: '',
    parentTags: [],
    attributeQuoteChar: ''
  };
  
  // 解析父标签层级
  const beforeLines = lines.slice(0, lineNumber - 1).join('\n') + '\n' + beforeCursor;
  context.parentTags = getParentTags(beforeLines);
  
  // 检测是否在属性值内（引号内）
  const lastQuote = Math.max(beforeCursor.lastIndexOf('"'), beforeCursor.lastIndexOf("'"));
  const lastOpenTag = beforeCursor.lastIndexOf('<');
  const lastCloseTag = beforeCursor.lastIndexOf('>');
  
  if (lastQuote > lastOpenTag && lastOpenTag > lastCloseTag) {
    // 在属性值内
    context.isInAttributeValue = true;
    context.attributeQuoteChar = beforeCursor.charAt(lastQuote);
    
    // 查找属性名
    const beforeQuote = beforeCursor.substring(0, lastQuote);
    const equalsPos = beforeQuote.lastIndexOf('=');
    if (equalsPos > 0) {
      const attrName = beforeQuote.substring(0, equalsPos).match(/(\w+)\s*$/);
      if (attrName) {
        context.currentAttribute = attrName[1];
      }
    }
    
    // 查找当前标签名
    const tagMatch = beforeCursor.match(/<(\w+)[^>]*$/);
    if (tagMatch) {
      context.currentTag = tagMatch[1];
    }
  } else if (lastOpenTag > lastCloseTag) {
    // 在标签内但不在属性值内
    context.isInTag = true;
    
    const tagContent = beforeCursor.substring(lastOpenTag + 1);
    const spacePos = tagContent.indexOf(' ');
    
    if (spacePos === -1) {
      // 还在输入标签名
      context.isInTagName = true;
      context.currentTag = tagContent;
    } else {
      // 在属性区域
      context.currentTag = tagContent.substring(0, spacePos);
      
      // 检测是否在输入属性名
      const afterTagName = tagContent.substring(spacePos + 1);
      const lastEquals = afterTagName.lastIndexOf('=');
      const lastSpace = afterTagName.lastIndexOf(' ');
      
      if (lastEquals === -1 || lastSpace > lastEquals) {
        context.isInAttributeName = true;
      }
    }
  } else {
    // 在标签内容中或准备输入新标签
    const trimmedBefore = beforeCursor.trim();
    
    // 更精确地检查是否在输入新标签
    if (trimmedBefore.endsWith('<') || (beforeCursor.includes('<') && !beforeCursor.includes('>'))) {
      // 检查最后一个 '<' 后面的内容
      const lastOpenIndex = beforeCursor.lastIndexOf('<');
      if (lastOpenIndex !== -1) {
        const afterOpen = beforeCursor.substring(lastOpenIndex + 1);
        // 如果 '<' 后面没有空格且没有 '>'，说明在输入标签名
        if (!afterOpen.includes(' ') && !afterOpen.includes('>')) {
          context.isInTagName = true;
          context.currentTag = afterOpen;
        }
      }
    } else {
      context.isInTagContent = true;
      if (context.parentTags.length > 0) {
        context.currentTag = context.parentTags[context.parentTags.length - 1];
      }
    }
  }
  
  return context;
}

// 获取父标签层级
function getParentTags(textBeforeCursor) {
  const tags = [];
  const tagRegex = /<\/?(\w+)[^>]*>/g;
  let match;
  
  while ((match = tagRegex.exec(textBeforeCursor)) !== null) {
    const isClosing = match[0].startsWith('</');
    const tagName = match[1];
    
    if (isClosing) {
      // 移除最后一个同名标签
      for (let i = tags.length - 1; i >= 0; i--) {
        if (tags[i] === tagName) {
          tags.splice(i, 1);
          break;
        }
      }
    } else {
      // 添加开启标签
      tags.push(tagName);
    }
  }
  
  return tags;
}

// 属性值补全
function getXmlAttributeValueCompletions(context, range) {
  const suggestions = [];
  
  // node标签的type属性
  if (context.currentTag === 'node' && context.currentAttribute === 'type') {
    const nodeTypes = [
      { value: 'REGEX', description: 'Regular expression match' },
      { value: 'EQU', description: 'Equal comparison' },
      { value: 'NEQ', description: 'Not equal comparison' },
      { value: 'INCL', description: 'Include check' },
      { value: 'NI', description: 'Not include check' },
      { value: 'START', description: 'Starts with check' },
      { value: 'END', description: 'Ends with check' },
      { value: 'NSTART', description: 'Not starts with' },
      { value: 'NEND', description: 'Not ends with' },
      { value: 'NCS_EQU', description: 'Case-insensitive equal' },
      { value: 'NCS_NEQ', description: 'Case-insensitive not equal' },
      { value: 'NCS_INCL', description: 'Case-insensitive include' },
      { value: 'NCS_NI', description: 'Case-insensitive not include' },
      { value: 'NCS_START', description: 'Case-insensitive starts with' },
      { value: 'NCS_END', description: 'Case-insensitive ends with' },
      { value: 'NCS_NSTART', description: 'Case-insensitive not starts with' },
      { value: 'NCS_NEND', description: 'Case-insensitive not ends with' },
      { value: 'MT', description: 'More than (greater than)' },
      { value: 'LT', description: 'Less than' },
      { value: 'ISNULL', description: 'Is null check' },
      { value: 'NOTNULL', description: 'Is not null check' },
      { value: 'PLUGIN', description: 'Plugin function call' }
    ];
    
    nodeTypes.forEach(type => {
      if (!suggestions.some(s => s.label === type.value)) {
        suggestions.push({
          label: type.value,
          kind: monaco.languages.CompletionItemKind.EnumMember,
          documentation: type.description,
          insertText: type.value,
          range: range
        });
      }
    });
  }
  
  // node标签的logic属性
  else if (context.currentTag === 'node' && context.currentAttribute === 'logic') {
    suggestions.push(
      { label: 'AND', kind: monaco.languages.CompletionItemKind.EnumMember, documentation: 'Logical AND operation', insertText: 'AND', range: range },
      { label: 'OR', kind: monaco.languages.CompletionItemKind.EnumMember, documentation: 'Logical OR operation', insertText: 'OR', range: range }
    );
  }
  
  // threshold标签的count_type属性
  else if (context.currentTag === 'threshold' && context.currentAttribute === 'count_type') {
    suggestions.push(
      { label: 'SUM', kind: monaco.languages.CompletionItemKind.EnumMember, documentation: 'Sum aggregation', insertText: 'SUM', range: range },
      { label: 'CLASSIFY', kind: monaco.languages.CompletionItemKind.EnumMember, documentation: 'Classification aggregation', insertText: 'CLASSIFY', range: range }
    );
  }
  
  // threshold或root标签的local_cache/type属性
  else if ((context.currentTag === 'threshold' && context.currentAttribute === 'local_cache') ||
           (context.currentTag === 'root' && context.currentAttribute === 'type')) {
    if (context.currentAttribute === 'local_cache') {
      suggestions.push(
        { label: 'true', kind: monaco.languages.CompletionItemKind.EnumMember, documentation: 'Enable local cache', insertText: 'true', range: range },
        { label: 'false', kind: monaco.languages.CompletionItemKind.EnumMember, documentation: 'Disable local cache', insertText: 'false', range: range }
      );
    } else if (context.currentAttribute === 'type') {
      suggestions.push(
        { label: 'DETECTION', kind: monaco.languages.CompletionItemKind.EnumMember, documentation: 'Detection ruleset type', insertText: 'DETECTION', range: range },
        { label: 'CLASSIFICATION', kind: monaco.languages.CompletionItemKind.EnumMember, documentation: 'Classification ruleset type', insertText: 'CLASSIFICATION', range: range }
      );
    }
  }
  
  // append标签的type属性
  else if (context.currentTag === 'append' && context.currentAttribute === 'type') {
    suggestions.push(
      { label: 'PLUGIN', kind: monaco.languages.CompletionItemKind.EnumMember, documentation: 'Plugin-based append', insertText: 'PLUGIN', range: range }
    );
  }
  
  // 时间范围建议 (threshold range属性)
  else if (context.currentTag === 'threshold' && context.currentAttribute === 'range') {
    const timeRanges = ['30s', '1m', '5m', '10m', '30m', '1h', '6h', '12h', '1d'];
    timeRanges.forEach(time => {
      if (!suggestions.some(s => s.label === time)) {
        suggestions.push({
          label: time,
          kind: monaco.languages.CompletionItemKind.Value,
          documentation: `Time range: ${time}`,
          insertText: time,
          range: range
        });
      }
    });
  }
  
  // 常见字段名建议
  else if (context.currentAttribute === 'field') {
    const commonFields = ['data_type', 'exe', 'argv', 'pid', 'sessionid', 'source_ip', 'dest_ip', 'sport', 'dport'];
    commonFields.forEach(field => {
      if (!suggestions.some(s => s.label === field)) {
        suggestions.push({
          label: field,
          kind: monaco.languages.CompletionItemKind.Field,
          documentation: `Common field: ${field}`,
          insertText: field,
          range: range
        });
      }
    });
  }
  
  return { suggestions };
}

// 属性名补全
function getXmlAttributeNameCompletions(context, range) {
  const suggestions = [];
  
  switch (context.currentTag) {
    case 'root':
      suggestions.push(
        { label: 'name', kind: monaco.languages.CompletionItemKind.Property, documentation: 'Ruleset name', insertText: 'name="${1:ruleset-name}"', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range },
        { label: 'type', kind: monaco.languages.CompletionItemKind.Property, documentation: 'Ruleset type', insertText: 'type="${1|DETECTION,CLASSIFICATION|}"', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range }
      );
      break;
      
    case 'rule':
      suggestions.push(
        { label: 'id', kind: monaco.languages.CompletionItemKind.Property, documentation: 'Unique rule identifier', insertText: 'id="${1:rule-id}"', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range },
        { label: 'name', kind: monaco.languages.CompletionItemKind.Property, documentation: 'Rule display name', insertText: 'name="${1:rule-name}"', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range },
        { label: 'author', kind: monaco.languages.CompletionItemKind.Property, documentation: 'Rule author', insertText: 'author="${1:author}"', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range }
      );
      break;
      
    case 'node':
      suggestions.push(
        { label: 'id', kind: monaco.languages.CompletionItemKind.Property, documentation: 'Node identifier for conditions', insertText: 'id="${1:node-id}"', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range },
        { label: 'type', kind: monaco.languages.CompletionItemKind.Property, documentation: 'Check type', insertText: 'type="${1|REGEX,EQU,INCL,PLUGIN|}"', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range },
        { label: 'field', kind: monaco.languages.CompletionItemKind.Property, documentation: 'Field to check', insertText: 'field="${1:field-name}"', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range },
        { label: 'logic', kind: monaco.languages.CompletionItemKind.Property, documentation: 'Logical operation for multiple values', insertText: 'logic="${1|AND,OR|}"', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range },
        { label: 'delimiter', kind: monaco.languages.CompletionItemKind.Property, documentation: 'Delimiter for multiple values', insertText: 'delimiter="${1:|}"', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range }
      );
      break;
      
    case 'filter':
      suggestions.push(
        { label: 'field', kind: monaco.languages.CompletionItemKind.Property, documentation: 'Field to filter on', insertText: 'field="${1:field-name}"', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range }
      );
      break;
      
    case 'checklist':
      suggestions.push(
        { label: 'condition', kind: monaco.languages.CompletionItemKind.Property, documentation: 'Logical condition using node IDs', insertText: 'condition="${1:a and b}"', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range }
      );
      break;
      
    case 'threshold':
      suggestions.push(
        { label: 'group_by', kind: monaco.languages.CompletionItemKind.Property, documentation: 'Fields to group by', insertText: 'group_by="${1:field1,field2}"', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range },
        { label: 'range', kind: monaco.languages.CompletionItemKind.Property, documentation: 'Time range for aggregation', insertText: 'range="${1:5m}"', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range },
        { label: 'count_type', kind: monaco.languages.CompletionItemKind.Property, documentation: 'Counting method', insertText: 'count_type="${1|SUM,CLASSIFY|}"', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range },
        { label: 'count_field', kind: monaco.languages.CompletionItemKind.Property, documentation: 'Field to count', insertText: 'count_field="${1:field}"', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range },
        { label: 'local_cache', kind: monaco.languages.CompletionItemKind.Property, documentation: 'Use local cache', insertText: 'local_cache="${1|true,false|}"', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range }
      );
      break;
      
    case 'append':
      suggestions.push(
        { label: 'field_name', kind: monaco.languages.CompletionItemKind.Property, documentation: 'Name of field to append', insertText: 'field_name="${1:field-name}"', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range },
        { label: 'type', kind: monaco.languages.CompletionItemKind.Property, documentation: 'Append type (PLUGIN for dynamic values)', insertText: 'type="PLUGIN"', range: range }
      );
      break;
  }
  
  return { suggestions };
}

// 标签名补全
function getXmlTagNameCompletions(context, range, fullText) {
  const suggestions = [];
  const parentTag = context.parentTags[context.parentTags.length - 1];
  
  // 根据父标签提供精确的子标签建议
  if (!parentTag) {
    // 根级别 - 只能有root标签
    if (!fullText.includes('<root')) {
      suggestions.push({
        label: 'root',
        kind: monaco.languages.CompletionItemKind.Module,
        documentation: 'Root element for ruleset',
        insertText: 'root name="${1:ruleset-name}" type="${2|DETECTION,CLASSIFICATION|}">\n    ${3}\n</root>',
        insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
        range: range
      });
    }
  } else if (parentTag === 'root') {
    // root内部 - 只能有rule标签，确保只添加一次
    if (!suggestions.some(s => s.label === 'rule')) {
      suggestions.push({
        label: 'rule',
        kind: monaco.languages.CompletionItemKind.Module,
        documentation: 'Rule definition',
        insertText: 'rule id="${1:rule-id}" name="${2:rule-name}" author="${3:author}">\n    ${4}\n</rule>',
        insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
        range: range
      });
    }
  } else if (parentTag === 'rule') {
    // rule内部 - 提供所有可能的子标签
    const ruleChildTags = [
      {
        label: 'filter',
        kind: monaco.languages.CompletionItemKind.Property,
        documentation: 'Filter condition for rule',
        insertText: 'filter field="${1:field-name}">${2:_$value}</filter>',
        insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
        range: range
      },
      {
        label: 'checklist',
        kind: monaco.languages.CompletionItemKind.Module,
        documentation: 'Checklist with conditions',
        insertText: 'checklist condition="${1:a and b}">\n    <node id="${2:a}" type="${3|REGEX,EQU,INCL,PLUGIN|}" field="${4:field-name}">${5:value}</node>\n    <node id="${6:b}" type="${7|REGEX,EQU,INCL,PLUGIN|}" field="${8:field-name}">${9:value}</node>\n</checklist>',
        insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
        range: range
      },
      {
        label: 'threshold',
        kind: monaco.languages.CompletionItemKind.Property,
        documentation: 'Threshold configuration',
        insertText: 'threshold group_by="${1:field1,field2}" range="${2:5m}" count_type="${3|SUM,CLASSIFY|}" count_field="${4:field}" local_cache="${5|true,false|}">${6:10}</threshold>',
        insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
        range: range
      },
      {
        label: 'append',
        kind: monaco.languages.CompletionItemKind.Property,
        documentation: 'Append field to result',
        insertText: 'append field_name="${1:field-name}"${2: type="PLUGIN"}>${3:value}</append>',
        insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
        range: range
      },
      {
        label: 'plugin',
        kind: monaco.languages.CompletionItemKind.Function,
        documentation: 'Plugin execution',
        insertText: 'plugin>${1:plugin_name(_$ORIDATA)}</plugin>',
        insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
        range: range
      },
      {
        label: 'del',
        kind: monaco.languages.CompletionItemKind.Property,
        documentation: 'Delete fields from result',
        insertText: 'del>${1:field1,field2}</del>',
        insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
        range: range
      }
    ];
    
    suggestions.push(...ruleChildTags);
  } else if (parentTag === 'checklist') {
    // checklist内部 - 只能有node标签，确保只添加一次
    if (!suggestions.some(s => s.label === 'node')) {
      suggestions.push({
        label: 'node',
        kind: monaco.languages.CompletionItemKind.Property,
        documentation: 'Check node',
        insertText: 'node id="${1:node-id}" type="${2|REGEX,EQU,INCL,PLUGIN|}" field="${3:field-name}"${4: logic="${5|AND,OR|}" delimiter="${6:||}"}>${7:value}</node>',
        insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
        range: range
      });
    }
  }
  
  return { suggestions };
}

// 标签内容补全
function getXmlTagContentCompletions(context, range, fullText) {
  const suggestions = [];
  
  // 插件函数补全
  if (context.currentTag === 'plugin' || (context.currentTag === 'node' && fullText.includes('type="PLUGIN"')) || (context.currentTag === 'append' && fullText.includes('type="PLUGIN"'))) {
    
    // 添加现有插件的建议
    pluginComponents.value.forEach(plugin => {
      const pluginLabel = `${plugin.id}(_$ORIDATA)`;
      if (!suggestions.some(s => s.label === pluginLabel)) {
        suggestions.push({
          label: pluginLabel,
          kind: monaco.languages.CompletionItemKind.Function,
          documentation: `Plugin: ${plugin.id}`,
          insertText: pluginLabel,
          range: range
        });
      }
    });
    
    // 添加通用插件模板
    if (!suggestions.some(s => s.label === 'plugin_name(_$ORIDATA)')) {
      suggestions.push({
        label: 'plugin_name(_$ORIDATA)',
        kind: monaco.languages.CompletionItemKind.Snippet,
        documentation: 'Plugin function with original data',
        insertText: '${1:plugin_name}(_$ORIDATA)',
        insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
        range: range
      });
    }
    
    if (!suggestions.some(s => s.label === 'plugin_name("arg1", arg2)')) {
      suggestions.push({
        label: 'plugin_name("arg1", arg2)',
        kind: monaco.languages.CompletionItemKind.Snippet,
        documentation: 'Plugin function with custom arguments',
        insertText: '${1:plugin_name}("${2:arg1}", ${3:arg2})',
        insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
        range: range
      });
    }
  }
  
  // 过滤器值建议
  if (context.currentTag === 'filter') {
    suggestions.push(
      { label: '_$data_type', kind: monaco.languages.CompletionItemKind.Variable, documentation: 'Dynamic value from raw data', insertText: '_$data_type', range: range },
      { label: '_$sessionid', kind: monaco.languages.CompletionItemKind.Variable, documentation: 'Dynamic session ID', insertText: '_$sessionid', range: range },
      { label: '59', kind: monaco.languages.CompletionItemKind.Value, documentation: 'Numeric value', insertText: '59', range: range }
    );
  }
  
  // 节点值建议
  if (context.currentTag === 'node') {
    suggestions.push(
      { label: '_$ORIDATA', kind: monaco.languages.CompletionItemKind.Variable, documentation: 'Original data reference', insertText: '_$ORIDATA', range: range },
      { label: 'value1|value2', kind: monaco.languages.CompletionItemKind.Snippet, documentation: 'Multiple values with delimiter', insertText: '${1:value1}|${2:value2}', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range }
    );
  }
  
  return { suggestions };
}

// 默认XML补全 - 只在特定情况下提供
function getDefaultXmlCompletions(fullText, range) {
  const suggestions = [];
  
  // 只在完全空白的文档中提供完整模板
  if (!fullText.trim() || (!fullText.includes('<root') && !fullText.includes('<'))) {
    suggestions.push({
      label: 'Complete Ruleset Template',
      kind: monaco.languages.CompletionItemKind.Snippet,
      documentation: 'Complete ruleset XML template',
      insertText: [
        '<root name="${1:ruleset-name}" type="${2|DETECTION,CLASSIFICATION|}">',
        '    <rule id="${3:rule-id}" name="${4:rule-name}" author="${5:author}">',
        '        <filter field="${6:field-name}">${7:filter-value}</filter>',
        '        <checklist condition="${8:condition}">',
        '            <node id="${9:node-id}" type="${10|REGEX,EQU,INCL,PLUGIN|}" field="${11:field-name}">${12:value}</node>',
        '        </checklist>',
        '        ${13}',
        '    </rule>',
        '</root>'
      ].join('\n'),
      insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
      range: range
    });
  }
  
  return { suggestions };
}

// Plugin Go代码智能补全
function getPluginGoCompletions(fullText, lineText, range, position) {
  const suggestions = [];
  
  // 解析当前Go代码上下文
  const context = parseGoContext(fullText, lineText, position);
  
  // 根据不同的上下文提供精确的补全
  let result;
  if (context.isInFunction) {
    result = getGoFunctionCompletions(context, range, fullText);
  } else if (context.isInImport) {
    result = getGoImportCompletions(context, range, fullText);
  } else if (context.isInPackage) {
    result = getGoPackageCompletions(context, range, fullText);
  } else {
    // 默认情况 - 根据当前位置提供建议
    result = getDefaultGoCompletions(fullText, context, range);
  }
  
  return result;
}

// 解析Go代码上下文
function parseGoContext(fullText, lineText, position) {
  const lines = fullText.split('\n');
  const currentLineIndex = position.lineNumber - 1;
  const beforeCursor = lineText.substring(0, position.column - 1);
  
  const context = {
    currentLine: lineText,
    beforeCursor,
    isInFunction: false,
    isInImport: false,
    isInPackage: false,
    currentFunction: '',
    hasPackage: fullText.includes('package '),
    hasImport: fullText.includes('import '),
    hasEvalFunction: fullText.includes('func Eval'),
    indentLevel: getIndentLevel(lineText)
  };
  
  // 检测是否在函数内部
  let braceCount = 0;
  let inFunction = false;
  let currentFunc = '';
  
  for (let i = 0; i <= currentLineIndex; i++) {
    const line = lines[i];
    
    // 检测函数声明
    const funcMatch = line.match(/func\s+(\w+)/);
    if (funcMatch && braceCount === 0) {
      currentFunc = funcMatch[1];
      inFunction = true;
    }
    
    // 计算大括号层级
    for (const char of line) {
      if (char === '{') braceCount++;
      if (char === '}') braceCount--;
    }
    
    // 如果大括号归零且不在当前行，说明函数结束
    if (braceCount === 0 && i < currentLineIndex && inFunction) {
      inFunction = false;
      currentFunc = '';
    }
  }
  
  context.isInFunction = inFunction && braceCount > 0;
  context.currentFunction = currentFunc;
  
  // 检测是否在import块内
  context.isInImport = fullText.includes('import (') && 
                      !fullText.substring(0, fullText.indexOf(lineText)).includes(')') &&
                      lineText.includes('"');
  
  // 检测是否在package行
  context.isInPackage = lineText.includes('package') || (!context.hasPackage && context.indentLevel === 0);
  
  return context;
}

// Go函数内补全
function getGoFunctionCompletions(context, range, fullText) {
  const suggestions = [];
  
  // 错误处理模式
  suggestions.push(
    {
      label: 'if err != nil',
      kind: monaco.languages.CompletionItemKind.Snippet,
      documentation: 'Standard Go error handling pattern',
      insertText: [
        'if err != nil {',
        '    return ${1|false,nil|}, err',
        '}'
      ].join('\n'),
      insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
      range: range
    },
    {
      label: 'if data == ""',
      kind: monaco.languages.CompletionItemKind.Snippet,
      documentation: 'Check for empty string data',
      insertText: [
        'if data == "" {',
        '    return false, errors.New("${1:empty data}")',
        '}'
      ].join('\n'),
      insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
      range: range
    }
  );
  
  // 字符串操作
  if (fullText.includes('strings')) {
    suggestions.push(
      { label: 'strings.Contains', kind: monaco.languages.CompletionItemKind.Function, documentation: 'Check if string contains substring', insertText: 'strings.Contains(${1:data}, "${2:substring}")', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range },
      { label: 'strings.HasPrefix', kind: monaco.languages.CompletionItemKind.Function, documentation: 'Check if string has prefix', insertText: 'strings.HasPrefix(${1:data}, "${2:prefix}")', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range },
      { label: 'strings.HasSuffix', kind: monaco.languages.CompletionItemKind.Function, documentation: 'Check if string has suffix', insertText: 'strings.HasSuffix(${1:data}, "${2:suffix}")', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range },
      { label: 'strings.ToLower', kind: monaco.languages.CompletionItemKind.Function, documentation: 'Convert string to lowercase', insertText: 'strings.ToLower(${1:data})', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range },
      { label: 'strings.Split', kind: monaco.languages.CompletionItemKind.Function, documentation: 'Split string by delimiter', insertText: 'strings.Split(${1:data}, "${2:,}")', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range }
    );
  }
  
  // 正则表达式操作
  if (fullText.includes('regexp')) {
    suggestions.push(
      { label: 'regexp.MatchString', kind: monaco.languages.CompletionItemKind.Function, documentation: 'Match string against regex pattern', insertText: 'regexp.MatchString("${1:pattern}", ${2:data})', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range },
      { label: 'regexp.Compile', kind: monaco.languages.CompletionItemKind.Function, documentation: 'Compile regex pattern', insertText: 'regexp.Compile("${1:pattern}")', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range }
    );
  }
  
  // JSON操作
  if (fullText.includes('json')) {
    suggestions.push(
      { label: 'json.Unmarshal', kind: monaco.languages.CompletionItemKind.Function, documentation: 'Unmarshal JSON data', insertText: 'json.Unmarshal([]byte(${1:data}), &${2:target})', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range },
      { label: 'json.Marshal', kind: monaco.languages.CompletionItemKind.Function, documentation: 'Marshal data to JSON', insertText: 'json.Marshal(${1:data})', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range }
    );
  }
  
  // 常用返回语句
  suggestions.push(
    { label: 'return true, nil', kind: monaco.languages.CompletionItemKind.Snippet, documentation: 'Return success', insertText: 'return true, nil', range: range },
    { label: 'return false, nil', kind: monaco.languages.CompletionItemKind.Snippet, documentation: 'Return failure without error', insertText: 'return false, nil', range: range },
    { label: 'return false, errors.New', kind: monaco.languages.CompletionItemKind.Snippet, documentation: 'Return failure with error', insertText: 'return false, errors.New("${1:error message}")', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range }
  );
  
  return { suggestions };
}

// Go导入补全
function getGoImportCompletions(context, range, fullText) {
  const suggestions = [];
  
  const commonImports = [
    { pkg: 'errors', desc: 'Error handling utilities' },
    { pkg: 'strings', desc: 'String manipulation functions' },
    { pkg: 'regexp', desc: 'Regular expression support' },
    { pkg: 'encoding/json', desc: 'JSON encoding and decoding' },
    { pkg: 'fmt', desc: 'Formatted I/O functions' },
    { pkg: 'strconv', desc: 'String conversion utilities' },
    { pkg: 'time', desc: 'Time and date functions' },
    { pkg: 'net/url', desc: 'URL parsing utilities' },
    { pkg: 'crypto/md5', desc: 'MD5 hash functions' },
    { pkg: 'crypto/sha256', desc: 'SHA256 hash functions' }
  ];
  
  commonImports.forEach(imp => {
    if (!fullText.includes(`"${imp.pkg}"`) && !suggestions.some(s => s.label === imp.pkg)) {
      suggestions.push({
        label: imp.pkg,
        kind: monaco.languages.CompletionItemKind.Module,
        documentation: imp.desc,
        insertText: `"${imp.pkg}"`,
        range: range
      });
    }
  });
  
  return { suggestions };
}

// Go包声明补全
function getGoPackageCompletions(context, range, fullText) {
  const suggestions = [];
  
  if (!context.hasPackage) {
    suggestions.push({
      label: 'package plugin',
      kind: monaco.languages.CompletionItemKind.Module,
      documentation: 'Plugin package declaration',
      insertText: 'package plugin',
      range: range
    });
  }
  
  return { suggestions };
}

// 默认Go补全
function getDefaultGoCompletions(fullText, context, range) {
  const suggestions = [];
  
  // 完整插件模板
  if (!context.hasPackage) {
    suggestions.push({
      label: 'Plugin Template',
      kind: monaco.languages.CompletionItemKind.Snippet,
      documentation: 'Complete plugin template with common imports',
      insertText: [
        'package plugin',
        '',
        'import (',
        '    "errors"',
        '    "strings"',
        '    "regexp"',
        ')',
        '',
        'func Eval(data string) (bool, error) {',
        '    if data == "" {',
        '        return false, errors.New("empty data")',
        '    }',
        '    ',
        '    ${1:// Your plugin logic here}',
        '    // Example: check if data contains specific pattern',
        '    // if strings.Contains(data, "suspicious") {',
        '    //     return true, nil',
        '    // }',
        '    ',
        '    return ${2:false}, nil',
        '}'
      ].join('\n'),
      insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
      range: range
    });
  }
  
  // 导入块
  if (context.hasPackage && !context.hasImport) {
    suggestions.push({
      label: 'import block',
      kind: monaco.languages.CompletionItemKind.Module,
      documentation: 'Import block with common packages',
      insertText: [
        'import (',
        '    "errors"',
        '    "strings"',
        '    "${1:additional-package}"',
        ')'
      ].join('\n'),
      insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
      range: range
    });
  }
  
  // Eval函数
  if (context.hasPackage && !context.hasEvalFunction) {
    suggestions.push(
      {
        label: 'func Eval (string)',
        kind: monaco.languages.CompletionItemKind.Function,
        documentation: 'Eval function with string parameter',
        insertText: [
          'func Eval(data string) (bool, error) {',
          '    ${1:// Your plugin logic here}',
          '    return ${2:false}, nil',
          '}'
        ].join('\n'),
        insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
        range: range
      },
      {
        label: 'func Eval (map)',
        kind: monaco.languages.CompletionItemKind.Function,
        documentation: 'Eval function with map parameter',
        insertText: [
          'func Eval(oriData map[string]interface{}) (map[string]interface{}, error) {',
          '    ${1:// Your plugin logic here}',
          '    return oriData, nil',
          '}'
        ].join('\n'),
        insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
        range: range
      }
    );
  }
  
  return { suggestions };
}

// 基础YAML智能补全
function getBaseYamlCompletions(fullText, lineText, range, position) {
  const suggestions = [];
  
  // 解析当前YAML上下文
  const context = parseYamlContext(fullText, lineText, position);
  
  // 根据不同的上下文提供精确的补全
  let result;
  if (context.isInValue) {
    result = getBaseYamlValueCompletions(context, range, fullText);
  } else if (context.isInKey) {
    result = getBaseYamlKeyCompletions(context, range, fullText);
  } else {
    // 默认情况 - 根据当前层级和已有配置提供建议
    result = getDefaultBaseYamlCompletions(fullText, context, range);
  }
  
  return result;
}

// 基础YAML值补全
function getBaseYamlValueCompletions(context, range, fullText) {
  const suggestions = [];
  
  // type属性值补全
  if (context.currentKey === 'type') {
    const componentTypes = [
      { value: 'kafka', description: 'Apache Kafka component' },
      { value: 'aliyun_sls', description: 'Alibaba Cloud SLS component' },
      { value: 'elasticsearch', description: 'Elasticsearch component' },
      { value: 'print', description: 'Console print component' }
    ];
    
    componentTypes.forEach(type => {
      if (!suggestions.some(s => s.label === type.value)) {
        suggestions.push({
          label: type.value,
          kind: monaco.languages.CompletionItemKind.EnumMember,
          documentation: type.description,
          insertText: type.value,
          range: range
        });
      }
    });
  }
  
  return { suggestions };
}

// 基础YAML键补全
function getBaseYamlKeyCompletions(context, range, fullText) {
  const suggestions = [];
  
  // 根级别配置
  if (context.indentLevel === 0) {
    if (!fullText.includes('type:')) {
      suggestions.push({
        label: 'type',
        kind: monaco.languages.CompletionItemKind.Property,
        documentation: 'Component type specification',
        insertText: 'type: ${1|kafka,aliyun_sls,elasticsearch,print|}',
        insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
        range: range
      });
    }
    
    if (!fullText.includes('name:')) {
      suggestions.push({
        label: 'name',
        kind: monaco.languages.CompletionItemKind.Property,
        documentation: 'Component name identifier',
        insertText: 'name: "${1:component-name}"',
        insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
        range: range
      });
    }
  }
  
  return { suggestions };
}

// 默认基础YAML补全
function getDefaultBaseYamlCompletions(fullText, context, range) {
  const suggestions = [];
  
  // 完整组件模板
  if (!fullText.includes('type:')) {
    suggestions.push(
      {
        label: 'Basic Component Template',
        kind: monaco.languages.CompletionItemKind.Snippet,
        documentation: 'Basic component configuration template',
        insertText: [
          'type: ${1|kafka,aliyun_sls,elasticsearch,print|}',
          'name: "${2:component-name}"'
        ].join('\n'),
        insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
        range: range
      }
    );
  }
  
  return { suggestions };
}

// 辅助函数
function getIndentLevel(line) {
  const match = line.match(/^(\s*)/);
  return match ? match[1].length : 0;
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
/* 导入编程字体 - 本地版本，避免网络超时 */
@import url('../assets/fonts/jetbrains-mono.css');

.monaco-editor-wrapper {
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0;
  border: none;
  overflow: hidden;
  font-family: "JetBrains Mono", "Fira Code", "Cascadia Code", "SF Mono", Monaco, Menlo, "Ubuntu Mono", Consolas, "Liberation Mono", "DejaVu Sans Mono", "Courier New", monospace;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
  font-feature-settings: "liga" 1, "calt" 1;
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

/* 字体优化 */
.monaco-editor .view-lines,
.monaco-diff-editor .view-lines {
  font-family: "JetBrains Mono", "Fira Code", "Cascadia Code", "SF Mono", Monaco, Menlo, "Ubuntu Mono", Consolas, "Liberation Mono", "DejaVu Sans Mono", "Courier New", monospace !important;
  font-size: 14px !important;
  line-height: 21px !important;
  font-weight: 400 !important;
  -webkit-font-smoothing: antialiased !important;
  -moz-osx-font-smoothing: grayscale !important;
  font-feature-settings: "liga" 1, "calt" 1 !important;
}

/* 行号字体优化 */
.monaco-editor .margin-view-overlays .line-numbers,
.monaco-diff-editor .margin-view-overlays .line-numbers {
  font-family: "JetBrains Mono", "Fira Code", "Cascadia Code", "SF Mono", Monaco, Menlo, "Ubuntu Mono", Consolas, "Liberation Mono", "DejaVu Sans Mono", "Courier New", monospace !important;
  font-size: 13px !important;
  font-weight: 400 !important;
  -webkit-font-smoothing: antialiased !important;
  -moz-osx-font-smoothing: grayscale !important;
}

/* minimap字体优化 */
.monaco-editor .minimap,
.monaco-diff-editor .minimap {
  font-family: "JetBrains Mono", "Fira Code", "Cascadia Code", "SF Mono", Monaco, Menlo, "Ubuntu Mono", Consolas, "Liberation Mono", "DejaVu Sans Mono", "Courier New", monospace !important;
  -webkit-font-smoothing: antialiased !important;
  -moz-osx-font-smoothing: grayscale !important;
}

/* 自动完成建议框字体优化 */
.monaco-editor .suggest-widget,
.monaco-diff-editor .suggest-widget {
  font-family: "JetBrains Mono", "Fira Code", "Cascadia Code", "SF Mono", Monaco, Menlo, "Ubuntu Mono", Consolas, "Liberation Mono", "DejaVu Sans Mono", "Courier New", monospace !important;
  font-size: 13px !important;
  -webkit-font-smoothing: antialiased !important;
  -moz-osx-font-smoothing: grayscale !important;
}

/* 悬停提示字体优化 */
.monaco-editor .monaco-hover,
.monaco-diff-editor .monaco-hover {
  font-family: "JetBrains Mono", "Fira Code", "Cascadia Code", "SF Mono", Monaco, Menlo, "Ubuntu Mono", Consolas, "Liberation Mono", "DejaVu Sans Mono", "Courier New", monospace !important;
  font-size: 13px !important;
  -webkit-font-smoothing: antialiased !important;
  -moz-osx-font-smoothing: grayscale !important;
}


</style> 