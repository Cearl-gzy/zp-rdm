<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { message } from 'ant-design-vue'
import { ReloadOutlined, SearchOutlined } from '@ant-design/icons-vue'
import { ClusterInfo, ClusterNodes, KeySlot } from '../../wailsjs/go/service/ServerService'
import type { ClusterNode } from '../types'

// Redis Cluster 集群视图：集群状态卡片 + 节点/槽位表 + 键槽查询
const props = defineProps<{ connId: string }>()
const { t } = useI18n()

const info = ref<Record<string, string>>({})
const nodes = ref<ClusterNode[]>([])
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    const [i, ns] = await Promise.all([
      ClusterInfo(props.connId) as unknown as Promise<Record<string, string>>,
      ClusterNodes(props.connId) as unknown as Promise<ClusterNode[]>,
    ])
    info.value = i || {}
    nodes.value = ns || []
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    loading.value = false
  }
}

onMounted(load)

const nodeColumns = computed(() => [
  { title: t('cluster.colAddr'), dataIndex: 'addr', key: 'addr', width: 200 },
  { title: t('cluster.colRole'), dataIndex: 'role', key: 'role', width: 90 },
  { title: t('cluster.colSlots'), key: 'slots' },
  { title: t('cluster.colSlotCount'), dataIndex: 'slotCount', key: 'slotCount', width: 80 },
  { title: t('cluster.colLink'), dataIndex: 'linkState', key: 'linkState', width: 90 },
])

// 键槽查询
const slotKey = ref('')
const slotResult = ref<number | null>(null)
const slotLoading = ref(false)

async function querySlot() {
  const k = slotKey.value.trim()
  if (!k) return
  slotLoading.value = true
  try {
    slotResult.value = await KeySlot(props.connId, k)
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    slotLoading.value = false
  }
}
</script>

<template>
  <div class="cluster-panel">
    <div class="cp-toolbar">
      <a-button size="small" :loading="loading" @click="load">
        <template #icon><ReloadOutlined /></template>
        {{ t('common.refresh') }}
      </a-button>
      <div class="cp-slot-query">
        <a-input
          v-model:value="slotKey"
          size="small"
          :placeholder="t('cluster.slotPlaceholder')"
          class="mono"
          style="width: 240px"
          @press-enter="querySlot"
        />
        <a-button size="small" :loading="slotLoading" @click="querySlot">
          <template #icon><SearchOutlined /></template>
        </a-button>
        <a-tag v-if="slotResult !== null" color="blue" class="cp-slot-tag">
          slot {{ slotResult }}
        </a-tag>
      </div>
    </div>

    <div class="cp-cards">
      <div class="cp-card">
        <div class="cp-card-label">{{ t('cluster.cardState') }}</div>
        <div class="cp-card-value" :class="info.cluster_state === 'ok' ? 'ok' : 'bad'">
          {{ info.cluster_state || '-' }}
        </div>
      </div>
      <div class="cp-card">
        <div class="cp-card-label">{{ t('cluster.cardSlotsAssigned') }}</div>
        <div class="cp-card-value num">{{ info.cluster_slots_assigned || '-' }}<span class="cp-dim"> / 16384</span></div>
      </div>
      <div class="cp-card">
        <div class="cp-card-label">{{ t('cluster.cardKnownNodes') }}</div>
        <div class="cp-card-value num">{{ info.cluster_known_nodes || '-' }}</div>
      </div>
      <div class="cp-card">
        <div class="cp-card-label">{{ t('cluster.cardSize') }}</div>
        <div class="cp-card-value num">{{ info.cluster_size || '-' }}</div>
      </div>
    </div>

    <div class="cp-table">
      <a-table
        :columns="nodeColumns"
        :data-source="nodes"
        :pagination="false"
        size="small"
        row-key="id"
        :loading="loading"
        :scroll="{ y: '100%' }"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'addr'">
            <span class="mono cp-addr">{{ record.addr }}</span>
            <a-tag v-if="record.myself" color="green" class="cp-tag">{{ t('cluster.tagCurrent') }}</a-tag>
            <div class="cp-node-id mono">{{ record.id.slice(0, 12) }}…</div>
          </template>
          <template v-else-if="column.key === 'role'">
            <a-tag :color="record.role === 'master' ? 'blue' : 'default'" class="cp-tag">
              {{ record.role === 'master' ? t('cluster.roleMaster') : t('cluster.roleSlave') }}
            </a-tag>
          </template>
          <template v-else-if="column.key === 'slots'">
            <span v-if="record.slots.length" class="mono cp-slots">{{ record.slots.join('  ') }}</span>
            <span v-else class="text-3">—</span>
          </template>
          <template v-else-if="column.key === 'linkState'">
            <span :class="record.linkState === 'connected' ? 'cp-ok' : 'cp-bad'">{{ record.linkState }}</span>
          </template>
        </template>
      </a-table>
    </div>
  </div>
</template>

<style scoped>
.cluster-panel { height: 100%; display: flex; flex-direction: column; gap: 10px; padding: 12px 16px; overflow: hidden; }
.cp-toolbar { display: flex; gap: 8px; align-items: center; flex-shrink: 0; }
.cp-slot-query { display: flex; gap: 6px; align-items: center; margin-left: auto; }
.cp-slot-tag { margin-inline-end: 0 !important; font-family: var(--mono, monospace); }
.cp-cards { display: flex; gap: 10px; flex-shrink: 0; }
.cp-card {
  flex: 1; padding: 10px 14px;
  border: 1px solid var(--border); border-radius: var(--radius-md);
  background: var(--bg-elevated);
}
.cp-card-label { font-size: 11px; color: var(--text-3); margin-bottom: 4px; }
.cp-card-value { font-size: 17px; font-weight: 600; color: var(--text-1); }
.cp-card-value.ok { color: #3fbf6b; }
.cp-card-value.bad { color: #e04536; }
.cp-card-value.num { font-family: var(--mono, monospace); }
.cp-dim { font-size: 12px; color: var(--text-3); font-weight: 400; }
.cp-table { flex: 1; min-height: 0; overflow: hidden; }
.cp-table :deep(.ant-table), .cp-table :deep(.ant-table-container), .cp-table :deep(.ant-table-body) { height: 100%; }
.cp-addr { font-size: 12px; color: var(--text-1); }
.cp-node-id { font-size: 10.5px; color: var(--text-3); }
.cp-tag { margin-inline-end: 0 !important; margin-left: 6px; }
.cp-slots { font-size: 11.5px; color: var(--text-2); word-break: break-all; }
.cp-ok { color: #3fbf6b; }
.cp-bad { color: #e04536; }
</style>
