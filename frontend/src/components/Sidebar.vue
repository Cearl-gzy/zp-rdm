<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, h } from 'vue'
import { message, Modal } from 'ant-design-vue'
import {
  PlusOutlined, SearchOutlined, BulbOutlined, BulbFilled,
  ImportOutlined, ExportOutlined, DatabaseOutlined,
  LoadingOutlined, MoreOutlined, ControlOutlined, ThunderboltOutlined,
  CloudSyncOutlined, SafetyCertificateOutlined, LockOutlined, FileProtectOutlined,
} from '@ant-design/icons-vue'
import { PickOpenFile, PickSaveFile, CheckUpdate, DownloadUpdate, ApplyUpdate } from '../../wailsjs/go/main/App'
import { PingRTT } from '../../wailsjs/go/service/ConnectionService'
import { BrowserOpenURL, Environment } from '../../wailsjs/runtime/runtime'
import { useI18n } from 'vue-i18n'
import { setLocale, localeOptions } from '../i18n'
import { useAppStore } from '../stores/app'
import { useConnectionStore } from '../stores/connection'
import { onAppEvent, AppEvents } from '../utils/events'
import { envColor } from '../utils/env'
import type { ConnectionProfile, UpdateInfo } from '../types'
import ConnectionForm from './ConnectionForm.vue'
import UpdateProgress from './UpdateProgress.vue'
import AuditViewer from './AuditViewer.vue'

const app = useAppStore()
const conn = useConnectionStore()
const { t, locale } = useI18n()

function onLangChange(v: string) { setLocale(v as 'zh-CN' | 'en') }

const keyword = ref('')
const formVisible = ref(false)
const editing = ref<ConnectionProfile | null>(null)
const auditVisible = ref(false)

// ---------- 分组管理（重命名 / 移动连接） ----------
const giVisible = ref(false)
const giTitle = ref('')
const giValue = ref('')
const giSaving = ref(false)
let giAction: ((v: string) => Promise<void>) | null = null

function openGroupInput(title: string, initial: string, action: (v: string) => Promise<void>) {
  giTitle.value = title
  giValue.value = initial
  giAction = action
  giVisible.value = true
}

async function onGroupInputOk() {
  const v = giValue.value.trim()
  if (!v || !giAction) return
  giSaving.value = true
  try {
    await giAction(v)
    giVisible.value = false
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    giSaving.value = false
  }
}

function openRenameGroup(name: string) {
  openGroupInput(t('sidebar.renameGroup'), name, async v => {
    if (v === name) return
    await conn.renameGroup(name, v)
    message.success(t('sidebar.groupRenamed'))
  })
}

function openNewGroupInput(p: ConnectionProfile) {
  openGroupInput(t('sidebar.newGroup'), '', async v => {
    await conn.setGroup(p.id, v)
    message.success(t('sidebar.groupMoved', { name: v }))
  })
}

// 可移动到的目标分组（排除当前所在分组）
function moveTargets(p: ConnectionProfile): string[] {
  return conn.groupNames.filter(n => n !== p.group)
}

async function onMoveGroup(p: ConnectionProfile, g: string) {
  try {
    await conn.setGroup(p.id, g)
    message.success(t('sidebar.groupMoved', { name: g || t('sidebar.defaultGroup') }))
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  }
}

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

function openForm(p?: ConnectionProfile) {
  editing.value = p ?? null
  formVisible.value = true
}

// 环境标签文本（prod/test/dev → 生产/测试/开发）
function envLabel(env?: string): string {
  if (!env) return ''
  const key = 'env' + env.charAt(0).toUpperCase() + env.slice(1)
  return t(`connection.${key}`)
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
    title: t('sidebar.deleteTitle'),
    content: t('sidebar.deleteContent', { name: p.name }),
    okText: t('common.delete'),
    okButtonProps: { danger: true },
    cancelText: t('common.cancel'),
    async onOk() {
      await conn.remove(p.id)
      message.success(t('sidebar.deleted'))
    },
  })
}

async function onExport() {
  try {
    const path = await PickSaveFile(t('sidebar.exportDialogTitle'), 'zp-rdm-connections.json')
    if (!path) return
    await conn.exportTo(path)
    message.success(t('sidebar.exportSuccess'))
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  }
}

async function onImport() {
  try {
    const path = await PickOpenFile(t('sidebar.importDialogTitle'))
    if (!path) return
    const n = await conn.importFrom(path)
    message.success(t('sidebar.importSuccess', { n }))
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  }
}

// ---------- 已打开连接的 PING RTT（5s 轮询） ----------
const rtts = ref<Record<string, number>>({})
let rttTimer: ReturnType<typeof setInterval> | null = null
async function pollRtt() {
  const ids = [...conn.openIds]
  await Promise.all(ids.map(async id => {
    try {
      rtts.value[id] = await PingRTT(id)
    } catch {
      delete rtts.value[id] // 失败时隐藏徽标
    }
  }))
}
function rttClass(ms: number): string {
  return ms < 50 ? 'rtt-good' : ms < 200 ? 'rtt-mid' : 'rtt-bad'
}

// 命令面板 / 快捷键触发的动作
const offHandlers: (() => void)[] = []
onMounted(() => {
  offHandlers.push(
    onAppEvent(AppEvents.NewConnection, () => openForm()),
    onAppEvent(AppEvents.ImportConnections, onImport),
    onAppEvent(AppEvents.ExportConnections, onExport),
  )
  pollRtt()
  rttTimer = setInterval(pollRtt, 5000)
})
onBeforeUnmount(() => {
  offHandlers.forEach(off => off())
  if (rttTimer) clearInterval(rttTimer)
})

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
// 平台感知的密码存储说明
const platform = ref('')
const securityText = computed(() => {
  if (platform.value === 'windows') return t('sidebar.securityWin')
  if (platform.value === 'darwin') return t('sidebar.securityMac')
  return t('sidebar.securityLinux')
})
onMounted(async () => {
  try { platform.value = (await Environment()).platform } catch { /* 忽略 */ }
})
async function onCheckUpdate() {
  checking.value = true
  try {
    const info = (await CheckUpdate()) as unknown as UpdateInfo
    if (!info.hasUpdate) {
      message.success(t('sidebar.alreadyLatest'))
      return
    }
    if (info.auto) {
      // Windows + 有校验哈希 → 应用内自动下载安装
      Modal.confirm({
        title: t('sidebar.updateTitle'),
        content: t('sidebar.updateAutoContent', { ver: info.version }),
        okText: t('sidebar.updateAutoOk'),
        cancelText: t('common.cancel'),
        onOk: () => runAutoUpdate(),
      })
    } else {
      Modal.confirm({
        title: t('sidebar.updateTitle'),
        content: t('sidebar.updateContent'),
        okText: t('sidebar.updateOk'),
        cancelText: t('common.cancel'),
        onOk: () => BrowserOpenURL(info.url),
      })
    }
  } catch (e: any) {
    message.warning(String(e?.message ?? e))
  } finally {
    checking.value = false
  }
}

// 自动更新：进度弹窗 → 下载+校验 → 确认重启 → ApplyUpdate 退出并静默安装
function runAutoUpdate() {
  const inst = Modal.info({
    title: t('sidebar.updateDownloading'),
    content: h(UpdateProgress, {
      verifyingText: t('sidebar.updateVerifying'),
      onDone: () => {
        inst.destroy()
        Modal.confirm({
          title: t('sidebar.updateReady'),
          content: t('sidebar.updateReadyContent'),
          okText: t('sidebar.updateRestart'),
          cancelText: t('common.cancel'),
          onOk: async () => {
            try {
              await ApplyUpdate()
              // 应用随即自动退出；若退出失败给出提示
            } catch (e: any) {
              message.error(String(e?.message ?? e))
            }
          },
        })
      },
    }),
    // AntDV 类型未暴露 style，但运行时支持（隐藏 OK 按钮，进度完成自动关闭）
    okButtonProps: { style: 'display: none' } as any,
    closable: false,
    maskClosable: false,
    keyboard: false,
  })
  DownloadUpdate().catch((e: any) => {
    inst.destroy()
    message.error(t('sidebar.updateFailed') + ': ' + String(e?.message ?? e))
  })
}
</script>

<template>
  <div class="sidebar" :style="{ width: app.sidebarWidth + 'px' }">
    <div class="side-resizer" @mousedown="startResize" />
    <div class="side-actions">
      <a-input
        v-model:value="keyword"
        :placeholder="t('sidebar.searchPlaceholder')"
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
              <ImportOutlined /> {{ t('sidebar.importConnections') }}
            </a-menu-item>
            <a-menu-item key="export" @click="onExport">
              <ExportOutlined /> {{ t('sidebar.exportConnections') }}
            </a-menu-item>
          </a-menu>
        </template>
      </a-dropdown>
      <a-tooltip :title="t('sidebar.newTooltip')">
        <a-button type="primary" size="small" class="new-btn" @click="openForm()">
          <template #icon><PlusOutlined /></template>
          {{ t('sidebar.new') }}
        </a-button>
      </a-tooltip>
    </div>

    <div class="conn-list">
      <div v-if="!filteredGroups.length" class="list-empty">
        <div class="empty-icon"><DatabaseOutlined /></div>
        <div class="list-empty-text">{{ keyword ? t('sidebar.noMatch') : t('sidebar.empty') }}</div>
        <button v-if="!keyword" class="empty-new-btn" @click="openForm()">
          <PlusOutlined />
          <span>{{ t('sidebar.newConnection') }}</span>
        </button>
      </div>

      <div v-for="g in filteredGroups" :key="g.name" class="group">
        <a-dropdown :trigger="['contextmenu']">
          <div class="group-header" @click="app.toggleGroupCollapsed(g.name)">
            <span class="group-name">{{ g.name }}</span>
            <span class="group-count num">{{ g.items.length }}</span>
          </div>
          <template #overlay>
            <a-menu>
              <a-menu-item key="rename" @click="openRenameGroup(g.name)">{{ t('sidebar.renameGroup') }}</a-menu-item>
            </a-menu>
          </template>
        </a-dropdown>
        <div v-if="!app.collapsedGroups.has(g.name)" class="group-items">
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
                  <span
                    v-if="p.env"
                    class="env-tag"
                    :style="{ color: envColor(p.env), borderColor: envColor(p.env) }"
                  >{{ envLabel(p.env) }}</span>
                  <LockOutlined v-if="p.readonly" class="ro-lock" :title="t('common.readonly')" />
                  <LoadingOutlined v-if="conn.opening === p.id" class="conn-loading" />
                </div>
                <div class="conn-addr num">{{ p.host }}:{{ p.port }}{{ p.ssh?.enabled ? ' · ssh' : '' }}</div>
              </div>
              <span
                v-if="conn.openIds.has(p.id) && rtts[p.id] !== undefined"
                class="rtt num"
                :class="rttClass(rtts[p.id])"
                :title="t('sidebar.rttTip')"
              >{{ rtts[p.id] }}ms</span>
              <span class="status-dot" :class="{ on: conn.openIds.has(p.id) }" />
              <a-dropdown :trigger="['click']" placement="bottomRight">
                <button class="item-more" @click.stop><MoreOutlined /></button>
                <template #overlay>
                  <a-menu>
                    <a-menu-item key="open" @click="onOpen(p.id)">{{ t('sidebar.openConnection') }}</a-menu-item>
                    <a-menu-item key="edit" @click="openForm(p)">{{ t('common.edit') }}</a-menu-item>
                    <a-menu-item key="dup" @click="conn.duplicate(p.id).then(() => message.success(t('sidebar.duplicated')))">{{ t('common.copy') }}</a-menu-item>
                    <a-sub-menu key="move" :title="t('sidebar.moveToGroup')">
                      <a-menu-item v-if="p.group" key="__default" @click="onMoveGroup(p, '')">{{ t('sidebar.defaultGroup') }}</a-menu-item>
                      <a-menu-item v-for="gn in moveTargets(p)" :key="gn" @click="onMoveGroup(p, gn)">{{ gn }}</a-menu-item>
                      <a-menu-item key="__new" @click="openNewGroupInput(p)">{{ t('sidebar.newGroup') }}</a-menu-item>
                    </a-sub-menu>
                    <a-menu-item v-if="conn.openIds.has(p.id)" key="close" @click="conn.close(p.id)">{{ t('sidebar.closeConnection') }}</a-menu-item>
                    <a-menu-divider />
                    <a-menu-item key="del" danger @click="onDelete(p)">{{ t('common.delete') }}</a-menu-item>
                  </a-menu>
                </template>
              </a-dropdown>
            </div>
            <template #overlay>
              <a-menu>
                <a-menu-item key="open" @click="onOpen(p.id)">{{ t('sidebar.openConnection') }}</a-menu-item>
                <a-menu-item key="edit" @click="openForm(p)">{{ t('common.edit') }}</a-menu-item>
                <a-menu-item key="dup" @click="conn.duplicate(p.id).then(() => message.success(t('sidebar.duplicated')))">{{ t('common.copy') }}</a-menu-item>
                <a-sub-menu key="move" :title="t('sidebar.moveToGroup')">
                  <a-menu-item v-if="p.group" key="__default" @click="onMoveGroup(p, '')">{{ t('sidebar.defaultGroup') }}</a-menu-item>
                  <a-menu-item v-for="gn in moveTargets(p)" :key="gn" @click="onMoveGroup(p, gn)">{{ gn }}</a-menu-item>
                  <a-menu-item key="__new" @click="openNewGroupInput(p)">{{ t('sidebar.newGroup') }}</a-menu-item>
                </a-sub-menu>
                <a-menu-item v-if="conn.openIds.has(p.id)" key="close" @click="conn.close(p.id)">{{ t('sidebar.closeConnection') }}</a-menu-item>
                <a-menu-divider />
                <a-menu-item key="del" danger @click="onDelete(p)">{{ t('common.delete') }}</a-menu-item>
              </a-menu>
            </template>
          </a-dropdown>
        </div>
      </div>
    </div>

    <div class="side-footer">
      <a-tooltip :title="t('sidebar.aboutUpdate')">
        <button class="footer-version num" @click="aboutVisible = true">
          ZP RDM v{{ app.version || '…' }}
        </button>
      </a-tooltip>
      <div class="footer-actions">
        <a-tooltip :title="t('audit.openTooltip')">
          <button class="icon-btn" @click="auditVisible = true">
            <FileProtectOutlined />
          </button>
        </a-tooltip>
        <a-tooltip :title="t('sidebar.paletteTooltip')">
          <button class="icon-btn" @click="app.togglePalette(true)">
            <ThunderboltOutlined />
          </button>
        </a-tooltip>
        <a-tooltip :title="t('app.language')">
          <a-select :value="locale" size="small" class="lang-select" :bordered="false" :options="localeOptions" @change="onLangChange" />
        </a-tooltip>
        <a-tooltip :title="app.dark ? t('sidebar.switchLight') : t('sidebar.switchDark')">
          <button class="icon-btn" @click="app.toggleTheme()">
            <component :is="app.dark ? BulbFilled : BulbOutlined" />
          </button>
        </a-tooltip>
      </div>
    </div>

    <ConnectionForm v-model:open="formVisible" :profile="editing" />
    <AuditViewer v-model:open="auditVisible" />

    <a-modal
      v-model:open="giVisible"
      :title="giTitle"
      :width="360"
      :ok-text="t('common.ok')"
      :cancel-text="t('common.cancel')"
      :confirm-loading="giSaving"
      @ok="onGroupInputOk"
    >
      <a-input
        v-model:value="giValue"
        :placeholder="t('sidebar.groupNamePlaceholder')"
        allow-clear
        style="margin-top: 8px"
        @keydown.enter="onGroupInputOk"
      />
    </a-modal>

    <a-modal v-model:open="aboutVisible" :title="t('sidebar.aboutTitle')" :width="380" :footer="null">
      <div class="about-body">
        <div class="about-logo" />
        <div class="about-name">ZP Redis Manager</div>
        <div class="about-version num">v{{ app.version || '…' }}</div>
        <div class="about-desc">{{ t('sidebar.aboutDesc') }}</div>
        <div class="about-security">
          <SafetyCertificateOutlined />
          <span>{{ securityText }}</span>
        </div>
        <a-button block :loading="checking" @click="onCheckUpdate">
          <template v-if="!checking" #icon><CloudSyncOutlined /></template>
          {{ checking ? t('sidebar.checking') : t('sidebar.checkUpdate') }}
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
.env-tag {
  font-size: 10px; line-height: 1.4; padding: 0 4px;
  border: 1px solid; border-radius: 3px; flex-shrink: 0;
}
.ro-lock { font-size: 11px; color: var(--text-3); flex-shrink: 0; }
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

.rtt { font-size: 10px; flex-shrink: 0; letter-spacing: 0.2px; }
.rtt-good { color: var(--success, #3f9d63); }
.rtt-mid { color: #f5a623; }
.rtt-bad { color: #ff6b60; }

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
.footer-actions { display: flex; gap: 2px; align-items: center; }
.lang-select { width: 88px; }

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
