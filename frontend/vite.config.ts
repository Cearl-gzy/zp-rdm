import {defineConfig} from 'vite'
import vue from '@vitejs/plugin-vue'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vue()],
  build: {
    chunkSizeWarningLimit: 1400,
    rollupOptions: {
      output: {
        manualChunks(id: string) {
          if (!id.includes('node_modules')) return
          if (id.includes('@codemirror') || id.includes('@lezer') || id.includes('/codemirror/')) return 'cm'
          // antd 与 icons 相互引用，必须同 chunk
          if (id.includes('ant-design-vue') || id.includes('@ant-design')) return 'antd'
          if (id.includes('/vue/') || id.includes('@vue') || id.includes('pinia') || id.includes('dayjs')) return 'vendor'
        },
      },
    },
  },
})
