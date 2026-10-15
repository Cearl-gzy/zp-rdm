import { createApp } from 'vue'
import { createPinia } from 'pinia'
import Antd from 'ant-design-vue'
import 'ant-design-vue/dist/reset.css'
import App from './App.vue'
import './style.css'

// 禁用 WebView 默认右键菜单；输入框保留系统菜单（复制/粘贴），
// 自定义右键菜单（如侧栏连接项）由 Vue 事件处理，不受 preventDefault 影响
document.addEventListener('contextmenu', e => {
  const el = e.target as HTMLElement | null
  if (el?.closest('input, textarea, [contenteditable="true"]')) return
  e.preventDefault()
})

const app = createApp(App)
app.use(createPinia())
app.use(Antd)
app.mount('#app')
