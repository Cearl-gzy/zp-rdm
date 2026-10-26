<script setup lang="ts">
import { ref, onMounted, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { message, Modal } from 'ant-design-vue'
import { ReloadOutlined, ClearOutlined } from '@ant-design/icons-vue'
import { SlowLog, SlowLogReset, LatencyLatest, LatencyHistory } from '../../wailsjs/go/service/ServerService'
import { useReadonly } from '../utils/readonly'
import type { SlowLogEntry, LatencyEvent, LatencySample } from '../types'
import { formatTime } from '../utils/format'
import { aggregate, summarize } from '../utils/slowlogAnalysis'
import TrendChart from '../components/TrendChart.vue'

const props = defineProps<{ connId: string; db: number }>()
const { t } = useI18n()
const ro = useReadonly(() => props.connId)

const mode = ref<'slowlog' | 'latency' | 'analysis'>('slowlog')

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

// ---------- LATENCY 延迟事件 ----------
const latEvents = ref<LatencyEvent[]>([])
const latLoading = ref(false)
const latSelected = ref('')
const latHistory = ref<LatencySample[]>([])
const latHistLoading = ref(false)

const latHistoryData = computed(() => latHistory.value.map(s => s.ms))
const fmtMs = (v: number) => `${v} ms`

async function loadLatency() {
  latLoading.value = true
  try {
    latEvents.value = ((await LatencyLatest(props.connId, props.db)) as unknown as LatencyEvent[]) || []
    // 默认选中首个事件并加载历史
    const first = latEvents.value[0]?.name ?? ''
    latSelected.value = first
    if (first) await loadLatHistory(first)
    else latHistory.value = []
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    latLoading.value = false
  }
}

async function loadLatHistory(event: string) {
  latHistLoading.value = true
  try {
    latHistory.value = ((await LatencyHistory(props.connId, props.db, event)) as unknown as LatencySample[]) || []
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    latHistLoading.value = false
  }
}

function selectLatEvent(event: string) {
  if (event === latSelected.value) return
  latSelected.value = event
  loadLatHistory(event)
}

// ---------- 分析模式：按命令聚合当前样本 ----------
const summary = computed(() => summarize(logs.value))
const aggRows = computed(() => aggregate(logs.value))

watch(mode, v => {
  if (v === 'latency') loadLatency()
  else if (!logs.value.length) load()
})

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

const latColumns = computed(() => [
  { title: t('slowlog.colEvent'), dataIndex: 'name', key: 'name' },
  { title: t('slowlog.colLast'), dataIndex: 'timestamp', key: 'timestamp', width: 170 },
  { title: t('slowlog.colLatest'), dataIndex: 'latestMs', key: 'latestMs', width: 110 },
  { title: t('slowlog.colMax'), dataIndex: 'maxMs', key: 'maxMs', width: 110 },
])

const aggColumns = computed(() => [
  { title: t('slowlog.colCmd'), dataIndex: 'cmd', key: 'cmd' },
  { title: t('slowlog.colCount'), dataIndex: 'count', key: 'count', width: 80 },
  { title: t('slowlog.colTotal'), dataIndex: 'totalUs', key: 'totalUs', width: 110 },
  { title: t('slowlog.colAvg'), dataIndex: 'avgUs', key: 'avgUs', width: 110 },
  { title: t('slowlog.colMax'), dataIndex: 'maxUs', key: 'maxUs', width: 110 },
  { title: t('slowlog.colLast'), dataIndex: 'lastTs', key: 'lastTs', width: 170 },
  { title: t('slowlog.colShare'), dataIndex: 'share', key: 'share', width: 160 },
])
</script>

<template>
  <div class="slowlog">
    <div class="sl-toolbar">
      <a-segmented v-model:value="mode" size="small"
        :options="[{ value: 'slowlog', label: t('slowlog.modeSlowlog') }, { value: 'analysis', label: t('slowlog.modeAnalysis') }, { value: 'latency', label: t('slowlog.modeLatency') }]" />
      <template v-if="mode === 'slowlog' || mode === 'analysis'">
        <a-select v-model:value="count" size="small" style="width: 120px" @change="load">
          <a-select-option v-for="n in [32, 128, 512, 1024]" :key="n" :value="n">{{ t('slowlog.recentN', { n }) }}</a-select-option>
        </a-select>
        <a-button size="small" :loading="loading" @click="load">
          <template #icon><ReloadOutlined /></template>
          {{ t('common.refresh') }}
        </a-button>
        <a-button v-if="mode === 'slowlog'" size="small" danger :disabled="ro" :title="ro ? t('common.readonlyTip') : ''" @click="reset">
          <template #icon><ClearOutlined /></template>
          SLOWLOG RESET
        </a-button>
        <span class="text-3" style="font-size: 11px; margin-left: auto">{{ mode === 'analysis' ? t('slowlog.analysisTip') : t('slowlog.thresholdTip') }}</span>
      </template>
      <template v-else>
        <a-button size="small" :loading="latLoading" @click="loadLatency">
          <template #icon><ReloadOutlined /></template>
          {{ t('common.refresh') }}
        </a-button>
        <span class="text-3" style="font-size: 11px; margin-left: auto">{{ t('slowlog.latencyTip') }}</span>
      </template>
    </div>

    <div v-if="mode === 'slowlog'" class="sl-table">
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

    <div v-else-if="mode === 'analysis'" class="sl-table">
      <a-empty v-if="!logs.length && !loading" style="margin-top: 60px" />
      <template v-else>
        <div class="sum-cards">
          <div class="sum-card"><div class="sum-val mono">{{ summary.total }}</div><div class="sum-label text-3">{{ t('slowlog.sumTotal') }}</div></div>
          <div class="sum-card"><div class="sum-val mono">{{ durationText(summary.avgUs) }}</div><div class="sum-label text-3">{{ t('slowlog.sumAvg') }}</div></div>
          <div class="sum-card"><div class="sum-val mono">{{ durationText(summary.p95Us) }}</div><div class="sum-label text-3">{{ t('slowlog.sumP95') }}</div></div>
          <div class="sum-card"><div class="sum-val mono dur" :class="durationClass(summary.maxUs)">{{ durationText(summary.maxUs) }}</div><div class="sum-label text-3">{{ t('slowlog.sumMax') }}</div></div>
        </div>
        <a-table
          :columns="aggColumns"
          :data-source="aggRows"
          :pagination="false"
          size="small"
          row-key="cmd"
          :loading="loading"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'cmd'"><span class="mono cmd">{{ record.cmd }}</span></template>
            <template v-else-if="column.key === 'count'"><span class="mono">{{ record.count }}</span></template>
            <template v-else-if="column.key === 'totalUs'"><span class="mono dur" :class="durationClass(record.totalUs)">{{ durationText(record.totalUs) }}</span></template>
            <template v-else-if="column.key === 'avgUs'"><span class="mono">{{ durationText(record.avgUs) }}</span></template>
            <template v-else-if="column.key === 'maxUs'"><span class="mono dur" :class="durationClass(record.maxUs)">{{ durationText(record.maxUs) }}</span></template>
            <template v-else-if="column.key === 'lastTs'"><span class="mono">{{ formatTime(record.lastTs) }}</span></template>
            <template v-else-if="column.key === 'share'">
              <div class="share-cell">
                <a-progress :percent="Math.round(record.share * 100)" size="small" :show-info="false" stroke-color="#f5a623" />
                <span class="mono text-3 share-num">{{ (record.share * 100).toFixed(1) }}%</span>
              </div>
            </template>
          </template>
        </a-table>
      </template>
    </div>

    <div v-else class="sl-table">
      <a-empty
        v-if="!latEvents.length && !latLoading"
        :description="t('slowlog.noLatencyEvents')"
        style="margin-top: 60px"
      />
      <template v-else>
        <div v-if="latSelected" class="lat-chart">
          <div class="lat-chart-title">
            <span class="mono">{{ latSelected }}</span>
            <span class="text-3">{{ t('slowlog.latHistory', { n: latHistory.length }) }}</span>
          </div>
          <a-spin :spinning="latHistLoading">
            <TrendChart :data="latHistoryData" color="#f5a623" :height="72" :format="fmtMs" />
          </a-spin>
        </div>
        <a-table
          :columns="latColumns"
          :data-source="latEvents"
          :pagination="false"
          size="small"
          row-key="name"
          :loading="latLoading"
          :custom-row="(r: LatencyEvent) => ({ onClick: () => selectLatEvent(r.name) })"
          :row-class-name="(r: LatencyEvent) => r.name === latSelected ? 'lat-row-active' : ''"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'name'"><span class="mono cmd">{{ record.name }}</span></template>
            <template v-else-if="column.key === 'timestamp'"><span class="mono">{{ formatTime(record.timestamp) }}</span></template>
            <template v-else-if="column.key === 'latestMs'">
              <span class="mono dur" :class="record.latestMs >= 100 ? 'dur-slow' : record.latestMs >= 10 ? 'dur-mid' : 'dur-fast'">{{ record.latestMs }} ms</span>
            </template>
            <template v-else-if="column.key === 'maxMs'"><span class="mono text-2">{{ record.maxMs }} ms</span></template>
          </template>
        </a-table>
      </template>
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
.lat-chart {
  margin-bottom: 14px;
  padding: 12px 14px;
  background: var(--bg-panel);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
}
.lat-chart-title {
  display: flex; align-items: baseline; gap: 10px;
  font-size: 12.5px; font-weight: 600;
  margin-bottom: 8px;
}
.sum-cards {
  display: flex; gap: 10px;
  margin-bottom: 14px;
}
.sum-card {
  flex: 1;
  padding: 10px 14px;
  background: var(--bg-panel);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
}
.sum-val { font-size: 17px; font-weight: 700; }
.sum-label { font-size: 11px; margin-top: 2px; }
.share-cell { display: flex; align-items: center; gap: 8px; }
.share-cell :deep(.ant-progress) { flex: 1; margin: 0; }
.share-num { width: 44px; text-align: right; font-size: 11px; }
:deep(.lat-row-active) td { background: var(--accent-soft) !important; }
:deep(.ant-table-row) { cursor: pointer; }
</style>
