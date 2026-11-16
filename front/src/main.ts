import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ProtoApp from './proto/ProtoApp.vue'
import './style.css'

createApp(ProtoApp).use(createPinia()).mount('#app')
