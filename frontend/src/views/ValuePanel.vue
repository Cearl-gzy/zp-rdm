<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { message, Modal } from 'ant-design-vue'
import {
  ReloadOutlined, EditOutlined, CopyOutlined, DeleteOutlined, ClockCircleOutlined,
} from '@ant-design/icons-vue'
import { GetValue } from '../../wailsjs/go/service/ValueService'
import { DeleteKeys, RenameKey, SetTTL, CopyKey } from '../../wailsjs/go/service/KeyService'
import { CopyKeyAcross } from '../../wailsjs/go/service/TransferService'
import { useConnectionStore } from '../stores/connection'
import { useReadonly } from '../utils/readonly'
import type { ValueDetail } from '../types'
import { formatBytes, formatTTL } from '../utils/format'
import StringEditor from './editors/StringEditor.vue'
import JSONEditor from './editors/JSONEditor.vue'
import HashEditor from './editors/HashEditor.vue'
import ListEditor from './editors/ListEditor.vue'
import SetEditor from './editors/SetEditor.vue'
import ZSetEditor from './editors/ZSetEditor.vue'
import StreamEditor from './editors/StreamEditor.vue'
import VectorEditor from './editors/VectorEditor.vue'
import TsEditor from './editors/TsEditor.vue'
import BloomEditor from './editors/BloomEditor.vue'

const props = defineProps<{ connId: string; db: number; keyName: string }>()
const emit = defineEmits<{
  deleted: [key: string]
  renamed: [oldKey: string, newKey: string]
}>()

const { t } = useI18n()
const ro = useReadonly(() => props.connId)

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
    message.success(t('value.renamed'))
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
  if (ro.value) return // 只读模式下 TTL 不可改
  ttlValue.value = detail.value?.ttl ?? -1
  ttlVisible.value = true
}
async function doSetTTL() {
  try {
    await SetTTL(props.connId, props.db, props.keyName, ttlValue.value)
    message.success(t('value.ttlUpdated'))
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
    message.success(t('value.copiedTo', { key: copyValue.value }))
    copyVisible.value = false
  } catch (e: any) {
    message.error(String(e?.message ?? e) || t('value.copyFailed'))
  } finally {
    copying.value = false
  }
}

// ---------- 删除 ----------
function doDelete() {
  Modal.confirm({
    title: t('value.deleteTitle'),
    content: t('value.deleteContent', { key: props.keyName }),
    okText: t('common.delete'),
    okButtonProps: { danger: true },
    cancelText: t('common.cancel'),
    async onOk() {
      try {
        await DeleteKeys(props.connId, props.db, [props.keyName])
        message.success(t('value.deleted'))
        emit('deleted', props.keyName)
      } catch (e: any) {
        message.error(String(e?.message ?? e))
      }
    },
  })
}

function copyName() {
  navigator.clipboard?.writeText(props.keyName)
  message.success(t('value.nameCopied'))
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
        <span class="meta-chip static num" :title="t('value.encodingTip', { enc: detail.encoding || '-' })">{{ detail.encoding || '-' }}</span>
        <a-tooltip :title="ro ? t('common.readonlyTip') : t('value.ttlTip')">
          <span class="meta-chip" :class="[ro ? 'static' : 'clickable', { warn: detail.ttl > 0 && detail.ttl < 300 }]" @click="openTTL">
            <ClockCircleOutlined />
            <span class="num">{{ formatTTL(detail.ttl) }}</span>
          </span>
        </a-tooltip>
        <span class="meta-chip static num">{{ formatBytes(detail.size) }}</span>
        <span v-if="detail.type !== 'string'" class="meta-chip static num">{{ t('value.items', { n: detail.length }) }}</span>
        <div class="vp-actions">
          <a-tooltip :title="t('common.refresh')"><button class="vp-icon-btn" @click="load"><ReloadOutlined :spin="loading" /></button></a-tooltip>
          <a-tooltip :title="ro ? t('common.readonlyTip') : t('value.rename')"><button class="vp-icon-btn" :disabled="ro" @click="openRename"><EditOutlined /></button></a-tooltip>
          <a-tooltip :title="ro ? t('common.readonlyTip') : t('value.copyKey')"><button class="vp-icon-btn" :disabled="ro" @click="openCopy"><CopyOutlined /></button></a-tooltip>
          <span class="vp-actions-divider" />
          <a-tooltip :title="ro ? t('common.readonlyTip') : t('common.delete')"><button class="vp-icon-btn danger" :disabled="ro" @click="doDelete"><DeleteOutlined /></button></a-tooltip>
        </div>
      </div>
    </div>

    <div class="vp-body">
      <a-spin :spinning="loading" style="height: 100%">
        <div v-if="missing" class="missing">
          <a-empty :description="t('value.missing')" />
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
          <VectorEditor v-else-if="detail.type === 'vectorset'" :conn-id="connId" :db="db" :key-name="keyName" :total="detail.length" />
          <TsEditor v-else-if="detail.type === 'TSDB-TYPE'" :conn-id="connId" :db="db" :key-name="keyName" :total="detail.length" />
          <BloomEditor v-else-if="detail.type === 'MBbloom--'" :conn-id="connId" :db="db" :key-name="keyName" :total="detail.length" />
          <a-empty v-else :description="t('value.unsupportedType', { type: detail.type })" style="margin-top: 80px" />
        </template>
      </a-spin>
    </div>

    <a-modal v-model:open="renameVisible" :title="t('value.renameTitle')" width="440px" :ok-text="t('common.confirm')" :cancel-text="t('common.cancel')" @ok="doRename">
      <a-input v-model:value="renameValue" class="mono" @press-enter="doRename" />
    </a-modal>

    <a-modal v-model:open="ttlVisible" :title="t('value.ttlTitle')" width="400px" :ok-text="t('common.confirm')" :cancel-text="t('common.cancel')" @ok="doSetTTL">
      <a-input-number v-model:value="ttlValue" :min="-1" style="width: 100%" :addon-after="t('value.seconds')" />
      <div class="text-3" style="font-size: 11px; margin-top: 6px">{{ t('value.ttlHint') }}</div>
    </a-modal>

    <a-modal v-model:open="copyVisible" :title="t('value.copyTitle')" width="440px" :ok-text="t('common.copy')" :cancel-text="t('common.cancel')" :confirm-loading="copying" @ok="doCopy">
      <a-form layout="vertical" size="small">
        <a-form-item :label="t('value.newKeyName')">
          <a-input v-model:value="copyValue" class="mono" :placeholder="t('value.newKeyName')" @press-enter="doCopy" />
        </a-form-item>
        <div style="display: flex; gap: 12px">
          <a-form-item :label="t('value.targetConn')" style="flex: 2">
            <a-select v-model:value="copyTargetConn" :options="copyTargetOptions" />
          </a-form-item>
          <a-form-item :label="t('value.targetDb')" style="flex: 1">
            <a-input-number v-model:value="copyTargetDb" :min="0" :max="targetDbCount - 1" style="width: 100%" />
          </a-form-item>
        </div>
        <a-checkbox v-if="copyTargetConn !== connId || copyTargetDb !== db" v-model:checked="copyReplace">
          {{ t('value.overwriteHint') }}
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
.vp-icon-btn:disabled { opacity: 0.4; cursor: not-allowed; }
.vp-icon-btn:disabled:hover { background: transparent; color: var(--text-3); }
.vp-icon-btn.danger:hover { background: var(--accent-soft); color: var(--accent); }
.vp-body { flex: 1; min-height: 0; padding: 12px 18px 14px; overflow: hidden; }
.vp-body :deep(.ant-spin-container), .vp-body :deep(.ant-spin-nested-loading) { height: 100%; }
.missing { height: 100%; display: flex; align-items: center; justify-content: center; }
</style>
