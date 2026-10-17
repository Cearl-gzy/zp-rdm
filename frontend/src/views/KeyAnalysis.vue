<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { message } from 'ant-design-vue'
import { ReloadOutlined, KeyOutlined, PieChartOutlined, DatabaseOutlined, FieldTimeOutlined } from '@ant-design/icons-vue'
import { AnalyzeKeys } from '../../wailsjs/go/service/KeyService'
import type { KeyAnalysis } from '../types'
import { formatBytes, formatTTL, formatNumber } from '../utils/format'
import BigKeyReport from './BigKeyReport.vue'
import HotKeyStats from './HotKeyStats.vue'

const props = defineProps<{ connId: string; db: number }>()

const { t } = useI18n()

const mode = ref<'overview' | 'bigkey' | 'hotkey'>('overview')
const data = ref<KeyAnalysis | null>(null)
const loading = ref(false)
const delimiter = ref(':')

async function analyze() {
  if (loading.value) return
  loading.value = true
  try {
    data.value = (await AnalyzeKeys(props.connId, props.db, delimiter.value)) as unknown as KeyAnalysis
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    loading.value = false
  }
}
watch(() => [props.connId, props.db], () => { data.value = null; analyze() }, { immediate: true })

const maxPrefixSize = computed(() => Math.max(1, ...(data.value?.prefixes.map(p => p.size) ?? [1])))
const maxBucketCount = computed(() => Math.max(1, ...(data.value?.ttlBuckets.map(b => b.count) ?? [1])))
const maxTopSize = computed(() => Math.max(1, data.value?.topKeys[0]?.size ?? 1))

const persistentRatio = computed(() => {
  if (!data.value || !data.value.scanned) return 0
  const persistent = data.value.ttlBuckets[0]?.count ?? 0
  return Math.round((persistent / data.value.scanned) * 100)
})
const avgSize = computed(() => {
  if (!data.value || !data.value.scanned) return 0
  return Math.round(data.value.totalMemory / data.value.scanned)
})
</script>

<template>
  <div class="analysis">
    <div class="an-mode-bar">
      <a-segmented
        v-model:value="mode"
        :options="[
          { value: 'overview', label: t('analysis.modeOverview') },
          { value: 'bigkey', label: t('analysis.modeBigKey') },
          { value: 'hotkey', label: t('analysis.modeHotKey') },
        ]"
      />
    </div>

    <div v-if="mode === 'bigkey'" class="an-sub">
      <BigKeyReport :conn-id="connId" :db="db" />
    </div>
    <div v-else-if="mode === 'hotkey'" class="an-sub">
      <HotKeyStats :conn-id="connId" :db="db" />
    </div>

    <template v-else>
    <div class="an-toolbar">
      <a-tooltip :title="t('analysis.delimiterTip')">
        <a-select v-model:value="delimiter" size="small" style="width: 72px" @change="analyze">
          <a-select-option value=":">:</a-select-option>
          <a-select-option value="/">/</a-select-option>
          <a-select-option value=".">.</a-select-option>
          <a-select-option value="_">_</a-select-option>
          <a-select-option value="-">-</a-select-option>
        </a-select>
      </a-tooltip>
      <a-button size="small" :loading="loading" @click="analyze">
        <template #icon><ReloadOutlined /></template>
        {{ t('analysis.reanalyze') }}
      </a-button>
      <span v-if="data" class="text-3 an-scan-info num">
        {{ t('analysis.scannedInfo', { scanned: formatNumber(data.scanned), total: formatNumber(data.totalKeys) }) }}
        <template v-if="data.truncated">{{ t('analysis.scanTruncated') }}</template>
      </span>
    </div>

    <div v-if="!data && loading" class="an-loading">
      <a-spin size="large" />
      <div class="text-3" style="margin-top: 12px; font-size: 12px">{{ t('analysis.scanning') }}</div>
    </div>

    <div v-else-if="data" class="an-body">
      <!-- 概览卡片 -->
      <div class="an-cards">
        <div class="an-card">
          <div class="an-card-icon"><KeyOutlined /></div>
          <div class="an-card-num num">{{ formatNumber(data.totalKeys) }}</div>
          <div class="an-card-label">{{ t('analysis.totalKeys') }}</div>
        </div>
        <div class="an-card">
          <div class="an-card-icon"><DatabaseOutlined /></div>
          <div class="an-card-num num">{{ formatBytes(data.totalMemory) }}</div>
          <div class="an-card-label">{{ t('analysis.totalMemory') }}</div>
        </div>
        <div class="an-card">
          <div class="an-card-icon"><PieChartOutlined /></div>
          <div class="an-card-num num">{{ formatBytes(avgSize) }}</div>
          <div class="an-card-label">{{ t('analysis.avgSize') }}</div>
        </div>
        <div class="an-card">
          <div class="an-card-icon"><FieldTimeOutlined /></div>
          <div class="an-card-num num">{{ persistentRatio }}%</div>
          <div class="an-card-label">{{ t('analysis.persistentRatio') }}</div>
        </div>
      </div>

      <div class="an-grid">
        <!-- 大 Key 排行 -->
        <div class="an-panel">
          <div class="an-panel-title">{{ t('analysis.topKeysTitle') }} <span class="an-panel-sub">{{ t('analysis.topKeysSub', { n: data.topKeys.length }) }}</span></div>
          <div v-if="!data.topKeys.length" class="an-empty">{{ t('common.empty') }}</div>
          <div v-for="k in data.topKeys" :key="k.key" class="top-row">
            <span class="type-badge" :class="`type-${k.type}`">{{ k.type }}</span>
            <span class="top-key mono" :title="k.key">{{ k.key }}</span>
            <span class="top-ttl num text-3">{{ k.ttl >= 0 ? formatTTL(k.ttl) : t('analysis.persistent') }}</span>
            <div class="top-bar-wrap">
              <div class="top-bar" :style="{ width: `${Math.max(2, (k.size / maxTopSize) * 100)}%` }" />
            </div>
            <span class="top-size num">{{ formatBytes(k.size) }}</span>
          </div>
        </div>

        <div class="an-col">
          <!-- TTL 分布 -->
          <div class="an-panel">
            <div class="an-panel-title">{{ t('analysis.ttlDistTitle') }}</div>
            <div v-for="b in data.ttlBuckets" :key="b.label" class="dist-row">
              <span class="dist-label">{{ b.label }}</span>
              <div class="dist-bar-wrap">
                <div class="dist-bar ttl" :style="{ width: `${(b.count / maxBucketCount) * 100}%` }" />
              </div>
              <span class="dist-count num">{{ formatNumber(b.count) }}</span>
            </div>
          </div>

          <!-- 前缀内存 -->
          <div class="an-panel">
            <div class="an-panel-title">{{ t('analysis.prefixMemTitle') }} <span class="an-panel-sub">TOP 15</span></div>
            <div v-if="!data.prefixes.length" class="an-empty">{{ t('common.empty') }}</div>
            <div v-for="p in data.prefixes" :key="p.prefix" class="dist-row">
              <span class="dist-label mono prefix" :title="p.prefix">{{ p.prefix }}</span>
              <div class="dist-bar-wrap">
                <div class="dist-bar mem" :style="{ width: `${Math.max(1.5, (p.size / maxPrefixSize) * 100)}%` }" />
              </div>
              <span class="dist-count num">{{ formatBytes(p.size) }}</span>
              <span class="dist-sub num text-3">{{ t('analysis.keysCount', { n: formatNumber(p.count) }) }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
    </template>
  </div>
</template>

<style scoped>
.analysis { height: 100%; display: flex; flex-direction: column; overflow: hidden; }
.an-mode-bar {
  padding: 10px 16px 0;
  flex-shrink: 0;
}
.an-sub {
  flex: 1; overflow-y: auto;
  padding: 0 16px 20px;
  display: flex; flex-direction: column;
}
.an-toolbar {
  display: flex; align-items: center; gap: 8px;
  padding: 10px 16px;
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
}
.an-scan-info { font-size: 11px; margin-left: auto; }
.an-loading {
  flex: 1; display: flex; flex-direction: column;
  align-items: center; justify-content: center;
}
.an-body { flex: 1; overflow-y: auto; padding: 14px 16px 20px; }

.an-cards {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 10px;
  margin-bottom: 14px;
}
.an-card {
  background: var(--bg-panel);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  padding: 14px 16px;
  position: relative;
  overflow: hidden;
  transition: border-color 0.15s var(--ease);
}
.an-card:hover { border-color: var(--border-strong); }
.an-card-icon {
  position: absolute; right: 12px; top: 12px;
  color: var(--text-3); opacity: 0.5; font-size: 15px;
}
.an-card-num { font-size: 20px; font-weight: 700; letter-spacing: -0.3px; }
.an-card-label { font-size: 11px; color: var(--text-3); margin-top: 4px; }

.an-grid {
  display: grid;
  grid-template-columns: 1.4fr 1fr;
  gap: 10px;
  align-items: start;
}
.an-col { display: flex; flex-direction: column; gap: 10px; }
.an-panel {
  background: var(--bg-panel);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  padding: 14px 16px;
}
.an-panel-title {
  font-size: 12.5px; font-weight: 600;
  margin-bottom: 12px;
  display: flex; align-items: baseline; gap: 8px;
}
.an-panel-sub { font-size: 10px; color: var(--text-3); font-weight: 400; letter-spacing: 0.4px; }
.an-empty { color: var(--text-3); font-size: 12px; padding: 20px 0; text-align: center; }

.top-row {
  display: flex; align-items: center; gap: 8px;
  padding: 5px 0;
}
.top-key {
  flex: 0 1 auto; min-width: 0; max-width: 38%;
  font-size: 12px;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.top-ttl { font-size: 10px; flex-shrink: 0; width: 52px; }
.top-bar-wrap {
  flex: 1; height: 5px;
  background: var(--bg-inset);
  border-radius: 3px;
  overflow: hidden;
}
.top-bar {
  height: 100%;
  background: linear-gradient(90deg, var(--accent), var(--accent-hover));
  border-radius: 3px;
  transition: width 0.4s var(--ease);
}
.top-size { font-size: 11px; color: var(--text-2); width: 68px; text-align: right; flex-shrink: 0; }

.dist-row {
  display: flex; align-items: center; gap: 8px;
  padding: 4px 0;
}
.dist-label {
  width: 64px; flex-shrink: 0;
  font-size: 11px; color: var(--text-2);
}
.dist-label.prefix {
  width: 110px;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.dist-bar-wrap {
  flex: 1; height: 5px;
  background: var(--bg-inset);
  border-radius: 3px;
  overflow: hidden;
}
.dist-bar { height: 100%; border-radius: 3px; transition: width 0.4s var(--ease); }
.dist-bar.ttl { background: var(--info); }
.dist-bar.mem { background: var(--warning); }
.dist-count { font-size: 11px; color: var(--text-2); width: 64px; text-align: right; flex-shrink: 0; }
.dist-sub { font-size: 10px; width: 56px; text-align: right; flex-shrink: 0; }
</style>
