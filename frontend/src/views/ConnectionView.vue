<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { useConnectionStore } from '../stores/connection'
import { useAppStore } from '../stores/app'
import { onAppEvent, AppEvents } from '../utils/events'
import type { ConnState } from '../types'
import KeyBrowser from './KeyBrowser.vue'
import KeyAnalysis from './KeyAnalysis.vue'
import ServerInfo from './ServerInfo.vue'
import CliTerminal from './CliTerminal.vue'
import SlowLog from './SlowLog.vue'
import PubSub from './PubSub.vue'
import SearchPanel from './SearchPanel.vue'
import ClusterPanel from './ClusterPanel.vue'
import ServerPanel from './ServerPanel.vue'
import {
  DatabaseOutlined, DashboardOutlined, CodeOutlined,
  HistoryOutlined, NotificationOutlined, PieChartOutlined, SearchOutlined, ClusterOutlined,
  SettingOutlined,
} from '@ant-design/icons-vue'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ connId: string; state: ConnState }>()

const conn = useConnectionStore()
const app = useAppStore()
const { t } = useI18n()

// RediSearch 模块加载时才提供搜索视图
const hasSearch = computed(() => (props.state.modules ?? []).some(m => /search/i.test(m)))
// 集群模式提供集群视图，且只有 db 0（隐藏数据库选择器）
const isCluster = computed(() => props.state.mode === 'cluster')
const views = computed(() => {
  const base = ['data', 'analysis']
  if (isCluster.value) base.push('cluster')
  base.push('info')
  if (!isCluster.value) base.push('server') // 配置/ACL 不支持集群（逐节点语义）
  base.push('cli', 'slowlog', 'pubsub')
  if (hasSearch.value) base.push('search')
  return base
})
const savedView = app.workspace.conns[props.connId]?.view ?? 'data'
const activeView = ref(views.value.includes(savedView) ? savedView : 'data')
watch(activeView, v => app.patchConnWorkspace(props.connId, { view: v }))

// 搜索面板"打开键"→ 切到数据视图（KeyBrowser 负责选中定位）
let offOpenKey: (() => void) | null = null
onMounted(() => {
  offOpenKey = onAppEvent<{ connId: string; key: string }>(AppEvents.OpenKey, d => {
    if (d?.connId === props.connId) activeView.value = 'data'
  })
})
onBeforeUnmount(() => offOpenKey?.())

const db = computed({
  get: () => conn.currentDb[props.connId] ?? 0,
  set: v => conn.setDb(props.connId, v),
})

const dbOptions = computed(() =>
  Array.from({ length: props.state.dbCount || 16 }, (_, i) => ({ value: i, label: `db${i}` }))
)
</script>

<template>
  <div class="conn-view">
    <div class="view-header">
      <div class="vh-left">
        <a-segmented v-model:value="activeView" size="small">
          <a-segmented-option value="data">
            <span class="seg-item"><DatabaseOutlined /> {{ t('connectionView.data') }}</span>
          </a-segmented-option>
          <a-segmented-option value="analysis">
            <span class="seg-item"><PieChartOutlined /> {{ t('connectionView.analysis') }}</span>
          </a-segmented-option>
          <a-segmented-option v-if="isCluster" value="cluster">
            <span class="seg-item"><ClusterOutlined /> {{ t('connectionView.cluster') }}</span>
          </a-segmented-option>
          <a-segmented-option value="info">
            <span class="seg-item"><DashboardOutlined /> {{ t('connectionView.monitor') }}</span>
          </a-segmented-option>
          <a-segmented-option v-if="!isCluster" value="server">
            <span class="seg-item"><SettingOutlined /> {{ t('connectionView.server') }}</span>
          </a-segmented-option>
          <a-segmented-option value="cli">
            <span class="seg-item"><CodeOutlined /> CLI</span>
          </a-segmented-option>
          <a-segmented-option value="slowlog">
            <span class="seg-item"><HistoryOutlined /> {{ t('connectionView.slowlog') }}</span>
          </a-segmented-option>
          <a-segmented-option value="pubsub">
            <span class="seg-item"><NotificationOutlined /> {{ t('connectionView.pubsub') }}</span>
          </a-segmented-option>
          <a-segmented-option v-if="hasSearch" value="search">
            <span class="seg-item"><SearchOutlined /> {{ t('connectionView.search') }}</span>
          </a-segmented-option>
        </a-segmented>
      </div>
      <div class="vh-right">
        <span class="meta-pill num">{{ state.version || 'unknown' }}</span>
        <span class="meta-pill">{{ state.mode }}</span>
        <template v-if="!isCluster">
          <span class="vh-divider" />
          <span class="text-3" style="font-size: 11.5px">{{ t('connectionView.database') }}</span>
          <a-select
            v-model:value="db"
            :options="dbOptions"
            size="small"
            style="width: 84px"
          />
        </template>
      </div>
    </div>

    <div class="view-body">
      <KeyBrowser v-show="activeView === 'data'" :conn-id="connId" :db="db" />
      <KeyAnalysis v-if="activeView === 'analysis'" :conn-id="connId" :db="db" />
      <ClusterPanel v-if="activeView === 'cluster'" :conn-id="connId" />
      <ServerInfo v-if="activeView === 'info'" :conn-id="connId" :db="db" />
      <ServerPanel v-if="activeView === 'server'" :conn-id="connId" :db="db" />
      <CliTerminal v-show="activeView === 'cli'" :conn-id="connId" :db="db" />
      <SlowLog v-if="activeView === 'slowlog'" :conn-id="connId" :db="db" />
      <PubSub v-if="activeView === 'pubsub'" :conn-id="connId" :db="db" />
      <SearchPanel v-if="activeView === 'search'" :conn-id="connId" :db="db" />
    </div>
  </div>
</template>

<style scoped>
.conn-view { height: 100%; display: flex; flex-direction: column; background: var(--bg-base); }
.view-header {
  display: flex; align-items: center; justify-content: space-between;
  padding: 9px 16px;
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
}
.vh-right { display: flex; align-items: center; gap: 8px; }
.meta-pill {
  font-size: 10.5px;
  color: var(--text-2);
  background: var(--bg-hover);
  border-radius: 4px;
  padding: 2px 7px;
  letter-spacing: 0.2px;
}
.vh-divider {
  width: 1px; height: 14px;
  background: var(--border-strong);
  margin: 0 2px;
}
.seg-item { display: inline-flex; align-items: center; gap: 5px; padding: 0 3px; font-size: 12.5px; }
.view-body { flex: 1; min-height: 0; }
</style>
