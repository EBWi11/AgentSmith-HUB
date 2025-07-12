import './style.css'
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router/index.js'
import store from './store/index.js'
import './monaco-loader.js'

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