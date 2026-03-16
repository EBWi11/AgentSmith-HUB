import './style.css'
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router/index.js'
import './monaco-loader.js'
import { initializeConfig } from './config/index.js'
import { initOidc } from './api/oidc.js'

function mountApp() {
  const app = createApp(App)
  const pinia = createPinia()

  app.use(pinia)
  app.use(router)

  // Make router globally accessible for API interceptors
  window.router = router

  // Global message component
  app.config.globalProperties.$message = {
    success: (message) => {}, // console.log('Success:', message),
    error: (message) => console.error('Error:', message),
    warning: (message) => console.warn('Warning:', message)
  }

  app.mount('#app')
}

// Initialize configuration before creating the app
async function initializeApp() {
  try {
    await initializeConfig()
    await initOidc() // 在配置加载后初始化 OIDC（需要已知的回调 URL 等）
  } catch (error) {
    console.warn('Failed to initialize configuration:', error)
  }

  mountApp()
}

initializeApp().catch(error => {
  console.error('Failed to initialize application:', error)
  mountApp()
})
