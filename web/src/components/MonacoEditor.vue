<template>
  <div class="monaco-editor-wrapper">
    <div ref="container" class="monaco-editor-container"></div>
  </div>
</template>

<script setup>
import { ref, watch, onMounted, onBeforeUnmount, onUnmounted, computed, nextTick } from 'vue';
import { useStore } from 'vuex';
import * as monaco from 'monaco-editor';
import { hubApi } from '@/api';
import { useDataCacheStore } from '@/stores/dataCache';
import eventManager from '@/utils/eventManager';

const props = defineProps({
  value: String,
  language: { type: String, default: 'yaml' },
  readOnly: { type: Boolean, default: true },
  errorLines: { type: Array, default: () => [] },
  originalValue: { type: String, default: '' }, // For diff mode
  diffMode: { type: Boolean, default: false }, // Enable diff mode
  componentId: { type: String, default: '' }, // For dynamic field completion (rulesets)
  componentType: { type: String, default: '' }, // Component type (input, output, ruleset)
});

// Properly declare emits
const emit = defineEmits(['update:value', 'save', 'line-change']);

const container = ref(null);
let editor = null;
let diffEditor = null;
const store = useStore();

// Use unified data cache store
const dataCache = useDataCacheStore();

// Get component lists from unified cache
const inputComponents = computed(() => dataCache.getComponentData('inputs') || []);
const outputComponents = computed(() => dataCache.getComponentData('outputs') || []);
const rulesetComponents = computed(() => dataCache.getComponentData('rulesets') || []);
const pluginComponents = computed(() => dataCache.getComponentData('plugins') || []);

// Smart plugin parameters cache - globally shared, supports event-driven cleanup
const globalPluginParametersCache = ref({});
let eventCleanupFunctions = []

// Initialize plugin parameters cache event listeners
const initializeParametersCache = () => {
  if (eventCleanupFunctions.length > 0) return // Already initialized
  
  // Use unified event manager to listen for plugin component changes
  const componentChangedCleanup = eventManager.on('componentChanged', (data) => {
    const { action, type, id } = data;
    if (type === 'plugins') {
      // console.log(`[Monaco] Plugin ${action}: clearing parameters cache for ${id}`);
      // Clear specific plugin's parameters cache
      if (id && globalPluginParametersCache.value[id]) {
        delete globalPluginParametersCache.value[id];
      }
      // If it's a delete operation, also clear plugin suggestions cache
      if (action === 'deleted') {
        globalPluginSuggestionsCache.clear();
        lastPluginDataHash = '';
      }
    }
  });
  
  // Listen for batch changes
  const pendingChangesCleanup = eventManager.on('pendingChangesApplied', (data) => {
    const { types } = data;
    if (Array.isArray(types) && types.includes('plugins')) {
      // console.log('[Monaco] Clearing all plugin parameters cache due to pending changes');
      globalPluginParametersCache.value = {};
      globalPluginSuggestionsCache.clear();
      lastPluginDataHash = '';
    }
  });
  
  const localChangesCleanup = eventManager.on('localChangesLoaded', (data) => {
    const { types } = data;
    if (Array.isArray(types) && types.includes('plugins')) {
      // console.log('[Monaco] Clearing all plugin parameters cache due to local changes');
      globalPluginParametersCache.value = {};
      globalPluginSuggestionsCache.clear();
      lastPluginDataHash = '';
    }
  });
  
  // Store cleanup functions
  eventCleanupFunctions.push(
    componentChangedCleanup,
    pendingChangesCleanup,
    localChangesCleanup
  );
  
      // console.log('[Monaco] Plugin parameters cache event listeners initialized via EventManager');
};

// Cleanup event listeners
const cleanupParametersCache = () => {
  eventCleanupFunctions.forEach(cleanup => cleanup());
  eventCleanupFunctions = [];
      // console.log('[Monaco] Plugin parameters cache event listeners cleaned up');
};

// Smart plugin parameters fetching (with cache)
const getPluginParameters = async (pluginId) => {
  // Initialize cache listeners
  if (eventCleanupFunctions.length === 0) {
    initializeParametersCache();
  }
  
  // Check cache
  if (globalPluginParametersCache.value[pluginId]) {
    return globalPluginParametersCache.value[pluginId];
  }
  
  try {
          // console.log(`[Monaco] Fetching parameters for plugin: ${pluginId}`);
    const parameters = await hubApi.getPluginParameters(pluginId);
    const result = parameters || [];
    
    // Cache result
    globalPluginParametersCache.value[pluginId] = result;
    
    return result;
  } catch (error) {
    console.warn(`Failed to fetch plugin parameters for ${pluginId}:`, error);
    // Cache empty result to avoid repeated requests
    globalPluginParametersCache.value[pluginId] = [];
    return [];
  }
};

// Plugin parameters cache - smart computed property that returns current cache state
const pluginParametersCache = computed(() => globalPluginParametersCache.value);

// Get dynamic field keys for ruleset completion
const dynamicFieldKeys = computed(() => {
  if ((props.componentType === 'ruleset' || props.componentType === 'rulesets') && props.componentId) {
    const rulesetFields = store.getters.getRulesetFields(props.componentId);
    return rulesetFields.fieldKeys || [];
  }
  return [];
});

// Watch for component changes to fetch field data
watch([() => props.componentId, () => props.componentType], ([newId, newType], [oldId, oldType]) => {
  if ((newType === 'ruleset' || newType === 'rulesets') && newId && (newId !== oldId || newType !== oldType)) {
    store.dispatch('fetchRulesetFields', newId);
  }
}, { immediate: true });

// Watch for plugin data changes to invalidate cache
watch([pluginComponents, pluginParametersCache], () => {
  // Clear plugin suggestions cache when plugin data changes
  globalPluginSuggestionsCache.clear();
  lastPluginDataHash = '';
}, { deep: true });

// Watch for dynamic field changes to trigger suggestions
watch(
  () => dynamicFieldKeys.value.length,
  (newLen, oldLen) => {
    if (newLen > 0 && newLen !== oldLen) {
      nextTick(() => {
        if (isEditorValid(editor)) {
          try {
            editor.trigger('dynamic-fields', 'editor.action.triggerSuggest', {});
              } catch (error) {
      // console.warn('Failed to trigger dynamic field suggestions:', error);
    }
        }
      });
    }
  }
);

// Get component lists when component is mounted
onMounted(async () => {
  // Preload component data for autocomplete
  try {
    await Promise.all([
      dataCache.fetchComponents('inputs'),
      dataCache.fetchComponents('outputs'),
      dataCache.fetchComponents('rulesets'),
      dataCache.fetchComponents('plugins')
    ]);
      } catch (error) {
      // console.warn('Failed to preload component data for Monaco:', error);
    }
  
  // Fetch dynamic field keys for rulesets
  if ((props.componentType === 'ruleset' || props.componentType === 'rulesets') && props.componentId) {
    store.dispatch('fetchRulesetFields', props.componentId);
  }
  
  // Setup Monaco theme
  setupMonacoTheme();
  
  // Completely disable Monaco's built-in YAML language support
  try {
    // Unregister all existing YAML completion providers
    const yamlProviders = monaco.languages.getLanguages().find(lang => lang.id === 'yaml');
    if (yamlProviders) {
      // Redefine YAML language, removing all built-in features
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
      // console.warn('Failed to disable built-in YAML support:', e);
    }
  
  // Register language providers
  registerLanguageProviders();
  
  // Register editor actions and shortcuts
  registerEditorActions();
  
  // Initialize editor
  initializeEditor();
  
  // Add window resize listener to ensure correct editor layout
  window.addEventListener('resize', handleResize);
  
  // Initial layout adjustment
  setTimeout(() => {
    handleResize();
  }, 200);

  // Plugin parameters cache management is now handled by the global eventManager system above
})

onUnmounted(() => {
  // Remove window size change monitoring
  window.removeEventListener('resize', handleResize);
  // Clean up event listeners
  cleanupParametersCache();
  disposeEditors();
})

// Setup Monaco theme
function setupMonacoTheme() {
  monaco.editor.defineTheme('agentsmith-theme', {
    base: 'vs',
    inherit: true,
    rules: [
      // Simple consistent XML highlighting
      { token: 'tag', foreground: 'e36209', fontStyle: 'bold' },              // XML tags - orange bold
      { token: 'tag.xml', foreground: 'e36209', fontStyle: 'bold' },           
      { token: 'attribute.name', foreground: '0969da', fontStyle: 'bold' },    // Attribute names - blue bold
      { token: 'attribute.name.xml', foreground: '0969da', fontStyle: 'bold' },
      { token: 'attribute.value', foreground: '22863a' },                      // Attribute values - green
      { token: 'attribute.value.xml', foreground: '22863a' },
      { token: 'delimiter', foreground: '6f42c1' },                            // Delimiters - purple
      { token: 'delimiter.xml', foreground: '6f42c1' },
      { token: 'comment', foreground: '6a737d', fontStyle: 'italic' },         // Comments - gray italic
      { token: 'comment.xml', foreground: '6a737d', fontStyle: 'italic' },
      
      // Numbers - tech blue
      { token: 'number', foreground: '0550ae' },
      
      // Keywords - accent orange-red
      { token: 'keyword', foreground: 'cf222e', fontStyle: 'bold' },
      
      // Properties - professional blue
      { token: 'property', foreground: '0969da' },
      
      // Comments - darker gray with italic for better readability
      { token: 'comment', foreground: '57606a', fontStyle: 'italic' },
      
      // Variables - amber for distinction
      { token: 'variable', foreground: 'bf8700' },
      
      // Types - modern purple
      { token: 'type', foreground: '8250df', fontStyle: 'bold' },
      
      // Project component reference keywords - distinct modern colors
      { token: 'project.component', foreground: '0969da', fontStyle: 'bold' },
      { token: 'project.input', foreground: '1a7f37', fontStyle: 'bold' },    // Rich green
      { token: 'project.output', foreground: 'd1242f', fontStyle: 'bold' },   // Modern red
      { token: 'project.ruleset', foreground: '8250df', fontStyle: 'bold' },  // Deep purple
      
      // YAML specific tokens
      { token: 'key', foreground: '0969da', fontStyle: 'bold' },
      { token: 'delimiter.colon', foreground: '656d76' },
      { token: 'delimiter.dash', foreground: '656d76' },
      
      // Go language tokens
      { token: 'keyword.go', foreground: 'cf222e', fontStyle: 'bold' },
      { token: 'type.go', foreground: '8250df', fontStyle: 'bold' },
      { token: 'function.go', foreground: '6639ba' },
    ],
    colors: {
      // Editor background - clean modern white with subtle warmth
      'editor.background': '#fafbfc',
      'editor.foreground': '#1f2328',
      
      // Line highlighting - minimal and subtle
      'editor.lineHighlightBackground': '#f6f8fa',
      'editor.lineHighlightBorder': '#d1d9e0',
      
      // Line numbers - modern contrast
      'editorLineNumber.foreground': '#656d76',
      'editorLineNumber.activeForeground': '#1f2328',
      'editorActiveLineNumber.foreground': '#1f2328',
      
      // Selection - sophisticated blue with transparency
      'editor.selectionBackground': '#0969da20',
      'editor.selectionHighlightBackground': '#0969da15',
      'editor.inactiveSelectionBackground': '#0969da10',
      
      // Cursor - professional dark
      'editorCursor.foreground': '#1f2328',
      
      // Error and warning colors - softer but still visible
      'editorError.foreground': '#d1242f',
      'editorError.background': '#fff5f5',
      'editorWarning.foreground': '#bf8700',
      'editorWarning.background': '#fffdf0',
      'editorInfo.foreground': '#0969da',
      
      // Gutter - clean and minimal
      'editorGutter.background': '#fafbfc',
      'editorGutter.addedBackground': '#1a7f37',
      'editorGutter.deletedBackground': '#d1242f',
      'editorGutter.modifiedBackground': '#0969da',
      
      // Scrollbar - enhanced visibility
      'scrollbarSlider.background': '#8c959f33',
      'scrollbarSlider.hoverBackground': '#8c959f55',
      'scrollbarSlider.activeBackground': '#8c959f77',
      
      // Minimap
      'minimap.background': '#f6f8fa',
      'minimap.selectionHighlight': '#0969da30',
      'minimap.errorHighlight': '#d1242f40',
      'minimap.warningHighlight': '#bf870040',
      
      // Find/replace widget
      'editorWidget.background': '#ffffff',
      'editorWidget.border': '#d1d9e0',
      'editorWidget.foreground': '#1f2328',
      
      // Suggest widget (autocomplete) - enhanced contrast for better readability
      'editorSuggestWidget.background': '#ffffff',
      'editorSuggestWidget.border': '#8c959f',
      'editorSuggestWidget.foreground': '#00ccb8',
      'editorSuggestWidget.selectedBackground': '#0b7999',
      'editorSuggestWidget.selectedForeground': '#ffffff',
      'editorSuggestWidget.highlightForeground': '#0550ae',
      'editorSuggestWidget.focusHighlightForeground': '#ffffff',
      
      // Hover widget - enhanced visibility
      'editorHoverWidget.background': '#ffffff',
      'editorHoverWidget.border': '#8c959f',
      'editorHoverWidget.foreground': '#1f2328',
      
      // Overview ruler
      'editorOverviewRuler.border': '#d1d9e0',
      'editorOverviewRuler.errorForeground': '#d1242f60',
      'editorOverviewRuler.warningForeground': '#bf870060',
      'editorOverviewRuler.infoForeground': '#0969da60',
      
      // Bracket match
      'editorBracketMatch.background': '#0969da20',
      'editorBracketMatch.border': '#0969da',
      
      // Indent guides - enhanced visibility
      'editorIndentGuide.background': '#8c959f40',
      'editorIndentGuide.activeBackground': '#57606a',
      
      // Rulers
      'editorRuler.foreground': '#d1d9e0',
      
      // Code lens - enhanced visibility
      'editorCodeLens.foreground': '#57606a',
      
      // Link
      'editorLink.activeForeground': '#0969da',
    }
  });
  
  monaco.editor.setTheme('agentsmith-theme');
}

// Utility function to deduplicate completion suggestions
function deduplicateCompletions(result, range, prefix) {
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
            sortText: `${prefix}_${String(index).padStart(3, '0')}_${label}`,
            detail: `${prefix.toUpperCase()}: ${label}`
          });
        }
      }
    });
    
    return {
      suggestions: uniqueSuggestions,
      incomplete: false
    };
  }
  
  return result || { suggestions: [], incomplete: false };
}

// Global registration flag to prevent conflicts
window.monacoProvidersRegistered = window.monacoProvidersRegistered || false;

// Register language providers
function registerLanguageProviders() {
  // Prevent duplicate registration to avoid Monaco internal conflicts
  if (window.monacoProvidersRegistered) {
    return;
  }
  
  // Register custom YAML language definition for project component keyword syntax highlighting
  monaco.languages.setMonarchTokensProvider('yaml', {
    defaultToken: '',
    ignoreCase: false,
    
    // Token patterns
    tokenizer: {
      root: [
        // Project component references - INPUT/OUTPUT/RULESET (must be followed by dot)
        [/\bINPUT(?=\.)/, 'project.input'],
        [/\bOUTPUT(?=\.)/, 'project.output'],
        [/\bRULESET(?=\.)/, 'project.ruleset'],
        
        // Comments
        [/#.*$/, 'comment'],
        
        // Strings
        [/"([^"\\]|\\.)*$/, 'string.invalid'],  // non-terminated string
        [/"/, 'string', '@dstring'],
        [/'([^'\\]|\\.)*$/, 'string.invalid'],  // non-terminated string
        [/'/, 'string', '@sstring'],
        
        // Numbers
        [/\d*\.\d+([eE][\-+]?\d+)?/, 'number.float'],
        [/0[xX][0-9a-fA-F]+/, 'number.hex'],
        [/\d+/, 'number'],
        
        // Delimiters
        [/[{}]/, 'delimiter.bracket'],
        [/\[/, 'delimiter.square'],
        [/\]/, 'delimiter.square'],
        [/:(?=\s|$)/, 'delimiter.colon'],
        [/,/, 'delimiter.comma'],
        [/-(?=\s)/, 'delimiter.dash'],
        [/\|/, 'delimiter.pipe'],
        [/>/, 'delimiter.greater'],
        
        // Keys (before colon)
        [/[a-zA-Z_][\w\-]*(?=\s*:)/, 'key'],
        
        // Identifiers
        [/[a-zA-Z_][\w\-]*/, 'identifier'],
        
        // Whitespace
        [/\s+/, ''],
      ],
      
      dstring: [
        [/[^\\"]+/, 'string'],
        [/\\./, 'string.escape'],
        [/"/, 'string', '@pop'],
      ],
      
      sstring: [
        [/[^\\']+/, 'string'],
        [/\\./, 'string.escape'],
        [/'/, 'string', '@pop'],
      ],
    },
  });

  


  // YAML language suggestions - for Input/Output/Project components
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
        
        // Get componentType from the current Vue component props
        // Use a global variable to store the current componentType
        const componentType = window.currentMonacoComponentType || 'unknown';
        
        if (componentType === 'input' || componentType === 'inputs') {
          result = getInputCompletions(textUntilPosition, lineUntilPosition, range, position);
        } else if (componentType === 'output' || componentType === 'outputs') {
          result = getOutputCompletions(textUntilPosition, lineUntilPosition, range, position);
        } else if (componentType === 'project' || componentType === 'projects') {
          // Check if this is a project flow definition (in content area)
          if (textUntilPosition.includes('content:') || lineUntilPosition.includes('->') || 
              lineUntilPosition.includes('INPUT.') || lineUntilPosition.includes('OUTPUT.') || 
              lineUntilPosition.includes('RULESET.')) {
            result = getProjectFlowCompletions(textUntilPosition, lineUntilPosition, range, position);
          } else {
            result = getProjectCompletions(textUntilPosition, lineUntilPosition, range, position);
          }
        } else {
          // Fallback to empty suggestions for unknown types
          result = { suggestions: [] };
        }
        
        // Simple deduplication
        return deduplicateCompletions(result, range, 'yaml');
        
        return { suggestions: [], incomplete: false };
      } catch (error) {
        console.error('YAML completion error:', error);
        return { suggestions: [], incomplete: false };
      }
    },
    
    // Include starting letters of common prefixes to trigger suggestions quickly
    triggerCharacters: [' ', ':', '\n', '\t', '-', '|', '.',
      'I','i','O','o','R','r',  // component prefixes
      'C','c',                  // content
      'T','t'                   // type
    ]
  });
  


  // XML language suggestions - for Ruleset components
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
        
        // Get componentType from global variable
        const componentType = window.currentMonacoComponentType || 'unknown';
        
        let result;
        if (componentType === 'ruleset' || componentType === 'rulesets' || componentType === 'unknown') {
          // For ruleset components or unknown XML (assume ruleset for backward compatibility)
          result = getRulesetXmlCompletions(textUntilPosition, lineUntilPosition, range, position);
        } else {
          // For non-ruleset XML, provide empty suggestions
          result = { suggestions: [] };
        }
        
        // Simple deduplication
        return deduplicateCompletions(result, range, 'xml');
        
        return { suggestions: [], incomplete: false };
      } catch (error) {
        console.error('XML completion error:', error);
        return { suggestions: [], incomplete: false };
      }
    },
    
    triggerCharacters: ['<', ' ', '=', '"', '\n', '\t', ',', '(', ')']
  });
  


  // Go language suggestions - for Plugin components
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
        
        // Disable Go autocomplete for plugins - keep it simple
        return { suggestions: [], incomplete: false };
      } catch (error) {
        console.error('Go completion error:', error);
        return { suggestions: [], incomplete: false };
      }
    },
    
    triggerCharacters: ['.', '(', ' ', '\n', '\t']
  });
  
  // Use Monaco's built-in XML language with custom styling
  // Don't override the XML tokenizer, just rely on Monaco's default XML parsing

  // Mark providers as registered globally
  window.monacoProvidersRegistered = true;
  

}

  // Initialize editor
function initializeEditor() {
  if (!container.value) return;
  
  // Set current componentType globally for completion providers
  window.currentMonacoComponentType = props.componentType;
  
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
        // Configure completion based on language type and read-only status
    quickSuggestions: props.readOnly ? false : true,
    snippetSuggestions: props.readOnly ? 'none' : 'inline',
    suggestOnTriggerCharacters: !props.readOnly,
    acceptSuggestionOnEnter: props.readOnly ? 'off' : 'on',
    tabCompletion: props.readOnly ? 'off' : 'on',
    suggestSelection: 'first',
    acceptSuggestionOnCommitCharacter: !props.readOnly,
    quickSuggestionsDelay: 100,
    // Disable built-in word completion, keep custom completions
    wordBasedSuggestions: false,
    suggest: {
      showWords: false
    },
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
    wordWrapColumn: 80,
    wrappingIndent: 'none',
  };
  
  // If diff mode, create diff editor
  if (props.diffMode && props.originalValue !== undefined) {
    diffEditor = monaco.editor.createDiffEditor(container.value, {
      ...options,
      originalEditable: false,
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
    
    // No need for metadata - using content-based detection
    
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
          try {
            // Get the line changes from the diff editor
            const lineChanges = diffEditor.getLineChanges();
            if (lineChanges && lineChanges.length > 0) {
              const firstChange = lineChanges[0];
              const modifiedEditor = diffEditor.getModifiedEditor();
              if (modifiedEditor && firstChange.modifiedStartLineNumber) {
                modifiedEditor.revealLineInCenter(firstChange.modifiedStartLineNumber);
              }
            }
          } catch (error) {
            console.warn('Failed to scroll to first difference:', error);
          }
        }
      }
    }, 300);
  } else {
    // Create regular editor
    editor = monaco.editor.create(container.value, options);
    
    // No need for metadata - using content-based detection
    
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
  
  // Listen for cursor position changes (for line-based validation)
  try {
    editor.onDidChangeCursorPosition((e) => {
      const lineNumber = e.position.lineNumber;
      emit('line-change', lineNumber);
    });
  } catch (error) {
    console.warn('Failed to add cursor position change listener:', error);
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

// Get editor language
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

// Store current decorator IDs
let currentDecorations = [];

// Update error line highlighting
function updateErrorLines(errorLines) {
  if (!isEditorValid(editor)) return;
  
  try {
    // Create a new decorator
    let newDecorations = [];
    
    // If there are any error lines, create a decorator
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
    
    // Update decorator: Remove old and apply new
    currentDecorations = editor.deltaDecorations(currentDecorations, newDecorations);
  } catch (error) {
    console.warn('Failed to update error lines:', error);
  }
}

// Monitoring value changes
watch(() => props.value, (newValue) => {
  if (editor && editor.getModel() && newValue !== editor.getValue()) {
    try {
      editor.setValue(newValue || '');
    } catch (error) {
      console.warn('Failed to set editor value:', error);
    }
  }
});

// Monitor language changes
watch(() => props.language, (newLanguage) => {
  if (editor && editor.getModel()) {
    try {
      const model = editor.getModel();
      if (model) {
        monaco.editor.setModelLanguage(model, getLanguage());
        
        // Set component type metadata for XML completion when switching to XML
        if (getLanguage() === 'xml') {
          model._associatedResource = { componentType: props.componentType };
        }
      }
    } catch (error) {
      console.warn('Failed to set editor language:', error);
    }
  }
});

// Monitor read-only status changes
watch(() => props.readOnly, (newReadOnly) => {
  if (isEditorValid(editor)) {
    try {
      editor.updateOptions({ readOnly: newReadOnly });
    } catch (error) {
      console.warn('Failed to update editor options:', error);
    }
  }
});

// Monitor error line changes
watch(() => props.errorLines, (newErrorLines) => {
  updateErrorLines(newErrorLines);
});

// Monitor diff mode changes
watch(() => [props.diffMode, props.originalValue], ([newDiffMode, newOriginalValue]) => {
  if (newDiffMode !== (diffEditor !== null)) {
    // The mode has changed and a new editor needs to be created
    disposeEditors();
    initializeEditor();
  } else if (isEditorValid(diffEditor) && newOriginalValue !== undefined) {
    try {
      // Only update the content of the original model
      const originalModel = diffEditor.getOriginalEditor().getModel();
      if (originalModel) {
        originalModel.setValue(newOriginalValue);
      }
    } catch (error) {
      console.warn('Failed to update diff editor original value:', error);
    }
  }
}, { deep: true });

// Monitor componentId changes to fetch field keys for rulesets
watch(() => [props.componentType, props.componentId], ([newType, newId], [oldType, oldId]) => {
  if (newType === 'ruleset' && newId && newId !== oldId) {
    store.dispatch('fetchRulesetFields', newId);
  }
  
  // Update global componentType for completion providers
  window.currentMonacoComponentType = newType;
});

// Handle window size changes
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

// Cleaning before component destruction
onBeforeUnmount(() => {
  // Remove window size change monitoring
  window.removeEventListener('resize', handleResize);
  // Clean up event listeners
  cleanupParametersCache();
  disposeEditors();
});

// Clean up editor instance
function disposeEditors() {
  try {
    if (isEditorValid(editor)) {
      editor.dispose();
    }
  } catch (error) {
    console.warn('Failed to dispose editor:', error);
  } finally {
    editor = null;
    currentDecorations = []; // Reset decorator array
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

// Register editor actions and shortcut keys
function registerEditorActions() {
  // Register intelligent code formatting action
  monaco.editor.addEditorAction({
    id: 'smart-format',
    label: 'Smart Format Document',
    keybindings: [
      monaco.KeyMod.CtrlCmd | monaco.KeyMod.Shift | monaco.KeyCode.KeyF
    ],
    contextMenuGroupId: 'navigation',
    contextMenuOrder: 1.5,
    run: function(editor) {
      // Intelligent formatting based on language type
      const model = editor.getModel();
      if (!model) return;
      
      const language = model.getLanguageId();
      const fullText = model.getValue();
      
      if (language === 'yaml') {
        formatYamlDocument(editor, fullText);
      } else if (language === 'xml') {
        formatXmlDocument(editor, fullText);
      } else if (language === 'go') {
        formatGoDocument(editor, fullText);
      }
    }
  });
  
  // Registration intelligent annotation switching
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
  
  // Suggested actions for quick registration completion
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

function formatYamlDocument(editor, content) {
  try {
    const lines = content.split('\n');
    const formattedLines = lines.map(line => {
      line = line.trimEnd();
      
      // Normalized indentation (2 spaces)
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

function formatXmlDocument(editor, content) {
  try {
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

function formatGoDocument(editor, content) {
  try {
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

// Smart comment toggle
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
  
  // Check if comments need to be added or removed
  for (let i = startLine; i <= endLine; i++) {
    const line = model.getLineContent(i);
    const trimmed = line.trim();
    if (trimmed && !trimmed.startsWith(commentPrefix.trim())) {
      isCommenting = true;
      break;
    }
  }
  
  // Execute comment or uncomment
  for (let i = startLine; i <= endLine; i++) {
    const line = model.getLineContent(i);
    const trimmed = line.trim();
    
    if (trimmed) {
      if (isCommenting) {
        // Add comment
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
        // Remove comment
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

// Input component smart completion
function getInputCompletions(fullText, lineText, range, position) {
  const suggestions = [];
  
  // Special handling: check if it's completion after INPUT.
  const currentWord = getCurrentWord(lineText, position.column);
  if (currentWord.includes('.')) {
    const [prefix, partial] = currentWord.split('.');
    const partialLower = (partial || '').toLowerCase();
    
    if (prefix === 'INPUT') {
      
              if (inputComponents.value.length > 0) {
          // Suggest all INPUT components, but filter out temporary components
          inputComponents.value.forEach(input => {
            if ((!partial || input.id.toLowerCase().includes(partialLower)) && 
                !suggestions.some(s => s.label === input.id) &&
                !input.hasTemp) {  // Filter out temporary components
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
        // If no input components, add a hint
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
    const afterColon = beforeCursor.substring(colonIndex + 1);
    // 冒号后面都算在值位置，统一处理
    context.isInValue = true;
    // 提取键名
    const beforeColon = beforeCursor.substring(0, colonIndex).trim();
    context.currentKey = beforeColon.split(/\s+/).pop() || '';
    // 提取当前值（用于过滤），去除前后空格
    context.currentValue = afterColon.trim();
  } else {
    // 在键位置
    context.isInKey = true;
  }
  
  // 解析当前所在的配置段
  context.parentSections = getYamlSections(lines, currentLineIndex);
  if (context.parentSections.length > 0) {
    context.currentSection = context.parentSections[context.parentSections.length - 1];
    // 添加父配置段信息，用于嵌套配置段的判断
    if (context.parentSections.length > 1) {
      context.parentSection = context.parentSections[context.parentSections.length - 2];
    }
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

    // 使用固定的输入类型列表，确保总是有枚举提示
    const availableInputTypes = [
      { value: 'kafka', description: 'Apache Kafka input source' },
      { value: 'kafka_azure', description: 'Azure Event Hubs (Kafka) input source' },
      { value: 'kafka_aws', description: 'AWS MSK (Kafka) input source' },
      { value: 'aliyun_sls', description: 'Alibaba Cloud SLS input source' }
    ];
    
    // 获取当前已输入的部分，用于过滤
    const currentValue = context.currentValue ? context.currentValue.toLowerCase() : '';
    
    availableInputTypes.forEach((type, index) => {
      // 如果没有输入或者当前类型包含输入的文本
      if (!currentValue || type.value.toLowerCase().includes(currentValue)) {
        if (!suggestions.some(s => s.label === type.value)) {
          // 计算排序权重：完全匹配 > 前缀匹配 > 包含匹配
          let sortWeight = '2'; // 默认包含匹配
          if (currentValue && type.value.toLowerCase() === currentValue) {
            sortWeight = '0'; // 完全匹配
          } else if (currentValue && type.value.toLowerCase().startsWith(currentValue)) {
            sortWeight = '1'; // 前缀匹配
          }
          
          suggestions.push({
            label: type.value,
            kind: monaco.languages.CompletionItemKind.EnumMember,
            documentation: type.description,
            insertText: type.value,
            range: range,
            sortText: `${sortWeight}_${String(index).padStart(2, '0')}_${type.value}`,
            detail: `Input Type: ${type.value}` // 添加详细信息以便区分
          });
        }
      }
    });
  }
  
  // compression属性值补全
  else if (context.currentKey === 'compression') {
    const compressionTypes = ['none', 'gzip', 'snappy', 'lz4', 'zstd'];
    // 获取当前已输入的部分，用于过滤
    const currentValue = context.currentValue ? context.currentValue.toLowerCase() : '';
    
    compressionTypes.forEach(comp => {
      // 如果没有输入或者当前类型包含输入的文本
      if (!currentValue || comp.toLowerCase().includes(currentValue)) {
        if (!suggestions.some(s => s.label === comp)) {
          suggestions.push({
            label: comp,
            kind: monaco.languages.CompletionItemKind.EnumMember,
            documentation: `${comp} compression`,
            insertText: comp,
            range: range,
            sortText: comp.toLowerCase().startsWith(currentValue) ? `0_${comp}` : `1_${comp}` // 前缀匹配优先
          });
        }
      }
    });
  }
  
  // enable属性值补全
  else if (context.currentKey === 'enable') {
    const enableValues = ['true', 'false'];
    const currentValue = context.currentValue ? context.currentValue.toLowerCase() : '';
    
    enableValues.forEach(val => {
      if (!currentValue || val.toLowerCase().includes(currentValue)) {
        suggestions.push({
          label: val,
          kind: monaco.languages.CompletionItemKind.EnumMember,
          documentation: val === 'true' ? 'Enable feature' : 'Disable feature',
          insertText: val,
          range: range,
          sortText: val.toLowerCase().startsWith(currentValue) ? `0_${val}` : `1_${val}` // 前缀匹配优先
        });
      }
    });
  }
  
  // mechanism属性值补全
  else if (context.currentKey === 'mechanism') {
    const mechanisms = ['plain', 'scram-sha-256', 'scram-sha-512'];
    const currentValue = context.currentValue ? context.currentValue.toLowerCase() : '';
    
    mechanisms.forEach(mech => {
      if (!currentValue || mech.toLowerCase().includes(currentValue)) {
        if (!suggestions.some(s => s.label === mech)) {
          suggestions.push({
            label: mech,
            kind: monaco.languages.CompletionItemKind.EnumMember,
            documentation: `SASL ${mech} mechanism`,
            insertText: mech,
            range: range,
            sortText: mech.toLowerCase().startsWith(currentValue) ? `0_${mech}` : `1_${mech}` // 前缀匹配优先
          });
        }
      }
    });
  }
  
  // skip_verify属性值补全
  else if (context.currentKey === 'skip_verify') {
    const skipVerifyValues = ['true', 'false'];
    const currentValue = context.currentValue ? context.currentValue.toLowerCase() : '';
    
    skipVerifyValues.forEach(val => {
      if (!currentValue || val.toLowerCase().includes(currentValue)) {
        suggestions.push({
          label: val,
          kind: monaco.languages.CompletionItemKind.EnumMember,
          documentation: val === 'true' ? 'Skip TLS certificate verification' : 'Enable TLS certificate verification',
          insertText: val,
          range: range,
          sortText: val.toLowerCase().startsWith(currentValue) ? `0_${val}` : `1_${val}` // 前缀匹配优先
        });
      }
    });
  }
  
  // Cursor_position attribute value completion
  else if (context.currentKey === 'cursor_position') {
    const cursorPositions = ['BEGIN_CURSOR', 'END_CURSOR'];
    const currentValue = context.currentValue ? context.currentValue.toLowerCase() : '';
    
    cursorPositions.forEach(pos => {
      if (!currentValue || pos.toLowerCase().includes(currentValue)) {
        suggestions.push({
          label: pos,
          kind: monaco.languages.CompletionItemKind.EnumMember,
          documentation: pos === 'BEGIN_CURSOR' ? 'Start from beginning' : 'Start from end',
          insertText: pos,
          range: range,
          sortText: pos.toLowerCase().startsWith(currentValue) ? `0_${pos}` : `1_${pos}` // 前缀匹配优先
        });
      }
    });
  }
  
  // Suggested endpoint format
  else if (context.currentKey === 'endpoint') {
    suggestions.push({
      label: 'region.log.aliyuncs.com',
      kind: monaco.languages.CompletionItemKind.Snippet,
      documentation: 'Aliyun SLS endpoint format',
      insertText: 'cn-beijing.log.aliyuncs.com',
      range: range
    });
  }
  
  // Array item suggestion - only provides formatting hints
  else if (context.currentKey === 'brokers' || context.beforeCursor.includes('- ')) {
    suggestions.push({
      label: 'broker-address:port',
      kind: monaco.languages.CompletionItemKind.Snippet,
      documentation: 'Kafka broker address format',
      insertText: 'localhost:9092',
      range: range
    });
  }
  
  return { suggestions };
}

// Input键补全
function getInputKeyCompletions(context, range, fullText) {
  let suggestions = [];
  
  // 根级别配置
  if (context.indentLevel === 0) {
    const hasType = fullText.includes('type:');
    if (!hasType) {
      // 'type' should be suggested first when absent
      suggestions.push({
        label: 'type',
        kind: monaco.languages.CompletionItemKind.Property,
        documentation: 'Input source type - choose from: kafka, kafka_azure, kafka_aws, aliyun_sls',
        insertText: 'type:',
        range: range,
        sortText: '000_type'
      });
    }
    
    // Provide corresponding configuration sections based on type
    const typeMatch = fullText.match(/type:\s*(kafka|kafka_azure|kafka_aws|aliyun_sls)/);
    if (typeMatch) {
      const inputType = typeMatch[1];
      
      if (inputType === 'kafka' && !fullText.includes('kafka:')) {
        suggestions.push({
          label: 'kafka',
          kind: monaco.languages.CompletionItemKind.Module,
          documentation: 'Kafka input configuration section',
          insertText: [
            'kafka:',
            '  brokers:',
            '    - "localhost:9092"',
            '  topic: "topic-name"',
            '  group: "group-name"',
            '  compression: "none"'
          ].join('\n'),
          insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
          range: range
        });
      }
      
      if (inputType === 'kafka_azure' && !fullText.includes('kafka:')) {
        suggestions.push({
          label: 'kafka',
          kind: monaco.languages.CompletionItemKind.Module,
          documentation: 'Azure Event Hubs (Kafka) input configuration section',
          insertText: [
            'kafka:',
            '  brokers:',
            '    - "namespace.servicebus.windows.net:9093"',
            '  topic: "topic-name"',
            '  group: "group-name"',
            '  compression: "none"',
            '  sasl:',
            '    enable: true',
            '    mechanism: "plain"',
            '    username: "$ConnectionString"',
            '    password: "Endpoint=sb://namespace.servicebus.windows.net/;SharedAccessKeyName=..."'
          ].join('\n'),
          insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
          range: range
        });
      }
      
      if (inputType === 'kafka_aws' && !fullText.includes('kafka:')) {
        suggestions.push({
          label: 'kafka',
          kind: monaco.languages.CompletionItemKind.Module,
          documentation: 'AWS MSK (Kafka) input configuration section',
          insertText: [
            'kafka:',
            '  brokers:',
            '    - "b-1.cluster.kafka.region.amazonaws.com:9092"',
            '  topic: "topic-name"',
            '  group: "group-name"',
            '  compression: "none"',
            '  sasl:',
            '    enable: true',
            '    mechanism: "scram-sha-512"',
            '    username: "username"',
            '    password: "password"'
          ].join('\n'),
          insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
          range: range
        });
      }
      
      if (inputType === 'aliyun_sls' && !fullText.includes('aliyun_sls:')) {
        suggestions.push({
          label: 'aliyun_sls',
          kind: monaco.languages.CompletionItemKind.Module,
          documentation: 'Aliyun SLS input configuration section',
          insertText: 'aliyun_sls:\n  endpoint: "cn-beijing.log.aliyuncs.com"\n  access_key_id: "YOUR_ACCESS_KEY_ID"\n  access_key_secret: "YOUR_ACCESS_KEY_SECRET"\n  project: "project-name"\n  logstore: "logstore-name"\n  consumer_group_name: "consumer-group"\n  consumer_name: "consumer-name"\n  cursor_position: "BEGIN_CURSOR"\n  query: "*"',
          insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
          range: range
        });
      }
    }
  }
  
  // Kafka配置段内部 (supports all kafka types)
  else if (context.currentSection === 'kafka') {
    const kafkaKeys = [
      { key: 'brokers', desc: 'Kafka broker addresses' },
      { key: 'topic', desc: 'Kafka topic name' },
      { key: 'group', desc: 'Consumer group name' },
      { key: 'compression', desc: 'Message compression type' },
      { key: 'sasl', desc: 'SASL authentication configuration' },
      { key: 'tls', desc: 'TLS configuration' }
    ];
    
    kafkaKeys.forEach(item => {
      if (!fullText.includes(`${item.key}:`) && !suggestions.some(s => s.label === item.key)) {
        suggestions.push({
          label: item.key,
          kind: monaco.languages.CompletionItemKind.Property,
          documentation: item.desc,
          insertText: `${item.key}:`,
          range: range
        });
      }
    });
  }
  
  // TLS配置段内部
  else if (context.currentSection === 'tls') {
    const tlsKeys = [
      { key: 'cert_path', desc: 'Path to client certificate file' },
      { key: 'key_path', desc: 'Path to client private key file' },
      { key: 'ca_file_path', desc: 'Path to CA certificate file' },
      { key: 'skip_verify', desc: 'Skip TLS verification' }
    ];
    
    tlsKeys.forEach(item => {
      if (!fullText.includes(`${item.key}:`) && !suggestions.some(s => s.label === item.key)) {
        suggestions.push({
          label: item.key,
          kind: monaco.languages.CompletionItemKind.Property,
          documentation: item.desc,
          insertText: `${item.key}:`,
          range: range
        });
      }
    });
  }
  
  // SASL配置段内部
  else if (context.currentSection === 'sasl') {
    const saslKeys = [
      { key: 'enable', desc: 'Enable SASL authentication' },
      { key: 'mechanism', desc: 'SASL mechanism' },
      { key: 'username', desc: 'SASL username' },
      { key: 'password', desc: 'SASL password' }
    ];
    
    saslKeys.forEach(item => {
      if (!fullText.includes(`${item.key}:`) && !suggestions.some(s => s.label === item.key)) {
        suggestions.push({
          label: item.key,
          kind: monaco.languages.CompletionItemKind.Property,
          documentation: item.desc,
          insertText: `${item.key}:`,
          range: range
        });
      }
    });
  }
  
  // Aliyun SLS配置段内部
  else if (context.currentSection === 'aliyun_sls') {
    const slsKeys = [
      { key: 'endpoint', desc: 'SLS service endpoint' },
      { key: 'access_key_id', desc: 'Access key ID' },
      { key: 'access_key_secret', desc: 'Access key secret' },
      { key: 'project', desc: 'SLS project name' },
      { key: 'logstore', desc: 'SLS logstore name' },
      { key: 'consumer_group_name', desc: 'Consumer group name' },
      { key: 'consumer_name', desc: 'Consumer name' },
      { key: 'cursor_position', desc: 'Cursor start position' },
      { key: 'cursor_start_time', desc: 'Unix timestamp (ms) for start cursor' },
      { key: 'query', desc: 'Log query filter' }
    ];
    
    slsKeys.forEach(item => {
      if (!fullText.includes(`${item.key}:`) && !suggestions.some(s => s.label === item.key)) {
        suggestions.push({
          label: item.key,
          kind: monaco.languages.CompletionItemKind.Property,
          documentation: item.desc,
          insertText: `${item.key}:`,
          range: range
        });
      }
    });
  }
  
  // Clean root-level meta suggestions if type already present
  if (context.indentLevel === 0 && fullText.includes('type:')) {
    suggestions = suggestions.filter(item => item.label !== 'name' && item.label !== 'enable');
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
          '    - "localhost:9092"',
          '  topic: "topic-name"',
          '  group: "consumer-group"',
          '  compression: "none"'
        ].join('\n'),
        insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
        range: range
      },
      {
        label: 'Azure Event Hubs Input Template',
        kind: monaco.languages.CompletionItemKind.Snippet,
        documentation: 'Complete Azure Event Hubs (Kafka) input configuration',
        insertText: [
          'type: kafka_azure',
          'kafka:',
          '  brokers:',
          '    - "namespace.servicebus.windows.net:9093"',
          '  topic: "topic-name"',
          '  group: "consumer-group"',
          '  compression: "none"',
          '  sasl:',
          '    enable: true',
          '    mechanism: "plain"',
          '    username: "$ConnectionString"',
          '    password: "Endpoint=sb://namespace.servicebus.windows.net/;SharedAccessKeyName=..."'
        ].join('\n'),
        insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
        range: range
      },
      {
        label: 'AWS MSK Input Template',
        kind: monaco.languages.CompletionItemKind.Snippet,
        documentation: 'Complete AWS MSK (Kafka) input configuration',
        insertText: [
          'type: kafka_aws',
          'kafka:',
          '  brokers:',
          '    - "b-1.cluster.kafka.region.amazonaws.com:9092"',
          '  topic: "topic-name"',
          '  group: "consumer-group"',
          '  compression: "none"',
          '  sasl:',
          '    enable: true',
          '    mechanism: "scram-sha-512"',
          '    username: "username"',
          '    password: "password"'
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
          '  endpoint: "cn-beijing.log.aliyuncs.com"',
          '  access_key_id: "YOUR_ACCESS_KEY_ID"',
          '  access_key_secret: "YOUR_ACCESS_KEY_SECRET"',
          '  project: "project-name"',
          '  logstore: "logstore-name"',
          '  consumer_group_name: "consumer-group"',
          '  consumer_name: "consumer-name"'
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
      // 提示所有OUTPUT组件，但过滤掉临时组件
      outputComponents.value.forEach(output => {
        if ((!partial || output.id.toLowerCase().includes(partialLower)) && 
            !suggestions.some(s => s.label === output.id) &&
            !output.hasTemp) {  // 过滤掉临时组件
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

    // 使用固定的输出类型列表，确保总是有枚举提示
    const availableOutputTypes = [
      { value: 'kafka', description: 'Apache Kafka output destination' },
      { value: 'kafka_azure', description: 'Azure Event Hubs (Kafka) output' },
      { value: 'kafka_aws', description: 'AWS MSK (Kafka) output' },
      { value: 'elasticsearch', description: 'Elasticsearch output destination' },
      { value: 'aliyun_sls', description: 'Alibaba Cloud SLS output destination' },
      { value: 'print', description: 'Console print output for debugging' }
    ];
    
    // 获取当前已输入的部分，用于过滤
    const currentValue = context.currentValue ? context.currentValue.toLowerCase() : '';
    
    availableOutputTypes.forEach((type, index) => {
      // 如果没有输入或者当前类型包含输入的文本
      if (!currentValue || type.value.toLowerCase().includes(currentValue)) {
        if (!suggestions.some(s => s.label === type.value)) {
          // 计算排序权重：完全匹配 > 前缀匹配 > 包含匹配
          let sortWeight = '2'; // 默认包含匹配
          if (currentValue && type.value.toLowerCase() === currentValue) {
            sortWeight = '0'; // 完全匹配
          } else if (currentValue && type.value.toLowerCase().startsWith(currentValue)) {
            sortWeight = '1'; // 前缀匹配
          }
          
          suggestions.push({
            label: type.value,
            kind: monaco.languages.CompletionItemKind.EnumMember,
            documentation: type.description,
            insertText: type.value,
            range: range,
            sortText: `${sortWeight}_${String(index).padStart(2, '0')}_${type.value}`,
            detail: `Output Type: ${type.value}` // 添加详细信息以便区分
          });
        }
      }
    });
  }
  
  // compression属性值补全
  else if (context.currentKey === 'compression') {
    const compressionTypes = ['none', 'gzip', 'snappy', 'lz4', 'zstd'];
    // 获取当前已输入的部分，用于过滤
    const currentValue = context.currentValue ? context.currentValue.toLowerCase() : '';
    
    compressionTypes.forEach(comp => {
      // 如果没有输入或者当前类型包含输入的文本
      if (!currentValue || comp.toLowerCase().includes(currentValue)) {
        if (!suggestions.some(s => s.label === comp)) {
          suggestions.push({
            label: comp,
            kind: monaco.languages.CompletionItemKind.EnumMember,
            documentation: `${comp} compression`,
            insertText: comp,
            range: range,
            sortText: comp.toLowerCase().startsWith(currentValue) ? `0_${comp}` : `1_${comp}` // 前缀匹配优先
          });
        }
      }
    });
  }
  
  // endpoint格式建议
  else if (context.currentKey === 'endpoint') {
    suggestions.push({
      label: 'region.log.aliyuncs.com',
      kind: monaco.languages.CompletionItemKind.Snippet,
      documentation: 'Aliyun SLS endpoint format',
      insertText: 'cn-beijing.log.aliyuncs.com',
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
        insertText: 'http://localhost:9200',
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

  // ES auth type属性值补全
  else if (context.currentKey === 'type' && context.currentSection === 'auth') {
    const authTypes = ['basic', 'api_key', 'bearer'];
    const currentValue = context.currentValue ? context.currentValue.toLowerCase() : '';
    
    authTypes.forEach(type => {
      if (!currentValue || type.toLowerCase().includes(currentValue)) {
        if (!suggestions.some(s => s.label === type)) {
          let description = '';
          switch (type) {
            case 'basic':
              description = 'Basic authentication with username/password';
              break;
            case 'api_key':
              description = 'API key authentication';
              break;
            case 'bearer':
              description = 'Bearer token authentication';
              break;
          }
          
          suggestions.push({
            label: type,
            kind: monaco.languages.CompletionItemKind.EnumMember,
            documentation: description,
            insertText: type,
            range: range,
            sortText: type.toLowerCase().startsWith(currentValue) ? `0_${type}` : `1_${type}` // 前缀匹配优先
          });
        }
      }
    });
  }
  
  // skip_verify属性值补全
  else if (context.currentKey === 'skip_verify') {
    const skipVerifyValues = ['true', 'false'];
    const currentValue = context.currentValue ? context.currentValue.toLowerCase() : '';
    
    skipVerifyValues.forEach(val => {
      if (!currentValue || val.toLowerCase().includes(currentValue)) {
        suggestions.push({
          label: val,
          kind: monaco.languages.CompletionItemKind.EnumMember,
          documentation: val === 'true' ? 'Skip TLS certificate verification' : 'Enable TLS certificate verification',
          insertText: val,
          range: range,
          sortText: val.toLowerCase().startsWith(currentValue) ? `0_${val}` : `1_${val}` // 前缀匹配优先
        });
      }
    });
  }
  
  return { suggestions };
}

// Output键补全
function getOutputKeyCompletions(context, range, fullText) {
  let suggestions = [];
  
  // 根级别配置
  if (context.indentLevel === 0) {
    const hasType = fullText.includes('type:');
    if (!hasType) {
      suggestions.push({
        label: 'type',
        kind: monaco.languages.CompletionItemKind.Property,
        documentation: 'Output destination type - choose from: kafka, kafka_azure, kafka_aws, elasticsearch, aliyun_sls, print',
        insertText: 'type:',
        range: range,
        sortText: '000_type'
      });
    }
    
    // 根据type提供相应的配置段
    const typeMatch = fullText.match(/type:\s*(kafka|kafka_azure|kafka_aws|elasticsearch|aliyun_sls|print)/);
    if (typeMatch) {
      const outputType = typeMatch[1];
      
      if (outputType === 'kafka' && !fullText.includes('kafka:')) {
        suggestions.push({
          label: 'kafka',
          kind: monaco.languages.CompletionItemKind.Module,
          documentation: 'Kafka output configuration section',
          insertText: 'kafka:\n  brokers:\n    - "localhost:9092"\n  topic: "topic-name"\n  compression: "none"',
          insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
          range: range
        });
      }
      
      if (outputType === 'kafka_azure' && !fullText.includes('kafka:')) {
        suggestions.push({
          label: 'kafka',
          kind: monaco.languages.CompletionItemKind.Module,
          documentation: 'Azure Event Hubs (Kafka) output configuration section',
          insertText: [
            'kafka:',
            '  brokers:',
            '    - "namespace.servicebus.windows.net:9093"',
            '  topic: "topic-name"',
            '  key: "key-field"',
            '  compression: "none"',
            '  sasl:',
            '    enable: true',
            '    mechanism: "plain"',
            '    username: "$ConnectionString"',
            '    password: "Endpoint=sb://namespace.servicebus.windows.net/;SharedAccessKeyName=..."'
          ].join('\n'),
          insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
          range: range
        });
      }
      
      if (outputType === 'kafka_aws' && !fullText.includes('kafka:')) {
        suggestions.push({
          label: 'kafka',
          kind: monaco.languages.CompletionItemKind.Module,
          documentation: 'AWS MSK (Kafka) output configuration section',
          insertText: [
            'kafka:',
            '  brokers:',
            '    - "b-1.cluster.kafka.region.amazonaws.com:9092"',
            '  topic: "topic-name"',
            '  key: "key-field"',
            '  compression: "none"',
            '  sasl:',
            '    enable: true',
            '    mechanism: "scram-sha-512"',
            '    username: "username"',
            '    password: "password"'
          ].join('\n'),
          insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
          range: range
        });
      }
      
      if (outputType === 'elasticsearch' && !fullText.includes('elasticsearch:')) {
        suggestions.push({
          label: 'elasticsearch',
          kind: monaco.languages.CompletionItemKind.Module,
          documentation: 'Elasticsearch output configuration section with auth example',
          insertText: [
            'elasticsearch:',
            '  hosts:',
            '    - "https://localhost:9200"',
            '  index: "index-name"',
            '  batch_size: 1000',
            '  flush_dur: "5s"',
            '  # auth:',
            '  #   type: basic',
            '  #   username: "elastic"',
            '  #   password: "password"'
          ].join('\n'),
          insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
          range: range
        });
      }
      
      if (outputType === 'aliyun_sls' && !fullText.includes('aliyun_sls:')) {
        suggestions.push({
          label: 'aliyun_sls',
          kind: monaco.languages.CompletionItemKind.Module,
          documentation: 'Aliyun SLS output configuration section',
          insertText: [
            'aliyun_sls:',
            '  endpoint: "cn-beijing.log.aliyuncs.com"',
            '  access_key_id: "YOUR_ACCESS_KEY_ID"',
            '  access_key_secret: "YOUR_ACCESS_KEY_SECRET"',
            '  project: "project-name"',
            '  logstore: "logstore-name"'
          ].join('\n'),
          insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
          range: range
        });
      }
    }
  }
  
  // Kafka配置段内部 (supports all kafka types)
  else if (context.currentSection === 'kafka') {
    const kafkaKeys = [
      { key: 'brokers', desc: 'Kafka broker addresses' },
      { key: 'topic', desc: 'Kafka topic name' },
      { key: 'key', desc: 'Partition key for Kafka messages' },
      { key: 'compression', desc: 'Message compression type' },
      { key: 'sasl', desc: 'SASL authentication configuration' },
      { key: 'tls', desc: 'TLS configuration' }
    ];
    
    kafkaKeys.forEach(item => {
      if (!fullText.includes(`${item.key}:`) && !suggestions.some(s => s.label === item.key)) {
        suggestions.push({
          label: item.key,
          kind: monaco.languages.CompletionItemKind.Property,
          documentation: item.desc,
          insertText: `${item.key}:`,
          range: range
        });
      }
    });
  }
  
  // Elasticsearch配置段内部
  else if (context.currentSection === 'elasticsearch') {
    const esKeys = [
      { key: 'hosts', desc: 'Elasticsearch cluster hosts' },
      { key: 'index', desc: 'Elasticsearch index name' },
      { key: 'batch_size', desc: 'Batch size for bulk operations' },
      { key: 'flush_dur', desc: 'Flush duration for batching' },
      { key: 'auth', desc: 'Authentication configuration (basic, api_key, bearer)' }
    ];
    
    esKeys.forEach(item => {
      if (!fullText.includes(`${item.key}:`) && !suggestions.some(s => s.label === item.key)) {
        suggestions.push({
          label: item.key,
          kind: monaco.languages.CompletionItemKind.Property,
          documentation: item.desc,
          insertText: `${item.key}:`,
          range: range
        });
      }
    });
  }
  
  // ES Auth配置段内部
  else if (context.currentSection === 'auth' && context.parentSection === 'elasticsearch') {
    const authKeys = [
      { key: 'type', desc: 'Authentication type: basic, api_key, bearer' },
      { key: 'username', desc: 'Username for basic authentication' },
      { key: 'password', desc: 'Password for basic authentication' },
      { key: 'api_key', desc: 'API key for api_key authentication' },
      { key: 'token', desc: 'Bearer token for bearer authentication' }
    ];
    
    authKeys.forEach(item => {
      if (!fullText.includes(`${item.key}:`) && !suggestions.some(s => s.label === item.key)) {
        suggestions.push({
          label: item.key,
          kind: monaco.languages.CompletionItemKind.Property,
          documentation: item.desc,
          insertText: `${item.key}:`,
          range: range
        });
      }
    });
  }
  
  // TLS配置段内部
  else if (context.currentSection === 'tls') {
    const tlsKeys = [
      { key: 'cert_path', desc: 'Path to client certificate file' },
      { key: 'key_path', desc: 'Path to client private key file' },
      { key: 'ca_file_path', desc: 'Path to CA certificate file' },
      { key: 'skip_verify', desc: 'Skip TLS verification' }
    ];
    
    tlsKeys.forEach(item => {
      if (!fullText.includes(`${item.key}:`) && !suggestions.some(s => s.label === item.key)) {
        suggestions.push({
          label: item.key,
          kind: monaco.languages.CompletionItemKind.Property,
          documentation: item.desc,
          insertText: `${item.key}:`,
          range: range
        });
      }
    });
  }
  
  // Aliyun SLS配置段内部
  else if (context.currentSection === 'aliyun_sls') {
    const slsKeys = [
      { key: 'endpoint', desc: 'SLS service endpoint' },
      { key: 'access_key_id', desc: 'Access key ID' },
      { key: 'access_key_secret', desc: 'Access key secret' },
      { key: 'project', desc: 'SLS project name' },
      { key: 'logstore', desc: 'SLS logstore name' }
    ];
    
    slsKeys.forEach(item => {
      if (!fullText.includes(`${item.key}:`) && !suggestions.some(s => s.label === item.key)) {
        suggestions.push({
          label: item.key,
          kind: monaco.languages.CompletionItemKind.Property,
          documentation: item.desc,
          insertText: `${item.key}:`,
          range: range
        });
      }
    });
  }
  
  // Remove name/enable suggestions (not needed)
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
          'kafka:',
          '  brokers:',
          '    - "localhost:9092"',
          '  topic: "topic-name"',
          '  key: "key-field"',
          '  compression: "none"'
        ].join('\n'),
        insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
        range: range
      },
      {
        label: 'Elasticsearch Output Template',
        kind: monaco.languages.CompletionItemKind.Snippet,
        documentation: 'Complete Elasticsearch output configuration with auth example',
        insertText: [
          'type: elasticsearch',
          'elasticsearch:',
          '  hosts:',
          '    - "http://localhost:9200"',
          '  index: "index-name"',
          '  batch_size: 1000',
          '  flush_dur: "5s"',
          '  # Uncomment below for authentication',
          '  # auth:',
          '  #   type: basic  # or api_key, bearer',
          '  #   username: "elastic"',
          '  #   password: "password"'
        ].join('\n'),
        insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
        range: range
      },
      {
        label: 'Azure Event Hubs Output Template',
        kind: monaco.languages.CompletionItemKind.Snippet,
        documentation: 'Complete Azure Event Hubs (Kafka) output configuration',
        insertText: [
          'type: kafka_azure',
          'kafka:',
          '  brokers:',
          '    - "namespace.servicebus.windows.net:9093"',
          '  topic: "topic-name"',
          '  key: "key-field"',
          '  compression: "none"',
          '  sasl:',
          '    enable: true',
          '    mechanism: "plain"',
          '    username: "$ConnectionString"',
          '    password: "Endpoint=sb://namespace.servicebus.windows.net/;SharedAccessKeyName=..."'
        ].join('\n'),
        insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
        range: range
      },
      {
        label: 'AWS MSK Output Template',
        kind: monaco.languages.CompletionItemKind.Snippet,
        documentation: 'Complete AWS MSK (Kafka) output configuration',
        insertText: [
          'type: kafka_aws',
          'kafka:',
          '  brokers:',
          '    - "b-1.cluster.kafka.region.amazonaws.com:9092"',
          '  topic: "topic-name"',
          '  key: "key-field"',
          '  compression: "none"',
          '  sasl:',
          '    enable: true',
          '    mechanism: "scram-sha-512"',
          '    username: "username"',
          '    password: "password"'
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
          'aliyun_sls:',
          '  endpoint: "cn-beijing.log.aliyuncs.com"',
          '  access_key_id: "YOUR_ACCESS_KEY_ID"',
          '  access_key_secret: "YOUR_ACCESS_KEY_SECRET"',
          '  project: "project-name"',
          '  logstore: "logstore-name"'
        ].join('\n'),
        insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
        range: range
      },
      {
        label: 'Print Output Template',
        kind: monaco.languages.CompletionItemKind.Snippet,
        documentation: 'Simple print output for debugging',
        insertText: 'type: print',
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
        '  INPUT.input-name -> OUTPUT.output-name'
      ].join('\n'),
      insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
      range: range
    });
  }
  
  const result = { suggestions };
  return result;
}



// Project flow completions
function getProjectFlowCompletions(fullText, lineText, range, position) {
  
  const suggestions = [];
  
  // Get the word at current cursor position
  const currentWord = getCurrentWord(lineText, position.column);
  

  
  // Detect current input context
  if (currentWord.includes('.')) {
    // User has already entered a prefix, such as "INPUT.", "OUTPUT.", "RULESET."
    const [prefix, partial] = currentWord.split('.');
    const partialLower = (partial || '').toLowerCase();
    
    // When a specific prefix is detected, only process suggestions for that prefix, don't add other prefix suggestions
    

    
                if (prefix === 'INPUT') {
        // Calculate the correct range, only replace the part after the dot
        const dotIndex = currentWord.indexOf('.');
        const replaceRange = {
          startLineNumber: position.lineNumber,
          endLineNumber: position.lineNumber,
          startColumn: position.column - (currentWord.length - dotIndex - 1),
          endColumn: position.column
        };
        
        if (inputComponents.value.length > 0) {
          // Suggest all INPUT components, but filter out temporary ones
          inputComponents.value.forEach(input => {
            if ((!partial || input.id.toLowerCase().includes(partialLower)) && 
                !suggestions.some(s => s.label === input.id) &&
                !input.hasTemp) {  // Filter out temporary components
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
          // If no input components, add a hint
          suggestions.push({
            label: 'No input components available',
            kind: monaco.languages.CompletionItemKind.Text,
            documentation: 'No input components found. Please create input components first.',
            insertText: '',
            range: replaceRange
          });
        }
        
        // After processing INPUT components, return directly without processing other logic
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
        

        
        if (rulesetComponents.value.length > 0) {
          // Suggest all RULESET components, but filter out temporary ones
          rulesetComponents.value.forEach(ruleset => {
            if ((!partial || ruleset.id.toLowerCase().includes(partialLower)) && 
                !suggestions.some(s => s.label === ruleset.id) &&
                !ruleset.hasTemp) {  // Filter out temporary components
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
          // If no ruleset components, add a hint
          suggestions.push({
            label: 'No ruleset components available',
            kind: monaco.languages.CompletionItemKind.Text,
            documentation: 'No ruleset components found. Please create ruleset components first.',
            insertText: '',
            range: replaceRange
          });
        }
        
        // After processing RULESET components, return directly
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
      
      // Suggest all OUTPUT components, but filter out temporary ones
      outputComponents.value.forEach(output => {
        const matches = !partial || output.id.toLowerCase().includes(partialLower);
        const alreadyExists = suggestions.some(s => s.label === output.id);
        const isNotTemp = !output.hasTemp;  // Filter out temporary components
        
        if (matches && !alreadyExists && isNotTemp) {
          suggestions.push({
            label: output.id,
            kind: monaco.languages.CompletionItemKind.Reference,
            documentation: `Output component: ${output.id}`,
            insertText: output.id,
            range: replaceRange
          });
        }
      });
      
      // After processing OUTPUT components, return directly
      return { suggestions };
    }
    
    // If no matching prefix, return empty suggestions
    return { suggestions: [] };
    
  } else {
    // User hasn't entered a prefix yet, provide prefix suggestions based on context
    const suggestionsMap = new Map();

    // Manage all prefix suggestions uniformly, ensure no duplicates
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

    // Determine which suggestions to provide based on context
    const arrowIndex = lineText.lastIndexOf('->');
    const isAfterArrow = arrowIndex !== -1 && position.column > arrowIndex + 2;

    if (isAfterArrow) {
      // After arrow: can only be RULESET or OUTPUT
      addSuggestion('RULESET', monaco.languages.CompletionItemKind.Module, 'Ruleset component reference', 'RULESET');
      addSuggestion('OUTPUT', monaco.languages.CompletionItemKind.Module, 'Output component reference', 'OUTPUT');
    } else {
      // Before arrow or new line: can be INPUT, RULESET, OUTPUT
      addSuggestion('INPUT', monaco.languages.CompletionItemKind.Module, 'Input component reference', 'INPUT');
      addSuggestion('RULESET', monaco.languages.CompletionItemKind.Module, 'Ruleset component reference', 'RULESET');
      addSuggestion('OUTPUT', monaco.languages.CompletionItemKind.Module, 'Output component reference', 'OUTPUT');
    }

    // Convert Map suggestions to array
    suggestions.push(...Array.from(suggestionsMap.values()));
    
    // Check if arrow operator should be suggested
    // Only when line has complete component reference (like INPUT.demo, RULESET.test) and not after arrow
    const hasCompleteComponentRef = /\b(INPUT|RULESET|OUTPUT)\.\w+/.test(lineText);
    if (!isAfterArrow && hasCompleteComponentRef) {
      suggestions.push({
        label: '->',
        kind: monaco.languages.CompletionItemKind.Operator,
        documentation: 'Flow operator',
        insertText: '-> ',
        range: range
      });
    }
  }
  
  // Final deduplication
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
  
  // Find word boundaries, special handling for component reference format (like INPUT.component_name)
  const wordStart = Math.max(
    beforeCursor.lastIndexOf(' '),
    beforeCursor.lastIndexOf('\t'),
    beforeCursor.lastIndexOf('|'),
    beforeCursor.lastIndexOf('>'),
    beforeCursor.lastIndexOf('-'),
    0  // Ensure it's not negative
  ) + 1;
  
  // For afterCursor, need to find the next separator, but preserve complete component reference
  const wordEnd = afterCursor.search(/[\s\t|>-]/) === -1 ? afterCursor.length : afterCursor.search(/[\s\t|>-]/);
  
  const word = beforeCursor.substring(wordStart) + afterCursor.substring(0, wordEnd);
  
  return word;
}

// Ruleset XML intelligent completions
function getRulesetXmlCompletions(fullText, lineText, range, position) {
  // Parse current XML context
  const context = parseXmlContext(fullText, position.lineNumber, position.column);
  
  // Provide accurate completions based on different contexts, avoid duplicates
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
    // Default case - return empty suggestions
    result = { suggestions: [] };
  }
  
  return result;
}

// Parse XML context - 超级简化版本
function parseXmlContext(fullText, lineNumber, column) {
  const lines = fullText.split('\n');
  const currentLine = lines[lineNumber - 1] || '';
  const beforeCursor = currentLine.substring(0, column - 1);
  
  const context = {
    currentLine,
    beforeCursor,
    isInAttributeName: false,
    isInAttributeValue: false,
    isInTagName: false,
    isInTagContent: false,
    currentTag: '',
    currentAttribute: '',
    parentTags: []
  };
  
  // 1. 检查当前行开头的标签名
  const lineTagMatch = currentLine.match(/^\s*<(\w+)/);
  if (lineTagMatch) {
    context.currentTag = lineTagMatch[1];
  }
  
  // 2. 检查光标是否在引号内（属性值）
  let inQuotes = false;
  let quoteChar = '';
  let attributeName = '';
  
  // 简单计算：从行开始到光标位置，计算引号数量
  for (let i = 0; i < beforeCursor.length; i++) {
    const char = beforeCursor[i];
    if ((char === '"' || char === "'") && !inQuotes) {
      // 开始引号
      inQuotes = true;
      quoteChar = char;
      // 往前找属性名
      const beforeQuote = beforeCursor.substring(0, i);
      const attrMatch = beforeQuote.match(/(\w+)\s*=\s*$/);
      if (attrMatch) {
        attributeName = attrMatch[1];
      }
    } else if (char === quoteChar && inQuotes) {
      // 结束引号
      inQuotes = false;
      quoteChar = '';
      attributeName = '';
    }
  }
  
  if (inQuotes) {
    // 在引号内 = 属性值
    context.isInAttributeValue = true;
    context.currentAttribute = attributeName;
  } else {
    // 不在引号内
    // 检查是否在输入标签名
    if (beforeCursor.match(/<[a-zA-Z]*$/)) {
      // 光标前是 < 加可选的字母（可能正在输入标签名）
      context.isInTagName = true;
    } else if (currentLine.includes('<') && !currentLine.includes('>')) {
      // 在未完成的标签内
      if (beforeCursor.includes(' ')) {
        // 有空格，说明在属性区域
        context.isInAttributeName = true;
      } else {
        // 没有空格，还在标签名
        context.isInTagName = true;
      }
    } else {
      // 在标签内容中
      context.isInTagContent = true;
    }
  }
  
  // 获取父标签
  const beforeLines = lines.slice(0, lineNumber - 1).join('\n') + '\n' + beforeCursor;
  context.parentTags = getParentTags(beforeLines);
  
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
  
  // check或node标签的type属性（支持新的check语法）
  if ((context.currentTag === 'check' || context.currentTag === 'node') && context.currentAttribute === 'type') {
    const checkTypes = [
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
    
    checkTypes.forEach(type => {
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
  
  // check或node标签的logic属性
  else if ((context.currentTag === 'check' || context.currentTag === 'node') && context.currentAttribute === 'logic') {
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
        { label: 'WHITELIST', kind: monaco.languages.CompletionItemKind.EnumMember, documentation: 'Whitelist ruleset type', insertText: 'WHITELIST', range: range }
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
  
  // Field name suggestions (common fields + dynamic fields from sample data)
  // Handle multiple field-related attributes
  else if (context.currentAttribute === 'field' || 
           context.currentAttribute === 'count_field') {
    // Add dynamic fields from sample data first (higher priority)
    if (dynamicFieldKeys.value && dynamicFieldKeys.value.length > 0) {
      dynamicFieldKeys.value.forEach(field => {
        if (!suggestions.some(s => s.label === field)) {
          suggestions.push({
            label: field,
            kind: monaco.languages.CompletionItemKind.Field,
            documentation: `Sample data field: ${field}`,
            insertText: field,
            range: range,
            sortText: `0_${field}` // Higher priority than common fields
          });
        }
      });
    }
    
  }
  
  // group_by attribute - supports comma-separated field lists
  else if (context.currentAttribute === 'group_by') {
    // Add individual fields
    if (dynamicFieldKeys.value && dynamicFieldKeys.value.length > 0) {
      dynamicFieldKeys.value.forEach(field => {
        if (!suggestions.some(s => s.label === field)) {
          suggestions.push({
            label: field,
            kind: monaco.languages.CompletionItemKind.Field,
            documentation: `Sample data field: ${field}`,
            insertText: field,
            range: range,
            sortText: `0_${field}` // Higher priority than common fields
          });
        }
      });
      
      // Add common field combinations
      const topFields = dynamicFieldKeys.value.slice(0, 3);
      if (topFields.length >= 2) {
        suggestions.push({
          label: topFields.slice(0, 2).join(','),
          kind: monaco.languages.CompletionItemKind.Snippet,
          documentation: 'Group by top 2 fields from sample data',
          insertText: topFields.slice(0, 2).join(','),
          range: range,
          sortText: '0_combo_2'
        });
      }
      if (topFields.length >= 3) {
        suggestions.push({
          label: topFields.join(','),
          kind: monaco.languages.CompletionItemKind.Snippet,
          documentation: 'Group by top 3 fields from sample data',
          insertText: topFields.join(','),
          range: range,
          sortText: '0_combo_3'
        });
      }
    }
  }
  
  return { suggestions };
}

// 属性名补全
function getXmlAttributeNameCompletions(context, range) {
  const suggestions = [];
  
  switch (context.currentTag) {
    case 'root':
      suggestions.push(
        { label: 'type', kind: monaco.languages.CompletionItemKind.Property, documentation: 'Ruleset type', insertText: 'type="WHITELIST"', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range },
        { label: 'name', kind: monaco.languages.CompletionItemKind.Property, documentation: 'Ruleset name', insertText: 'name="ruleset-name"', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range },
        { label: 'author', kind: monaco.languages.CompletionItemKind.Property, documentation: 'Ruleset author', insertText: 'author="name"', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range }
      );
      break;
      
    case 'rule':
      suggestions.push(
        { label: 'id', kind: monaco.languages.CompletionItemKind.Property, documentation: 'Unique rule identifier', insertText: 'id="rule-id"', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range },
        { label: 'name', kind: monaco.languages.CompletionItemKind.Property, documentation: 'Rule display name', insertText: 'name="rule-name"', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range },

      );
      break;
      
    case 'check':
    case 'node':
      // Generate smart field suggestion with available fields  
      let checkFieldTemplate = 'field="field-name"';
      if (dynamicFieldKeys.value && dynamicFieldKeys.value.length > 0) {
        checkFieldTemplate = `field="${dynamicFieldKeys.value[0]}"`;
      }
      
      const checkAttrs = [
        { label: 'type', kind: monaco.languages.CompletionItemKind.Property, documentation: 'Check type', insertText: 'type="EQU"', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range },
        { label: 'field', kind: monaco.languages.CompletionItemKind.Property, documentation: 'Field to check', insertText: checkFieldTemplate, insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range },
        { label: 'logic', kind: monaco.languages.CompletionItemKind.Property, documentation: 'Logical operation for multiple values', insertText: 'logic="OR"', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range },
        { label: 'delimiter', kind: monaco.languages.CompletionItemKind.Property, documentation: 'Delimiter for multiple values', insertText: 'delimiter="|"', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range }
      ];
      
      // 在checklist内部的check节点需要id属性
      if (context.parentTags.includes('checklist')) {
        checkAttrs.unshift({ label: 'id', kind: monaco.languages.CompletionItemKind.Property, documentation: 'Node identifier for conditions (required in checklist)', insertText: 'id="node-id"', range: range });
      }
      
      suggestions.push(...checkAttrs);
      break;
      
    case 'checklist':
      suggestions.push(
        { label: 'condition', kind: monaco.languages.CompletionItemKind.Property, documentation: 'Logical condition using node IDs', insertText: 'condition="a and b"', range: range }
      );
      break;
      
    case 'threshold':
      // Generate smart group_by suggestion with available fields
      let groupByTemplate = 'group_by="field1"';
      if (dynamicFieldKeys.value && dynamicFieldKeys.value.length > 0) {
        const topFields = dynamicFieldKeys.value.slice(0, 3).join(',');
        groupByTemplate = `group_by="${topFields}"`;
      }
      
      // Generate smart count_field suggestion with available fields  
      let countFieldTemplate = 'count_field="field"';
      if (dynamicFieldKeys.value && dynamicFieldKeys.value.length > 0) {
        countFieldTemplate = `count_field="${dynamicFieldKeys.value[0]}"`;
      }
      
      suggestions.push(
        { label: 'group_by', kind: monaco.languages.CompletionItemKind.Property, documentation: 'Fields to group by', insertText: groupByTemplate, insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range },
        { label: 'range', kind: monaco.languages.CompletionItemKind.Property, documentation: 'Time range for aggregation', insertText: 'range="5m"', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range },
        { label: 'count_type', kind: monaco.languages.CompletionItemKind.Property, documentation: 'Counting method', insertText: 'count_type="CLASSIFY"', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range },
        { label: 'count_field', kind: monaco.languages.CompletionItemKind.Property, documentation: 'Field to count', insertText: countFieldTemplate, insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range },
        { label: 'local_cache', kind: monaco.languages.CompletionItemKind.Property, documentation: 'Use local cache', insertText: 'local_cache="true"', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range }
      );
      break;
      
    case 'append':
      suggestions.push(
        { label: 'field', kind: monaco.languages.CompletionItemKind.Property, documentation: 'Name of field to append', insertText: 'field="field-name"', insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet, range: range },
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
        insertText: 'root author="name">\n' +
            '    <rule id="rule_id">\n' +
            '        <!-- Operations can be in any order -->\n' +
            '        <check type="EQU" field="status">active</check>\n' +
            '    \n' +
            '        <threshold group_by="user_id" range="5m">10</threshold>\n' +
            '    \n' +
            '        <checklist condition="a or b">\n' +
            '            <check id="a" type="INCL" field="message">error</check>\n' +
            '            <check id="b" type="REGEX" field="path">.*\\.log$</check>\n' +
            '        </checklist>\n' +
            '\n' +
            '        <append field="processed">true</append>\n' +
            '        <plugin>notify("alert")</plugin>\n' +
            '        <del>temp_field</del>\n' +
            '    \n' +
            '    </rule>\n' +
            '</root',
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
        documentation: 'Rule definition (operations can be in any order)',
        insertText: 'rule id="rule_id" name="rule_name">\n    <check type="EQU" field="field">value</check>\n</rule',
        range: range
      });
    }
  } else if (parentTag === 'rule') {
    // rule内部 - 提供所有可能的子标签，强调可以任意顺序
    const ruleChildTags = [
      {
        label: 'check',
        kind: monaco.languages.CompletionItemKind.Property,
        documentation: 'Standalone check condition (can be placed anywhere in rule)',
        insertText: 'check type="EQU" field="field">value</check',
        range: range,
        sortText: '1_check' // Higher priority
      },
      {
        label: 'checklist',
        kind: monaco.languages.CompletionItemKind.Module,
        documentation: 'Checklist with conditional logic (can be placed anywhere in rule)',
        insertText: 'checklist condition="a and b">\n    <check id="a" type="EQU" field="field">value</check>\n    <check id="b" type="INCL" field="field">value</check>\n</checklist',
        range: range,
        sortText: '2_checklist'
      },
      {
        label: 'threshold',
        kind: monaco.languages.CompletionItemKind.Property,
        documentation: 'Threshold configuration (can be placed anywhere in rule)',
        insertText: 'threshold group_by="user_id" range="5m">10</threshold',
        range: range,
        sortText: '3_threshold'
      },
      {
        label: 'append',
        kind: monaco.languages.CompletionItemKind.Property,
        documentation: 'Append field to result (can be placed anywhere in rule)',
        insertText: 'append field="field_name">value</append',
        range: range,
        sortText: '4_append'
      },
      {
        label: 'plugin',
        kind: monaco.languages.CompletionItemKind.Function,
        documentation: 'Plugin execution (can be placed anywhere in rule)',
        insertText: 'plugin>plugin_name()</plugin',
        range: range,
        sortText: '5_plugin'
      },
      {
        label: 'del',
        kind: monaco.languages.CompletionItemKind.Property,
        documentation: 'Delete fields from result (can be placed anywhere in rule)',
        insertText: 'del>field</del',
        insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
        range: range,
        sortText: '6_del'
      }
    ];
    
    suggestions.push(...ruleChildTags);
  } else if (parentTag === 'checklist') {
    // checklist内部 - 只能有check标签（注意：不是node）
    if (!suggestions.some(s => s.label === 'check')) {
      suggestions.push({
        label: 'check',
        kind: monaco.languages.CompletionItemKind.Property,
        documentation: 'Check node within checklist (must have id attribute for condition)',
        insertText: 'check id="id" type="INCL" field="field">value</check',
        range: range
      });
    }
  }
  
  return { suggestions };
}

// 标签内容补全
function getXmlTagContentCompletions(context, range, fullText) {
  const suggestions = [];
  
  // Check if user is inside function parameters (parentheses)
  const functionParamInfo = checkIfInFunctionParameters(context);
  
  if (functionParamInfo.isInParams) {
    // User is editing function parameters, provide field suggestions
    addFieldSuggestionsForFunctionParams(suggestions, range, context);
    // Return early - don't show plugin functions when editing parameters
    return { suggestions };
  }
  
  // Enhanced plugin function completion with parameter information (optimized)
  // Only show plugin functions when NOT in function parameters
  if (context.currentTag === 'plugin' || 
      (context.currentTag === 'check' && fullText.includes('type="PLUGIN"')) || 
      (context.currentTag === 'node' && fullText.includes('type="PLUGIN"')) || 
      (context.currentTag === 'append' && fullText.includes('type="PLUGIN"'))) {
    
    // Determine if we're in a check context (which requires bool return type)
    const isInCheckNode = (context.currentTag === 'check' || context.currentTag === 'node') && fullText.includes('type="PLUGIN"');
    
    // Use cached plugin suggestions (no async needed)
    const pluginSuggestions = getPluginSuggestions(range, isInCheckNode);
    suggestions.push(...pluginSuggestions);
    
    // Only add generic plugin templates if no real plugins exist
    const hasRealPlugins = (pluginComponents.value || []).some(plugin => !plugin.hasTemp);
    if (!hasRealPlugins) {
      suggestions.push({
        label: 'plugin_name(_$ORIDATA)',
        kind: monaco.languages.CompletionItemKind.Snippet,
        documentation: 'Plugin function with original data (template)',
        insertText: 'plugin_name(_$ORIDATA)',
        range: range,
        sortText: '9_template_oridata' // Very low priority
      });
      
      suggestions.push({
        label: 'plugin_name("arg1", arg2)',
        kind: monaco.languages.CompletionItemKind.Snippet,
        documentation: 'Plugin function with custom arguments (template)',
        insertText: 'plugin_name("arg1", arg2)',
        range: range,
        sortText: '9_template_custom' // Very low priority
      });
    }
  }
  
  // check或node标签的值建议
  if (context.currentTag === 'check' || context.currentTag === 'node') {
    suggestions.push(
      { label: '_$ORIDATA', kind: monaco.languages.CompletionItemKind.Variable, documentation: 'Original data reference', insertText: '_$ORIDATA', range: range, sortText: '00_ORIDATA' }
    );
    
    // 为field引用添加建议
    if (dynamicFieldKeys.value && dynamicFieldKeys.value.length > 0) {
      suggestions.push({
        label: '_$field_reference',
        kind: monaco.languages.CompletionItemKind.Snippet,
        documentation: 'Reference to another field value',
        insertText: '_$' + dynamicFieldKeys.value[0],
        range: range,
        sortText: '01_field_ref'
      });
    }
  }
  
  // del标签内容补全 - 字段列表
  if (context.currentTag === 'del') {
    // Add individual fields from sample data
    if (dynamicFieldKeys.value && dynamicFieldKeys.value.length > 0) {
      dynamicFieldKeys.value.forEach(field => {
        if (!suggestions.some(s => s.label === field)) {
          suggestions.push({
            label: field,
            kind: monaco.languages.CompletionItemKind.Field,
            documentation: `Delete field: ${field}`,
            insertText: field,
            range: range,
            sortText: `0_${field}` // Higher priority than common fields
          });
        }
      });
      
      // Add common field combinations for deletion
      const topFields = dynamicFieldKeys.value.slice(0, 4);
      if (topFields.length >= 2) {
        suggestions.push({
          label: topFields.slice(0, 2).join(','),
          kind: monaco.languages.CompletionItemKind.Snippet,
          documentation: 'Delete multiple fields from sample data',
          insertText: topFields.slice(0, 2).join(','),
          range: range,
          sortText: '0_delete_combo_2'
        });
      }
      if (topFields.length >= 3) {
        suggestions.push({
          label: topFields.slice(0, 3).join(','),
          kind: monaco.languages.CompletionItemKind.Snippet,
          documentation: 'Delete multiple fields from sample data',
          insertText: topFields.slice(0, 3).join(','),
          range: range,
          sortText: '0_delete_combo_3'
        });
      }
    }
    
  }
  
  // ---------------------------------------------------------------------
  //  字段占位符补全 ($field[<cursor>])
  // ---------------------------------------------------------------------
  // 适用于 filter / node / append / plugin 等标签的文本内容区域，
  // 当光标位于 "$field[" 之后且尚未输入右括号时，提供字段名补全。
  // ---------------------------------------------------------------------
  const fieldPlaceholderMatch = context.beforeCursor.match(/\$field\[[^\]]*$/);
  if (fieldPlaceholderMatch) {
    // 计算替换范围：从左方括号后的首字符开始至光标位置
    const startColumn = context.beforeCursor.lastIndexOf('[') + 2; // +2 因为 monaco column 从 1 开始
    const customRange = range; // 使用默认 range 以避免未定义的 position 引用

    if (dynamicFieldKeys.value && dynamicFieldKeys.value.length > 0) {
      dynamicFieldKeys.value.forEach(field => {
        if (!suggestions.some(s => s.label === field)) {
          suggestions.push({
            label: field,
            kind: monaco.languages.CompletionItemKind.Field,
            documentation: `Sample data field: ${field}`,
            insertText: field,
            range: customRange,
            sortText: `0_${field}`
          });
        }
      });
    }

  }
  
  return { suggestions };
}

// Check if user is inside function parameters
function checkIfInFunctionParameters(context) {
  const { beforeCursor } = context;
  
  // Look for pattern like: functionName(cursor_position)
  // Find the last opening parenthesis
  const lastOpenParen = beforeCursor.lastIndexOf('(');
  const lastCloseParen = beforeCursor.lastIndexOf(')');
  
  // If there's an opening parenthesis after the last closing parenthesis, we're likely inside parameters
  if (lastOpenParen > lastCloseParen && lastOpenParen !== -1) {
    // Check if there's a function name before the opening parenthesis
    const beforeParen = beforeCursor.substring(0, lastOpenParen);
    const functionMatch = beforeParen.match(/([a-zA-Z_][a-zA-Z0-9_]*)\s*$/);
    
    if (functionMatch) {
      // Additional validation: make sure there's actual content after the opening parenthesis
      // or the cursor is right after a comma (indicating a parameter position)
      const afterParen = beforeCursor.substring(lastOpenParen + 1);
      const isValidParamPosition = 
        afterParen.length > 0 || // Content after opening paren
        beforeCursor.endsWith('(') || // Right after opening paren
        beforeCursor.match(/,\s*$/) || // After a comma
        beforeCursor.match(/\(\s*$/) // After opening paren with spaces
      
      if (isValidParamPosition) {
        return {
          isInParams: true,
          functionName: functionMatch[1],
          parameterText: afterParen
        };
      }
    }
  }
  
  return { isInParams: false };
}

// Add field suggestions for function parameters
function addFieldSuggestionsForFunctionParams(suggestions, range, context) {
  // Add dynamic fields from sample data
  if (dynamicFieldKeys.value && dynamicFieldKeys.value.length > 0) {
    dynamicFieldKeys.value.forEach(field => {
      if (!suggestions.some(s => s.label === field)) {
        suggestions.push({
          label: field,
          kind: monaco.languages.CompletionItemKind.Field,
          documentation: `Field from sample data: ${field}`,
          insertText: field,
          range: range,
          sortText: `0_${field}` // Higher priority than common fields
        });
      }
    });
  }
  
  // Add special data references (highest priority)
  const specialRefs = [
    { label: '_$ORIDATA', desc: 'Original data reference', insertText: '_$ORIDATA' }
  ];
  
  specialRefs.forEach(ref => {
    if (!suggestions.some(s => s.label === ref.label)) {
      suggestions.push({
        label: ref.label,
        kind: monaco.languages.CompletionItemKind.Variable,
        documentation: ref.desc,
        insertText: ref.insertText,
        range: range,
        sortText: `00_${ref.label}` // Highest priority
      });
    }
  });
  
  // Add common parameter patterns
  const patterns = [
    { label: 'true', desc: 'Boolean true value', insertText: '"true"' },
    { label: 'false', desc: 'Boolean false value', insertText: '"false"' }
  ];
  
  patterns.forEach(pattern => {
    if (!suggestions.some(s => s.label === pattern.label)) {
      suggestions.push({
        label: pattern.label,
        kind: monaco.languages.CompletionItemKind.Value,
        documentation: pattern.desc,
        insertText: pattern.insertText,
        range: range,
        sortText: `3_${pattern.label}` // Lowest priority
      });
    }
  });
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

// 全局插件补全建议缓存 - 跨组件实例共享
const globalPluginSuggestionsCache = new Map();
let lastPluginDataHash = '';

// 计算插件数据hash用于缓存失效检测
const calculatePluginDataHash = () => {
  try {
    const plugins = pluginComponents.value?.filter(p => !p.hasTemp) || [];
    const pluginIds = plugins.map(p => p.id).sort().join(',');
    const parameterKeys = Object.keys(pluginParametersCache.value || {}).sort().join(',');
    return `${pluginIds}:${parameterKeys}`;
  } catch (error) {
    console.warn('Error calculating plugin data hash:', error);
    return Date.now().toString(); // fallback to timestamp
  }
};

// 智能获取插件补全建议（带缓存）
const getPluginSuggestions = (range, isInCheckNode = false) => {
  const cacheKey = `${isInCheckNode ? 'check' : 'all'}_plugins`;
  const currentHash = calculatePluginDataHash();
  
  // 检查数据是否有变化
  if (currentHash !== lastPluginDataHash) {
    // 插件数据有变化，清理所有缓存
    globalPluginSuggestionsCache.clear();
    lastPluginDataHash = currentHash;
  }
  
  // 检查缓存
  if (globalPluginSuggestionsCache.has(cacheKey)) {
    const cached = globalPluginSuggestionsCache.get(cacheKey);
    // 更新range（因为每次调用时的range可能不同）
    return cached.map(suggestion => ({
      ...suggestion,
      range: range
    }));
  }
  
  // 构建新的补全建议
  const suggestions = [];
  const validPlugins = (pluginComponents.value || []).filter(plugin => !plugin.hasTemp);
  
  validPlugins.forEach(plugin => {
    // For checknode, only show plugins with bool return type
    if (isInCheckNode && plugin.returnType !== 'bool') {
      return; // Skip this plugin
    }
    
    const cachedParameters = (pluginParametersCache.value || {})[plugin.id];
    const hasParameterInfo = plugin.id in (pluginParametersCache.value || {});
    
    let insertText = plugin.id;
    let documentation = `Plugin: ${plugin.id}`;
    
    if (hasParameterInfo && cachedParameters && cachedParameters.length > 0) {
      // Create parameter template based on actual plugin signature
      const paramSnippets = cachedParameters.map((param) => {
        switch (param.type) {
          case 'string':
            return param.name;
          case 'int':
          case 'float':
            return param.name;
          case 'bool':
            return 'true';
          case '...interface{}':
            return '_$ORIDATA';
          default:
            return param.type.includes('interface') ? '_$ORIDATA' : param.name;
        }
      }).join(', ');
      
      insertText = `${plugin.id}(${paramSnippets})`;
      
      const paramDocs = cachedParameters.map(p => 
        `${p.name}: ${p.type}${p.required ? ' (required)' : ' (optional)'}`
      ).join('\n');
      documentation = `Plugin: ${plugin.id}\n\nParameters:\n${paramDocs}`;
    } else if (hasParameterInfo) {
      insertText = `${plugin.id}()`;
      documentation = `Plugin: ${plugin.id}\n\nNo parameters required`;
    } else {
      insertText = `${plugin.id}()`;
      documentation = `Plugin: ${plugin.id}\n\nLoading parameter information...`;
      
      // 异步获取参数，但不阻塞当前补全
      getPluginParameters(plugin.id).then(() => {
        // 参数获取完成后，清理缓存以便下次使用最新数据
        globalPluginSuggestionsCache.delete(cacheKey);
      }).catch(error => {
        console.debug(`Could not fetch parameters for plugin ${plugin.id}:`, error);
      });
    }
    
    // Create user-friendly label
    let pluginLabel;
    if (hasParameterInfo && cachedParameters && cachedParameters.length > 0) {
      const simpleParams = cachedParameters.map(param => param.name).join(', ');
      pluginLabel = `${plugin.id}(${simpleParams})`;
    } else {
      pluginLabel = `${plugin.id}()`;
    }
    
    suggestions.push({
      label: pluginLabel,
      kind: monaco.languages.CompletionItemKind.Function,
      documentation: documentation,
      insertText: insertText,
      range: range, // 这个会在返回时动态更新
      sortText: `0_${plugin.id}` // Higher priority for actual plugins
    });
  });
  
  // 缓存结果（不包含range，因为range每次都不同）
  const cacheableSuggestions = suggestions.map(s => ({ ...s, range: null }));
  globalPluginSuggestionsCache.set(cacheKey, cacheableSuggestions);
  
  return suggestions;
};

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

/* 项目组件关键字样式 - INPUT/OUTPUT/RULESET */
.monaco-editor .token.project\.input,
.monaco-diff-editor .token.project\.input {
  color: #28a745 !important;
  font-weight: bold !important;
}

.monaco-editor .token.project\.output,
.monaco-diff-editor .token.project\.output {
  color: #e36209 !important;
  font-weight: bold !important;
}

.monaco-editor .token.project\.ruleset,
.monaco-diff-editor .token.project\.ruleset {
  color: #6f42c1 !important;
  font-weight: bold !important;
}

/* 错误行样式 - 柔和现代风格 */
.monaco-error-line {
  background-color: rgba(209, 36, 47, 0.08) !important;  /* 极淡的现代红色背景 */
  border-left: 2px solid rgba(209, 36, 47, 0.4) !important;  /* 更细更淡的边框 */
  box-shadow: inset 0 0 0 1px rgba(209, 36, 47, 0.05) !important;  /* 细微边框效果 */
}

.monaco-error-line-decoration {
  background-color: rgba(209, 36, 47, 0.6) !important;  /* 柔和的装饰颜色 */
  width: 3px !important;
  margin-left: 3px !important;
  border-radius: 1px !important;
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