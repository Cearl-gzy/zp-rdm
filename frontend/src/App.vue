<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { theme } from 'ant-design-vue'
import zhCN from 'ant-design-vue/es/locale/zh_CN'
import enUS from 'ant-design-vue/es/locale/en_US'
import { CloseOutlined, LockOutlined } from '@ant-design/icons-vue'
import { useAppStore } from './stores/app'
import { useConnectionStore } from './stores/connection'
import { emitAppEvent, AppEvents } from './utils/events'
import { envColor } from './utils/env'
import TitleBar from './components/TitleBar.vue'
import Sidebar from './components/Sidebar.vue'
import Welcome from './components/Welcome.vue'
import CommandPalette from './components/CommandPalette.vue'
import ConnectionView from './views/ConnectionView.vue'

const app = useAppStore()
const conn = useConnectionStore()
const { t, locale } = useI18n()

const antdLocale = computed(() => (locale.value === 'en' ? enUS : zhCN))

onMounted(async () => {
  await app.initSettings()
  await conn.loadProfiles()
  conn.restoreWorkspace()
})

// ---------- 全局快捷键 ----------
function onGlobalKeydown(e: KeyboardEvent) {
  const mod = e.ctrlKey || e.metaKey

  // Ctrl+K 命令面板
  if (mod && !e.shiftKey && !e.altKey && e.key.toLowerCase() === 'k') {
    e.preventDefault()
    app.togglePalette()
    return
  }
  // 面板打开时其余快捷键让位给面板
  if (app.paletteOpen) return

  // Ctrl+N 新建连接
  if (mod && !e.shiftKey && !e.altKey && e.key.toLowerCase() === 'n') {
    e.preventDefault()
    emitAppEvent(AppEvents.NewConnection)
    return
  }
  // Ctrl+W 关闭当前标签
  if (mod && !e.shiftKey && !e.altKey && e.key.toLowerCase() === 'w') {
    if (conn.activeTab) {
      e.preventDefault()
      conn.close(conn.activeTab)
    }
    return
  }
  // F5 / Ctrl+R 刷新当前视图（阻止 WebView 整页刷新）
  if (e.key === 'F5' || (mod && e.key.toLowerCase() === 'r')) {
    e.preventDefault()
    emitAppEvent(AppEvents.Refresh)
    return
  }
}
window.addEventListener('keydown', onGlobalKeydown)
onBeforeUnmount(() => window.removeEventListener('keydown', onGlobalKeydown))

const themeConfig = computed(() => ({
  algorithm: app.dark ? theme.darkAlgorithm : theme.defaultAlgorithm,
  token: {
    colorPrimary: '#e04536',
    colorInfo: '#e04536',
    colorError: '#e04536',
    borderRadius: 6,
    fontSize: 13,
    colorBgContainer: app.dark ? '#161a23' : '#ffffff',
    colorBgElevated: app.dark ? '#1c212d' : '#ffffff',
    colorBgLayout: app.dark ? '#0b0d12' : '#f5f6f8',
    colorText: app.dark ? '#e8eaf0' : '#1c2230',
    colorTextSecondary: app.dark ? '#9ba3b4' : '#596274',
    colorBorder: app.dark ? 'rgba(255,255,255,0.07)' : 'rgba(17,24,39,0.08)',
    colorBorderSecondary: app.dark ? 'rgba(255,255,255,0.05)' : 'rgba(17,24,39,0.05)',
  },
}))
</script>

<template>
  <a-config-provider :locale="antdLocale" :theme="themeConfig">
    <div class="window-root">
      <TitleBar />
      <div class="app-split">
      <Sidebar />
      <div class="main-area">
        <template v-if="conn.openTabs.length">
          <!-- 自定义页签栏 -->
          <div class="tabbar">
            <div
              v-for="tab in conn.openTabs"
              :key="tab.id"
              class="tabbar-item"
              :class="{ active: conn.activeTab === tab.id }"
              :title="t('app.tabMiddleClose', { name: tab.state.name })"
              @click="conn.activeTab = tab.id"
              @click.middle.prevent="conn.close(tab.id)"
            >
              <span class="tab-color" :style="{ background: tab.state.color || '#e04536' }" />
              <span class="status-dot" :class="{ on: true }" />
              <span
                v-if="tab.state.env"
                class="tab-env"
                :style="{ color: envColor(tab.state.env), borderColor: envColor(tab.state.env) }"
              >{{ tab.state.env.toUpperCase() }}</span>
              <span class="tab-name">{{ tab.state.name }}</span>
              <LockOutlined v-if="tab.state.readonly" class="tab-lock" :title="t('common.readonly')" />
              <span class="tab-db num">db{{ conn.currentDb[tab.id] ?? 0 }}</span>
              <button class="tab-close" @click.stop="conn.close(tab.id)">
                <CloseOutlined />
              </button>
            </div>
          </div>
          <div class="tab-body">
            <template v-for="tab in conn.openTabs" :key="tab.id">
              <ConnectionView
                v-show="conn.activeTab === tab.id"
                :conn-id="tab.id"
                :state="tab.state"
              />
            </template>
          </div>
        </template>
        <Welcome v-else />
      </div>
      </div>
      <CommandPalette />
    </div>
  </a-config-provider>
</template>

<style scoped>
.main-area {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  background: var(--bg-base);
}

.tabbar {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 8px 12px 0;
  overflow-x: auto;
  flex-shrink: 0;
  scrollbar-width: none;
}
.tabbar::-webkit-scrollbar { display: none; }

.tabbar-item {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  height: 32px;
  padding: 0 8px 0 12px;
  border-radius: var(--radius-md) var(--radius-md) 0 0;
  border: 1px solid transparent;
  border-bottom: none;
  cursor: pointer;
  color: var(--text-2);
  font-size: 12.5px;
  white-space: nowrap;
  transition: background 0.15s var(--ease), color 0.15s, border-color 0.15s;
  position: relative;
}
.tabbar-item:hover { background: var(--bg-hover); color: var(--text-1); }
.tabbar-item.active {
  background: var(--bg-panel);
  border-color: var(--border);
  color: var(--text-1);
  font-weight: 500;
}
.tabbar-item.active::after {
  content: '';
  position: absolute;
  left: 0; right: 0; bottom: -1px;
  height: 1px;
  background: var(--bg-panel);
}
.tab-color { width: 3px; height: 12px; border-radius: 2px; flex-shrink: 0; }
.tab-name { max-width: 160px; overflow: hidden; text-overflow: ellipsis; }
.tab-env {
  font-size: 9px; line-height: 1.5; padding: 0 3px;
  border: 1px solid; border-radius: 3px; flex-shrink: 0;
}
.tab-lock { font-size: 11px; color: var(--text-3); flex-shrink: 0; }
.tab-db {
  font-size: 10px;
  color: var(--text-3);
  background: var(--bg-hover);
  border-radius: 4px;
  padding: 1px 5px;
}
.tabbar-item.active .tab-db { background: var(--bg-active); color: var(--text-2); }
.tab-close {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px; height: 18px;
  border: none;
  background: transparent;
  border-radius: 4px;
  color: var(--text-3);
  cursor: pointer;
  font-size: 10px;
  opacity: 0;
  transition: opacity 0.12s, background 0.12s, color 0.12s;
  padding: 0;
}
.tabbar-item:hover .tab-close, .tabbar-item.active .tab-close { opacity: 1; }
.tab-close:hover { background: var(--bg-active); color: var(--text-1); }

.tab-body {
  flex: 1;
  min-height: 0;
  background: var(--bg-panel);
  border-top: 1px solid var(--border);
}
</style>
