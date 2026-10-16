<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { message, Modal } from 'ant-design-vue'
import { PlusOutlined, ReloadOutlined, EditOutlined, DeleteOutlined } from '@ant-design/icons-vue'
import { ListGet, ListSet, ListPush, ListRemoveIndex } from '../../../wailsjs/go/service/ValueService'
import type { ListItem, ScanItems } from '../../types'

const props = defineProps<{ connId: string; db: number; keyName: string; total: number }>()

const { t } = useI18n()

const items = ref<ListItem[]>([])
const total = ref(props.total)
const page = ref(1)
const pageSize = ref(100)
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    const res = (await ListGet(props.connId, props.db, props.keyName, page.value, pageSize.value)) as unknown as ScanItems
    total.value = res.total
    items.value = (res.items as ListItem[]) || []
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    loading.value = false
  }
}

onMounted(load)

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)))

// 新增 / 编辑
const editVisible = ref(false)
const editMode = ref<'add' | 'edit'>('add')
const editValue = ref('')
const editIndex = ref(0)
const pushSide = ref<'left' | 'right'>('right')
const saving = ref(false)

function openAdd() {
  editMode.value = 'add'
  editValue.value = ''
  pushSide.value = 'right'
  editVisible.value = true
}

function openEdit(item: ListItem) {
  editMode.value = 'edit'
  editIndex.value = item.index
  editValue.value = item.value
  editVisible.value = true
}

async function save() {
  saving.value = true
  try {
    if (editMode.value === 'add') {
      await ListPush(props.connId, props.db, props.keyName, pushSide.value, editValue.value)
    } else {
      await ListSet(props.connId, props.db, props.keyName, editIndex.value, editValue.value)
    }
    message.success(t('editors.saved'))
    editVisible.value = false
    load()
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    saving.value = false
  }
}

function remove(item: ListItem) {
  Modal.confirm({
    title: t('editors.deleteElement'),
    content: t('editors.deleteElementContent', { index: item.index }),
    okText: t('common.delete'), okButtonProps: { danger: true }, cancelText: t('common.cancel'),
    async onOk() {
      try {
        await ListRemoveIndex(props.connId, props.db, props.keyName, item.index)
        message.success(t('editors.deleted'))
        load()
      } catch (e: any) {
        message.error(String(e?.message ?? e))
      }
    },
  })
}

const columns = computed(() => [
  { title: t('editors.index'), dataIndex: 'index', key: 'index', width: 90 },
  { title: t('common.value'), dataIndex: 'value', key: 'value' },
  { title: t('common.actions'), key: 'ops', width: 90 },
])
</script>

<template>
  <div class="coll-editor">
    <div class="ce-toolbar">
      <a-button size="small" :loading="loading" @click="load">
        <template #icon><ReloadOutlined /></template>
      </a-button>
      <a-button size="small" type="primary" @click="openAdd">
        <template #icon><PlusOutlined /></template>
        {{ t('editors.addElement') }}
      </a-button>
      <span class="text-3" style="font-size: 11px; margin-left: auto">{{ t('editors.totalItems', { n: total }) }}</span>
    </div>

    <div class="ce-table">
      <a-table
        :columns="columns"
        :data-source="items"
        :pagination="false"
        size="small"
        row-key="index"
        :scroll="{ y: '100%' }"
        :loading="loading"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'index'">
            <span class="mono text-3">{{ record.index }}</span>
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

    <div class="ce-footer">
      <a-pagination
        v-model:current="page"
        v-model:page-size="pageSize"
        :total="total"
        size="small"
        :show-size-changer="total > 100"
        :page-size-options="['50', '100', '200', '500']"
        show-less-items
        @change="load"
      />
    </div>

    <a-modal
      v-model:open="editVisible"
      :title="editMode === 'add' ? t('editors.addElement') : t('editors.editIndexTitle', { index: editIndex })"
      width="480px"
      :confirm-loading="saving"
      :ok-text="t('common.save')" :cancel-text="t('common.cancel')"
      @ok="save"
    >
      <a-form layout="vertical" size="small">
        <a-form-item v-if="editMode === 'add'" :label="t('editors.pushSide')">
          <a-radio-group v-model:value="pushSide">
            <a-radio-button value="left">{{ t('editors.pushLeft') }}</a-radio-button>
            <a-radio-button value="right">{{ t('editors.pushRight') }}</a-radio-button>
          </a-radio-group>
        </a-form-item>
        <a-form-item :label="t('common.value')">
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
.ce-footer { flex-shrink: 0; display: flex; justify-content: flex-end; }
.cell-value { color: var(--text-2); user-select: text; word-break: break-all; white-space: pre-wrap; }
</style>
