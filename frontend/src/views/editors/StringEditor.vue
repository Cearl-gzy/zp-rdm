<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { message } from 'ant-design-vue'
import { SaveOutlined, FormatPainterOutlined, CloudDownloadOutlined, WarningOutlined } from '@ant-design/icons-vue'
import { SetString, SetStringHex, GetFullString, GetStringHex } from '../../../wailsjs/go/service/ValueService'
import { tryFormatJson } from '../../utils/format'
import CodeEditor from '../../components/CodeEditor.vue'

const props = defineProps<{
  connId: string
  db: number
  keyName: string
  value: string
  truncated: boolean
  ttl: number
  binary: boolean
}>()
const emit = defineEmits<{ saved: [] }>()

const { t } = useI18n()

const text = ref(props.value)
// 二进制值默认 Hex 视图
const viewMode = ref<'text' | 'json' | 'hex'>(props.binary ? 'hex' : 'text')
const saving = ref(false)
const loadingFull = ref(false)
// 截断状态随"加载完整值"本地解除
const truncatedLocal = ref(props.truncated)

watch(() => props.value, v => {
  text.value = v
  truncatedLocal.value = props.truncated
  hexCache.value = null // 键切换后 hex 缓存失效
  if (viewMode.value === 'hex') enterHexMode()
})
watch(() => props.truncated, v => { truncatedLocal.value = v })
watch(() => props.binary, v => {
  viewMode.value = v ? 'hex' : 'text'
  if (v) enterHexMode()
})
onMounted(() => {
  if (viewMode.value === 'hex') enterHexMode()
})

// ---------- Hex ----------
const hexCache = ref<string | null>(null) // 二进制值从后端获取的完整 hex
const hexEdit = ref('')                   // hex 模式编辑缓冲
const hexLoading = ref(false)

function groupHex(h: string): string {
  return h.replace(/([0-9a-f]{2})(?=[0-9a-f])/gi, '$1 ')
}

// 文本 → 空格分组的 hex 流（可编辑格式，非 dump）
function textToHexStream(s: string): string {
  return Array.from(new TextEncoder().encode(s))
    .map(b => b.toString(16).padStart(2, '0'))
    .join(' ')
}

const hexDisplay = computed(() => {
  if (hexCache.value !== null) return hexCache.value
  return textToHexStream(text.value)
})

async function enterHexMode() {
  if (!props.binary || hexCache.value !== null) {
    hexEdit.value = hexDisplay.value
    return
  }
  // 二进制：从后端取完整 hex（避免 JSON 传输的 UTF-8 替换字符污染）
  hexLoading.value = true
  try {
    hexCache.value = groupHex(await GetStringHex(props.connId, props.db, props.keyName))
    hexEdit.value = hexCache.value
  } catch (e: any) {
    message.error(String(e?.message ?? e))
    hexEdit.value = hexDisplay.value
  } finally {
    hexLoading.value = false
  }
}

function onModeChange() {
  if (viewMode.value === 'hex') {
    enterHexMode()
  } else if (viewMode.value === 'json') {
    const f = tryFormatJson(text.value)
    if (f) text.value = f
    else message.warning(t('editors.invalidJson'))
  }
}

function formatJson() {
  const f = tryFormatJson(text.value)
  if (f) { text.value = f; message.success(t('editors.formatted')) }
  else message.warning(t('editors.invalidJson'))
}

// ---------- 加载完整值 ----------
async function loadFull() {
  loadingFull.value = true
  try {
    if (viewMode.value === 'hex' && props.binary) {
      hexCache.value = groupHex(await GetStringHex(props.connId, props.db, props.keyName))
      hexEdit.value = hexCache.value
    } else {
      text.value = await GetFullString(props.connId, props.db, props.keyName)
      if (viewMode.value === 'hex') hexEdit.value = textToHexStream(text.value)
    }
    truncatedLocal.value = false
    message.success(t('editors.loadedFull'))
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    loadingFull.value = false
  }
}

// ---------- 保存 ----------
async function save() {
  saving.value = true
  try {
    const ttl = props.ttl > 0 ? props.ttl : -1 // -1 保持原 TTL
    if (viewMode.value === 'hex') {
      await SetStringHex(props.connId, props.db, props.keyName, hexEdit.value, ttl)
    } else {
      await SetString(props.connId, props.db, props.keyName, text.value, ttl)
    }
    message.success(t('editors.saved'))
    emit('saved')
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="string-editor">
    <div class="se-toolbar">
      <a-radio-group v-model:value="viewMode" size="small" option-type="button" button-style="solid" @change="onModeChange">
        <a-radio-button value="text">{{ t('editors.text') }}</a-radio-button>
        <a-radio-button value="json">JSON</a-radio-button>
        <a-radio-button value="hex">Hex</a-radio-button>
      </a-radio-group>
      <a-button v-if="viewMode === 'json'" size="small" @click="formatJson">
        <template #icon><FormatPainterOutlined /></template>
        {{ t('editors.format') }}
      </a-button>
      <span class="text-3 se-count">{{ viewMode === 'hex' ? t('editors.bytesCount', { n: hexEdit.replace(/\s/g, '').length / 2 }) : t('editors.charsCount', { n: text.length }) }}</span>

      <a-tag v-if="binary" class="se-tag binary-tag">
        <WarningOutlined /> {{ t('editors.binaryContent') }}
      </a-tag>
      <template v-if="truncatedLocal">
        <a-tag class="se-tag" color="warning">{{ t('editors.truncatedTag') }}</a-tag>
        <a-button size="small" type="link" :loading="loadingFull" class="load-full-btn" @click="loadFull">
          <template #icon><CloudDownloadOutlined /></template>
          {{ t('editors.loadFull') }}
        </a-button>
      </template>

      <a-button type="primary" size="small" style="margin-left: auto" :loading="saving" @click="save">
        <template #icon><SaveOutlined /></template>
        {{ t('common.save') }}
      </a-button>
    </div>

    <div v-if="binary && viewMode !== 'hex'" class="binary-hint">
      {{ t('editors.binaryHint') }}
    </div>

    <div v-if="viewMode === 'hex'" class="se-hex-wrap">
      <a-spin :spinning="hexLoading" class="se-hex-spin">
        <textarea
          v-model="hexEdit"
          class="se-hex mono"
          :class="{ 'is-binary': binary }"
          spellcheck="false"
          :placeholder="t('editors.hexPlaceholder')"
        />
      </a-spin>
    </div>
    <div v-else class="se-code-wrap">
      <CodeEditor v-model="text" :lang="viewMode === 'json' ? 'json' : 'text'" />
    </div>
  </div>
</template>

<style scoped>
.string-editor { height: 100%; display: flex; flex-direction: column; gap: 8px; }
.se-toolbar { display: flex; align-items: center; gap: 8px; flex-shrink: 0; flex-wrap: wrap; }
.se-count { font-size: 11px; font-family: var(--mono); }
.se-tag { margin-inline-end: 0 !important; }
.binary-tag {
  color: var(--warning) !important;
  border-color: rgba(245, 166, 35, 0.4) !important;
  background: rgba(245, 166, 35, 0.1) !important;
}
.load-full-btn { font-size: 11.5px; height: 22px; padding: 0 6px; }

.binary-hint {
  flex-shrink: 0;
  font-size: 11.5px;
  color: var(--warning);
  background: rgba(245, 166, 35, 0.08);
  border: 1px solid rgba(245, 166, 35, 0.25);
  border-radius: var(--radius-sm);
  padding: 5px 10px;
}

.se-code-wrap {
  flex: 1; min-height: 0;
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  background: var(--bg-elevated);
  overflow: hidden;
  transition: border-color 0.15s;
}
.se-code-wrap:focus-within { border-color: var(--border-strong); }

.se-hex-wrap { flex: 1; min-height: 0; display: flex; }
.se-hex-spin,
.se-hex-wrap :deep(.ant-spin-nested-loading),
.se-hex-wrap :deep(.ant-spin-container) {
  display: flex; flex: 1; min-height: 0;
}
.se-hex {
  flex: 1;
  width: 100%;
  resize: none;
  background: var(--bg-elevated);
  color: var(--text-1);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  padding: 10px 12px;
  font-size: 12.5px;
  line-height: 1.7;
  letter-spacing: 0.5px;
  outline: none;
  user-select: text;
  transition: border-color 0.15s;
}
.se-hex:focus { border-color: var(--accent); }
.se-hex.is-binary { color: var(--warning); }
</style>
