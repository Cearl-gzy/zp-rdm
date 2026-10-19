<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { message } from 'ant-design-vue'
import { SearchOutlined, DownloadOutlined } from '@ant-design/icons-vue'
import { BigKeyReport as fetchBigKeyReport } from '../../wailsjs/go/service/KeyService'
import type { BigKeyReport } from '../types'
import { formatBytes, formatTTL, formatNumber } from '../utils/format'
import { exportReport, csvSection } from '../utils/export'

const props = defineProps<{ connId: string; db: number }>()

const { t } = useI18n()

const data = ref<BigKeyReport | null>(null)
const loading = ref(false)
const loaded = ref(false)
const pattern = ref('*')
const maxScan = ref(50000)
const topN = ref(50)

async function scan() {
  if (loading.value) return
  loading.value = true
  try {
    data.value = (await fetchBigKeyReport(props.connId, props.db, pattern.value || '*', maxScan.value, topN.value)) as unknown as BigKeyReport
    loaded.value = true
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    loading.value = false
  }
}
watch(() => [props.connId, props.db], () => {
  data.value = null
  loaded.value = false
  scan()
})
onMounted(scan)

const maxTopSize = computed(() => Math.max(1, data.value?.top[0]?.size ?? 1))
const maxTypeSize = computed(() => Math.max(1, ...(data.value?.types.map(t => t.size) ?? [1])))

// 导出当前报告（JSON 原始结构 / CSV 分区表）
function onExport({ key }: { key: string }) {
  const d = data.value
  if (!d) return
  if (key === 'json') {
    exportReport('zp-rdm-bigkey-report.json', JSON.stringify(d, null, 2) + '\n', t('analysis.exportDone'))
    return
  }
  const csv = [
    csvSection('Summary', ['pattern', 'total_keys', 'scanned', 'elapsed_ms', 'truncated'],
      [[pattern.value || '*', d.totalKeys, d.scanned, d.elapsedMs, d.truncated ? 1 : 0]]),
    csvSection('Type Stats', ['type', 'count', 'size_bytes'],
      d.types.map(ts => [ts.type, ts.count, ts.size])),
    csvSection('TOP Big Keys', ['key', 'type', 'size_bytes', 'elements', 'ttl_seconds'],
      d.top.map(k => [k.key, k.type, k.size, k.len, k.ttl])),
  ].join('\n\n') + '\n'
  exportReport('zp-rdm-bigkey-report.csv', csv, t('analysis.exportDone'))
}
</script>

<template>
  <div class="bk">
    <div class="bk-toolbar">
      <a-input
        v-model:value="pattern" size="small" class="bk-pattern mono"
        :placeholder="t('analysis.bigkey.patternTip')" allow-clear
        @press-enter="scan"
      />
      <a-select v-model:value="maxScan" size="small" style="width: 108px">
        <a-select-option :value="10000">{{ t('analysis.bigkey.sample', { n: '1万' }) }}</a-select-option>
        <a-select-option :value="50000">{{ t('analysis.bigkey.sample', { n: '5万' }) }}</a-select-option>
        <a-select-option :value="100000">{{ t('analysis.bigkey.sample', { n: '10万' }) }}</a-select-option>
        <a-select-option :value="200000">{{ t('analysis.bigkey.sample', { n: '20万' }) }}</a-select-option>
      </a-select>
      <a-select v-model:value="topN" size="small" style="width: 92px">
        <a-select-option :value="20">TOP 20</a-select-option>
        <a-select-option :value="50">TOP 50</a-select-option>
        <a-select-option :value="100">TOP 100</a-select-option>
        <a-select-option :value="200">TOP 200</a-select-option>
      </a-select>
      <a-button size="small" type="primary" :loading="loading" @click="scan">
        <template #icon><SearchOutlined /></template>
        {{ t('analysis.bigkey.scan') }}
      </a-button>
      <a-dropdown v-if="data">
        <a-button size="small">
          <template #icon><DownloadOutlined /></template>
          {{ t('analysis.export') }}
        </a-button>
        <template #overlay>
          <a-menu @click="onExport">
            <a-menu-item key="json">{{ t('analysis.exportJson') }}</a-menu-item>
            <a-menu-item key="csv">{{ t('analysis.exportCsv') }}</a-menu-item>
          </a-menu>
        </template>
      </a-dropdown>
      <span v-if="data" class="text-3 bk-meta num">
        {{ t('analysis.bigkey.meta', { scanned: formatNumber(data.scanned), total: formatNumber(data.totalKeys), ms: formatNumber(data.elapsedMs) }) }}
      </span>
    </div>

    <a-alert
      v-if="data?.truncated" type="warning" show-icon class="bk-alert"
      :message="t('analysis.bigkey.truncated', { n: formatNumber(data.scanned) })"
    />

    <div v-if="!loaded && loading" class="bk-loading">
      <a-spin size="large" />
      <div class="text-3" style="margin-top: 12px; font-size: 12px">{{ t('analysis.scanning') }}</div>
    </div>

    <div v-else-if="data" class="bk-body">
      <!-- 类型聚合 -->
      <div class="bk-panel">
        <div class="bk-panel-title">{{ t('analysis.bigkey.typeAgg') }}</div>
        <div v-if="!data.types.length" class="bk-empty">{{ t('common.empty') }}</div>
        <div v-for="ts in data.types" :key="ts.type" class="bk-dist-row">
          <span class="type-badge" :class="`type-${ts.type}`">{{ ts.type }}</span>
          <div class="bk-bar-wrap">
            <div class="bk-bar mem" :style="{ width: `${Math.max(1.5, (ts.size / maxTypeSize) * 100)}%` }" />
          </div>
          <span class="bk-dist-count num">{{ formatBytes(ts.size) }}</span>
          <span class="bk-dist-sub num text-3">{{ t('analysis.keysCount', { n: formatNumber(ts.count) }) }}</span>
        </div>
      </div>

      <!-- TOP 大键 -->
      <div class="bk-panel">
        <div class="bk-panel-title">
          {{ t('analysis.bigkey.topTitle') }}
          <span class="bk-panel-sub">TOP {{ data.top.length }} · MEMORY USAGE</span>
        </div>
        <div v-if="!data.top.length" class="bk-empty">{{ t('common.empty') }}</div>
        <div class="bk-row bk-head">
          <span class="bk-col-type" />
          <span class="bk-col-key">{{ t('analysis.bigkey.colKey') }}</span>
          <span class="bk-col-len">{{ t('analysis.bigkey.colLen') }}</span>
          <span class="bk-col-ttl">TTL</span>
          <span class="bk-col-bar" />
          <span class="bk-col-size">{{ t('analysis.bigkey.colSize') }}</span>
        </div>
        <div v-for="k in data.top" :key="k.key" class="bk-row">
          <span class="bk-col-type"><span class="type-badge" :class="`type-${k.type}`">{{ k.type }}</span></span>
          <span class="bk-col-key mono" :title="k.key">{{ k.key }}</span>
          <span class="bk-col-len num">{{ formatNumber(k.len) }}</span>
          <span class="bk-col-ttl num text-3">{{ k.ttl >= 0 ? formatTTL(k.ttl) : t('analysis.persistent') }}</span>
          <div class="bk-col-bar bk-bar-wrap">
            <div class="bk-bar" :style="{ width: `${Math.max(2, (k.size / maxTopSize) * 100)}%` }" />
          </div>
          <span class="bk-col-size num">{{ formatBytes(k.size) }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.bk { display: flex; flex-direction: column; }
.bk-toolbar {
  display: flex; align-items: center; gap: 8px;
  padding: 12px 0 10px;
}
.bk-pattern { width: 200px; }
.bk-meta { font-size: 11px; margin-left: auto; }
.bk-alert { margin-bottom: 10px; }
.bk-loading {
  display: flex; flex-direction: column;
  align-items: center; justify-content: center;
  padding: 80px 0;
}
.bk-body { display: flex; flex-direction: column; gap: 10px; }
.bk-panel {
  background: var(--bg-panel);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  padding: 14px 16px;
}
.bk-panel-title {
  font-size: 12.5px; font-weight: 600;
  margin-bottom: 12px;
  display: flex; align-items: baseline; gap: 8px;
}
.bk-panel-sub { font-size: 10px; color: var(--text-3); font-weight: 400; letter-spacing: 0.4px; }
.bk-empty { color: var(--text-3); font-size: 12px; padding: 16px 0; text-align: center; }

.bk-dist-row { display: flex; align-items: center; gap: 8px; padding: 4px 0; }
.bk-dist-count { font-size: 11px; color: var(--text-2); width: 70px; text-align: right; flex-shrink: 0; }
.bk-dist-sub { font-size: 10px; width: 60px; text-align: right; flex-shrink: 0; }

.bk-row {
  display: flex; align-items: center; gap: 8px;
  padding: 5px 0;
}
.bk-head {
  padding: 0 0 6px;
  border-bottom: 1px solid var(--border);
  margin-bottom: 4px;
  font-size: 10px; color: var(--text-3); letter-spacing: 0.5px;
}
.bk-col-type { width: 52px; flex-shrink: 0; }
.bk-col-key {
  flex: 0 1 auto; min-width: 0; max-width: 34%;
  font-size: 12px;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.bk-head .bk-col-key { max-width: 34%; }
.bk-col-len { font-size: 11px; color: var(--text-2); width: 72px; text-align: right; flex-shrink: 0; }
.bk-col-ttl { font-size: 10px; width: 52px; flex-shrink: 0; }
.bk-col-bar { flex: 1; }
.bk-col-size { font-size: 11px; color: var(--text-2); width: 74px; text-align: right; flex-shrink: 0; }

.bk-bar-wrap {
  height: 5px;
  background: var(--bg-inset);
  border-radius: 3px;
  overflow: hidden;
}
.bk-bar {
  height: 100%;
  background: linear-gradient(90deg, var(--accent), var(--accent-hover));
  border-radius: 3px;
  transition: width 0.4s var(--ease);
}
.bk-bar.mem { background: var(--warning); }
</style>
