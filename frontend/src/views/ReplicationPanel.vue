<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { message } from 'ant-design-vue'
import { ReloadOutlined, CrownOutlined, NodeIndexOutlined, LinkOutlined } from '@ant-design/icons-vue'
import { ReplicationInfo as fetchReplicationInfo } from '../../wailsjs/go/service/ServerService'
import type { ReplicationInfo } from '../types'
import { formatBytes, formatNumber } from '../utils/format'

const props = defineProps<{ connId: string; db: number }>()

const { t } = useI18n()

const data = ref<ReplicationInfo | null>(null)
const loading = ref(false)

async function load() {
  if (loading.value) return
  loading.value = true
  try {
    data.value = (await fetchReplicationInfo(props.connId, props.db)) as unknown as ReplicationInfo
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    loading.value = false
  }
}
watch(() => [props.connId, props.db], () => { data.value = null; load() })
onMounted(load)

const isMaster = computed(() => data.value?.role === 'master')

// 副本滞后字节 = master 偏移 - 副本偏移（可能为负：副本刚追上，按 0 显示）
function behindBytes(replicaOffset: number): number {
  return Math.max(0, (data.value?.offset ?? 0) - replicaOffset)
}
function lagClass(lag: number): string {
  if (lag <= 1) return 'ok'
  if (lag < 10) return 'warn'
  return 'bad'
}
</script>

<template>
  <div class="repl">
    <div class="repl-toolbar">
      <a-button size="small" :loading="loading" @click="load">
        <template #icon><ReloadOutlined /></template>
        {{ t('serverPanel.repRefresh') }}
      </a-button>
      <span v-if="data" class="role-badge" :class="isMaster ? 'master' : 'replica'">
        <CrownOutlined v-if="isMaster" /><NodeIndexOutlined v-else />
        {{ isMaster ? 'MASTER' : 'REPLICA' }}
      </span>
    </div>

    <div v-if="!data && loading" class="repl-loading">
      <a-spin size="large" />
    </div>

    <div v-else-if="data" class="repl-body">
      <!-- 本节点卡片 -->
      <div class="repl-card self">
        <div class="repl-card-title">
          {{ t('serverPanel.repSelf') }}
          <span class="role-badge sm" :class="isMaster ? 'master' : 'replica'">{{ data.role.toUpperCase() }}</span>
        </div>
        <div class="repl-metrics">
          <div class="repl-metric">
            <div class="repl-metric-num num">{{ formatNumber(data.offset) }}</div>
            <div class="repl-metric-label">{{ t('serverPanel.repOffset') }}</div>
          </div>
          <template v-if="isMaster">
            <div class="repl-metric">
              <div class="repl-metric-num num">{{ formatNumber(data.replicas.length) }}</div>
              <div class="repl-metric-label">{{ t('serverPanel.repReplicaCount') }}</div>
            </div>
            <div class="repl-metric">
              <div class="repl-metric-num num">{{ data.backlogActive ? formatBytes(data.backlogHistlen) : '—' }}</div>
              <div class="repl-metric-label">
                {{ t('serverPanel.repBacklog') }}
                <span v-if="data.backlogActive" class="text-3" style="font-size: 10px">/ {{ formatBytes(data.backlogSize) }}</span>
              </div>
            </div>
          </template>
        </div>
      </div>

      <!-- master 视角：下游副本列表 -->
      <template v-if="isMaster">
        <div v-if="!data.replicas.length" class="repl-empty">{{ t('serverPanel.repNoReplicas') }}</div>
        <div v-for="r in data.replicas" :key="r.addr" class="repl-card replica-card">
          <div class="repl-link-line"><LinkOutlined /></div>
          <div class="repl-row">
            <span class="repl-addr mono">{{ r.addr }}</span>
            <a-tag :color="r.state === 'online' ? 'success' : 'warning'" class="repl-tag">{{ r.state }}</a-tag>
            <span class="repl-lag" :class="lagClass(r.lag)">{{ t('serverPanel.repLag', { s: r.lag }) }}</span>
            <span class="repl-behind num text-3">{{ t('serverPanel.repBehind', { n: formatBytes(behindBytes(r.offset)) }) }}</span>
            <span class="repl-off num text-3">offset {{ formatNumber(r.offset) }}</span>
          </div>
        </div>
      </template>

      <!-- 副本视角：上游 master 链路 -->
      <template v-else>
        <div class="repl-card upstream">
          <div class="repl-card-title">{{ t('serverPanel.repUpstream') }}</div>
          <div class="repl-row">
            <span class="repl-addr mono">{{ data.masterAddr || '—' }}</span>
            <a-tag :color="data.linkStatus === 'up' ? 'success' : 'error'" class="repl-tag">
              {{ data.linkStatus === 'up' ? t('serverPanel.repLinkUp') : t('serverPanel.repLinkDown') }}
            </a-tag>
            <a-tag v-if="data.syncInProgress" color="processing" class="repl-tag">{{ t('serverPanel.repSyncing') }}</a-tag>
            <span class="repl-off num text-3">{{ t('serverPanel.repLastIO', { s: data.lastIOSeconds }) }}</span>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.repl { height: 100%; display: flex; flex-direction: column; }
.repl-toolbar {
  display: flex; align-items: center; gap: 10px;
  padding: 10px 0;
  flex-shrink: 0;
}
.repl-loading {
  flex: 1; display: flex; align-items: center; justify-content: center;
}
.repl-body { flex: 1; overflow-y: auto; padding-bottom: 16px; display: flex; flex-direction: column; gap: 10px; }

.role-badge {
  display: inline-flex; align-items: center; gap: 5px;
  font-size: 11px; font-weight: 700; letter-spacing: 0.6px;
  padding: 2px 9px; border-radius: 4px;
}
.role-badge.master { color: var(--accent); background: var(--bg-hover); }
.role-badge.replica { color: var(--info); background: var(--bg-hover); }
.role-badge.sm { font-size: 10px; padding: 1px 7px; }

.repl-card {
  background: var(--bg-panel);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  padding: 14px 16px;
}
.repl-card-title {
  font-size: 12.5px; font-weight: 600;
  margin-bottom: 12px;
  display: flex; align-items: center; gap: 8px;
}
.repl-metrics { display: flex; gap: 36px; }
.repl-metric-num { font-size: 18px; font-weight: 700; letter-spacing: -0.3px; }
.repl-metric-label { font-size: 11px; color: var(--text-3); margin-top: 3px; }

.replica-card { position: relative; padding: 12px 16px; }
.repl-link-line {
  position: absolute; left: 26px; top: -11px;
  color: var(--text-3); font-size: 10px;
  background: var(--bg-base); padding: 0 4px;
}
.repl-row { display: flex; align-items: center; gap: 10px; }
.repl-addr { font-size: 12.5px; font-weight: 600; }
.repl-tag { margin-inline-end: 0; font-size: 10.5px; }
.repl-lag { font-size: 11px; font-weight: 600; }
.repl-lag.ok { color: var(--success, #52c41a); }
.repl-lag.warn { color: var(--warning); }
.repl-lag.bad { color: var(--error, #ff4d4f); }
.repl-behind { font-size: 11px; }
.repl-off { font-size: 10.5px; margin-left: auto; }

.repl-empty {
  color: var(--text-3); font-size: 12px;
  text-align: center; padding: 24px 0;
  border: 1px dashed var(--border); border-radius: var(--radius-lg);
}
</style>
