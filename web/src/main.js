import './style.css'
import { createApp } from 'vue'
import App from './App.vue'
import router from './router/index.js'
import store from './store/index.js'
import './monaco-loader.js'

const app = createApp(App)
app.use(router)
app.use(store)

// Make router globally accessible for API interceptors
window.router = router

app.mount('#app') 