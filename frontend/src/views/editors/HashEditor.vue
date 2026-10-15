<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { message, Modal } from 'ant-design-vue'
import { PlusOutlined, ReloadOutlined, EditOutlined, DeleteOutlined, SearchOutlined } from '@ant-design/icons-vue'
import { HashGet, HashSet, HashDel, HashRenameField } from '../../../wailsjs/go/service/ValueService'
import type { HashItem, ScanItems } from '../../types'

const props = defineProps<{ connId: string; db: number; keyName: string; total: number }>()

const items = ref<HashItem[]>([])
const cursor = ref(0)
const total = ref(props.total)
const loading = ref(false)
const pattern = ref('')

async function load(reset = false) {
  if (loading.value) return
  loading.value = true
  try {
    if (reset) { items.value = []; cursor.value = 0 }
    const res = (await HashGet(props.connId, props.db, props.keyName, cursor.value, pattern.value.trim(), 200)) as unknown as ScanItems
    cursor.value = Number(res.cursor)
    total.value = res.total
    const exist = new Set(items.value.map(i => i.field))
    for (const it of (res.items as HashItem[]) || []) {
      if (!exist.has(it.field)) items.value.push(it)
    }
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    loading.value = false
  }
}

onMounted(() => load(true))

// 新增 / 编辑
const editVisible = ref(false)
const editMode = ref<'add' | 'edit'>('add')
const editField = ref('')
const editValue = ref('')
const originalField = ref('')
const saving = ref(false)

function openAdd() {
  editMode.value = 'add'
  editField.value = ''
  editValue.value = ''
  editVisible.value = true
}

function openEdit(item: HashItem) {
  editMode.value = 'edit'
  originalField.value = item.field
  editField.value = item.field
  editValue.value = item.value
  editVisible.value = true
}

async function save() {
  if (!editField.value) { message.warning('字段名不能为空'); return }
  saving.value = true
  try {
    if (editMode.value === 'edit' && originalField.value !== editField.value) {
      await HashRenameField(props.connId, props.db, props.keyName, originalField.value, editField.value)
      await HashSet(props.connId, props.db, props.keyName, editField.value, editValue.value)
    } else {
      await HashSet(props.connId, props.db, props.keyName, editField.value, editValue.value)
    }
    message.success('已保存')
    editVisible.value = false
    load(true)
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    saving.value = false
  }
}

function remove(item: HashItem) {
  Modal.confirm({
    title: '删除字段',
    content: `确定删除字段「${item.field}」吗？`,
    okText: '删除', okButtonProps: { danger: true }, cancelText: '取消',
    async onOk() {
      try {
        await HashDel(props.connId, props.db, props.keyName, [item.field])
        items.value = items.value.filter(i => i.field !== item.field)
        total.value--
        message.success('已删除')
      } catch (e: any) {
        message.error(String(e?.message ?? e))
      }
    },
  })
}

const columns = [
  { title: '字段', dataIndex: 'field', key: 'field', width: '38%' },
  { title: '值', dataIndex: 'value', key: 'value' },
  { title: '操作', key: 'ops', width: 90 },
]
</script>

<template>
  <div class="coll-editor">
    <div class="ce-toolbar">
      <a-input
        v-model:value="pattern"
        size="small"
        placeholder="字段匹配，如 name*"
        allow-clear
        style="width: 220px"
        @press-enter="load(true)"
      >
        <template #prefix><SearchOutlined class="text-3" /></template>
      </a-input>
      <a-button size="small" :loading="loading" @click="load(true)">
        <template #icon><ReloadOutlined /></template>
      </a-button>
      <a-button size="small" type="primary" @click="openAdd">
        <template #icon><PlusOutlined /></template>
        添加字段
      </a-button>
      <span class="text-3" style="font-size: 11px; margin-left: auto">共 {{ total }} 字段</span>
    </div>

    <div class="ce-table">
      <a-table
        :columns="columns"
        :data-source="items"
        :pagination="false"
        size="small"
        row-key="field"
        :scroll="{ y: '100%' }"
        :loading="loading && !items.length"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'field'">
            <span class="mono cell-strong" :title="record.field">{{ record.field }}</span>
          </template>
          <template v-else-if="column.key === 'value'">
            <span class="mono cell-value" :title="record.value">{{ record.value }}</span>
          </template>
          <template v-else-if="column.key === 'ops'">
            <a-button size="small" type="text" @click="openEdit(record)"><template #icon><EditOutlined /></template></a-button>
            <a-button size="small" type="text" danger @click="remove(record)"><template #icon><DeleteOutlined /></template></a-button>
          </template>
        </template>
      </a-table>
    </div>

    <div class="ce-footer" v-if="cursor !== 0">
      <a-button size="small" type="link" :loading="loading" @click="load(false)">加载更多（已加载 {{ items.length }}）</a-button>
    </div>

    <a-modal
      v-model:open="editVisible"
      :title="editMode === 'add' ? '添加字段' : '编辑字段'"
      width="480px"
      :confirm-loading="saving"
      ok-text="保存" cancel-text="取消"
      @ok="save"
    >
      <a-form layout="vertical" size="small">
        <a-form-item label="字段名" required>
          <a-input v-model:value="editField" class="mono" />
        </a-form-item>
        <a-form-item label="值">
          <a-textarea v-model:value="editValue" :rows="5" class="mono" />
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
.ce-footer { flex-shrink: 0; text-align: center; }
.cell-strong { font-weight: 600; user-select: text; }
.cell-value { color: var(--text-2); user-select: text; word-break: break-all; white-space: pre-wrap; }
</style>
