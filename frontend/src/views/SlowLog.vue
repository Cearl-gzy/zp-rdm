<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { message, Modal } from 'ant-design-vue'
import { ReloadOutlined, ClearOutlined } from '@ant-design/icons-vue'
import { SlowLog, SlowLogReset } from '../../wailsjs/go/service/ServerService'
import type { SlowLogEntry } from '../types'
import { formatTime } from '../utils/format'

const props = defineProps<{ connId: string; db: number }>()

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
    title: '清空慢日志',
    content: '确定执行 SLOWLOG RESET 吗？',
    okText: '清空', okButtonProps: { danger: true }, cancelText: '取消',
    async onOk() {
      try {
        await SlowLogReset(props.connId, props.db)
        message.success('已清空')
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

const columns = [
  { title: 'ID', dataIndex: 'id', key: 'id', width: 80 },
  { title: '时间', dataIndex: 'timestamp', key: 'timestamp', width: 170 },
  { title: '耗时', dataIndex: 'durationUs', key: 'durationUs', width: 110 },
  { title: '客户端', dataIndex: 'clientAddr', key: 'clientAddr', width: 160 },
  { title: '命令', key: 'args' },
]
</script>

<template>
  <div class="slowlog">
    <div class="sl-toolbar">
      <a-select v-model:value="count" size="small" style="width: 120px" @change="load">
        <a-select-option :value="32">最近 32 条</a-select-option>
        <a-select-option :value="128">最近 128 条</a-select-option>
        <a-select-option :value="512">最近 512 条</a-select-option>
        <a-select-option :value="1024">最近 1024 条</a-select-option>
      </a-select>
      <a-button size="small" :loading="loading" @click="load">
        <template #icon><ReloadOutlined /></template>
        刷新
      </a-button>
      <a-button size="small" danger @click="reset">
        <template #icon><ClearOutlined /></template>
        SLOWLOG RESET
      </a-button>
      <span class="text-3" style="font-size: 11px; margin-left: auto">阈值由 slowlog-log-slower-than 配置决定</span>
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
