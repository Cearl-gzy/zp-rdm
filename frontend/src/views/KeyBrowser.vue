<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { message, Modal } from 'ant-design-vue'
import {
  SearchOutlined, ReloadOutlined, PlusOutlined, DeleteOutlined,
  FolderOutlined, UnorderedListOutlined, PartitionOutlined, DatabaseOutlined,
  MoreOutlined, DownloadOutlined, UploadOutlined, DownOutlined,
} from '@ant-design/icons-vue'
import { ScanKeys, DeleteKeys, BatchSetTTL, MoveKeys } from '../../wailsjs/go/service/KeyService'
import { CreateKey } from '../../wailsjs/go/service/ValueService'
import { ExportKeys, ExportDatabase, ImportKeys } from '../../wailsjs/go/service/TransferService'
import { PickSaveFile, PickOpenFile } from '../../wailsjs/go/main/App'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import { useConnectionStore } from '../stores/connection'
import { useAppStore } from '../stores/app'
import { onAppEvent, AppEvents } from '../utils/events'
import type { KeyInfo, ScanResult } from '../types'
import { buildKeyTree, type KeyNode } from '../utils/keyTree'
import { formatBytes, formatTTL } from '../utils/format'
import ValuePanel from './ValuePanel.vue'

const props = defineProps<{ connId: string; db: number }>()
const conn = useConnectionStore()
const app = useAppStore()
const { t } = useI18n()

// 从工作区会话恢复浏览状态
const savedWs = app.workspace.conns[props.connId] ?? {}

const keys = ref<KeyInfo[]>([])
const cursor = ref(0)
const total = ref(0)
const loading = ref(false)
const pattern = ref(savedWs.pattern ?? '')
const typeFilter = ref('')
const viewMode = ref<'tree' | 'list'>(savedWs.viewMode === 'list' ? 'list' : 'tree')
const delimiter = ref(savedWs.delimiter || ':')
const selectedKey = ref(savedWs.selectedKey ?? '')
const checkedKeys = ref<string[]>([])
// 树展开目录：从工作区恢复并持久化
const expandedKeys = ref<string[]>(savedWs.expandedKeys ?? [])
const treeRef = ref<{ scrollTo?: (scroll: { key: string; align?: string }) => void } | null>(null)

watch(expandedKeys, v => {
  app.patchConnWorkspace(props.connId, { expandedKeys: [...v] })
}, { deep: true })

// 目录前缀链：'a:b:c' → ['a', 'a:b']（不含叶子本身）
function ancestorDirs(key: string): string[] {
  const parts = key.split(delimiter.value)
  const out: string[] = []
  let p = ''
  for (let i = 0; i < parts.length - 1; i++) {
    p = i === 0 ? parts[i] : p + delimiter.value + parts[i]
    out.push(p)
  }
  return out
}

// 展开到指定键（列表模式也记录展开状态），树模式下滚动到可视区域
function expandToKey(key: string) {
  if (!key) return
  const set = new Set(expandedKeys.value)
  let changed = false
  for (const d of ancestorDirs(key)) {
    if (!set.has(d)) { set.add(d); changed = true }
  }
  if (changed) expandedKeys.value = [...set]
  if (viewMode.value === 'tree') {
    nextTick(() => treeRef.value?.scrollTo?.({ key, align: 'top' }))
  }
}

// 浏览状态变化 → 防抖持久化到工作区
watch([pattern, viewMode, delimiter], () => {
  app.patchConnWorkspace(props.connId, {
    pattern: pattern.value, viewMode: viewMode.value, delimiter: delimiter.value,
  })
})
watch(selectedKey, v => app.patchConnWorkspace(props.connId, { selectedKey: v }))

const PAGE_SIZE = 500

const treeData = computed<KeyNode[]>(() => buildKeyTree(keys.value, delimiter.value))

const keyTypes = ['string', 'hash', 'list', 'set', 'zset', 'stream', 'ReJSON-RL']

// 新建键可选类型：ReJSON-RL 模块加载时才提供 json（CreateKey 用 'json' 分支）
const hasJsonModule = computed(() =>
  (conn.openTabs.find(tab => tab.id === props.connId)?.state?.modules ?? []).some(m => /rejson/i.test(m)),
)
const createTypes = computed(() =>
  hasJsonModule.value ? ['string', 'hash', 'list', 'set', 'zset', 'stream', 'json'] : keyTypes.slice(0, 6),
)
const jsonExample = '{"a": 1}'

async function scan(reset = false) {
  if (loading.value) return
  loading.value = true
  try {
    if (reset) { keys.value = []; cursor.value = 0 }
    const res = (await ScanKeys({
      connId: props.connId,
      db: props.db,
      cursor: cursor.value,
      pattern: pattern.value.trim(),
      count: PAGE_SIZE,
      type: typeFilter.value,
    } as any)) as unknown as ScanResult
    cursor.value = Number(res.cursor)
    total.value = res.total
    const existing = new Set(keys.value.map(k => k.key))
    for (const k of res.keys || []) {
      if (!existing.has(k.key)) keys.value.push(k)
    }
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    loading.value = false
  }
}

function refresh(keepSelection = false) {
  if (!keepSelection) selectedKey.value = ''
  checkedKeys.value = []
  scan(true)
}

// 首次加载保留工作区恢复的选中键，之后切换 db/连接才清空
let firstLoad = true
watch(() => [props.db, props.connId], async () => {
  if (firstLoad) {
    await scan(true)
    firstLoad = false
    // 恢复上次会话：展开到选中键并滚动到位
    if (selectedKey.value) expandToKey(selectedKey.value)
    return
  }
  refresh(false)
}, { immediate: true })

// F5 / Ctrl+R 刷新：仅当前激活标签页的浏览器响应
let offRefresh: (() => void) | null = null
// 搜索面板"打开键"：选中并展开定位（ConnectionView 负责切到数据视图）
let offOpenKey: (() => void) | null = null
onMounted(() => {
  offRefresh = onAppEvent(AppEvents.Refresh, () => {
    if (conn.activeTab === props.connId) refresh()
  })
  offOpenKey = onAppEvent<{ connId: string; key: string }>(AppEvents.OpenKey, d => {
    if (d?.connId !== props.connId || !d.key) return
    selectedKey.value = d.key
    expandToKey(d.key)
  })
})
onBeforeUnmount(() => { offRefresh?.(); offOpenKey?.() })

// ---------- 树虚拟滚动：容器高度自适应 ----------
const contentRef = ref<HTMLElement>()
const treeHeight = ref(600)
let resizeObserver: ResizeObserver | null = null
onMounted(() => {
  if (!contentRef.value) return
  treeHeight.value = contentRef.value.clientHeight || 600
  resizeObserver = new ResizeObserver(entries => {
    const h = entries[0]?.contentRect.height
    if (h && h > 0) treeHeight.value = h
  })
  resizeObserver.observe(contentRef.value)
})
onBeforeUnmount(() => resizeObserver?.disconnect())

function onTreeSelect(sel: (string | number)[], info: any) {
  const node = info.node as KeyNode & { dataRef?: KeyNode }
  const data = (node.dataRef ?? node) as KeyNode
  if (data.isLeaf) {
    selectedKey.value = data.key
  } else {
    // 点击目录时切换展开
    const k = String(data.key)
    const i = expandedKeys.value.indexOf(k)
    if (i >= 0) expandedKeys.value.splice(i, 1)
    else expandedKeys.value.push(k)
    expandedKeys.value = [...expandedKeys.value]
  }
}

function onRowClick(record: KeyInfo) {
  selectedKey.value = record.key
  expandToKey(record.key) // 列表模式选中后切回树视图时保持定位
}

// ---------- 新建键 ----------
const createVisible = ref(false)
const createForm = ref({ key: '', type: 'string', field: '', value: '', score: 0, ttl: 0 })
const creating = ref(false)

function openCreate() {
  createForm.value = { key: pattern.value && !pattern.value.includes('*') ? pattern.value : '', type: 'string', field: '', value: '', score: 0, ttl: 0 }
  createVisible.value = true
}

async function doCreate() {
  const f = createForm.value
  if (!f.key) { message.warning(t('keyBrowser.enterKeyName')); return }
  creating.value = true
  try {
    await CreateKey(props.connId, props.db, f.key, f.type, f.value, f.field, f.score, f.ttl)
    message.success(t('keyBrowser.created'))
    createVisible.value = false
    refresh()
    selectedKey.value = f.key
    expandToKey(f.key)
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    creating.value = false
  }
}

// ---------- 删除 ----------
function removeLocal(ks: string[]) {
  const set = new Set(ks)
  keys.value = keys.value.filter(k => !set.has(k.key))
  checkedKeys.value = checkedKeys.value.filter(k => !set.has(k))
  if (set.has(selectedKey.value)) selectedKey.value = ''
}

async function deleteChecked() {
  // 树勾选会包含目录节点（前缀），只删除真实存在的叶子键
  const targets = checkedLeafKeys()
  if (!targets.length) { message.warning(t('keyBrowser.noValidSelection')); return }
  Modal.confirm({
    title: t('keyBrowser.batchDeleteTitle'),
    content: t('keyBrowser.deleteConfirmContent', { n: targets.length }),
    okText: t('common.delete'),
    okButtonProps: { danger: true },
    cancelText: t('common.cancel'),
    async onOk() {
      try {
        const n = await DeleteKeys(props.connId, props.db, targets)
        message.success(t('keyBrowser.deletedN', { n }))
        removeLocal(targets)
      } catch (e: any) {
        message.error(String(e?.message ?? e))
      }
    },
  })
}

function onDeleted(key: string) { removeLocal([key]) }
function onRenamed(oldKey: string, newKey: string) {
  const item = keys.value.find(k => k.key === oldKey)
  if (item) item.key = newKey
  selectedKey.value = newKey
}

// ---------- 批量 TTL / 移动 DB ----------
const ttlVisible = ref(false)
const ttlSeconds = ref(3600)
const ttlRunning = ref(false)
const moveVisible = ref(false)
const moveTargetDb = ref(1)
const moveRunning = ref(false)

// 当前连接的 db 总数（用于移动目标上限提示）
const dbCount = computed(() => conn.openTabs.find(tab => tab.id === props.connId)?.state?.dbCount ?? 16)

function onBatchAction({ key }: { key: string }) {
  if (!checkedLeafKeys().length) { message.warning(t('keyBrowser.noValidSelection')); return }
  if (key === 'ttl') { ttlSeconds.value = 3600; ttlVisible.value = true }
  else if (key === 'ttl-clear') clearCheckedTTL()
  else if (key === 'move') { moveTargetDb.value = props.db === 0 ? 1 : 0; moveVisible.value = true }
}

async function setCheckedTTL(seconds: number) {
  const targets = checkedLeafKeys()
  ttlRunning.value = true
  try {
    const n = await BatchSetTTL(props.connId, props.db, targets, seconds)
    message.success(seconds < 0
      ? t('keyBrowser.ttlClearedN', { n })
      : t('keyBrowser.ttlSetN', { n, s: seconds }))
    if (seconds < 0) return
    ttlVisible.value = false
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    ttlRunning.value = false
  }
}

function clearCheckedTTL() {
  Modal.confirm({
    title: t('keyBrowser.clearTtlTitle'),
    content: t('keyBrowser.clearTtlContent'),
    okText: t('keyBrowser.clearText'),
    cancelText: t('common.cancel'),
    onOk: () => setCheckedTTL(-1),
  })
}

async function doMoveChecked() {
  const targets = checkedLeafKeys()
  if (moveTargetDb.value === props.db) { message.warning(t('keyBrowser.sameDb')); return }
  moveRunning.value = true
  try {
    const n = await MoveKeys(props.connId, props.db, targets, moveTargetDb.value)
    const skipped = targets.length - n
    message.success(skipped > 0
      ? t('keyBrowser.movedNSkipped', { n, db: moveTargetDb.value, skipped })
      : t('keyBrowser.movedN', { n, db: moveTargetDb.value }))
    moveVisible.value = false
    // 移动的键已不在当前 db，先从本地列表移除移动成功的部分无法区分，直接刷新
    refresh()
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    moveRunning.value = false
  }
}

// ---------- 数据导入导出 ----------
// 勾选中过滤出真实叶子键
function checkedLeafKeys(): string[] {
  const realKeys = new Set(keys.value.map(k => k.key))
  return checkedKeys.value.filter(k => realKeys.has(k))
}

const exportVisible = ref(false)
const exportFormat = ref<'json' | 'csv'>('json')
const exporting = ref(false)

function openExport() {
  if (!checkedLeafKeys().length) { message.warning(t('keyBrowser.checkKeysFirst')); return }
  exportVisible.value = true
}

async function doExportSelected() {
  const targets = checkedLeafKeys()
  try {
    const path = await PickSaveFile(t('keyBrowser.exportSelectedTitle'), `keys-${props.connId}-db${props.db}.${exportFormat.value}`)
    if (!path) return
    exporting.value = true
    const n = await ExportKeys(props.connId, props.db, targets, exportFormat.value, path)
    message.success(t('keyBrowser.exportedN', { n }))
    exportVisible.value = false
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    exporting.value = false
  }
}

// 整库导出（JSONL + 进度）
const exportDbVisible = ref(false)
const exportDbPattern = ref('')
const exportDbRunning = ref(false)
const exportDbDone = ref(0)

async function doExportDatabase() {
  let path = ''
  try {
    path = await PickSaveFile(t('keyBrowser.exportDbTitle'), `dump-${props.connId}-db${props.db}.jsonl`)
  } catch { return }
  if (!path) return
  exportDbVisible.value = false
  exportDbRunning.value = true
  exportDbDone.value = 0
  EventsOn('export:progress', (n: number) => { exportDbDone.value = n })
  try {
    const n = await ExportDatabase(props.connId, props.db, exportDbPattern.value.trim() || '*', path)
    message.success(t('keyBrowser.exportDbDoneN', { n }))
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    EventsOff('export:progress')
    exportDbRunning.value = false
  }
}

// 导入
const importing = ref(false)
async function doImport() {
  let path = ''
  try {
    path = await PickOpenFile(t('keyBrowser.importTitle'))
  } catch { return }
  if (!path) return
  Modal.confirm({
    title: t('keyBrowser.importTitle'),
    content: t('keyBrowser.importConfirmContent'),
    okText: t('keyBrowser.importText'),
    cancelText: t('common.cancel'),
    async onOk() {
      importing.value = true
      try {
        const n = await ImportKeys(props.connId, props.db, path)
        message.success(t('keyBrowser.importedN', { n }))
        refresh()
      } catch (e: any) {
        message.error(String(e?.message ?? e))
      } finally {
        importing.value = false
      }
    },
  })
}

const listColumns = computed(() => [
  { title: t('keyBrowser.keyName'), dataIndex: 'key', key: 'key', ellipsis: true },
  { title: t('common.type'), dataIndex: 'type', key: 'type', width: 76 },
  { title: 'TTL', dataIndex: 'ttl', key: 'ttl', width: 80 },
  { title: t('keyBrowser.size'), dataIndex: 'size', key: 'size', width: 80 },
])
</script>

<template>
  <div class="key-browser">
    <div class="key-side">
      <div class="kb-toolbar">
        <a-input
          v-model:value="pattern"
          size="small"
          :placeholder="t('keyBrowser.patternPlaceholder')"
          allow-clear
          class="kb-search"
          @press-enter="scan(true)"
        >
          <template #prefix><SearchOutlined class="text-3" /></template>
        </a-input>
        <a-select
          v-model:value="typeFilter"
          size="small"
          allow-clear
          :placeholder="t('common.type')"
          style="width: 86px"
          @change="scan(true)"
        >
          <a-select-option v-for="kt in keyTypes" :key="kt" :value="kt">{{ kt }}</a-select-option>
        </a-select>
        <a-tooltip :title="t('keyBrowser.refreshTooltip')">
          <a-button size="small" :loading="loading" @click="refresh">
            <template #icon><ReloadOutlined /></template>
          </a-button>
        </a-tooltip>
        <a-tooltip :title="t('keyBrowser.newKeyTooltip')">
          <a-button size="small" type="primary" @click="openCreate">
            <template #icon><PlusOutlined /></template>
          </a-button>
        </a-tooltip>
      </div>

      <div class="kb-toolbar2">
        <div class="view-toggle">
          <button
            class="vt-btn"
            :class="{ active: viewMode === 'tree' }"
            :title="t('keyBrowser.treeView')"
            @click="viewMode = 'tree'"
          ><PartitionOutlined /></button>
          <button
            class="vt-btn"
            :class="{ active: viewMode === 'list' }"
            :title="t('keyBrowser.listView')"
            @click="viewMode = 'list'"
          ><UnorderedListOutlined /></button>
        </div>
        <a-tooltip :title="t('keyBrowser.delimiterTooltip')">
          <a-select v-model:value="delimiter" size="small" style="width: 64px">
            <a-select-option value=":">:</a-select-option>
            <a-select-option value="/">/</a-select-option>
            <a-select-option value=".">.</a-select-option>
            <a-select-option value="_">_</a-select-option>
            <a-select-option value="-">-</a-select-option>
          </a-select>
        </a-tooltip>
        <a-dropdown :trigger="['click']" placement="bottomRight">
          <a-button size="small" :loading="exportDbRunning || importing">
            <template #icon><MoreOutlined /></template>
          </a-button>
          <template #overlay>
            <a-menu>
              <a-menu-item key="export-sel" @click="openExport">
                <DownloadOutlined /> {{ t('keyBrowser.exportSelectedMenu', { n: checkedLeafKeys().length }) }}
              </a-menu-item>
              <a-menu-item key="export-db" @click="exportDbVisible = true">
                <DownloadOutlined /> {{ t('keyBrowser.exportDbMenu') }}
              </a-menu-item>
              <a-menu-divider />
              <a-menu-item key="import" @click="doImport">
                <UploadOutlined /> {{ t('keyBrowser.importMenu') }}
              </a-menu-item>
            </a-menu>
          </template>
        </a-dropdown>
        <transition name="fade">
          <div v-if="checkedKeys.length" class="batch-bar">
            <a-dropdown trigger="click">
              <a-button size="small">
                {{ t('keyBrowser.batchActions', { n: checkedKeys.length }) }} <DownOutlined />
              </a-button>
              <template #overlay>
                <a-menu @click="onBatchAction">
                  <a-menu-item key="ttl">{{ t('keyBrowser.setTtlMenu') }}</a-menu-item>
                  <a-menu-item key="ttl-clear">{{ t('keyBrowser.clearTtlMenu') }}</a-menu-item>
                  <a-menu-item key="move">{{ t('keyBrowser.moveToDbMenu') }}</a-menu-item>
                </a-menu>
              </template>
            </a-dropdown>
            <a-button size="small" danger class="batch-del" @click="deleteChecked">
              <template #icon><DeleteOutlined /></template>
              {{ t('common.delete') }}
            </a-button>
          </div>
        </transition>
      </div>
      <div v-if="exportDbRunning" class="export-progress">
        <span class="text-2">{{ t('keyBrowser.exportingDb') }}</span>
        <span class="num text-2">{{ t('keyBrowser.keysCount', { n: exportDbDone }) }}</span>
      </div>

      <div ref="contentRef" class="key-content" :class="{ 'vs-mode': viewMode === 'tree' }">
        <a-tree
          v-if="viewMode === 'tree'"
          ref="treeRef"
          :tree-data="treeData"
          :field-names="{ title: 'title', key: 'key', children: 'children' }"
          v-model:expanded-keys="expandedKeys"
          v-model:checked-keys="checkedKeys"
          checkable
          :selectable="true"
          :selected-keys="selectedKey ? [selectedKey] : []"
          :height="treeHeight"
          block-node
          class="key-tree"
          @select="onTreeSelect"
        >
          <template #title="node">
            <span v-if="!node.isLeaf" class="folder-node">
              <FolderOutlined class="folder-icon" />
              <span class="folder-name">{{ node.title }}</span>
              <span class="folder-count num">{{ node.count }}</span>
            </span>
            <span v-else class="leaf-node">
              <span class="type-badge" :class="`type-${node.type}`">{{ node.type }}</span>
              <span class="leaf-title mono" :title="node.key">{{ node.title }}</span>
              <span
                v-if="node.ttl > 0"
                class="leaf-ttl num"
                :class="{ urgent: node.ttl < 300 }"
              >{{ formatTTL(node.ttl) }}</span>
            </span>
          </template>
        </a-tree>

        <a-table
          v-else
          :columns="listColumns"
          :data-source="keys"
          :pagination="{
            pageSize: 100,
            size: 'small',
            showSizeChanger: true,
            pageSizeOptions: ['50', '100', '200', '500'],
            hideOnSinglePage: true,
            showTotal: (n: number) => t('keyBrowser.keysCount', { n }),
          }"
          size="small"
          row-key="key"
          :custom-row="(r: KeyInfo) => ({ onClick: () => onRowClick(r) })"
          :row-class-name="(r: KeyInfo) => r.key === selectedKey ? 'row-selected' : ''"
          class="key-table"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'type'">
              <span class="type-badge" :class="`type-${record.type}`">{{ record.type }}</span>
            </template>
            <template v-else-if="column.key === 'ttl'">
              <span class="mono text-2">{{ formatTTL(record.ttl) }}</span>
            </template>
            <template v-else-if="column.key === 'size'">
              <span class="mono text-2">{{ formatBytes(record.size) }}</span>
            </template>
            <template v-else-if="column.key === 'key'">
              <span class="mono" :title="record.key">{{ record.key }}</span>
            </template>
          </template>
        </a-table>
      </div>

      <div class="key-footer">
        <span class="footer-stat num">{{ keys.length }}<span class="footer-sep">/</span>{{ total }} {{ t('keyBrowser.keysUnit') }}</span>
        <a-button v-if="cursor !== 0" size="small" type="link" :loading="loading" @click="scan(false)">
          {{ t('common.loadMore') }}
        </a-button>
        <span v-else-if="keys.length" class="footer-done">{{ t('keyBrowser.allLoaded') }}</span>
      </div>
    </div>

    <div class="value-side">
      <ValuePanel
        v-if="selectedKey"
        :key="`${connId}:${db}:${selectedKey}`"
        :conn-id="connId"
        :db="db"
        :key-name="selectedKey"
        @deleted="onDeleted"
        @renamed="onRenamed"
      />
      <div v-else class="value-empty">
        <div class="empty-inner">
          <DatabaseOutlined class="empty-icon" />
          <div class="empty-title">{{ t('keyBrowser.emptyTitle') }}</div>
          <div class="empty-sub">{{ t('keyBrowser.emptySub') }}</div>
        </div>
      </div>
    </div>

    <a-modal
      v-model:open="createVisible"
      :title="t('keyBrowser.createTitle')"
      width="520px"
      :confirm-loading="creating"
      :ok-text="t('common.create')"
      :cancel-text="t('common.cancel')"
      @ok="doCreate"
    >
      <a-form layout="vertical" size="small">
        <div style="display: flex; gap: 12px">
          <a-form-item :label="t('keyBrowser.keyName')" required style="flex: 2">
            <a-input v-model:value="createForm.key" placeholder="user:1001" class="mono" />
          </a-form-item>
          <a-form-item :label="t('common.type')" style="flex: 1">
            <a-select v-model:value="createForm.type">
              <a-select-option v-for="kt in createTypes" :key="kt" :value="kt">
                <span class="type-badge" :class="`type-${kt}`">{{ kt }}</span>
              </a-select-option>
            </a-select>
          </a-form-item>
        </div>
        <a-form-item v-if="['hash', 'stream'].includes(createForm.type)" :label="t('keyBrowser.fieldName')">
          <a-input v-model:value="createForm.field" placeholder="field" class="mono" />
        </a-form-item>
        <a-form-item :label="createForm.type === 'zset' ? t('keyBrowser.member') : createForm.type === 'string' ? t('common.value') : createForm.type === 'json' ? t('keyBrowser.jsonDoc') : t('keyBrowser.initialValue')">
          <a-textarea v-model:value="createForm.value" :rows="3" class="mono" :placeholder="createForm.type === 'json' ? jsonExample : ''" />
        </a-form-item>
        <a-form-item v-if="createForm.type === 'zset'" :label="t('keyBrowser.scoreLabel')">
          <a-input-number v-model:value="createForm.score" style="width: 100%" />
        </a-form-item>
        <a-form-item :label="t('keyBrowser.ttlLabel')">
          <a-input-number v-model:value="createForm.ttl" :min="0" style="width: 100%" />
        </a-form-item>
      </a-form>
    </a-modal>

    <!-- 导出选中键 -->
    <a-modal
      v-model:open="exportVisible"
      :title="t('keyBrowser.exportSelectedTitle')"
      width="420px"
      :confirm-loading="exporting"
      :ok-text="t('keyBrowser.exportText')"
      :cancel-text="t('common.cancel')"
      @ok="doExportSelected"
    >
      <a-form layout="vertical" size="small">
        <a-form-item :label="t('keyBrowser.exportFormatLabel', { n: checkedLeafKeys().length })">
          <a-radio-group v-model:value="exportFormat">
            <a-radio-button value="json">{{ t('keyBrowser.exportJsonOption') }}</a-radio-button>
            <a-radio-button value="csv">{{ t('keyBrowser.exportCsvOption') }}</a-radio-button>
          </a-radio-group>
        </a-form-item>
        <div class="text-3" style="font-size: 11px; line-height: 1.6">
          {{ t('keyBrowser.exportJsonHint') }}
        </div>
      </a-form>
    </a-modal>

    <!-- 导出整库 -->
    <a-modal
      v-model:open="exportDbVisible"
      :title="t('keyBrowser.exportDbTitle')"
      width="420px"
      :ok-text="t('keyBrowser.startExport')"
      :cancel-text="t('common.cancel')"
      @ok="doExportDatabase"
    >
      <a-form layout="vertical" size="small">
        <a-form-item :label="t('keyBrowser.exportPatternLabel')">
          <a-input v-model:value="exportDbPattern" :placeholder="t('keyBrowser.patternExample')" class="mono" @press-enter="doExportDatabase" />
        </a-form-item>
        <div class="text-3" style="font-size: 11px; line-height: 1.6">
          {{ t('keyBrowser.exportDbHint') }}
        </div>
      </a-form>
    </a-modal>

    <!-- 批量设置 TTL -->
    <a-modal
      v-model:open="ttlVisible"
      :title="t('keyBrowser.batchTtlTitle')"
      width="420px"
      :ok-text="t('keyBrowser.setText')"
      :cancel-text="t('common.cancel')"
      :confirm-loading="ttlRunning"
      @ok="setCheckedTTL(ttlSeconds)"
    >
      <a-form layout="vertical" size="small">
        <a-form-item :label="t('keyBrowser.ttlApplyLabel', { n: checkedLeafKeys().length })">
          <a-input-number v-model:value="ttlSeconds" :min="1" style="width: 100%" />
        </a-form-item>
        <div class="text-3" style="font-size: 11px">
          {{ t('keyBrowser.ttlPresets') }}
        </div>
      </a-form>
    </a-modal>

    <!-- 批量移动到其他 DB -->
    <a-modal
      v-model:open="moveVisible"
      :title="t('keyBrowser.batchMoveTitle')"
      width="420px"
      :ok-text="t('keyBrowser.moveText')"
      :cancel-text="t('common.cancel')"
      :confirm-loading="moveRunning"
      @ok="doMoveChecked"
    >
      <a-form layout="vertical" size="small">
        <a-form-item :label="t('keyBrowser.moveTargetLabel', { db, count: dbCount })">
          <a-input-number v-model:value="moveTargetDb" :min="0" :max="dbCount - 1" style="width: 100%" />
        </a-form-item>
        <div class="text-3" style="font-size: 11px; line-height: 1.6">
          {{ t('keyBrowser.moveHint') }}
        </div>
      </a-form>
    </a-modal>
  </div>
</template>

<style scoped>
.key-browser { height: 100%; display: flex; }
.key-side {
  width: 42%;
  min-width: 330px;
  display: flex;
  flex-direction: column;
  border-right: 1px solid var(--border);
}
.kb-toolbar {
  display: flex; gap: 6px; padding: 10px 12px 8px;
}
.kb-search { flex: 1; min-width: 0; }
.kb-toolbar2 {
  display: flex; gap: 8px; padding: 0 12px 10px;
  align-items: center;
}
.view-toggle {
  display: inline-flex;
  background: var(--bg-inset);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  padding: 2px;
  gap: 2px;
}
.vt-btn {
  display: inline-flex; align-items: center; justify-content: center;
  width: 24px; height: 20px;
  border: none; background: transparent;
  border-radius: var(--radius-sm);
  color: var(--text-3);
  cursor: pointer;
  font-size: 12px;
  transition: all 0.15s var(--ease);
  padding: 0;
}
.vt-btn:hover { color: var(--text-1); }
.vt-btn.active {
  background: var(--bg-elevated);
  color: var(--text-1);
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.15), inset 0 0 0 1px var(--border-strong);
}
.batch-bar { margin-left: auto; display: flex; align-items: center; gap: 8px; }
.export-progress {
  display: flex; align-items: center; justify-content: space-between;
  padding: 4px 14px;
  font-size: 11px;
  background: var(--accent-soft);
  border-top: 1px solid var(--border);
  flex-shrink: 0;
}

.key-content { flex: 1; overflow: auto; padding: 0 8px; }
/* 树虚拟滚动模式：滚动交给 a-tree 内部的 virtual list，外层只负责定高 */
.key-content.vs-mode { overflow: hidden; }
.key-tree { background: transparent; font-size: 12.5px; }
.key-tree :deep(.ant-tree-node-content-wrapper) {
  display: inline-flex; min-width: 0;
  border-radius: var(--radius-sm);
  transition: background 0.1s;
}
.key-tree :deep(.ant-tree-node-selected) { background: var(--accent-soft) !important; }
.key-tree :deep(.ant-tree-checkbox) { margin-top: 2px; }
.folder-node { display: inline-flex; align-items: center; gap: 6px; }
.folder-icon { color: var(--warning); font-size: 12px; opacity: 0.85; }
.folder-name { color: var(--text-2); font-weight: 500; }
.folder-count { font-size: 10px; color: var(--text-3); }
.leaf-node { display: inline-flex; align-items: center; gap: 7px; min-width: 0; }
.leaf-title { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 360px; }
.leaf-ttl { font-size: 10px; color: var(--text-3); }
.leaf-ttl.urgent { color: var(--warning); }

.key-table { height: 100%; }
.key-table :deep(.ant-table-row) { cursor: pointer; }
.key-table :deep(.row-selected) > td { background: var(--accent-soft) !important; }

.key-footer {
  display: flex; align-items: center; justify-content: space-between;
  padding: 5px 14px;
  border-top: 1px solid var(--border);
  flex-shrink: 0;
  min-height: 30px;
}
.footer-stat { font-size: 11px; color: var(--text-2); }
.footer-sep { color: var(--text-3); margin: 0 3px; }
.footer-done { font-size: 10.5px; color: var(--text-3); }
.key-footer :deep(.ant-btn-link) { font-size: 11.5px; padding: 0 4px; height: 22px; }

.value-side { flex: 1; min-width: 0; }
.value-empty {
  height: 100%; display: flex; align-items: center; justify-content: center;
}
.empty-inner { text-align: center; color: var(--text-3); }
.empty-icon { font-size: 38px; opacity: 0.3; margin-bottom: 14px; }
.empty-title { font-size: 13px; color: var(--text-2); }
.empty-sub { font-size: 11px; margin-top: 6px; opacity: 0.75; letter-spacing: 0.3px; }
</style>
