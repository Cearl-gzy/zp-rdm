<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, computed } from 'vue'
import { message, Modal } from 'ant-design-vue'
import { ReloadOutlined, DisconnectOutlined } from '@ant-design/icons-vue'
import { GetInfo, ClientList, KillClient } from '../../wailsjs/go/service/ServerService'
import type { InfoMap, ClientConnInfo } from '../types'
import { formatBytes, formatDuration, formatNumber } from '../utils/format'
import TrendChart from '../components/TrendChart.vue'

const props = defineProps<{ connId: string; db: number }>()

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

onMounted(() => { load(); startAuto() })
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

const keyspaceColumns = [
  { title: '数据库', dataIndex: 'db', key: 'db' },
  { title: '键数', dataIndex: 'keys', key: 'keys' },
  { title: '有过期', dataIndex: 'expires', key: 'expires' },
  { title: '平均 TTL', dataIndex: 'avgTtl', key: 'avgTtl' },
]

const clientColumns = [
  { title: '地址', dataIndex: 'addr', key: 'addr' },
  { title: '名称', dataIndex: 'name', key: 'name', width: 110 },
  { title: 'DB', dataIndex: 'db', key: 'db', width: 60 },
  { title: '空闲(s)', dataIndex: 'idle', key: 'idle', width: 80 },
  { title: '命令', dataIndex: 'cmd', key: 'cmd', width: 90 },
  { title: '操作', key: 'ops', width: 70 },
]

function killClient(addr: string) {
  Modal.confirm({
    title: '断开客户端',
    content: `确定断开 ${addr} 吗？`,
    okText: '断开', okButtonProps: { danger: true }, cancelText: '取消',
    async onOk() {
      try {
        await KillClient(props.connId, props.db, addr)
        message.success('已断开')
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
      <a-checkbox v-model:checked="autoRefresh">自动刷新（2s）</a-checkbox>
      <a-button size="small" :loading="loading" @click="load">
        <template #icon><ReloadOutlined /></template>
        刷新
      </a-button>
    </div>

    <div class="si-scroll">
      <!-- 概览卡片 -->
      <div class="stat-grid">
        <div class="stat-card">
          <div class="sc-label">Redis 版本</div>
          <div class="sc-value num">{{ server.redis_version || '-' }}</div>
          <div class="sc-sub">{{ server.redis_mode || 'standalone' }} · {{ server.os || '' }}</div>
        </div>
        <div class="stat-card">
          <div class="sc-label">运行时间</div>
          <div class="sc-value num">{{ formatDuration(num(server.uptime_in_seconds)) }}</div>
          <div class="sc-sub">进程 {{ server.process_id }} · 角色 {{ server.role || '-' }}</div>
        </div>
        <div class="stat-card">
          <div class="sc-label">已用内存</div>
          <div class="sc-value num">{{ formatBytes(num(memory.used_memory)) }}</div>
          <div class="mem-bar">
            <div
              class="mem-bar-fill"
              :class="{ danger: memPercent > 85 }"
              :style="{ width: Math.max(memPercent, 2) + '%' }"
            />
          </div>
          <div class="sc-sub">
            峰值 {{ formatBytes(num(memory.used_memory_peak)) }}
            <template v-if="num(memory.maxmemory)"> / 上限 {{ formatBytes(num(memory.maxmemory)) }} ({{ memPercent }}%)</template>
            <template v-else> / 无上限</template>
          </div>
        </div>
        <div class="stat-card">
          <div class="sc-label">连接客户端</div>
          <div class="sc-value num">{{ clientsInfo.connected_clients ?? clients.length }}</div>
          <div class="sc-sub">阻塞 {{ clientsInfo.blocked_clients ?? 0 }} · 追踪 {{ clientsInfo.tracking_clients ?? 0 }}</div>
        </div>
        <div class="stat-card">
          <div class="sc-label">缓存命中率</div>
          <div class="sc-value num" :class="{ good: hitRate > 90, mid: hitRate <= 90 && hitRate > 70, bad: hitRate <= 70 }">
            {{ hitRate }}<span class="sc-unit">%</span>
          </div>
          <div class="sc-sub">命中 {{ formatNumber(num(stats.keyspace_hits)) }} / 未中 {{ formatNumber(num(stats.keyspace_misses)) }}</div>
        </div>
        <div class="stat-card">
          <div class="sc-label">持久化</div>
          <div class="sc-value num sc-value-sm">
            {{ persistence.rdb_bgsave_in_progress === '1' ? 'BGSAVE 中' : 'RDB' }}<span class="sc-unit"> + </span>{{ persistence.aof_enabled === '1' ? 'AOF' : '无 AOF' }}
          </div>
          <div class="sc-sub">最近保存 {{ persistence.rdb_last_save_time ? new Date(num(persistence.rdb_last_save_time) * 1000).toLocaleTimeString() : '-' }}</div>
        </div>
      </div>

      <!-- 趋势 -->
      <div class="trend-grid">
        <div class="trend-card">
          <div class="tc-title">OPS / 秒<span class="tc-live" /></div>
          <TrendChart :data="opsHistory" color="#e04536" :format="v => formatNumber(Math.round(v))" />
        </div>
        <div class="trend-card">
          <div class="tc-title">内存占用<span class="tc-live blue" /></div>
          <TrendChart :data="memHistory" color="#5ea1f7" :format="formatBytes" />
        </div>
        <div class="trend-card">
          <div class="tc-title">客户端连接<span class="tc-live green" /></div>
          <TrendChart :data="clientsHistory" color="#3f9d63" :format="v => String(Math.round(v))" />
        </div>
        <div class="trend-card">
          <div class="tc-title">区间命中率<span class="tc-live amber" /></div>
          <TrendChart :data="hitHistory" color="#d9a03f" :format="v => v.toFixed(1) + '%'" :max="100" />
        </div>
        <div class="trend-card">
          <div class="tc-title">累计统计</div>
          <div class="cumulative">
            <div class="cu-row"><span>处理命令</span><span class="num">{{ formatNumber(num(stats.total_commands_processed)) }}</span></div>
            <div class="cu-row"><span>总连接数</span><span class="num">{{ formatNumber(num(stats.total_connections_received)) }}</span></div>
            <div class="cu-row"><span>过期键</span><span class="num">{{ formatNumber(num(stats.expired_keys)) }}</span></div>
            <div class="cu-row"><span>淘汰键</span><span class="num">{{ formatNumber(num(stats.evicted_keys)) }}</span></div>
            <div class="cu-row"><span>网络输入</span><span class="num">{{ formatBytes(num(stats.total_net_input_bytes)) }}</span></div>
            <div class="cu-row"><span>网络输出</span><span class="num">{{ formatBytes(num(stats.total_net_output_bytes)) }}</span></div>
          </div>
        </div>
      </div>

      <!-- 键空间 -->
      <div class="section">
        <div class="sec-title">键空间</div>
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
        <a-empty v-if="!keyspaceRows.length" description="暂无键空间数据" :image-style="{ height: '40px' }" />
      </div>

      <!-- 客户端 -->
      <div class="section">
        <div class="sec-title">客户端连接<span class="sec-count num">{{ clients.length }}</span></div>
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
              <a-tooltip title="断开该客户端">
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
.sec-count {
  font-size: 10.5px; font-weight: 500;
  color: var(--text-3);
  background: var(--bg-hover);
  border-radius: 8px;
  padding: 1px 7px;
}
</style>
