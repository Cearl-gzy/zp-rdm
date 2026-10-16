<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { message, Modal } from 'ant-design-vue'
import { ReloadOutlined, ClearOutlined } from '@ant-design/icons-vue'
import { SlowLog, SlowLogReset } from '../../wailsjs/go/service/ServerService'
import type { SlowLogEntry } from '../types'
import { formatTime } from '../utils/format'

const props = defineProps<{ connId: string; db: number }>()
const { t } = useI18n()

const logs = ref<SlowLogEntry[]>([])
const loading = ref(false)
const count = ref(128)

async function load() {
  loading.value = true
  try {
    logs.value = ((await SlowLog(props.connId, props.db, count.value)) as unknown as SlowLogEntry[]) || []
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    loading.value = false
  }
}

onMounted(load)

function reset() {
  Modal.confirm({
    title: t('slowlog.resetTitle'),
    content: t('slowlog.resetContent'),
    okText: t('slowlog.resetOk'), okButtonProps: { danger: true }, cancelText: t('common.cancel'),
    async onOk() {
      try {
        await SlowLogReset(props.connId, props.db)
        message.success(t('slowlog.resetDone'))
        load()
      } catch (e: any) {
        message.error(String(e?.message ?? e))
      }
    },
  })
}

function durationText(us: number): string {
  if (us >= 1e6) return (us / 1e6).toFixed(2) + ' s'
  if (us >= 1e3) return (us / 1e3).toFixed(1) + ' ms'
  return us + ' µs'
}

function durationClass(us: number): string {
  if (us >= 1e6) return 'dur-slow'
  if (us >= 1e5) return 'dur-mid'
  return 'dur-fast'
}

const columns = computed(() => [
  { title: 'ID', dataIndex: 'id', key: 'id', width: 80 },
  { title: t('slowlog.colTime'), dataIndex: 'timestamp', key: 'timestamp', width: 170 },
  { title: t('slowlog.colDuration'), dataIndex: 'durationUs', key: 'durationUs', width: 110 },
  { title: t('slowlog.colClient'), dataIndex: 'clientAddr', key: 'clientAddr', width: 160 },
  { title: t('slowlog.colCmd'), key: 'args' },
])
</script>

<template>
  <div class="slowlog">
    <div class="sl-toolbar">
      <a-select v-model:value="count" size="small" style="width: 120px" @change="load">
        <a-select-option v-for="n in [32, 128, 512, 1024]" :key="n" :value="n">{{ t('slowlog.recentN', { n }) }}</a-select-option>
      </a-select>
      <a-button size="small" :loading="loading" @click="load">
        <template #icon><ReloadOutlined /></template>
        {{ t('common.refresh') }}
      </a-button>
      <a-button size="small" danger @click="reset">
        <template #icon><ClearOutlined /></template>
        SLOWLOG RESET
      </a-button>
      <span class="text-3" style="font-size: 11px; margin-left: auto">{{ t('slowlog.thresholdTip') }}</span>
    </div>

    <div class="sl-table">
      <a-table
        :columns="columns"
        :data-source="logs"
        :pagination="{ pageSize: 20, size: 'small', showLessItems: true }"
        size="small"
        row-key="id"
        :loading="loading"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'id'"><span class="mono text-3">#{{ record.id }}</span></template>
          <template v-else-if="column.key === 'timestamp'"><span class="mono">{{ formatTime(record.timestamp) }}</span></template>
          <template v-else-if="column.key === 'durationUs'">
            <span class="mono dur" :class="durationClass(record.durationUs)">{{ durationText(record.durationUs) }}</span>
          </template>
          <template v-else-if="column.key === 'clientAddr'">
            <span class="mono text-2">{{ record.clientAddr || '-' }}</span>
          </template>
          <template v-else-if="column.key === 'args'">
            <span class="mono cmd" :title="record.args.join(' ')">{{ record.args.join(' ') }}</span>
          </template>
        </template>
      </a-table>
    </div>
  </div>
</template>

<style scoped>
.slowlog { height: 100%; display: flex; flex-direction: column; }
.sl-toolbar {
  display: flex; align-items: center; gap: 8px;
  padding: 8px 16px;
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
}
.sl-table { flex: 1; overflow: auto; padding: 10px 16px; }
.dur { font-weight: 700; }
.dur-fast { color: #3fbf6b; }
.dur-mid { color: #f5a623; }
.dur-slow { color: #e04536; }
.cmd { color: var(--text-2); user-select: text; word-break: break-all; white-space: pre-wrap; }
</style>
