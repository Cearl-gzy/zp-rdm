<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import { message } from 'ant-design-vue'
import { SaveOutlined, FormatPainterOutlined, CloudDownloadOutlined } from '@ant-design/icons-vue'
import { SetJSON, GetFullJSON } from '../../../wailsjs/go/service/ValueService'
import { tryFormatJson } from '../../utils/format'
import CodeEditor from '../../components/CodeEditor.vue'

// RedisJSON 文档编辑器（ReJSON-RL 类型）
const props = defineProps<{
  connId: string
  db: number
  keyName: string
  value: string
  truncated: boolean
  ttl: number
}>()
const emit = defineEmits<{ saved: [] }>()

const text = ref(props.value)
const saving = ref(false)
const loadingFull = ref(false)
const truncatedLocal = ref(props.truncated)

function formatNow() {
  const f = tryFormatJson(text.value)
  if (f) text.value = f
}

watch(() => props.value, v => {
  text.value = v
  truncatedLocal.value = props.truncated
  formatNow()
})
watch(() => props.truncated, v => { truncatedLocal.value = v })
onMounted(formatNow)

function formatJson() {
  const f = tryFormatJson(text.value)
  if (f) { text.value = f; message.success('已格式化') }
  else message.warning('内容不是合法 JSON')
}

async function loadFull() {
  loadingFull.value = true
  try {
    text.value = await GetFullJSON(props.connId, props.db, props.keyName)
    formatNow()
    truncatedLocal.value = false
    message.success('已加载完整内容')
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    loadingFull.value = false
  }
}

async function save() {
  saving.value = true
  try {
    const ttl = props.ttl > 0 ? props.ttl : -1 // -1 保持原 TTL
    await SetJSON(props.connId, props.db, props.keyName, text.value, ttl)
    message.success('已保存')
    emit('saved')
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="json-editor">
    <div class="je-toolbar">
      <a-tag class="je-tag" color="purple">RedisJSON</a-tag>
      <a-button size="small" @click="formatJson">
        <template #icon><FormatPainterOutlined /></template>
        格式化
      </a-button>
      <span class="text-3 je-count">{{ text.length }} 字符</span>

      <template v-if="truncatedLocal">
        <a-tag class="je-tag" color="warning">已截断（预览 1MB）</a-tag>
        <a-button size="small" type="link" :loading="loadingFull" class="load-full-btn" @click="loadFull">
          <template #icon><CloudDownloadOutlined /></template>
          加载完整值
        </a-button>
      </template>

      <a-button type="primary" size="small" style="margin-left: auto" :loading="saving" @click="save">
        <template #icon><SaveOutlined /></template>
        保存
      </a-button>
    </div>

    <div class="je-code-wrap">
      <CodeEditor v-model="text" lang="json" />
    </div>
  </div>
</template>

<style scoped>
.json-editor { height: 100%; display: flex; flex-direction: column; gap: 8px; }
.je-toolbar { display: flex; align-items: center; gap: 8px; flex-shrink: 0; flex-wrap: wrap; }
.je-count { font-size: 11px; font-family: var(--mono); }
.je-tag { margin-inline-end: 0 !important; }
.load-full-btn { font-size: 11.5px; height: 22px; padding: 0 6px; }
.je-code-wrap {
  flex: 1; min-height: 0;
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  background: var(--bg-elevated);
  overflow: hidden;
  transition: border-color 0.15s;
}
.je-code-wrap:focus-within { border-color: var(--border-strong); }
</style>
