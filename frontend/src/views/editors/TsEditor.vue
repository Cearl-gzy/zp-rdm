<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { message } from 'ant-design-vue'
import { PlusOutlined, ReloadOutlined, ClockCircleOutlined } from '@ant-design/icons-vue'
import { TsInfo, TsRange, TsAdd } from '../../../wailsjs/go/service/TsService'
import { useReadonly } from '../../utils/readonly'
import { formatBytes } from '../../utils/format'
import type { TsInfo as TsInfoT, TsSample } from '../../types'

const props = defineProps<{ connId: string; db: number; keyName: string; total: number }>()

const { t } = useI18n()
const ro = useReadonly(() => props.connId)

// ---------- 概览（TS.INFO） ----------
const info = ref<TsInfoT | null>(null)

async function loadInfo() {
  try {
    info.value = (await TsInfo(props.connId, props.db, props.keyName)) as unknown as TsInfoT
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  }
}

function fmtTs(ts: number): string {
  if (!ts) return '—'
  return new Date(ts).toLocaleString()
}

function fmtRetention(ms: number): string {
  if (!ms) return t('ts.retainForever')
  const s = Math.floor(ms / 1000)
  if (s < 3600) return `${Math.floor(s / 60)}m`
  if (s < 86400) return `${(s / 3600).toFixed(1)}h`
  return `${(s / 86400).toFixed(1)}d`
}

const infoPills = computed(() => {
  const i = info.value
  if (!i) return []
  return [
    { label: t('ts.samples'), value: String(i.totalSamples) },
    { label: t('ts.memory'), value: formatBytes(i.memoryUsage) },
    { label: t('ts.retention'), value: fmtRetention(i.retentionMs) },
    { label: t('ts.chunks'), value: String(i.chunkCount) },
  ]
})

const labelEntries = computed(() => Object.entries(info.value?.labels ?? {}))

// ---------- 区间查询（TS.RANGE） ----------
const RANGE_PRESETS = [
  { key: 'all', ms: 0 },
  { key: '5m', ms: 5 * 60_000 },
  { key: '1h', ms: 3_600_000 },
  { key: '1d', ms: 86_400_000 },
] as const
const AGGS = ['', 'avg', 'sum', 'min', 'max', 'count', 'first', 'last']

const preset = ref<string>('all')
const agg = ref('')
const bucketMs = ref<number | null>(60_000)
const samples = ref<TsSample[]>([])
const loading = ref(false)

function rangeBounds(): [number, number] {
  const p = RANGE_PRESETS.find(x => x.key === preset.value) ?? RANGE_PRESETS[0]
  if (!p.ms) return [0, 0]
  const last = info.value?.lastTimestamp || Date.now()
  return [last - p.ms + 1, last]
}

async function query() {
  loading.value = true
  try {
    const [from, to] = rangeBounds()
    samples.value = (await TsRange(
      props.connId, props.db, props.keyName, from, to,
      agg.value, agg.value ? (bucketMs.value ?? 60_000) : 0, 0,
    )) as unknown as TsSample[]
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    loading.value = false
  }
}

onMounted(async () => { await loadInfo(); await query() })

// ---------- SVG 折线图 ----------
const chart = computed(() => {
  const pts = samples.value
  if (pts.length < 2) return null
  const W = 640, H = 140, PAD = 4
  const xs = pts.map(p => p.ts)
  const ys = pts.map(p => p.value)
  const x0 = Math.min(...xs), x1 = Math.max(...xs)
  const y0 = Math.min(...ys), y1 = Math.max(...ys)
  const dx = x1 - x0 || 1, dy = y1 - y0 || 1
  const px = (ts: number) => PAD + ((ts - x0) / dx) * (W - 2 * PAD)
  const py = (v: number) => H - PAD - ((v - y0) / dy) * (H - 2 * PAD)
  return {
    w: W, h: H,
    line: pts.map(p => `${px(p.ts).toFixed(1)},${py(p.value).toFixed(1)}`).join(' '),
    min: y0, max: y1,
  }
})

// 最近样本表（倒序前 50 条）
const latestSamples = computed(() => [...samples.value].reverse().slice(0, 50))

// ---------- 追加样本（TS.ADD） ----------
const addValue = ref<number | null>(null)
const addTs = ref<number | null>(null)
const adding = ref(false)

async function doAdd() {
  if (addValue.value == null) {
    message.warning(t('ts.valueRequired'))
    return
  }
  adding.value = true
  try {
    await TsAdd(props.connId, props.db, props.keyName, addTs.value ?? 0, addValue.value)
    message.success(t('ts.added'))
    addValue.value = null
    addTs.value = null
    await loadInfo()
    await query()
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    adding.value = false
  }
}
</script>

<template>
  <div class="ts-editor">
    <div class="ts-toolbar">
      <div class="ts-pills">
        <span v-for="p in infoPills" :key="p.label" class="pill">
          <span class="pill-label">{{ p.label }}</span>
          <span class="pill-value num">{{ p.value }}</span>
        </span>
      </div>
      <a-tooltip :title="t('common.refresh')">
        <button class="icon-btn" @click="loadInfo(); query()"><ReloadOutlined /></button>
      </a-tooltip>
    </div>

    <div v-if="labelEntries.length || (info && info.rules.length)" class="ts-meta">
      <span v-for="[k, v] in labelEntries" :key="k" class="meta-tag">{{ k }}={{ v }}</span>
      <span v-for="r in info?.rules ?? []" :key="r.destKey" class="meta-tag rule">
        ↓ {{ r.aggregator }}({{ r.bucketMs }}ms) → {{ r.destKey }}
      </span>
    </div>

    <div class="ts-query">
      <a-radio-group v-model:value="preset" size="small" @change="query">
        <a-radio-button v-for="p in RANGE_PRESETS" :key="p.key" :value="p.key">{{ t(`ts.range_${p.key}`) }}</a-radio-button>
      </a-radio-group>
      <a-select v-model:value="agg" size="small" class="agg-select" @change="query">
        <a-select-option v-for="a in AGGS" :key="a" :value="a">
          {{ a === '' ? t('ts.noAgg') : a }}
        </a-select-option>
      </a-select>
      <a-input-number
        v-if="agg"
        v-model:value="bucketMs" size="small" :min="1" :step="1000"
        class="bucket-input" :addon-after="'ms'"
        @change="query"
      />
      <span class="text-3 query-hint">{{ t('ts.queryHint', { n: samples.length }) }}</span>
    </div>

    <a-spin :spinning="loading">
      <div v-if="chart" class="ts-chart">
        <svg :viewBox="`0 0 ${chart.w} ${chart.h}`" preserveAspectRatio="none">
          <polyline :points="chart.line" fill="none" stroke="var(--accent)" stroke-width="1.5" vector-effect="non-scaling-stroke" />
        </svg>
        <div class="chart-scale num">
          <span>{{ chart.max }}</span>
          <span>{{ chart.min }}</span>
        </div>
      </div>
      <a-empty v-else-if="!loading" :description="t('ts.empty')" style="margin: 24px 0" />

      <div v-if="latestSamples.length" class="ts-table">
        <div class="ts-row ts-head">
          <span>{{ t('ts.colTime') }}</span>
          <span>{{ t('ts.colTs') }}</span>
          <span class="col-val">{{ t('ts.colValue') }}</span>
        </div>
        <div v-for="s in latestSamples" :key="s.ts" class="ts-row num">
          <span>{{ fmtTs(s.ts) }}</span>
          <span class="text-3">{{ s.ts }}</span>
          <span class="col-val">{{ s.value }}</span>
        </div>
      </div>
    </a-spin>

    <div v-if="!ro" class="ts-add">
      <a-input-number v-model:value="addValue" size="small" :placeholder="t('ts.valuePlaceholder')" class="add-value" />
      <a-input-number v-model:value="addTs" size="small" :min="0" :placeholder="t('ts.tsPlaceholder')" class="add-ts" />
      <a-tooltip :title="t('ts.nowTip')">
        <button class="icon-btn bordered" @click="addTs = Date.now()"><ClockCircleOutlined /></button>
      </a-tooltip>
      <a-button type="primary" size="small" :loading="adding" @click="doAdd">
        <template v-if="!adding" #icon><PlusOutlined /></template>
        {{ t('ts.add') }}
      </a-button>
    </div>
  </div>
</template>

<style scoped>
.ts-editor { display: flex; flex-direction: column; gap: 10px; padding: 12px; height: 100%; overflow-y: auto; }
.ts-toolbar { display: flex; align-items: center; gap: 8px; }
.ts-pills { display: flex; gap: 6px; flex-wrap: wrap; flex: 1; }
.pill {
  display: inline-flex; align-items: center; gap: 5px;
  padding: 2px 8px; border: 1px solid var(--border); border-radius: 10px;
  font-size: 11px;
}
.pill-label { color: var(--text-3); }
.pill-value { color: var(--text-1); font-weight: 600; }

.icon-btn {
  display: inline-flex; align-items: center; justify-content: center;
  width: 26px; height: 26px; border: none; background: transparent;
  border-radius: var(--radius-sm); color: var(--text-3); cursor: pointer;
  font-size: 13px; transition: background 0.12s, color 0.12s; padding: 0;
}
.icon-btn:hover { background: var(--bg-hover); color: var(--text-1); }
.icon-btn.bordered { border: 1px solid var(--border); }

.ts-meta { display: flex; gap: 6px; flex-wrap: wrap; }
.meta-tag {
  font-size: 10.5px; padding: 1px 7px; border-radius: 3px;
  background: var(--bg-hover); color: var(--text-2);
  font-family: var(--mono, monospace);
}
.meta-tag.rule { color: var(--accent); background: var(--accent-soft); }

.ts-query { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.agg-select { width: 110px; }
.bucket-input { width: 150px; }
.query-hint { font-size: 11px; }

.ts-chart { position: relative; border: 1px solid var(--border); border-radius: var(--radius-md); padding: 4px; }
.ts-chart svg { width: 100%; height: 140px; display: block; }
.chart-scale {
  position: absolute; top: 6px; right: 10px;
  display: flex; flex-direction: column; align-items: flex-end;
  font-size: 10px; color: var(--text-3); line-height: 1.5;
}

.ts-table { border: 1px solid var(--border); border-radius: var(--radius-md); overflow: hidden; }
.ts-row {
  display: grid; grid-template-columns: 1.2fr 1fr 1fr; gap: 8px;
  padding: 4px 10px; font-size: 11.5px;
  border-top: 1px solid var(--border);
}
.ts-row:first-child { border-top: none; }
.ts-head { background: var(--bg-hover); color: var(--text-3); font-size: 10.5px; }
.col-val { text-align: right; }

.ts-add { display: flex; align-items: center; gap: 8px; }
.add-value { width: 160px; }
.add-ts { flex: 1; min-width: 120px; }
</style>
