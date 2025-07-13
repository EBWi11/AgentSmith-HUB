import './style.css'
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router/index.js'
import store from './store/index.js'
import './monaco-loader.js'
import { initializeConfig } from './config/index.js'

// Initialize configuration before creating the app
async function initializeApp() {
  try {
    // Load runtime configuration
    await initializeConfig()
    console.log('Configuration initialized successfully')
  } catch (error) {
    console.warn('Failed to initialize configuration:', error)
    // Continue with default configuration
  }
  
  const app = createApp(App)
  const pinia = createPinia()

  app.use(pinia)
  app.use(router)
  app.use(store)

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

// Start the application
initializeApp().catch(error => {
  console.error('Failed to initialize application:', error)
  // Mount the app anyway with default configuration
  const app = createApp(App)
  const pinia = createPinia()
  
  app.use(pinia)
  app.use(router)
  app.use(store)
  
  window.router = router
  
  app.config.globalProperties.$message = {
    success: (message) => {},
    error: (message) => console.error('Error:', message),
    warning: (message) => console.warn('Warning:', message)
  }
  
  app.mount('#app')
}) 