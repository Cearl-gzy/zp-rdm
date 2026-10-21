<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { message, Modal } from 'ant-design-vue'
import { ReloadOutlined, ClearOutlined, ExportOutlined, MedicineBoxOutlined } from '@ant-design/icons-vue'
import { MemoryStats, MemoryDoctor, MemoryPurge, MemoryInfoSection } from '../../wailsjs/go/service/ServerService'
import { formatBytes } from '../utils/format'
import { exportReport } from '../utils/export'

// 内存医生：MEMORY STATS 构成 + MEMORY DOCTOR 诊断 + PURGE + 报告导出
const props = defineProps<{ connId: string; db: number }>()
const { t } = useI18n()

const loading = ref(false)
const supported = ref(true)
const stats = ref<Record<string, any>>({})
const info = ref<Record<string, string>>({})
const doctor = ref('')

const num = (v: any): number => (typeof v === 'number' ? v : parseFloat(v) || 0)
const infoNum = (k: string): number => parseFloat(info.value[k] ?? '') || 0

async function load() {
  loading.value = true
  try {
    stats.value = (await MemoryStats(props.connId, props.db)) ?? {}
    supported.value = true
  } catch (e: any) {
    if (/unknown (sub)?command/i.test(String(e?.message ?? e))) {
      supported.value = false
      stats.value = {}
    } else {
      message.error(String(e?.message ?? e))
    }
  }
  try {
    info.value = (await MemoryInfoSection(props.connId, props.db)) ?? {}
  } catch { /* 忽略 */ }
  try {
    doctor.value = await MemoryDoctor(props.connId, props.db)
  } catch { doctor.value = '' }
  loading.value = false
}

// 数据集 vs 开销构成条
const datasetBytes = computed(() => num(stats.value['dataset.bytes']))
const overheadBytes = computed(() => num(stats.value['overhead.total']))
const totalAllocated = computed(() => num(stats.value['total.allocated']))
const datasetPct = computed(() => {
  const t = totalAllocated.value
  return t > 0 ? Math.min(100, (datasetBytes.value / t) * 100) : 0
})
const overheadPct = computed(() => {
  const t = totalAllocated.value
  return t > 0 ? Math.min(100 - datasetPct.value, (overheadBytes.value / t) * 100) : 0
})

// 诊断结论着色：检测出问题 → 警告；健康 → 成功
const doctorBad = computed(() => /detected a few issues/i.test(doctor.value))

const fragmentation = computed(() => infoNum('mem_fragmentation_ratio'))
const fragClass = computed(() => {
  const f = fragmentation.value
  if (f <= 0) return ''
  if (f > 1.5) return 'bad'
  if (f > 1.2) return 'mid'
  return 'good'
})

// 开销明细行
interface OverheadRow { label: string; bytes: number; note?: string }
const overheadRows = computed<OverheadRow[]>(() => {
  const s = stats.value
  const rows: OverheadRow[] = [
    { label: 'startup.allocated', bytes: num(s['startup.allocated']), note: t('memory.noteStartup') },
    { label: 'replication.backlog', bytes: num(s['replication.backlog']) },
    { label: 'clients.normal', bytes: num(s['clients.normal']) },
    { label: 'clients.slaves', bytes: num(s['clients.slaves']) },
    { label: 'aof.buffer', bytes: num(s['aof.buffer']) },
    { label: 'lua.caches', bytes: num(s['lua.caches']) },
  ]
  if ('functions.caches' in s) rows.push({ label: 'functions.caches', bytes: num(s['functions.caches']) })
  rows.push(
    { label: 'overhead.hashtable.main', bytes: num(s['overhead.hashtable.main']) },
    { label: 'overhead.hashtable.expires', bytes: num(s['overhead.hashtable.expires']) },
  )
  // db.N 嵌套：{overhead.hashtable.main, overhead.hashtable.expires}
  for (const k of Object.keys(s)) {
    if (!/^db\.\d+$/.test(k)) continue
    const dbm = s[k]
    if (dbm && typeof dbm === 'object') {
      const main = num(dbm['overhead.hashtable.main'])
      const exp = num(dbm['overhead.hashtable.expires'])
      if (main || exp) rows.push({ label: k, bytes: main + exp, note: `main ${formatBytes(main)} / expires ${formatBytes(exp)}` })
    }
  }
  return rows.filter(r => r.bytes > 0 || r.label === 'startup.allocated')
})

const keysCount = computed(() => num(stats.value['keys.count']))
const bytesPerKey = computed(() => num(stats.value['keys.bytes-per-key']))

function purge() {
  Modal.confirm({
    title: t('memory.purgeTitle'),
    content: t('memory.purgeContent'),
    okText: t('common.confirm'),
    cancelText: t('common.cancel'),
    async onOk() {
      try {
        await MemoryPurge(props.connId, props.db)
        message.success(t('memory.purged'))
        await load()
      } catch (e: any) {
        message.error(String(e?.message ?? e))
      }
    },
  })
}

async function exportJson() {
  const report = {
    generatedAt: new Date().toISOString(),
    connId: props.connId,
    db: props.db,
    infoMemory: info.value,
    memoryStats: stats.value,
    memoryDoctor: doctor.value,
  }
  await exportReport('zp-rdm-memory-report.json', JSON.stringify(report, null, 2), t('memory.exported'))
}

onMounted(load)
</script>

<template>
  <div class="mem-panel">
    <a-alert
      v-if="!supported"
      type="warning"
      show-icon
      :message="t('memory.unsupported')"
    />
    <template v-else>
      <div class="mp-toolbar">
        <span class="mp-title"><MedicineBoxOutlined /> {{ t('memory.title') }}</span>
        <div class="mp-actions">
          <a-button size="small" @click="exportJson">
            <template #icon><ExportOutlined /></template>
            {{ t('memory.export') }}
          </a-button>
          <a-button size="small" danger @click="purge">
            <template #icon><ClearOutlined /></template>
            MEMORY PURGE
          </a-button>
          <a-button size="small" type="text" :loading="loading" @click="load">
            <template #icon><ReloadOutlined /></template>
          </a-button>
        </div>
      </div>

      <!-- 概览卡片 -->
      <div class="mp-cards">
        <div class="mp-card">
          <div class="mp-card-label">{{ t('memory.usedMemory') }}</div>
          <div class="mp-card-value num">{{ formatBytes(infoNum('used_memory')) }}</div>
          <div class="mp-card-sub text-3">{{ t('memory.peak') }} {{ formatBytes(infoNum('used_memory_peak')) }}</div>
        </div>
        <div class="mp-card">
          <div class="mp-card-label">{{ t('memory.rss') }}</div>
          <div class="mp-card-value num">{{ formatBytes(infoNum('used_memory_rss')) }}</div>
          <div class="mp-card-sub text-3">{{ t('memory.allocator') }} {{ info['mem_allocator'] || '-' }}</div>
        </div>
        <div class="mp-card">
          <div class="mp-card-label">{{ t('memory.fragmentation') }}</div>
          <div class="mp-card-value num" :class="fragClass">{{ fragmentation || '-' }}</div>
          <div class="mp-card-sub text-3">{{ t('memory.fragTip') }}</div>
        </div>
        <div class="mp-card">
          <div class="mp-card-label">{{ t('memory.keys') }}</div>
          <div class="mp-card-value num">{{ keysCount }}</div>
          <div class="mp-card-sub text-3">{{ formatBytes(bytesPerKey) }} / key</div>
        </div>
      </div>

      <!-- 数据集 vs 开销构成条 -->
      <div class="mp-section">
        <div class="mp-sec-title">{{ t('memory.composition') }} <span class="text-3 num">{{ formatBytes(totalAllocated) }}</span></div>
        <div class="comp-bar">
          <div class="comp-dataset" :style="{ width: datasetPct + '%' }" />
          <div class="comp-overhead" :style="{ width: overheadPct + '%' }" />
        </div>
        <div class="comp-legend">
          <span><i class="dot dot-dataset" />{{ t('memory.dataset') }} {{ formatBytes(datasetBytes) }}（{{ datasetPct.toFixed(1) }}%）</span>
          <span><i class="dot dot-overhead" />{{ t('memory.overhead') }} {{ formatBytes(overheadBytes) }}（{{ overheadPct.toFixed(1) }}%）</span>
        </div>
      </div>

      <div class="mp-cols">
        <!-- 开销明细 -->
        <div class="mp-section mp-col">
          <div class="mp-sec-title">{{ t('memory.overheadDetail') }}</div>
          <div class="ov-list">
            <div v-for="r in overheadRows" :key="r.label" class="ov-item">
              <span class="mono ov-label">{{ r.label }}</span>
              <span v-if="r.note" class="text-3 ov-note">{{ r.note }}</span>
              <span class="num ov-bytes">{{ formatBytes(r.bytes) }}</span>
            </div>
          </div>
        </div>

        <!-- 分配器 + 诊断 -->
        <div class="mp-col">
          <div class="mp-section">
            <div class="mp-sec-title">{{ t('memory.allocatorStats') }}</div>
            <div class="ov-list">
              <div class="ov-item"><span class="mono ov-label">allocator.allocated</span><span class="num ov-bytes">{{ formatBytes(num(stats['allocator.allocated'])) }}</span></div>
              <div class="ov-item"><span class="mono ov-label">allocator.active</span><span class="num ov-bytes">{{ formatBytes(num(stats['allocator.active'])) }}</span></div>
              <div class="ov-item"><span class="mono ov-label">allocator.resident</span><span class="num ov-bytes">{{ formatBytes(num(stats['allocator.resident'])) }}</span></div>
              <div class="ov-item"><span class="mono ov-label">allocator-fragmentation</span><span class="num ov-bytes">{{ stats['allocator-fragmentation.ratio'] ?? '-' }}</span></div>
            </div>
          </div>

          <div class="mp-section mp-doctor" :class="{ bad: doctorBad }">
            <div class="mp-sec-title">
              {{ t('memory.doctor') }}
              <a-tag v-if="doctor" :color="doctorBad ? 'warning' : 'success'" style="margin-left: 8px">
                {{ doctorBad ? t('memory.doctorIssues') : t('memory.doctorOk') }}
              </a-tag>
            </div>
            <pre class="doctor-text">{{ doctor || t('memory.doctorEmpty') }}</pre>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.mem-panel { height: 100%; display: flex; flex-direction: column; gap: 10px; padding: 12px 16px; overflow-y: auto; }
.mp-toolbar { display: flex; align-items: center; justify-content: space-between; flex-shrink: 0; }
.mp-title { font-size: 13px; font-weight: 600; display: inline-flex; align-items: center; gap: 6px; }
.mp-actions { display: flex; align-items: center; gap: 6px; }

.mp-cards { display: grid; grid-template-columns: repeat(4, 1fr); gap: 10px; flex-shrink: 0; }
.mp-card {
  background: var(--bg-panel);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  padding: 12px 14px;
}
.mp-card-label { font-size: 11px; color: var(--text-3); margin-bottom: 4px; }
.mp-card-value { font-size: 17px; font-weight: 600; }
.mp-card-value.good { color: var(--success, #3f9d63); }
.mp-card-value.mid { color: #f5a623; }
.mp-card-value.bad { color: #ff6b60; }
.mp-card-sub { font-size: 10.5px; margin-top: 2px; }

.mp-section {
  background: var(--bg-panel);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  padding: 12px 14px;
  flex-shrink: 0;
}
.mp-sec-title { font-size: 12.5px; font-weight: 600; margin-bottom: 10px; display: flex; align-items: center; gap: 8px; }

.comp-bar { display: flex; height: 14px; border-radius: 4px; overflow: hidden; background: var(--bg-inset); }
.comp-dataset { background: #3fbf6b; transition: width 0.3s; }
.comp-overhead { background: #f5a623; transition: width 0.3s; }
.comp-legend { display: flex; gap: 18px; margin-top: 8px; font-size: 11px; color: var(--text-2); }
.dot { display: inline-block; width: 8px; height: 8px; border-radius: 2px; margin-right: 5px; }
.dot-dataset { background: #3fbf6b; }
.dot-overhead { background: #f5a623; }

.mp-cols { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; align-items: start; }
.mp-col { display: flex; flex-direction: column; gap: 10px; min-width: 0; }

.ov-list { display: flex; flex-direction: column; gap: 4px; }
.ov-item {
  display: flex; align-items: center; gap: 8px;
  padding: 5px 9px;
  background: var(--bg-inset);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
}
.ov-label { font-size: 11.5px; flex-shrink: 0; }
.ov-note { flex: 1; font-size: 10.5px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ov-label + .ov-bytes { margin-left: auto; }
.ov-bytes { font-size: 11.5px; flex-shrink: 0; }
.ov-note ~ .ov-bytes { margin-left: 0; }

.mp-doctor { border-left: 3px solid var(--success, #3f9d63); }
.mp-doctor.bad { border-left-color: #f5a623; }
.doctor-text {
  margin: 0;
  font-family: var(--mono);
  font-size: 11.5px;
  line-height: 1.7;
  color: var(--text-2);
  white-space: pre-wrap;
  word-break: break-word;
  user-select: text;
  max-height: 260px;
  overflow-y: auto;
}
</style>
