<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useConnectionStore } from '../stores/connection'
import { useAppStore } from '../stores/app'
import type { ConnState } from '../types'
import KeyBrowser from './KeyBrowser.vue'
import KeyAnalysis from './KeyAnalysis.vue'
import ServerInfo from './ServerInfo.vue'
import CliTerminal from './CliTerminal.vue'
import SlowLog from './SlowLog.vue'
import PubSub from './PubSub.vue'
import {
  DatabaseOutlined, DashboardOutlined, CodeOutlined,
  HistoryOutlined, NotificationOutlined, PieChartOutlined,
} from '@ant-design/icons-vue'

const props = defineProps<{ connId: string; state: ConnState }>()

const conn = useConnectionStore()
const app = useAppStore()

const views = ['data', 'analysis', 'info', 'cli', 'slowlog', 'pubsub']
const savedView = app.workspace.conns[props.connId]?.view ?? 'data'
const activeView = ref(views.includes(savedView) ? savedView : 'data')
watch(activeView, v => app.patchConnWorkspace(props.connId, { view: v }))

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
            <span class="seg-item"><DatabaseOutlined /> 数据</span>
          </a-segmented-option>
          <a-segmented-option value="analysis">
            <span class="seg-item"><PieChartOutlined /> 分析</span>
          </a-segmented-option>
          <a-segmented-option value="info">
            <span class="seg-item"><DashboardOutlined /> 监控</span>
          </a-segmented-option>
          <a-segmented-option value="cli">
            <span class="seg-item"><CodeOutlined /> CLI</span>
          </a-segmented-option>
          <a-segmented-option value="slowlog">
            <span class="seg-item"><HistoryOutlined /> 慢日志</span>
          </a-segmented-option>
          <a-segmented-option value="pubsub">
            <span class="seg-item"><NotificationOutlined /> 发布订阅</span>
          </a-segmented-option>
        </a-segmented>
      </div>
      <div class="vh-right">
        <span class="meta-pill num">{{ state.version || 'unknown' }}</span>
        <span class="meta-pill">{{ state.mode }}</span>
        <span class="vh-divider" />
        <span class="text-3" style="font-size: 11.5px">数据库</span>
        <a-select
          v-model:value="db"
          :options="dbOptions"
          size="small"
          style="width: 84px"
        />
      </div>
    </div>

    <div class="view-body">
      <KeyBrowser v-show="activeView === 'data'" :conn-id="connId" :db="db" />
      <KeyAnalysis v-if="activeView === 'analysis'" :conn-id="connId" :db="db" />
      <ServerInfo v-if="activeView === 'info'" :conn-id="connId" :db="db" />
      <CliTerminal v-show="activeView === 'cli'" :conn-id="connId" :db="db" />
      <SlowLog v-if="activeView === 'slowlog'" :conn-id="connId" :db="db" />
      <PubSub v-if="activeView === 'pubsub'" :conn-id="connId" :db="db" />
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
