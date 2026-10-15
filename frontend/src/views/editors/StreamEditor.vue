<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { message, Modal } from 'ant-design-vue'
import { PlusOutlined, ReloadOutlined, DeleteOutlined, ArrowUpOutlined, ArrowDownOutlined, MinusCircleOutlined } from '@ant-design/icons-vue'
import { StreamGet, StreamAdd, StreamDel } from '../../../wailsjs/go/service/ValueService'
import type { StreamItem, ScanItems } from '../../types'

const props = defineProps<{ connId: string; db: number; keyName: string; total: number }>()

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
  if (!Object.keys(fields).length) { message.warning('至少填写一个字段'); return }
  saving.value = true
  try {
    await StreamAdd(props.connId, props.db, props.keyName, fields)
    message.success('已添加')
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
    title: '删除条目',
    content: `确定删除条目 ${item.id} 吗？`,
    okText: '删除', okButtonProps: { danger: true }, cancelText: '取消',
    async onOk() {
      try {
        await StreamDel(props.connId, props.db, props.keyName, [item.id])
        message.success('已删除')
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

const columns = [
  { title: 'ID', dataIndex: 'id', key: 'id', width: 190 },
  { title: '字段', key: 'fields' },
  { title: '操作', key: 'ops', width: 70 },
]
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
        {{ rev ? '最新优先' : '最早优先' }}
      </a-button>
      <a-select v-model:value="count" size="small" style="width: 110px" @change="load">
        <a-select-option :value="50">最近 50</a-select-option>
        <a-select-option :value="100">最近 100</a-select-option>
        <a-select-option :value="500">最近 500</a-select-option>
        <a-select-option :value="1000">最近 1000</a-select-option>
      </a-select>
      <a-button size="small" type="primary" @click="openAdd">
        <template #icon><PlusOutlined /></template>
        添加条目
      </a-button>
      <span class="text-3" style="font-size: 11px; margin-left: auto">共 {{ total }} 条目</span>
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
            <a-button size="small" type="text" danger @click="remove(record)"><template #icon><DeleteOutlined /></template></a-button>
          </template>
        </template>
      </a-table>
    </div>

    <a-modal
      v-model:open="addVisible"
      title="添加 Stream 条目"
      width="520px"
      :confirm-loading="saving"
      ok-text="添加" cancel-text="取消"
      @ok="save"
    >
      <div v-for="(p, i) in fieldPairs" :key="i" class="pair-row">
        <a-input v-model:value="p.k" placeholder="字段名" class="mono pair-key" />
        <a-input v-model:value="p.v" placeholder="值" class="mono pair-val" />
        <a-button type="text" size="small" :disabled="fieldPairs.length <= 1" @click="removePair(i)">
          <template #icon><MinusCircleOutlined /></template>
        </a-button>
      </div>
      <a-button size="small" type="dashed" block style="margin-top: 8px" @click="addPair">
        <template #icon><PlusOutlined /></template>
        添加字段
      </a-button>
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
</style>
