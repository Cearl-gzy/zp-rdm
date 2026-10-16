<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { message, Modal } from 'ant-design-vue'
import {
  ReloadOutlined, EditOutlined, CopyOutlined, DeleteOutlined, ClockCircleOutlined,
} from '@ant-design/icons-vue'
import { GetValue } from '../../wailsjs/go/service/ValueService'
import { DeleteKeys, RenameKey, SetTTL, CopyKey } from '../../wailsjs/go/service/KeyService'
import { CopyKeyAcross } from '../../wailsjs/go/service/TransferService'
import { useConnectionStore } from '../stores/connection'
import type { ValueDetail } from '../types'
import { formatBytes, formatTTL } from '../utils/format'
import StringEditor from './editors/StringEditor.vue'
import JSONEditor from './editors/JSONEditor.vue'
import HashEditor from './editors/HashEditor.vue'
import ListEditor from './editors/ListEditor.vue'
import SetEditor from './editors/SetEditor.vue'
import ZSetEditor from './editors/ZSetEditor.vue'
import StreamEditor from './editors/StreamEditor.vue'

const props = defineProps<{ connId: string; db: number; keyName: string }>()
const emit = defineEmits<{
  deleted: [key: string]
  renamed: [oldKey: string, newKey: string]
}>()

const detail = ref<ValueDetail | null>(null)
const loading = ref(false)
const missing = ref(false)

async function load() {
  loading.value = true
  missing.value = false
  try {
    detail.value = (await GetValue(props.connId, props.db, props.keyName)) as unknown as ValueDetail
  } catch (e: any) {
    detail.value = null
    const msg = String(e?.message ?? e)
    if (msg.includes('nil')) {
      missing.value = true
    } else {
      message.error(msg)
    }
  } finally {
    loading.value = false
  }
}

onMounted(load)

// ---------- 重命名 ----------
const renameVisible = ref(false)
const renameValue = ref('')
function openRename() {
  renameValue.value = props.keyName
  renameVisible.value = true
}
async function doRename() {
  if (!renameValue.value || renameValue.value === props.keyName) {
    renameVisible.value = false
    return
  }
  try {
    await RenameKey(props.connId, props.db, props.keyName, renameValue.value)
    message.success('重命名成功')
    emit('renamed', props.keyName, renameValue.value)
    renameVisible.value = false
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  }
}

// ---------- TTL ----------
const ttlVisible = ref(false)
const ttlValue = ref(-1)
function openTTL() {
  ttlValue.value = detail.value?.ttl ?? -1
  ttlVisible.value = true
}
async function doSetTTL() {
  try {
    await SetTTL(props.connId, props.db, props.keyName, ttlValue.value)
    message.success('TTL 已更新')
    ttlVisible.value = false
    load()
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  }
}

// ---------- 复制（支持跨连接） ----------
const conn = useConnectionStore()
const copyVisible = ref(false)
const copyValue = ref('')
const copyTargetConn = ref('')
const copyTargetDb = ref(0)
const copyReplace = ref(false)
const copying = ref(false)

const copyTargetOptions = computed(() =>
  conn.openTabs.map(t => ({ value: t.id, label: t.state.name }))
)
const targetDbCount = computed(() => {
  const tab = conn.openTabs.find(t => t.id === copyTargetConn.value)
  return tab?.state.dbCount || 16
})

function openCopy() {
  copyValue.value = props.keyName + ':copy'
  copyTargetConn.value = props.connId
  copyTargetDb.value = props.db
  copyReplace.value = false
  copyVisible.value = true
}
async function doCopy() {
  if (!copyValue.value) return
  copying.value = true
  try {
    if (copyTargetConn.value === props.connId && copyTargetDb.value === props.db) {
      await CopyKey(props.connId, props.db, props.keyName, copyValue.value)
    } else {
      await CopyKeyAcross(
        props.connId, props.db,
        copyTargetConn.value, copyTargetDb.value,
        props.keyName, copyValue.value, copyReplace.value,
      )
    }
    message.success(`已复制到 ${copyValue.value}`)
    copyVisible.value = false
  } catch (e: any) {
    message.error(String(e?.message ?? e) || '复制失败')
  } finally {
    copying.value = false
  }
}

// ---------- 删除 ----------
function doDelete() {
  Modal.confirm({
    title: '删除键',
    content: `确定删除「${props.keyName}」吗？`,
    okText: '删除',
    okButtonProps: { danger: true },
    cancelText: '取消',
    async onOk() {
      try {
        await DeleteKeys(props.connId, props.db, [props.keyName])
        message.success('已删除')
        emit('deleted', props.keyName)
      } catch (e: any) {
        message.error(String(e?.message ?? e))
      }
    },
  })
}

function copyName() {
  navigator.clipboard?.writeText(props.keyName)
  message.success('键名已复制')
}
</script>

<template>
  <div class="value-panel">
    <div class="vp-header">
      <div class="vp-title-row">
        <span class="vp-key mono" :title="keyName" @click="copyName">{{ keyName }}</span>
      </div>
      <div class="vp-meta-row" v-if="detail">
        <span class="type-badge" :class="`type-${detail.type}`">{{ detail.type }}</span>
        <span class="meta-chip static num" :title="`内部编码: ${detail.encoding || '-'}`">{{ detail.encoding || '-' }}</span>
        <a-tooltip title="点击修改 TTL">
          <span class="meta-chip clickable" :class="{ warn: detail.ttl > 0 && detail.ttl < 300 }" @click="openTTL">
            <ClockCircleOutlined />
            <span class="num">{{ formatTTL(detail.ttl) }}</span>
          </span>
        </a-tooltip>
        <span class="meta-chip static num">{{ formatBytes(detail.size) }}</span>
        <span v-if="detail.type !== 'string'" class="meta-chip static num">{{ detail.length }} 项</span>
        <div class="vp-actions">
          <a-tooltip title="刷新"><button class="vp-icon-btn" @click="load"><ReloadOutlined :spin="loading" /></button></a-tooltip>
          <a-tooltip title="重命名"><button class="vp-icon-btn" @click="openRename"><EditOutlined /></button></a-tooltip>
          <a-tooltip title="复制键"><button class="vp-icon-btn" @click="openCopy"><CopyOutlined /></button></a-tooltip>
          <span class="vp-actions-divider" />
          <a-tooltip title="删除"><button class="vp-icon-btn danger" @click="doDelete"><DeleteOutlined /></button></a-tooltip>
        </div>
      </div>
    </div>

    <div class="vp-body">
      <a-spin :spinning="loading" style="height: 100%">
        <div v-if="missing" class="missing">
          <a-empty description="键不存在或已过期" />
        </div>
        <template v-else-if="detail">
          <StringEditor
            v-if="detail.type === 'string'"
            :conn-id="connId" :db="db" :key-name="keyName"
            :value="detail.strValue" :truncated="detail.truncated" :ttl="detail.ttl"
            :binary="detail.binary"
            @saved="load"
          />
          <JSONEditor
            v-else-if="detail.type === 'ReJSON-RL'"
            :conn-id="connId" :db="db" :key-name="keyName"
            :value="detail.strValue" :truncated="detail.truncated" :ttl="detail.ttl"
            @saved="load"
          />
          <HashEditor v-else-if="detail.type === 'hash'" :conn-id="connId" :db="db" :key-name="keyName" :total="detail.length" />
          <ListEditor v-else-if="detail.type === 'list'" :conn-id="connId" :db="db" :key-name="keyName" :total="detail.length" />
          <SetEditor v-else-if="detail.type === 'set'" :conn-id="connId" :db="db" :key-name="keyName" :total="detail.length" />
          <ZSetEditor v-else-if="detail.type === 'zset'" :conn-id="connId" :db="db" :key-name="keyName" :total="detail.length" />
          <StreamEditor v-else-if="detail.type === 'stream'" :conn-id="connId" :db="db" :key-name="keyName" :total="detail.length" />
          <a-empty v-else :description="`暂不支持 ${detail.type} 类型`" style="margin-top: 80px" />
        </template>
      </a-spin>
    </div>

    <a-modal v-model:open="renameVisible" title="重命名键" width="440px" ok-text="确定" cancel-text="取消" @ok="doRename">
      <a-input v-model:value="renameValue" class="mono" @press-enter="doRename" />
    </a-modal>

    <a-modal v-model:open="ttlVisible" title="设置 TTL" width="400px" ok-text="确定" cancel-text="取消" @ok="doSetTTL">
      <a-input-number v-model:value="ttlValue" :min="-1" style="width: 100%" addon-after="秒" />
      <div class="text-3" style="font-size: 11px; margin-top: 6px">-1 表示永久（移除过期时间）</div>
    </a-modal>

    <a-modal v-model:open="copyVisible" title="复制键" width="440px" ok-text="复制" cancel-text="取消" :confirm-loading="copying" @ok="doCopy">
      <a-form layout="vertical" size="small">
        <a-form-item label="新键名">
          <a-input v-model:value="copyValue" class="mono" placeholder="新键名" @press-enter="doCopy" />
        </a-form-item>
        <div style="display: flex; gap: 12px">
          <a-form-item label="目标连接" style="flex: 2">
            <a-select v-model:value="copyTargetConn" :options="copyTargetOptions" />
          </a-form-item>
          <a-form-item label="目标 DB" style="flex: 1">
            <a-input-number v-model:value="copyTargetDb" :min="0" :max="targetDbCount - 1" style="width: 100%" />
          </a-form-item>
        </div>
        <a-checkbox v-if="copyTargetConn !== connId || copyTargetDb !== db" v-model:checked="copyReplace">
          覆盖同名键（RESTORE REPLACE）
        </a-checkbox>
      </a-form>
    </a-modal>
  </div>
</template>

<style scoped>
.value-panel { height: 100%; display: flex; flex-direction: column; }
.vp-header {
  padding: 12px 18px 10px;
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
}
.vp-title-row { margin-bottom: 9px; }
.vp-key {
  font-size: 13.5px; font-weight: 600;
  word-break: break-all;
  cursor: pointer;
  user-select: text;
  line-height: 1.45;
  transition: color 0.12s;
}
.vp-key:hover { color: var(--accent); }
.vp-meta-row { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.meta-chip {
  display: inline-flex; align-items: center; gap: 5px;
  font-size: 10.5px; color: var(--text-2);
  background: var(--bg-inset);
  border: 1px solid var(--border);
  padding: 2px 8px;
  border-radius: 5px;
  line-height: 1.5;
}
.meta-chip.static { cursor: default; }
.meta-chip.clickable { cursor: pointer; transition: border-color 0.12s, color 0.12s; }
.meta-chip.clickable:hover { border-color: var(--accent); color: var(--accent); }
.meta-chip.warn { color: var(--warning); }
.vp-actions { margin-left: auto; display: flex; align-items: center; gap: 2px; }
.vp-actions-divider { width: 1px; height: 14px; background: var(--border-strong); margin: 0 4px; }
.vp-icon-btn {
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
.vp-icon-btn:hover { background: var(--bg-hover); color: var(--text-1); }
.vp-icon-btn.danger:hover { background: var(--accent-soft); color: var(--accent); }
.vp-body { flex: 1; min-height: 0; padding: 12px 18px 14px; overflow: hidden; }
.vp-body :deep(.ant-spin-container), .vp-body :deep(.ant-spin-nested-loading) { height: 100%; }
.missing { height: 100%; display: flex; align-items: center; justify-content: center; }
</style>
