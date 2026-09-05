import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { copyFileSync, existsSync, mkdirSync, readdirSync } from 'fs'
import { join } from 'path'

const __dirname = fileURLToPath(new URL('.', import.meta.url))

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
  
  // Copy png directory
  const pngSrcDir = join(docsDir, 'png')
  const pngDestDir = join(publicDir, 'png')
  
  if (existsSync(pngSrcDir)) {
    if (!existsSync(pngDestDir)) {
      mkdirSync(pngDestDir, { recursive: true })
    }
    
    // Copy all PNG files
    const pngFiles = readdirSync(pngSrcDir).filter(file => file.endsWith('.png'))
    pngFiles.forEach(file => {
      const srcPath = join(pngSrcDir, file)
      const destPath = join(pngDestDir, file)
      copyFileSync(srcPath, destPath)
      console.log(`✓ Copied png/${file} to public directory`)
    })
  } else {
    console.warn(`⚠ Warning: png directory not found in docs directory`)
  }
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
    include: [
      'monaco-editor/esm/vs/language/json/json.worker',
      'monaco-editor/esm/vs/editor/editor.worker'
    ]
  }
})
