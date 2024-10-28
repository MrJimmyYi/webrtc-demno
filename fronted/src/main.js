// src/main.js
import { createApp } from 'vue'
import App from './App.vue'

// 定义 `global` 为 `window`
window.global = window

// 引入 buffer 模块并设置全局 Buffer
import { Buffer } from 'buffer'
window.Buffer = Buffer

// 引入 process 模块并设置全局 process
import process from 'process'
window.process = process

createApp(App).mount('#app')
