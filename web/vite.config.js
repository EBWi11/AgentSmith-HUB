import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { copyFileSync, existsSync, mkdirSync } from 'fs'
import { join } from 'path'

// Function to copy docs to public directory
function copyDocsToPublic() {
  const docsDir = join(__dirname, '../docs')
  const publicDir = join(__dirname, 'public')
  
  // Ensure public directory exists
  if (!existsSync(publicDir)) {
    mkdirSync(publicDir, { recursive: true })
  }
  
  // Copy markdown files
  const mdFiles = ['agentsmith-hub-guide.md', 'agentsmith-hub-guide-zh.md']
  mdFiles.forEach(file => {
    const srcPath = join(docsDir, file)
    const destPath = join(publicDir, file)
    if (existsSync(srcPath)) {
      copyFileSync(srcPath, destPath)
      console.log(`✓ Copied ${file} to public directory`)
    } else {
      console.warn(`⚠ Warning: ${file} not found in docs directory`)
    }
  })
}

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [
    vue(),
    {
      name: 'copy-docs',
      buildStart() {
        copyDocsToPublic()
      }
    }
  ],
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