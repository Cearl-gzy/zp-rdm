<script setup lang="ts">
import { ref, computed, watch, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { message } from 'ant-design-vue'
import { SearchOutlined, PlayCircleOutlined, StopOutlined, DownloadOutlined } from '@ant-design/icons-vue'
import {
  HotKeyFreq, HotKeySampleStart, HotKeySampleStop, HotKeySampleStats,
} from '../../wailsjs/go/service/ServerService'
import type { HotKeyFreqReport, HotKeySampleReport, HotKeySampleStats as SampleStats } from '../types'
import { formatNumber } from '../utils/format'
import { exportReport, csvSection } from '../utils/export'

const props = defineProps<{ connId: string; db: number }>()

const { t } = useI18n()

const mode = ref<'freq' | 'monitor'>('monitor')

// ---------- OBJECT FREQ ----------
const freqData = ref<HotKeyFreqReport | null>(null)
const freqLoading = ref(false)

async function freqScan() {
  if (freqLoading.value) return
  freqLoading.value = true
  try {
    freqData.value = (await HotKeyFreq(props.connId, props.db, 20000)) as unknown as HotKeyFreqReport
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    freqLoading.value = false
  }
}
const maxFreq = computed(() => Math.max(1, ...(freqData.value?.keys.map(k => k.freq) ?? [1])))

// ---------- MONITOR 采样 ----------
const sampling = ref(false)
const stats = ref<SampleStats | null>(null)
const report = ref<HotKeySampleReport | null>(null)
let pollTimer: ReturnType<typeof setInterval> | null = null

async function poll() {
  try {
    const st = (await HotKeySampleStats(props.connId, props.db)) as unknown as SampleStats
    stats.value = st
    if (!st.running && sampling.value) {
      // 服务端已停止（超时/断线）→ 取报告
      await stop(true)
    }
  } catch { /* 连接断开时下一次轮询继续尝试 */ }
}

async function start() {
  try {
    await HotKeySampleStart(props.connId, props.db)
    sampling.value = true
    report.value = null
    stats.value = null
    pollTimer = setInterval(poll, 1000)
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  }
}

async function stop(silent = false) {
  if (pollTimer) { clearInterval(pollTimer); pollTimer = null }
  try {
    report.value = (await HotKeySampleStop(props.connId, props.db)) as unknown as HotKeySampleReport
  } catch (e: any) {
    if (!silent) message.error(String(e?.message ?? e))
  } finally {
    sampling.value = false
  }
}

function stopPolling() {
  if (pollTimer) { clearInterval(pollTimer); pollTimer = null }
}
onBeforeUnmount(stopPolling)

// 切换连接/库时重置视图状态（服务端采样按 conn|db 独立，最多 300s 自动停止）
watch(() => [props.connId, props.db], () => {
  stopPolling()
  sampling.value = false
  stats.value = null
  report.value = null
  freqData.value = null
})

const maxHits = computed(() => Math.max(1, ...(report.value?.keys.map(k => k.hits) ?? [1])))

// 导出当前结果（JSON 原始结构 / CSV 分区表）；MONITOR 与 FREQ 模式各自导出当前视图数据
function onExport({ key }: { key: string }) {
  if (mode.value === 'monitor') {
    const d = report.value
    if (!d) return
    if (key === 'json') {
      exportReport('zp-rdm-hotkey-report.json', JSON.stringify(d, null, 2) + '\n', t('analysis.exportDone'))
      return
    }
    const csv = [
      csvSection('Sample Summary', ['duration_sec', 'total_cmds', 'unique_keys', 'truncated'],
        [[d.durationSec, d.totalCmds, d.uniqueKeys, d.truncated ? 1 : 0]]),
      csvSection('Hot Keys', ['key', 'hits', 'reads', 'writes'],
        d.keys.map(k => [k.key, k.hits, k.reads, k.writes])),
    ].join('\n\n') + '\n'
    exportReport('zp-rdm-hotkey-report.csv', csv, t('analysis.exportDone'))
    return
  }
  const d = freqData.value
  if (!d?.available) return
  if (key === 'json') {
    exportReport('zp-rdm-hotkey-freq.json', JSON.stringify(d, null, 2) + '\n', t('analysis.exportDone'))
    return
  }
  const csv = [
    csvSection('Freq Summary', ['policy', 'scanned'], [[d.policy, d.scanned]]),
    csvSection('LFU Hot Keys', ['key', 'type', 'freq'], d.keys.map(k => [k.key, k.type, k.freq])),
  ].join('\n\n') + '\n'
  exportReport('zp-rdm-hotkey-freq.csv', csv, t('analysis.exportDone'))
}
</script>

<template>
  <div class="hk">
    <div class="hk-toolbar">
      <a-segmented
        v-model:value="mode"
        :options="[
          { value: 'monitor', label: t('analysis.hotkey.monitorMode') },
          { value: 'freq', label: t('analysis.hotkey.freqMode') },
        ]"
      />

      <template v-if="mode === 'monitor'">
        <a-button v-if="!sampling" size="small" type="primary" @click="start">
          <template #icon><PlayCircleOutlined /></template>
          {{ t('analysis.hotkey.start') }}
        </a-button>
        <a-button v-else size="small" danger @click="stop(false)">
          <template #icon><StopOutlined /></template>
          {{ t('analysis.hotkey.stop') }}
        </a-button>
        <span v-if="sampling && stats" class="text-3 hk-live num">
          <span class="hk-dot" />
          {{ t('analysis.hotkey.live', { s: stats.elapsedSec, cmds: formatNumber(stats.totalCmds), keys: formatNumber(stats.uniqueKeys) }) }}
        </span>
        <a-dropdown v-if="report && !sampling">
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
        <span class="text-3 hk-tip">{{ t('analysis.hotkey.monitorTip') }}</span>
      </template>

      <template v-else>
        <a-button size="small" type="primary" :loading="freqLoading" @click="freqScan">
          <template #icon><SearchOutlined /></template>
          {{ t('analysis.bigkey.scan') }}
        </a-button>
        <span v-if="freqData?.available" class="text-3 hk-meta num">
          {{ t('analysis.hotkey.freqScanned', { n: formatNumber(freqData.scanned) }) }}
        </span>
        <a-dropdown v-if="freqData?.available">
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
        <span class="text-3 hk-tip">{{ t('analysis.hotkey.freqTip') }}</span>
      </template>
    </div>

    <!-- MONITOR 采样结果 -->
    <template v-if="mode === 'monitor'">
      <a-alert
        v-if="report?.truncated" type="warning" show-icon class="hk-alert"
        :message="t('analysis.hotkey.autoStopped')"
      />
      <div v-if="report" class="hk-panel">
        <div class="hk-panel-title">
          {{ t('analysis.hotkey.reportTitle') }}
          <span class="hk-panel-sub num">
            {{ t('analysis.hotkey.reportMeta', { cmds: formatNumber(report.totalCmds), keys: formatNumber(report.uniqueKeys), s: report.durationSec.toFixed(1) }) }}
          </span>
        </div>
        <div v-if="!report.keys.length" class="hk-empty">{{ t('common.empty') }}</div>
        <div class="hk-row hk-head">
          <span class="hk-col-key">{{ t('analysis.bigkey.colKey') }}</span>
          <span class="hk-col-hits">{{ t('analysis.hotkey.colHits') }}</span>
          <span class="hk-col-rw">{{ t('analysis.hotkey.colReads') }}</span>
          <span class="hk-col-rw">{{ t('analysis.hotkey.colWrites') }}</span>
          <span class="hk-col-bar" />
        </div>
        <div v-for="k in report.keys" :key="k.key" class="hk-row">
          <span class="hk-col-key mono" :title="k.key">{{ k.key }}</span>
          <span class="hk-col-hits num">{{ formatNumber(k.hits) }}</span>
          <span class="hk-col-rw num read">{{ formatNumber(k.reads) }}</span>
          <span class="hk-col-rw num write">{{ formatNumber(k.writes) }}</span>
          <div class="hk-col-bar hk-bar-wrap">
            <div class="hk-bar" :style="{ width: `${Math.max(2, (k.hits / maxHits) * 100)}%` }" />
          </div>
        </div>
      </div>
      <div v-else-if="!sampling" class="hk-placeholder text-3">
        {{ t('analysis.hotkey.placeholder') }}
      </div>
    </template>

    <!-- OBJECT FREQ 结果 -->
    <template v-else>
      <a-alert
        v-if="freqData && !freqData.available" type="info" show-icon class="hk-alert"
        :message="t('analysis.hotkey.freqUnavailable', { policy: freqData.policy || 'unknown' })"
        :description="t('analysis.hotkey.freqUnavailableHint')"
      />
      <div v-if="freqData?.available" class="hk-panel">
        <div class="hk-panel-title">
          {{ t('analysis.hotkey.freqTitle') }}
          <span class="hk-panel-sub">TOP {{ freqData.keys.length }} · OBJECT FREQ</span>
        </div>
        <div v-if="!freqData.keys.length" class="hk-empty">{{ t('common.empty') }}</div>
        <div v-for="k in freqData.keys" :key="k.key" class="hk-row">
          <span class="hk-col-type"><span class="type-badge" :class="`type-${k.type}`">{{ k.type }}</span></span>
          <span class="hk-col-key mono" :title="k.key">{{ k.key }}</span>
          <div class="hk-col-bar hk-bar-wrap">
            <div class="hk-bar" :style="{ width: `${Math.max(2, (k.freq / maxFreq) * 100)}%` }" />
          </div>
          <span class="hk-col-hits num">{{ formatNumber(k.freq) }}</span>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.hk { display: flex; flex-direction: column; }
.hk-toolbar {
  display: flex; align-items: center; gap: 10px;
  padding: 12px 0 10px;
}
.hk-tip { font-size: 11px; margin-left: auto; }
.hk-meta { font-size: 11px; }
.hk-live { font-size: 11px; display: inline-flex; align-items: center; gap: 6px; }
.hk-dot {
  width: 6px; height: 6px; border-radius: 50%;
  background: var(--error, #ff4d4f);
  animation: hk-blink 1s ease-in-out infinite;
}
@keyframes hk-blink { 50% { opacity: 0.25; } }
.hk-alert { margin-bottom: 10px; }
.hk-placeholder { padding: 48px 0; text-align: center; font-size: 12px; }

.hk-panel {
  background: var(--bg-panel);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  padding: 14px 16px;
}
.hk-panel-title {
  font-size: 12.5px; font-weight: 600;
  margin-bottom: 12px;
  display: flex; align-items: baseline; gap: 8px;
}
.hk-panel-sub { font-size: 10px; color: var(--text-3); font-weight: 400; }
.hk-empty { color: var(--text-3); font-size: 12px; padding: 16px 0; text-align: center; }

.hk-row {
  display: flex; align-items: center; gap: 8px;
  padding: 5px 0;
}
.hk-head {
  padding: 0 0 6px;
  border-bottom: 1px solid var(--border);
  margin-bottom: 4px;
  font-size: 10px; color: var(--text-3); letter-spacing: 0.5px;
}
.hk-col-type { width: 52px; flex-shrink: 0; }
.hk-col-key {
  flex: 0 1 auto; min-width: 0; max-width: 40%;
  font-size: 12px;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.hk-head .hk-col-key { flex: 1; max-width: none; }
.hk-col-hits { font-size: 11px; color: var(--text-2); width: 70px; text-align: right; flex-shrink: 0; }
.hk-col-rw { font-size: 11px; width: 60px; text-align: right; flex-shrink: 0; }
.hk-col-rw.read { color: var(--info); }
.hk-col-rw.write { color: var(--warning); }
.hk-col-bar { flex: 1; }

.hk-bar-wrap {
  height: 5px;
  background: var(--bg-inset);
  border-radius: 3px;
  overflow: hidden;
}
.hk-bar {
  height: 100%;
  background: linear-gradient(90deg, var(--accent), var(--accent-hover));
  border-radius: 3px;
  transition: width 0.4s var(--ease);
}
</style>
