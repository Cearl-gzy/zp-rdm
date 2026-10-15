<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { message, Modal } from 'ant-design-vue'
import {
  PlusOutlined, SearchOutlined, BulbOutlined, BulbFilled,
  ImportOutlined, ExportOutlined, DatabaseOutlined,
  LoadingOutlined, MoreOutlined, ControlOutlined, ThunderboltOutlined,
  CloudSyncOutlined, SafetyCertificateOutlined,
} from '@ant-design/icons-vue'
import { PickOpenFile, PickSaveFile, CheckUpdate } from '../../wailsjs/go/main/App'
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
import { useAppStore } from '../stores/app'
import { useConnectionStore } from '../stores/connection'
import { onAppEvent, AppEvents } from '../utils/events'
import type { ConnectionProfile } from '../types'
import ConnectionForm from './ConnectionForm.vue'

const app = useAppStore()
const conn = useConnectionStore()

const keyword = ref('')
const collapsedGroups = ref<Set<string>>(new Set())
const formVisible = ref(false)
const editing = ref<ConnectionProfile | null>(null)

const filteredGroups = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return conn.groups
  return conn.groups
    .map(g => ({
      name: g.name,
      items: g.items.filter(p =>
        p.name.toLowerCase().includes(kw) ||
        `${p.host}:${p.port}`.includes(kw) ||
        g.name.toLowerCase().includes(kw)
      ),
    }))
    .filter(g => g.items.length)
})

function toggleGroup(name: string) {
  if (collapsedGroups.value.has(name)) collapsedGroups.value.delete(name)
  else collapsedGroups.value.add(name)
  collapsedGroups.value = new Set(collapsedGroups.value)
}

function openForm(p?: ConnectionProfile) {
  editing.value = p ?? null
  formVisible.value = true
}

async function onOpen(id: string) {
  try {
    await conn.open(id)
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  }
}

function onDelete(p: ConnectionProfile) {
  Modal.confirm({
    title: '删除连接',
    content: `确定删除「${p.name}」吗？此操作不可恢复。`,
    okText: '删除',
    okButtonProps: { danger: true },
    cancelText: '取消',
    async onOk() {
      await conn.remove(p.id)
      message.success('已删除')
    },
  })
}

async function onExport() {
  try {
    const path = await PickSaveFile('导出连接配置', 'zp-rdm-connections.json')
    if (!path) return
    await conn.exportTo(path)
    message.success('导出成功')
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  }
}

async function onImport() {
  try {
    const path = await PickOpenFile('导入连接配置')
    if (!path) return
    const n = await conn.importFrom(path)
    message.success(`成功导入 ${n} 个连接`)
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  }
}

// 命令面板 / 快捷键触发的动作
const offHandlers: (() => void)[] = []
onMounted(() => {
  offHandlers.push(
    onAppEvent(AppEvents.NewConnection, () => openForm()),
    onAppEvent(AppEvents.ImportConnections, onImport),
    onAppEvent(AppEvents.ExportConnections, onExport),
  )
})
onBeforeUnmount(() => offHandlers.forEach(off => off()))

// ---------- 侧栏宽度拖拽 ----------
let dragging = false
function startResize(e: MouseEvent) {
  e.preventDefault()
  dragging = true
  const startX = e.clientX
  const startW = app.sidebarWidth
  document.body.style.cursor = 'col-resize'
  const onMove = (ev: MouseEvent) => {
    if (!dragging) return
    app.setSidebarWidth(startW + ev.clientX - startX)
  }
  const onUp = () => {
    dragging = false
    document.body.style.cursor = ''
    window.removeEventListener('mousemove', onMove)
    window.removeEventListener('mouseup', onUp)
  }
  window.addEventListener('mousemove', onMove)
  window.addEventListener('mouseup', onUp)
}

// ---------- 关于 / 检查更新 ----------
const aboutVisible = ref(false)
const checking = ref(false)
async function onCheckUpdate() {
  checking.value = true
  try {
    const url = await CheckUpdate()
    if (url) {
      Modal.confirm({
        title: '发现新版本',
        content: '是否打开下载页面？',
        okText: '前往下载',
        cancelText: '取消',
        onOk: () => BrowserOpenURL(url),
      })
    } else {
      message.success('当前已是最新版本')
    }
  } catch (e: any) {
    message.warning(String(e?.message ?? e))
  } finally {
    checking.value = false
  }
}
</script>

<template>
  <div class="sidebar" :style="{ width: app.sidebarWidth + 'px' }">
    <div class="side-resizer" @mousedown="startResize" />
    <div class="side-actions">
      <a-input
        v-model:value="keyword"
        placeholder="搜索连接"
        allow-clear
        size="small"
        class="search-input"
      >
        <template #prefix><SearchOutlined class="text-3" /></template>
      </a-input>
      <a-dropdown :trigger="['click']" placement="bottomRight">
        <button class="icon-btn bordered"><ControlOutlined /></button>
        <template #overlay>
          <a-menu>
            <a-menu-item key="import" @click="onImport">
              <ImportOutlined /> 导入连接
            </a-menu-item>
            <a-menu-item key="export" @click="onExport">
              <ExportOutlined /> 导出连接
            </a-menu-item>
          </a-menu>
        </template>
      </a-dropdown>
      <a-tooltip title="新建连接（Ctrl+N）">
        <a-button type="primary" size="small" class="new-btn" @click="openForm()">
          <template #icon><PlusOutlined /></template>
          新建
        </a-button>
      </a-tooltip>
    </div>

    <div class="conn-list">
      <div v-if="!filteredGroups.length" class="list-empty">
        <div class="empty-icon"><DatabaseOutlined /></div>
        <div class="list-empty-text">{{ keyword ? '没有匹配的连接' : '暂无连接' }}</div>
        <button v-if="!keyword" class="empty-new-btn" @click="openForm()">
          <PlusOutlined />
          <span>新建连接</span>
        </button>
      </div>

      <div v-for="g in filteredGroups" :key="g.name" class="group">
        <div class="group-header" @click="toggleGroup(g.name)">
          <span class="group-name">{{ g.name }}</span>
          <span class="group-count num">{{ g.items.length }}</span>
        </div>
        <div v-if="!collapsedGroups.has(g.name)" class="group-items">
          <a-dropdown
            v-for="p in g.items"
            :key="p.id"
            :trigger="['contextmenu']"
          >
            <div
              class="conn-item"
              :class="{ active: conn.activeTab === p.id, open: conn.openIds.has(p.id) }"
              @click="onOpen(p.id)"
              @dblclick="openForm(p)"
            >
              <span class="conn-color" :style="{ background: p.color || '#e04536' }" />
              <div class="conn-meta">
                <div class="conn-name">
                  {{ p.name }}
                  <LoadingOutlined v-if="conn.opening === p.id" class="conn-loading" />
                </div>
                <div class="conn-addr num">{{ p.host }}:{{ p.port }}{{ p.ssh?.enabled ? ' · ssh' : '' }}</div>
              </div>
              <span class="status-dot" :class="{ on: conn.openIds.has(p.id) }" />
              <a-dropdown :trigger="['click']" placement="bottomRight">
                <button class="item-more" @click.stop><MoreOutlined /></button>
                <template #overlay>
                  <a-menu>
                    <a-menu-item key="open" @click="onOpen(p.id)">打开连接</a-menu-item>
                    <a-menu-item key="edit" @click="openForm(p)">编辑</a-menu-item>
                    <a-menu-item key="dup" @click="conn.duplicate(p.id).then(() => message.success('已复制'))">复制</a-menu-item>
                    <a-menu-item v-if="conn.openIds.has(p.id)" key="close" @click="conn.close(p.id)">关闭连接</a-menu-item>
                    <a-menu-divider />
                    <a-menu-item key="del" danger @click="onDelete(p)">删除</a-menu-item>
                  </a-menu>
                </template>
              </a-dropdown>
            </div>
            <template #overlay>
              <a-menu>
                <a-menu-item key="open" @click="onOpen(p.id)">打开连接</a-menu-item>
                <a-menu-item key="edit" @click="openForm(p)">编辑</a-menu-item>
                <a-menu-item key="dup" @click="conn.duplicate(p.id).then(() => message.success('已复制'))">复制</a-menu-item>
                <a-menu-item v-if="conn.openIds.has(p.id)" key="close" @click="conn.close(p.id)">关闭连接</a-menu-item>
                <a-menu-divider />
                <a-menu-item key="del" danger @click="onDelete(p)">删除</a-menu-item>
              </a-menu>
            </template>
          </a-dropdown>
        </div>
      </div>
    </div>

    <div class="side-footer">
      <a-tooltip title="关于 / 检查更新">
        <button class="footer-version num" @click="aboutVisible = true">
          ZP RDM v{{ app.version || '…' }}
        </button>
      </a-tooltip>
      <div class="footer-actions">
        <a-tooltip title="命令面板（Ctrl+K）">
          <button class="icon-btn" @click="app.togglePalette(true)">
            <ThunderboltOutlined />
          </button>
        </a-tooltip>
        <a-tooltip :title="app.dark ? '切换浅色' : '切换深色'">
          <button class="icon-btn" @click="app.toggleTheme()">
            <component :is="app.dark ? BulbFilled : BulbOutlined" />
          </button>
        </a-tooltip>
      </div>
    </div>

    <ConnectionForm v-model:open="formVisible" :profile="editing" />

    <a-modal v-model:open="aboutVisible" title="关于 ZP Redis Manager" :width="380" :footer="null">
      <div class="about-body">
        <div class="about-logo" />
        <div class="about-name">ZP Redis Manager</div>
        <div class="about-version num">v{{ app.version || '…' }}</div>
        <div class="about-desc">轻量、精美、功能完善的 Redis 桌面管理工具</div>
        <div class="about-security">
          <SafetyCertificateOutlined />
          <span>连接密码已通过 Windows DPAPI 加密存储</span>
        </div>
        <a-button block :loading="checking" @click="onCheckUpdate">
          <template v-if="!checking" #icon><CloudSyncOutlined /></template>
          {{ checking ? '检查中…' : '检查更新' }}
        </a-button>
      </div>
    </a-modal>
  </div>
</template>

<style scoped>
.sidebar {
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  background: var(--bg-panel);
  border-right: 1px solid var(--border);
  position: relative;
}

.side-resizer {
  position: absolute;
  top: 0; right: -2px; bottom: 0;
  width: 5px;
  cursor: col-resize;
  z-index: 10;
}
.side-resizer::after {
  content: '';
  position: absolute;
  top: 0; bottom: 0; left: 2px;
  width: 1px;
  background: transparent;
  transition: background 0.15s;
}
.side-resizer:hover::after { background: var(--accent); }

.icon-btn {
  display: inline-flex; align-items: center; justify-content: center;
  width: 26px; height: 26px;
  border: none; background: transparent;
  border-radius: var(--radius-sm);
  color: var(--text-3);
  cursor: pointer;
  font-size: 13px;
  transition: background 0.12s, color 0.12s;
  padding: 0;
}
.icon-btn:hover { background: var(--bg-hover); color: var(--text-1); }
.icon-btn.bordered { border: 1px solid var(--border); }

.side-actions {
  display: flex; gap: 6px;
  padding: 12px 12px 10px;
  align-items: center;
}
.search-input { flex: 1; min-width: 0; }
.new-btn { flex-shrink: 0; }

.conn-list {
  flex: 1;
  overflow-y: auto;
  padding: 2px 8px 8px;
}
.list-empty {
  display: flex; flex-direction: column; align-items: center; gap: 8px;
  padding: 56px 16px;
}
.empty-icon {
  font-size: 24px;
  color: var(--text-3);
  opacity: 0.55;
  margin-bottom: 2px;
}
.list-empty-text { color: var(--text-3); font-size: 12px; }
.empty-new-btn {
  display: inline-flex; align-items: center; gap: 6px;
  margin-top: 8px;
  padding: 7px 16px;
  background: transparent;
  border: 1px dashed var(--border-strong);
  border-radius: var(--radius-md);
  color: var(--text-2);
  font-size: 12.5px;
  cursor: pointer;
  transition: border-color 0.15s var(--ease), color 0.15s, background 0.15s, transform 0.1s;
}
.empty-new-btn:hover {
  border-color: var(--accent);
  border-style: solid;
  color: var(--accent);
  background: var(--accent-soft);
}
.empty-new-btn:active { transform: translateY(1px); }

.group { margin-top: 6px; }
.group-header {
  display: flex; align-items: center;
  padding: 6px 8px 4px;
  cursor: pointer;
  user-select: none;
}
.group-name {
  flex: 1;
  font-size: 10.5px; font-weight: 600;
  letter-spacing: 0.8px;
  text-transform: uppercase;
  color: var(--text-3);
}
.group-count {
  font-size: 10px;
  color: var(--text-3);
  opacity: 0.8;
}
.group-items { display: flex; flex-direction: column; gap: 1px; }

.conn-item {
  display: flex; align-items: center; gap: 9px;
  padding: 7px 8px;
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: background 0.12s var(--ease);
  position: relative;
}
.conn-item:hover { background: var(--bg-hover); }
.conn-item.active { background: var(--bg-active); }
.conn-item.active::before {
  content: '';
  position: absolute;
  left: -8px; top: 50%;
  transform: translateY(-50%);
  width: 2px; height: 16px;
  border-radius: 1px;
  background: var(--accent);
}
.conn-color { width: 3px; height: 24px; border-radius: 2px; flex-shrink: 0; opacity: 0.9; }
.conn-meta { flex: 1; min-width: 0; }
.conn-name {
  font-size: 12.5px; font-weight: 500;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  line-height: 1.3;
  display: flex; align-items: center; gap: 6px;
}
.conn-loading { font-size: 11px; color: var(--text-3); }
.conn-addr {
  font-size: 10.5px; color: var(--text-3);
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.item-more {
  display: none;
  align-items: center; justify-content: center;
  width: 20px; height: 20px;
  border: none; background: transparent;
  border-radius: 4px;
  color: var(--text-3);
  cursor: pointer;
  font-size: 12px;
  padding: 0;
  flex-shrink: 0;
}
.conn-item:hover .item-more { display: inline-flex; }
.item-more:hover { background: var(--bg-active); color: var(--text-1); }

.side-footer {
  display: flex; align-items: center; justify-content: space-between;
  padding: 9px 14px;
  border-top: 1px solid var(--border);
}
.footer-version {
  font-size: 10.5px;
  color: var(--text-3);
  border: none;
  background: transparent;
  padding: 2px 4px;
  border-radius: 4px;
  cursor: pointer;
  transition: color 0.12s, background 0.12s;
}
.footer-version:hover { color: var(--text-1); background: var(--bg-hover); }
.footer-actions { display: flex; gap: 2px; }

/* ---------- 关于弹窗 ---------- */
.about-body {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  padding: 8px 0 4px;
  text-align: center;
}
.about-logo {
  width: 56px; height: 56px;
  border-radius: 14px;
  background: linear-gradient(145deg, #ef5b43, #bd3c27);
  box-shadow: 0 6px 18px rgba(224, 69, 54, 0.35);
  margin-bottom: 6px;
  position: relative;
}
.about-logo::after {
  content: '';
  position: absolute;
  inset: 16px 12px;
  border-radius: 4px;
  background:
    linear-gradient(rgba(255,255,255,0.92), rgba(255,255,255,0.92)) top / 100% 5px no-repeat,
    linear-gradient(rgba(255,255,255,0.72), rgba(255,255,255,0.72)) center / 100% 5px no-repeat,
    linear-gradient(rgba(255,255,255,0.5), rgba(255,255,255,0.5)) bottom / 100% 5px no-repeat;
}
.about-name { font-size: 15px; font-weight: 600; color: var(--text-1); }
.about-version { font-size: 12px; color: var(--text-3); }
.about-desc { font-size: 12px; color: var(--text-2); margin: 2px 0 8px; }
.about-security {
  display: inline-flex; align-items: center; gap: 5px;
  font-size: 11px;
  color: var(--success, #3f9d63);
  background: rgba(63, 157, 99, 0.1);
  border-radius: var(--radius-sm);
  padding: 3px 9px;
  margin-bottom: 12px;
}
</style>
