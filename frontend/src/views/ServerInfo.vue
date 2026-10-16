<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { message, Modal } from 'ant-design-vue'
import { ReloadOutlined, DisconnectOutlined } from '@ant-design/icons-vue'
import { GetInfo, ClientList, KillClient, SearchIndexes } from '../../wailsjs/go/service/ServerService'
import { useConnectionStore } from '../stores/connection'
import type { InfoMap, ClientConnInfo } from '../types'
import { formatBytes, formatDuration, formatNumber } from '../utils/format'
import TrendChart from '../components/TrendChart.vue'

const props = defineProps<{ connId: string; db: number }>()
const conn = useConnectionStore()
const { t } = useI18n()

// 已加载模块（连接打开时由后端 MODULE LIST 探测）
const modules = computed(() => conn.openTabs.find(t => t.id === props.connId)?.state?.modules ?? [])
const searchIndexes = ref<string[]>([])

async function loadSearchIndexes() {
  // search 模块名常见为 search / ft / searchlight
  if (!modules.value.some(m => /search/i.test(m))) { searchIndexes.value = []; return }
  try {
    searchIndexes.value = (await SearchIndexes(props.connId, props.db)) as unknown as string[]
  } catch {
    searchIndexes.value = []
  }
}

const info = ref<InfoMap>({})
const clients = ref<ClientConnInfo[]>([])
const loading = ref(false)
const autoRefresh = ref(true)
const opsHistory = ref<number[]>([])
const memHistory = ref<number[]>([])
const clientsHistory = ref<number[]>([])
const hitHistory = ref<number[]>([])
let prevHits = -1
let prevMisses = -1
let timer: ReturnType<typeof setInterval> | null = null

function pushHistory(h: number[], v: number) {
  h.push(v)
  if (h.length > 60) h.shift()
}

function num(s?: string): number {
  const n = Number(s)
  return Number.isFinite(n) ? n : 0
}

async function load() {
  if (loading.value) return
  loading.value = true
  try {
    const [inf, cls] = await Promise.all([
      GetInfo(props.connId, props.db, []) as unknown as Promise<InfoMap>,
      ClientList(props.connId, props.db).catch(() => [] as ClientConnInfo[]),
    ])
    info.value = inf
    clients.value = (cls as ClientConnInfo[]) || []
    pushHistory(opsHistory.value, num(inf.stats?.instantaneous_ops_per_sec))
    pushHistory(memHistory.value, num(inf.memory?.used_memory))
    pushHistory(clientsHistory.value, num(inf.clients?.connected_clients))
    // 区间命中率：本采样周期内 命中 / (命中+未中)，比累计值更能反映当前状态
    const hits = num(inf.stats?.keyspace_hits)
    const misses = num(inf.stats?.keyspace_misses)
    if (prevHits >= 0) {
      const dh = hits - prevHits
      const dm = misses - prevMisses
      if (dh + dm > 0) pushHistory(hitHistory.value, (dh / (dh + dm)) * 100)
    }
    prevHits = hits
    prevMisses = misses
  } catch (e: any) {
    message.error(String(e?.message ?? e))
    stopAuto()
  } finally {
    loading.value = false
  }
}

function startAuto() {
  stopAuto()
  timer = setInterval(() => { if (autoRefresh.value) load() }, 2000)
}
function stopAuto() {
  if (timer) { clearInterval(timer); timer = null }
}

onMounted(() => { load(); startAuto(); loadSearchIndexes() })
onBeforeUnmount(stopAuto)

const server = computed(() => info.value.server ?? {})
const memory = computed(() => info.value.memory ?? {})
const stats = computed(() => info.value.stats ?? {})
const clientsInfo = computed(() => info.value.clients ?? {})
const persistence = computed(() => info.value.persistence ?? {})
const keyspace = computed(() => info.value.keyspace ?? {})

const keyspaceRows = computed(() =>
  Object.entries(keyspace.value)
    .filter(([k]) => k.startsWith('db'))
    .map(([dbName, v]) => {
      const m = Object.fromEntries(v.split(',').map(kv => kv.split('=')))
      return { db: dbName, keys: num(m.keys), expires: num(m.expires), avgTtl: num(m.avg_ttl) }
    })
)

const memPercent = computed(() => {
  const used = num(memory.value.used_memory)
  const max = num(memory.value.maxmemory)
  if (!max) return 0
  return Math.min(100, Math.round((used / max) * 100))
})

const hitRate = computed(() => {
  const hits = num(stats.value.keyspace_hits)
  const misses = num(stats.value.keyspace_misses)
  const total = hits + misses
  if (!total) return 100
  return Math.round((hits / total) * 1000) / 10
})

const keyspaceColumns = computed(() => [
  { title: t('server.colDb'), dataIndex: 'db', key: 'db' },
  { title: t('server.colKeys'), dataIndex: 'keys', key: 'keys' },
  { title: t('server.colExpires'), dataIndex: 'expires', key: 'expires' },
  { title: t('server.colAvgTtl'), dataIndex: 'avgTtl', key: 'avgTtl' },
])

const clientColumns = computed(() => [
  { title: t('server.colAddr'), dataIndex: 'addr', key: 'addr' },
  { title: t('server.colName'), dataIndex: 'name', key: 'name', width: 110 },
  { title: 'DB', dataIndex: 'db', key: 'db', width: 60 },
  { title: t('server.colIdle'), dataIndex: 'idle', key: 'idle', width: 80 },
  { title: t('server.colCmd'), dataIndex: 'cmd', key: 'cmd', width: 90 },
  { title: t('common.actions'), key: 'ops', width: 70 },
])

function killClient(addr: string) {
  Modal.confirm({
    title: t('server.killTitle'),
    content: t('server.killContent', { addr }),
    okText: t('server.killOk'), okButtonProps: { danger: true }, cancelText: t('common.cancel'),
    async onOk() {
      try {
        await KillClient(props.connId, props.db, addr)
        message.success(t('server.killed'))
        load()
      } catch (e: any) {
        message.error(String(e?.message ?? e))
      }
    },
  })
}
</script>

<template>
  <div class="server-info">
    <div class="si-toolbar">
      <a-checkbox v-model:checked="autoRefresh">{{ t('server.autoRefresh') }}</a-checkbox>
      <a-button size="small" :loading="loading" @click="load">
        <template #icon><ReloadOutlined /></template>
        {{ t('common.refresh') }}
      </a-button>
    </div>

    <div class="si-scroll">
      <!-- 概览卡片 -->
      <div class="stat-grid">
        <div class="stat-card">
          <div class="sc-label">{{ t('server.redisVersion') }}</div>
          <div class="sc-value num">{{ server.redis_version || '-' }}</div>
          <div class="sc-sub">{{ server.redis_mode || 'standalone' }} · {{ server.os || '' }}</div>
        </div>
        <div class="stat-card">
          <div class="sc-label">{{ t('server.uptime') }}</div>
          <div class="sc-value num">{{ formatDuration(num(server.uptime_in_seconds)) }}</div>
          <div class="sc-sub">{{ t('server.process') }} {{ server.process_id }} · {{ t('server.role') }} {{ server.role || '-' }}</div>
        </div>
        <div class="stat-card">
          <div class="sc-label">{{ t('server.usedMemory') }}</div>
          <div class="sc-value num">{{ formatBytes(num(memory.used_memory)) }}</div>
          <div class="mem-bar">
            <div
              class="mem-bar-fill"
              :class="{ danger: memPercent > 85 }"
              :style="{ width: Math.max(memPercent, 2) + '%' }"
            />
          </div>
          <div class="sc-sub">
            {{ t('server.peak') }} {{ formatBytes(num(memory.used_memory_peak)) }}
            <template v-if="num(memory.maxmemory)"> / {{ t('server.limit') }} {{ formatBytes(num(memory.maxmemory)) }} ({{ memPercent }}%)</template>
            <template v-else> / {{ t('server.noLimit') }}</template>
          </div>
        </div>
        <div class="stat-card">
          <div class="sc-label">{{ t('server.connectedClients') }}</div>
          <div class="sc-value num">{{ clientsInfo.connected_clients ?? clients.length }}</div>
          <div class="sc-sub">{{ t('server.blocked') }} {{ clientsInfo.blocked_clients ?? 0 }} · {{ t('server.tracking') }} {{ clientsInfo.tracking_clients ?? 0 }}</div>
        </div>
        <div class="stat-card">
          <div class="sc-label">{{ t('server.hitRate') }}</div>
          <div class="sc-value num" :class="{ good: hitRate > 90, mid: hitRate <= 90 && hitRate > 70, bad: hitRate <= 70 }">
            {{ hitRate }}<span class="sc-unit">%</span>
          </div>
          <div class="sc-sub">{{ t('server.hit') }} {{ formatNumber(num(stats.keyspace_hits)) }} / {{ t('server.miss') }} {{ formatNumber(num(stats.keyspace_misses)) }}</div>
        </div>
        <div class="stat-card">
          <div class="sc-label">{{ t('server.persistence') }}</div>
          <div class="sc-value num sc-value-sm">
            {{ persistence.rdb_bgsave_in_progress === '1' ? t('server.bgsaveInProgress') : 'RDB' }}<span class="sc-unit"> + </span>{{ persistence.aof_enabled === '1' ? 'AOF' : t('server.noAof') }}
          </div>
          <div class="sc-sub">{{ t('server.lastSave') }} {{ persistence.rdb_last_save_time ? new Date(num(persistence.rdb_last_save_time) * 1000).toLocaleTimeString() : '-' }}</div>
        </div>
      </div>

      <!-- 模块 -->
      <div class="section">
        <div class="sec-title">{{ t('server.loadedModules') }}<span class="sec-count num">{{ modules.length }}</span></div>
        <div v-if="modules.length" class="mod-list">
          <a-tag v-for="m in modules" :key="m" class="mono mod-tag">{{ m }}</a-tag>
          <span v-if="searchIndexes.length" class="text-3 mod-indexes">
            {{ t('server.searchIndexes') }}
            <span v-for="(ix, i) in searchIndexes" :key="ix" class="mono">{{ i ? ', ' : '' }}{{ ix }}</span>
          </span>
        </div>
        <div v-else class="text-3" style="font-size: 12px">{{ t('server.noModules') }}</div>
      </div>

      <!-- 趋势 -->
      <div class="trend-grid">
        <div class="trend-card">
          <div class="tc-title">{{ t('server.opsPerSec') }}<span class="tc-live" /></div>
          <TrendChart :data="opsHistory" color="#e04536" :format="v => formatNumber(Math.round(v))" />
        </div>
        <div class="trend-card">
          <div class="tc-title">{{ t('server.memUsage') }}<span class="tc-live blue" /></div>
          <TrendChart :data="memHistory" color="#5ea1f7" :format="formatBytes" />
        </div>
        <div class="trend-card">
          <div class="tc-title">{{ t('server.clientConns') }}<span class="tc-live green" /></div>
          <TrendChart :data="clientsHistory" color="#3f9d63" :format="v => String(Math.round(v))" />
        </div>
        <div class="trend-card">
          <div class="tc-title">{{ t('server.intervalHitRate') }}<span class="tc-live amber" /></div>
          <TrendChart :data="hitHistory" color="#d9a03f" :format="v => v.toFixed(1) + '%'" :max="100" />
        </div>
        <div class="trend-card">
          <div class="tc-title">{{ t('server.cumulative') }}</div>
          <div class="cumulative">
            <div class="cu-row"><span>{{ t('server.totalCommands') }}</span><span class="num">{{ formatNumber(num(stats.total_commands_processed)) }}</span></div>
            <div class="cu-row"><span>{{ t('server.totalConnections') }}</span><span class="num">{{ formatNumber(num(stats.total_connections_received)) }}</span></div>
            <div class="cu-row"><span>{{ t('server.expiredKeys') }}</span><span class="num">{{ formatNumber(num(stats.expired_keys)) }}</span></div>
            <div class="cu-row"><span>{{ t('server.evictedKeys') }}</span><span class="num">{{ formatNumber(num(stats.evicted_keys)) }}</span></div>
            <div class="cu-row"><span>{{ t('server.netInput') }}</span><span class="num">{{ formatBytes(num(stats.total_net_input_bytes)) }}</span></div>
            <div class="cu-row"><span>{{ t('server.netOutput') }}</span><span class="num">{{ formatBytes(num(stats.total_net_output_bytes)) }}</span></div>
          </div>
        </div>
      </div>

      <!-- 键空间 -->
      <div class="section">
        <div class="sec-title">{{ t('server.keyspace') }}</div>
        <a-table
          :columns="keyspaceColumns"
          :data-source="keyspaceRows"
          :pagination="false"
          size="small"
          row-key="db"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'db'"><a-tag class="mono">{{ record.db }}</a-tag></template>
            <template v-else-if="column.key === 'avgTtl'">
              <span class="mono text-2">{{ record.avgTtl ? (record.avgTtl / 1000).toFixed(1) + 's' : '-' }}</span>
            </template>
            <template v-else>
              <span class="mono">{{ formatNumber(record[column.dataIndex as 'keys' | 'expires']) }}</span>
            </template>
          </template>
        </a-table>
        <a-empty v-if="!keyspaceRows.length" :description="t('server.emptyKeyspace')" :image-style="{ height: '40px' }" />
      </div>

      <!-- 客户端 -->
      <div class="section">
        <div class="sec-title">{{ t('server.clientConnections') }}<span class="sec-count num">{{ clients.length }}</span></div>
        <a-table
          :columns="clientColumns"
          :data-source="clients"
          :pagination="{ pageSize: 10, size: 'small', showLessItems: true }"
          size="small"
          row-key="id"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'addr'"><span class="mono">{{ record.addr }}</span></template>
            <template v-else-if="column.key === 'name'"><span class="mono text-2">{{ record.name || '-' }}</span></template>
            <template v-else-if="column.key === 'ops'">
              <a-tooltip :title="t('server.killTooltip')">
                <a-button size="small" type="text" danger @click="killClient(record.addr)">
                  <template #icon><DisconnectOutlined /></template>
                </a-button>
              </a-tooltip>
            </template>
          </template>
        </a-table>
      </div>
    </div>
  </div>
</template>

<style scoped>
.server-info { height: 100%; display: flex; flex-direction: column; }
.si-toolbar {
  display: flex; align-items: center; gap: 12px;
  padding: 8px 18px;
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
}
.si-scroll { flex: 1; overflow-y: auto; padding: 16px 18px 28px; }

.stat-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(185px, 1fr));
  gap: 10px;
  margin-bottom: 10px;
}
.stat-card {
  background: var(--bg-panel);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  padding: 14px 16px 13px;
  transition: border-color 0.15s var(--ease);
}
.stat-card:hover { border-color: var(--border-strong); }
.sc-label {
  font-size: 10.5px; color: var(--text-3);
  margin-bottom: 8px;
  letter-spacing: 0.6px;
  font-weight: 600;
}
.sc-value {
  font-size: 21px; font-weight: 700;
  line-height: 1.15;
  letter-spacing: -0.3px;
  margin-bottom: 5px;
}
.sc-value-sm { font-size: 16px; line-height: 1.6; }
.sc-value.good { color: var(--success); }
.sc-value.mid { color: var(--warning); }
.sc-value.bad { color: var(--accent); }
.sc-unit { font-size: 12px; font-weight: 500; color: var(--text-3); }
.sc-sub { font-size: 10.5px; color: var(--text-3); margin-top: 5px; }

.mem-bar {
  height: 4px;
  border-radius: 2px;
  background: var(--bg-hover);
  overflow: hidden;
  margin: 2px 0 1px;
}
.mem-bar-fill {
  height: 100%;
  border-radius: 2px;
  background: var(--info);
  transition: width 0.4s var(--ease);
}
.mem-bar-fill.danger { background: var(--accent); }

.trend-grid {
  display: grid;
  grid-template-columns: 1fr 1fr 1fr;
  gap: 10px;
  margin-bottom: 10px;
}
@media (max-width: 1100px) { .trend-grid { grid-template-columns: 1fr; } }
.trend-card {
  background: var(--bg-panel);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  padding: 14px 16px;
  transition: border-color 0.15s var(--ease);
}
.trend-card:hover { border-color: var(--border-strong); }
.tc-title {
  font-size: 12px; font-weight: 600; color: var(--text-2);
  margin-bottom: 10px;
  display: flex; align-items: center; gap: 7px;
}
.tc-live {
  width: 6px; height: 6px; border-radius: 50%;
  background: var(--accent);
  box-shadow: 0 0 5px rgba(224, 69, 54, 0.6);
}
.tc-live.blue { background: var(--info); box-shadow: 0 0 5px rgba(94, 161, 247, 0.6); }
.tc-live.green { background: #3f9d63; box-shadow: 0 0 5px rgba(63, 157, 99, 0.6); }
.tc-live.amber { background: #d9a03f; box-shadow: 0 0 5px rgba(217, 160, 63, 0.6); }
.cumulative { display: flex; flex-direction: column; gap: 7px; }
.cu-row {
  display: flex; justify-content: space-between; align-items: baseline;
  font-size: 11.5px; color: var(--text-3);
}
.cu-row .num { color: var(--text-1); font-size: 12px; }

.section {
  background: var(--bg-panel);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  padding: 14px 16px;
  margin-bottom: 10px;
}
.sec-title {
  font-size: 12.5px; font-weight: 600;
  margin-bottom: 10px;
  display: flex; align-items: center; gap: 8px;
}
.mod-list { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.mod-tag { margin-inline-end: 0 !important; }
.mod-indexes { font-size: 11px; margin-left: 8px; }
.sec-count {
  font-size: 10.5px; font-weight: 500;
  color: var(--text-3);
  background: var(--bg-hover);
  border-radius: 8px;
  padding: 1px 7px;
}
</style>
