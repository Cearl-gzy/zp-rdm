<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { message } from 'ant-design-vue'
import { ReloadOutlined, DownloadOutlined, SearchOutlined } from '@ant-design/icons-vue'
import { AuditDates, AuditRead, PickSaveFile, SaveTextFile } from '../../wailsjs/go/main/App'

interface AuditEntry {
  time: string
  conn: string
  connId: string
  cmd: string
  args: string
}

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ 'update:open': [boolean] }>()
const { t } = useI18n()

const dates = ref<string[]>([])
const date = ref('')
const entries = ref<AuditEntry[]>([])
const loading = ref(false)
const connFilter = ref('')
const keyword = ref('')

async function loadDates() {
  try {
    dates.value = ((await AuditDates()) as unknown as string[]) || []
    if (dates.value.length && !dates.value.includes(date.value)) {
      date.value = dates.value[0]
    }
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  }
}

async function load() {
  if (!date.value) { entries.value = []; return }
  loading.value = true
  try {
    entries.value = ((await AuditRead(date.value)) as unknown as AuditEntry[]) || []
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    loading.value = false
  }
}

watch(() => props.open, v => { if (v) { connFilter.value = ''; keyword.value = ''; loadDates().then(load) } })
watch(date, load)

const connOptions = computed(() => {
  const set = new Map<string, string>()
  for (const e of entries.value) set.set(e.connId || e.conn, e.conn)
  return [...set.entries()].map(([id, name]) => ({ value: id, label: name }))
})

const filtered = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  return entries.value.filter(e => {
    if (connFilter.value && (e.connId || e.conn) !== connFilter.value) return false
    if (kw && !`${e.cmd} ${e.args}`.toLowerCase().includes(kw)) return false
    return true
  })
})

const columns = computed(() => [
  { title: t('audit.colTime'), dataIndex: 'time', key: 'time', width: 165 },
  { title: t('audit.colConn'), dataIndex: 'conn', key: 'conn', width: 130 },
  { title: t('audit.colCmd'), dataIndex: 'cmd', key: 'cmd', width: 130 },
  { title: t('audit.colArgs'), dataIndex: 'args', key: 'args' },
])

const exporting = ref(false)
async function doExport() {
  exporting.value = true
  try {
    const path = await PickSaveFile(t('audit.export'), `audit-${date.value || 'all'}.jsonl`)
    if (!path) return
    // 导出过滤后的视图（新→旧），JSONL 与原格式一致
    const content = filtered.value.map(e => JSON.stringify(e)).join('\n') + '\n'
    await SaveTextFile(path, content)
    message.success(t('audit.exported', { n: filtered.value.length }))
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    exporting.value = false
  }
}

// 危险命令高亮（与 CLI 护栏同族）
const DANGER = /^(FLUSHALL|FLUSHDB|SHUTDOWN|DEBUG|CONFIG|DEL|UNLINK|EVAL|EVALSHA|SCRIPT|RESTORE|MIGRATE|SWAPDB|ACL|FAILOVER|CLUSTER)\b/
function cmdClass(cmd: string): string {
  return DANGER.test(cmd) ? 'cmd-danger' : 'cmd-normal'
}
</script>

<template>
  <a-modal :open="props.open" :title="t('audit.title')" :width="880" :footer="null" @update:open="emit('update:open', $event)">
    <div v-if="!dates.length && !loading" class="audit-empty">
      <a-empty :description="t('audit.noDates')" />
    </div>
    <template v-else>
      <div class="audit-toolbar">
        <a-select v-model:value="date" size="small" style="width: 130px" :placeholder="t('audit.date')">
          <a-select-option v-for="d in dates" :key="d" :value="d"><span class="mono">{{ d }}</span></a-select-option>
        </a-select>
        <a-select v-model:value="connFilter" size="small" style="width: 160px" :placeholder="t('audit.connFilter')" allow-clear>
          <a-select-option value="">{{ t('audit.allConns') }}</a-select-option>
          <a-select-option v-for="o in connOptions" :key="o.value" :value="o.value">{{ o.label }}</a-select-option>
        </a-select>
        <a-input v-model:value="keyword" size="small" :placeholder="t('audit.keyword')" allow-clear style="flex: 1">
          <template #prefix><SearchOutlined class="text-3" /></template>
        </a-input>
        <a-button size="small" :loading="loading" @click="loadDates().then(load)">
          <template #icon><ReloadOutlined /></template>
        </a-button>
        <a-button size="small" :loading="exporting" :disabled="!filtered.length" @click="doExport">
          <template #icon><DownloadOutlined /></template>
          {{ t('audit.export') }}
        </a-button>
        <span class="text-3" style="font-size: 11px">{{ t('audit.total', { n: filtered.length }) }}</span>
      </div>

      <a-table
        :columns="columns"
        :data-source="filtered"
        :pagination="{ pageSize: 50, size: 'small', showLessItems: true, hideOnSinglePage: true }"
        size="small"
        :row-key="(r: AuditEntry) => r.time + r.cmd + r.args"
        :loading="loading"
        :scroll="{ y: 440 }"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'time'">
            <span class="mono text-3" style="font-size: 11px">{{ record.time }}</span>
          </template>
          <template v-else-if="column.key === 'conn'">
            <span style="font-size: 11.5px">{{ record.conn }}</span>
          </template>
          <template v-else-if="column.key === 'cmd'">
            <span class="mono cmd" :class="cmdClass(record.cmd)">{{ record.cmd }}</span>
          </template>
          <template v-else-if="column.key === 'args'">
            <span class="mono cell-args" :title="record.args">{{ record.args }}</span>
          </template>
        </template>
      </a-table>
      <div class="text-3" style="font-size: 10.5px; margin-top: 6px">{{ t('audit.cappedTip', { n: 2000 }) }}</div>
    </template>
  </a-modal>
</template>

<style scoped>
.audit-toolbar { display: flex; gap: 6px; align-items: center; margin-bottom: 10px; }
.audit-empty { padding: 30px 0; }
.cmd { font-size: 11px; font-weight: 600; }
.cmd-danger { color: var(--accent, #e04536); }
.cmd-normal { color: var(--text-2); }
.cell-args {
  font-size: 11px; color: var(--text-2);
  user-select: text; word-break: break-all; white-space: pre-wrap;
}
</style>
