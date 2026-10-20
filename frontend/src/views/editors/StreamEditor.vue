<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { message, Modal } from 'ant-design-vue'
import { PlusOutlined, ReloadOutlined, DeleteOutlined, ArrowUpOutlined, ArrowDownOutlined, MinusCircleOutlined, TeamOutlined, CheckOutlined } from '@ant-design/icons-vue'
import { StreamGet, StreamAdd, StreamDel, StreamGroups, StreamConsumers, StreamPending, StreamAck, StreamGroupCreate, StreamGroupDestroy, StreamConsumerDelete } from '../../../wailsjs/go/service/ValueService'
import { useReadonly } from '../../utils/readonly'
import type { StreamItem, ScanItems, StreamGroup, StreamConsumer, StreamPendingEntry } from '../../types'

const props = defineProps<{ connId: string; db: number; keyName: string; total: number }>()

const { t } = useI18n()
const ro = useReadonly(() => props.connId)

const items = ref<StreamItem[]>([])
const total = ref(props.total)
const count = ref(100)
const rev = ref(true)
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    const res = (await StreamGet(props.connId, props.db, props.keyName, count.value, rev.value)) as unknown as ScanItems
    total.value = res.total
    items.value = (res.items as StreamItem[]) || []
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    loading.value = false
  }
}

onMounted(load)

function toggleOrder() {
  rev.value = !rev.value
  load()
}

const addVisible = ref(false)
const fieldPairs = ref<{ k: string; v: string }[]>([{ k: '', v: '' }])
const saving = ref(false)

function openAdd() {
  fieldPairs.value = [{ k: '', v: '' }]
  addVisible.value = true
}

function addPair() { fieldPairs.value.push({ k: '', v: '' }) }
function removePair(i: number) { fieldPairs.value.splice(i, 1) }

async function save() {
  const fields: Record<string, string> = {}
  for (const p of fieldPairs.value) {
    if (p.k) fields[p.k] = p.v
  }
  if (!Object.keys(fields).length) { message.warning(t('stream.fieldRequired')); return }
  saving.value = true
  try {
    await StreamAdd(props.connId, props.db, props.keyName, fields)
    message.success(t('stream.added'))
    addVisible.value = false
    load()
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    saving.value = false
  }
}

function remove(item: StreamItem) {
  Modal.confirm({
    title: t('stream.deleteEntryTitle'),
    content: t('stream.deleteEntryConfirm', { id: item.id }),
    okText: t('common.delete'), okButtonProps: { danger: true }, cancelText: t('common.cancel'),
    async onOk() {
      try {
        await StreamDel(props.connId, props.db, props.keyName, [item.id])
        message.success(t('stream.deleted'))
        load()
      } catch (e: any) {
        message.error(String(e?.message ?? e))
      }
    },
  })
}

function fieldsText(item: StreamItem): string {
  return JSON.stringify(item.fields)
}

const columns = computed(() => [
  { title: 'ID', dataIndex: 'id', key: 'id', width: 190 },
  { title: t('common.field'), key: 'fields' },
  { title: t('common.actions'), key: 'ops', width: 70 },
])

// ---------- 消费者组 ----------

const groupsVisible = ref(false)
const groups = ref<StreamGroup[]>([])
const groupsLoading = ref(false)

function openGroups() {
  groupsVisible.value = true
  loadGroups()
}

async function loadGroups() {
  groupsLoading.value = true
  try {
    groups.value = (await StreamGroups(props.connId, props.db, props.keyName)) as unknown as StreamGroup[] || []
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    groupsLoading.value = false
  }
}

const groupColumns = computed(() => [
  { title: t('stream.groupName'), dataIndex: 'name', key: 'name' },
  { title: t('stream.consumers'), dataIndex: 'consumers', key: 'consumers', width: 70 },
  { title: t('stream.pending'), dataIndex: 'pending', key: 'pending', width: 70 },
  { title: t('stream.lag'), dataIndex: 'lag', key: 'lag', width: 70 },
  { title: t('stream.lastDeliveredId'), dataIndex: 'lastDeliveredId', key: 'lastDeliveredId', width: 150 },
  { title: t('common.actions'), key: 'ops', width: 150 },
])

// 展开组 → 加载消费者
const consumersByGroup = ref<Record<string, StreamConsumer[]>>({})
const consumersLoading = ref<Record<string, boolean>>({})

async function onGroupExpand(expanded: boolean, record: StreamGroup) {
  if (!expanded) return
  consumersLoading.value = { ...consumersLoading.value, [record.name]: true }
  try {
    const cs = (await StreamConsumers(props.connId, props.db, props.keyName, record.name)) as unknown as StreamConsumer[]
    consumersByGroup.value = { ...consumersByGroup.value, [record.name]: cs || [] }
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    consumersLoading.value = { ...consumersLoading.value, [record.name]: false }
  }
}

function removeConsumer(group: string, name: string) {
  Modal.confirm({
    title: t('stream.deleteConsumerTitle'),
    content: t('stream.deleteConsumerConfirm', { name }),
    okText: t('common.delete'), okButtonProps: { danger: true }, cancelText: t('common.cancel'),
    async onOk() {
      try {
        const n = await StreamConsumerDelete(props.connId, props.db, props.keyName, group, name)
        message.success(t('stream.consumerDeleted', { n }))
        onGroupExpand(true, { name: group } as StreamGroup)
        loadGroups()
      } catch (e: any) {
        message.error(String(e?.message ?? e))
      }
    },
  })
}

const consumerColumns = computed(() => [
  { title: t('stream.consumers'), dataIndex: 'name', key: 'name' },
  { title: t('stream.pending'), dataIndex: 'pending', key: 'pending', width: 80 },
  { title: t('stream.idle'), dataIndex: 'idleMs', key: 'idleMs', width: 100 },
  { title: t('common.actions'), key: 'ops', width: 60 },
])

// 新建组
const cgVisible = ref(false)
const cgName = ref('')
const cgStart = ref<'new' | 'all' | 'custom'>('new')
const cgCustomId = ref('')
const cgSaving = ref(false)

function openCreateGroup() {
  cgName.value = ''
  cgStart.value = 'new'
  cgCustomId.value = ''
  cgVisible.value = true
}

async function createGroup() {
  if (!cgName.value.trim()) { message.warning(t('stream.groupNameRequired')); return }
  let startID = '$'
  if (cgStart.value === 'all') startID = '0'
  else if (cgStart.value === 'custom') {
    startID = cgCustomId.value.trim()
    if (!startID) { message.warning(t('stream.startIdRequired')); return }
  }
  cgSaving.value = true
  try {
    await StreamGroupCreate(props.connId, props.db, props.keyName, cgName.value.trim(), startID, false)
    message.success(t('stream.created'))
    cgVisible.value = false
    loadGroups()
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    cgSaving.value = false
  }
}

function destroyGroup(g: StreamGroup) {
  Modal.confirm({
    title: t('stream.deleteGroupTitle'),
    content: t('stream.deleteGroupConfirm', { name: g.name }),
    okText: t('common.delete'), okButtonProps: { danger: true }, cancelText: t('common.cancel'),
    async onOk() {
      try {
        await StreamGroupDestroy(props.connId, props.db, props.keyName, g.name)
        message.success(t('stream.deleted'))
        loadGroups()
      } catch (e: any) {
        message.error(String(e?.message ?? e))
      }
    },
  })
}

// 待确认（pending）
const pdVisible = ref(false)
const pdGroup = ref('')
const pdItems = ref<StreamPendingEntry[]>([])
const pdLoading = ref(false)

async function openPending(g: StreamGroup) {
  pdGroup.value = g.name
  pdVisible.value = true
  loadPending()
}

async function loadPending() {
  pdLoading.value = true
  try {
    pdItems.value = (await StreamPending(props.connId, props.db, props.keyName, pdGroup.value, 200)) as unknown as StreamPendingEntry[] || []
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    pdLoading.value = false
  }
}

async function ack(ids: string[]) {
  try {
    const n = await StreamAck(props.connId, props.db, props.keyName, pdGroup.value, ids)
    message.success(t('stream.ackedN', { n }))
    loadPending()
    loadGroups()
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  }
}

const pendingColumns = computed(() => [
  { title: t('stream.messageId'), dataIndex: 'id', key: 'id', width: 170 },
  { title: t('stream.consumers'), dataIndex: 'consumer', key: 'consumer' },
  { title: t('stream.idle'), dataIndex: 'idleMs', key: 'idleMs', width: 90 },
  { title: t('stream.deliveryCount'), dataIndex: 'deliveryCount', key: 'deliveryCount', width: 80 },
  { title: t('common.actions'), key: 'ops', width: 70 },
])

function idleText(ms: number): string {
  if (ms < 1000) return `${ms}ms`
  const s = Math.floor(ms / 1000)
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}m${s % 60}s`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h${m % 60}m`
  return `${Math.floor(h / 24)}d${h % 24}h`
}
</script>

<template>
  <div class="coll-editor">
    <div class="ce-toolbar">
      <a-button size="small" :loading="loading" @click="load">
        <template #icon><ReloadOutlined /></template>
      </a-button>
      <a-button size="small" @click="toggleOrder">
        <template #icon>
          <component :is="rev ? ArrowDownOutlined : ArrowUpOutlined" />
        </template>
        {{ rev ? t('stream.newestFirst') : t('stream.oldestFirst') }}
      </a-button>
      <a-select v-model:value="count" size="small" style="width: 110px" @change="load">
        <a-select-option :value="50">{{ t('stream.lastN', { n: 50 }) }}</a-select-option>
        <a-select-option :value="100">{{ t('stream.lastN', { n: 100 }) }}</a-select-option>
        <a-select-option :value="500">{{ t('stream.lastN', { n: 500 }) }}</a-select-option>
        <a-select-option :value="1000">{{ t('stream.lastN', { n: 1000 }) }}</a-select-option>
      </a-select>
      <a-button size="small" type="primary" :disabled="ro" :title="ro ? t('common.readonlyTip') : ''" @click="openAdd">
        <template #icon><PlusOutlined /></template>
        {{ t('stream.addEntry') }}
      </a-button>
      <a-button size="small" @click="openGroups">
        <template #icon><TeamOutlined /></template>
        {{ t('stream.consumerGroups') }}
      </a-button>
      <span class="text-3" style="font-size: 11px; margin-left: auto">{{ t('stream.totalEntries', { n: total }) }}</span>
    </div>

    <div class="ce-table">
      <a-table
        :columns="columns"
        :data-source="items"
        :pagination="false"
        size="small"
        row-key="id"
        :scroll="{ y: '100%' }"
        :loading="loading"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'id'">
            <span class="mono stream-id">{{ record.id }}</span>
          </template>
          <template v-else-if="column.key === 'fields'">
            <span class="mono cell-value" :title="fieldsText(record)">{{ fieldsText(record) }}</span>
          </template>
          <template v-else-if="column.key === 'ops'">
            <a-button size="small" type="text" danger :disabled="ro" @click="remove(record)"><template #icon><DeleteOutlined /></template></a-button>
          </template>
        </template>
      </a-table>
    </div>

    <a-modal
      v-model:open="addVisible"
      :title="t('stream.addModalTitle')"
      width="520px"
      :confirm-loading="saving"
      :ok-text="t('common.add')" :cancel-text="t('common.cancel')"
      @ok="save"
    >
      <div v-for="(p, i) in fieldPairs" :key="i" class="pair-row">
        <a-input v-model:value="p.k" :placeholder="t('stream.fieldName')" class="mono pair-key" />
        <a-input v-model:value="p.v" :placeholder="t('common.value')" class="mono pair-val" />
        <a-button type="text" size="small" :disabled="fieldPairs.length <= 1" @click="removePair(i)">
          <template #icon><MinusCircleOutlined /></template>
        </a-button>
      </div>
      <a-button size="small" type="dashed" block style="margin-top: 8px" @click="addPair">
        <template #icon><PlusOutlined /></template>
        {{ t('stream.addField') }}
      </a-button>
    </a-modal>

    <!-- 消费者组管理抽屉 -->
    <a-drawer v-model:open="groupsVisible" :title="t('stream.consumerGroups')" width="760">
      <div class="cg-toolbar">
        <a-button size="small" :loading="groupsLoading" @click="loadGroups">
          <template #icon><ReloadOutlined /></template>
        </a-button>
        <a-button size="small" type="primary" :disabled="ro" :title="ro ? t('common.readonlyTip') : ''" @click="openCreateGroup">
          <template #icon><PlusOutlined /></template>
          {{ t('stream.createGroup') }}
        </a-button>
      </div>
      <a-table
        :columns="groupColumns"
        :data-source="groups"
        :pagination="false"
        size="small"
        row-key="name"
        :loading="groupsLoading"
        @expand="onGroupExpand"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'name'">
            <span class="mono" style="color: var(--accent)">{{ record.name }}</span>
          </template>
          <template v-else-if="column.key === 'pending'">
            <a-tag v-if="record.pending > 0" color="warning" class="cg-tag">{{ record.pending }}</a-tag>
            <span v-else class="text-3">0</span>
          </template>
          <template v-else-if="column.key === 'lastDeliveredId'">
            <span class="mono" style="font-size: 11px">{{ record.lastDeliveredId }}</span>
          </template>
          <template v-else-if="column.key === 'ops'">
            <a-button size="small" type="link" @click="openPending(record)">{{ t('stream.pending') }}</a-button>
            <a-button size="small" type="link" danger :disabled="ro" @click="destroyGroup(record)">{{ t('common.delete') }}</a-button>
          </template>
        </template>
        <template #expandedRowRender="{ record }">
          <a-table
            :columns="consumerColumns"
            :data-source="consumersByGroup[record.name] || []"
            :pagination="false"
            size="small"
            row-key="name"
            :loading="consumersLoading[record.name]"
            class="cg-sub-table"
          >
            <template #bodyCell="{ column, record: c }">
              <template v-if="column.key === 'idleMs'">{{ idleText(c.idleMs) }}</template>
              <template v-else-if="column.key === 'ops'">
                <a-button size="small" type="text" danger :disabled="ro" @click="removeConsumer(record.name, c.name)">
                  <template #icon><DeleteOutlined /></template>
                </a-button>
              </template>
            </template>
            <template #emptyText>{{ t('stream.noConsumers') }}</template>
          </a-table>
        </template>
      </a-table>

      <!-- 待确认二级抽屉 -->
      <a-drawer v-model:open="pdVisible" :title="t('stream.pendingTitle', { group: pdGroup })" width="640">
        <div class="cg-toolbar">
          <a-button size="small" :loading="pdLoading" @click="loadPending">
            <template #icon><ReloadOutlined /></template>
          </a-button>
          <span class="text-3" style="font-size: 11px">{{ t('stream.maxShown', { n: 200 }) }}</span>
        </div>
        <a-table
          :columns="pendingColumns"
          :data-source="pdItems"
          :pagination="false"
          size="small"
          row-key="id"
          :loading="pdLoading"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'id'">
              <span class="mono" style="font-size: 11px; color: #f472b6">{{ record.id }}</span>
            </template>
            <template v-else-if="column.key === 'idleMs'">{{ idleText(record.idleMs) }}</template>
            <template v-else-if="column.key === 'deliveryCount'">
              <a-tag v-if="record.deliveryCount > 1" color="error" class="cg-tag">{{ record.deliveryCount }}</a-tag>
              <span v-else>{{ record.deliveryCount }}</span>
            </template>
            <template v-else-if="column.key === 'ops'">
              <a-tooltip :title="t('stream.ackTooltip')">
                <a-button size="small" type="text" :disabled="ro" @click="ack([record.id])">
                  <template #icon><CheckOutlined /></template>
                </a-button>
              </a-tooltip>
            </template>
          </template>
        </a-table>
      </a-drawer>
    </a-drawer>

    <!-- 新建消费组 -->
    <a-modal
      v-model:open="cgVisible"
      :title="t('stream.createGroup')"
      width="440px"
      :confirm-loading="cgSaving"
      :ok-text="t('common.create')" :cancel-text="t('common.cancel')"
      @ok="createGroup"
    >
      <a-form layout="vertical" size="small">
        <a-form-item :label="t('stream.groupName')" required>
          <a-input v-model:value="cgName" :placeholder="t('stream.groupNamePlaceholder')" class="mono" />
        </a-form-item>
        <a-form-item :label="t('stream.startPosition')">
          <a-radio-group v-model:value="cgStart">
            <a-radio value="new">{{ t('stream.startNew') }}</a-radio>
            <a-radio value="all">{{ t('stream.startFromBeginning') }}</a-radio>
            <a-radio value="custom">{{ t('stream.startCustomId') }}</a-radio>
          </a-radio-group>
        </a-form-item>
        <a-form-item v-if="cgStart === 'custom'" :label="t('stream.startMessageId')">
          <a-input v-model:value="cgCustomId" :placeholder="t('stream.startIdPlaceholder')" class="mono" />
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<style scoped>
.coll-editor { height: 100%; display: flex; flex-direction: column; gap: 8px; }
.ce-toolbar { display: flex; gap: 6px; align-items: center; flex-shrink: 0; }
.ce-table { flex: 1; min-height: 0; overflow: hidden; }
.ce-table :deep(.ant-table), .ce-table :deep(.ant-table-container), .ce-table :deep(.ant-table-body) { height: 100%; }
.cell-value { color: var(--text-2); user-select: text; word-break: break-all; white-space: pre-wrap; }
.stream-id { color: #f472b6; font-size: 11.5px; user-select: text; }
.pair-row { display: flex; gap: 8px; margin-bottom: 8px; align-items: center; }
.pair-key { width: 36%; }
.pair-val { flex: 1; }
.cg-toolbar { display: flex; gap: 6px; align-items: center; margin-bottom: 8px; }
.cg-tag { margin-inline-end: 0 !important; }
.cg-sub-table { margin: 4px 0 4px 12px; }
</style>
