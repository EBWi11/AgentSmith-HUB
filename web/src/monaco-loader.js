import * as monaco from 'monaco-editor/esm/vs/editor/editor.api';
import 'monaco-editor/esm/vs/language/json/monaco.contribution';
import 'monaco-editor/esm/vs/basic-languages/xml/xml.contribution';
import editorWorker from 'monaco-editor/esm/vs/editor/editor.worker?worker';
import jsonWorker from 'monaco-editor/esm/vs/language/json/json.worker?worker';

// 注册Monaco编辑器的workers
self.MonacoEnvironment = {
  getWorker(_, label) {
    if (label === 'json') {
      return new jsonWorker();
    }
    return new editorWorker();
  }
};

// 注册YAML语言
monaco.languages.register({ id: 'yaml' });
monaco.languages.setMonarchTokensProvider('yaml', {
  tokenizer: {
    root: [
      [/^[\t ]*[A-Za-z_\-0-9]+:/, 'keyword'],
      [/^[\t ]*-/, 'keyword'],
      [/".*?"/, 'string'],
      [/'.*?'/, 'string'],
      [/\d+/, 'number'],
      [/\btrue\b|\bfalse\b|\bnull\b/, 'keyword'],
      [/#.*$/, 'comment'],
    ]
  }
});

// 注册GO语言
monaco.languages.register({ id: 'go' });
monaco.languages.setMonarchTokensProvider('go', {
  tokenizer: {
    root: [
      [/\/\/.*$/, 'comment'],
      [/\/\*/, 'comment', '@comment'],
      [/"(?:\\.|[^"\\])*"/, 'string'],
      [/'(?:\\.|[^'\\])*'/, 'string'],
      [/\b(?:func|package|import|const|var|type|struct|interface|map|chan|go|defer|if|else|switch|case|for|range|return|break|continue|fallthrough|select|default)\b/, 'keyword'],
      [/\b(?:true|false|nil|iota)\b/, 'keyword'],
      [/\b(?:int|int8|int16|int32|int64|uint|uint8|uint16|uint32|uint64|float32|float64|complex64|complex128|byte|rune|string|bool|error)\b/, 'type'],
      [/\d+/, 'number'],
      [/[A-Z][a-zA-Z0-9_]*/, 'type.identifier'],
      [/[a-z][a-zA-Z0-9_]*/, 'identifier'],
    ],
    comment: [
      [/[^/*]+/, 'comment'],
      [/\/\*/, 'comment', '@push'],
      [/\*\//, 'comment', '@pop'],
      [/[/*]/, 'comment']
    ]
  }
});

// 注册XML语言的额外功能
monaco.languages.setLanguageConfiguration('xml', {
  autoClosingPairs: [
    { open: '<', close: '>' },
    { open: '"', close: '"' },
    { open: "'", close: "'" }
  ],
  brackets: [
    ['<', '>']
  ],
  comments: {
    blockComment: ['<!--', '-->']
  }
});

export default monaco; 