import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url))
    }
  },
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/api/, '')
      },
      '/mcp': {
        target: 'http://localhost:8080',
        changeOrigin: true
      },
      '/ping': {
        target: 'http://localhost:8080',
        changeOrigin: true
      },
      '/token-check': {
        target: 'http://localhost:8080',
        changeOrigin: true
      }
    }
  },
  optimizeDeps: {
    include: ['monaco-editor/esm/vs/language/json/json.worker', 
              'monaco-editor/esm/vs/language/css/css.worker', 
              'monaco-editor/esm/vs/language/html/html.worker', 
              'monaco-editor/esm/vs/language/typescript/ts.worker', 
              'monaco-editor/esm/vs/editor/editor.worker']
  },
  build: {
    rollupOptions: {
      output: {
        manualChunks: {
          jsonWorker: ['monaco-editor/esm/vs/language/json/json.worker'],
          cssWorker: ['monaco-editor/esm/vs/language/css/css.worker'],
          htmlWorker: ['monaco-editor/esm/vs/language/html/html.worker'],
          tsWorker: ['monaco-editor/esm/vs/language/typescript/ts.worker'],
          editorWorker: ['monaco-editor/esm/vs/editor/editor.worker'],
        }
      }
    }
  }
}) 