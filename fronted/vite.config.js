// vite.config.js
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import polyfillNode from 'vite-plugin-polyfill-node'

export default defineConfig({
  plugins: [
    vue(),
    polyfillNode(), // 使用 vite-plugin-polyfill-node 插件
  ],
  resolve: {
    alias: {
      // 为 Node.js 模块提供浏览器端替代品
      'stream': 'stream-browserify',
      'buffer': 'buffer',
      'process': 'process/browser',
    },
  },
  define: {
    // 将 `global` 定义为 `window`，解决 `global is not defined` 问题
    global: 'window',
    // 定义 `process.env` 为一个空对象，避免未定义错误
    'process.env': {},
  },
})
