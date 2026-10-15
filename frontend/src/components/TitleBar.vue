<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount } from 'vue'
import {
  WindowMinimise, WindowToggleMaximise, WindowIsMaximised, Quit,
} from '../../wailsjs/runtime/runtime'
import { MinusOutlined, CloseOutlined, BorderOutlined, BlockOutlined } from '@ant-design/icons-vue'

const maximised = ref(false)

async function syncState() {
  maximised.value = await WindowIsMaximised()
}

function onResize() { syncState() }

onMounted(() => {
  syncState()
  window.addEventListener('resize', onResize)
})
onBeforeUnmount(() => window.removeEventListener('resize', onResize))

function minimise() { WindowMinimise() }
async function toggleMaximise() {
  await WindowToggleMaximise()
  syncState()
}
function close() { Quit() }
</script>

<template>
  <div class="titlebar" @dblclick.self="toggleMaximise">
    <div class="tb-drag" @dblclick="toggleMaximise">
      <div class="tb-brand">
        <span class="tb-logo">ZP</span>
        <span class="tb-title">ZP Redis Manager</span>
      </div>
    </div>
    <div class="tb-controls">
      <button class="tb-btn" title="最小化" @click="minimise">
        <MinusOutlined />
      </button>
      <button class="tb-btn" :title="maximised ? '还原' : '最大化'" @click="toggleMaximise">
        <component :is="maximised ? BlockOutlined : BorderOutlined" class="max-icon" />
      </button>
      <button class="tb-btn close" title="关闭" @click="close">
        <CloseOutlined />
      </button>
    </div>
  </div>
</template>

<style scoped>
.titlebar {
  height: 36px;
  flex-shrink: 0;
  display: flex;
  align-items: stretch;
  background: var(--bg-panel);
  border-bottom: 1px solid var(--border);
  position: relative;
  z-index: 100;
}
.tb-drag {
  flex: 1;
  display: flex;
  align-items: center;
  --wails-draggable: drag;
  min-width: 0;
}
.tb-brand {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-left: 12px;
  --wails-draggable: no-drag;
}
.tb-logo {
  width: 18px; height: 18px;
  border-radius: 5px;
  display: inline-flex; align-items: center; justify-content: center;
  font-size: 8.5px; font-weight: 800; letter-spacing: 0.3px;
  color: #fff;
  background: linear-gradient(140deg, #f0583f 0%, #b3251a 100%);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.2);
}
.tb-title {
  font-size: 11.5px;
  font-weight: 600;
  color: var(--text-2);
  letter-spacing: 0.2px;
  white-space: nowrap;
}
.tb-controls {
  display: flex;
  align-items: stretch;
  --wails-draggable: no-drag;
}
.tb-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  border: none;
  background: transparent;
  color: var(--text-2);
  cursor: pointer;
  font-size: 12px;
  transition: background 0.12s, color 0.12s;
  padding: 0;
}
.tb-btn:hover { background: var(--bg-hover); color: var(--text-1); }
.tb-btn.close:hover { background: #e81123; color: #fff; }
.max-icon { font-size: 11px; }
</style>
