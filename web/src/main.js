import './style.css'
import { createApp } from 'vue'
import App from './App.vue'
import router from './router/index.js'
import store from './store/index.js'

const app = createApp(App)
app.use(router)
app.use(store)
app.mount('#app') 