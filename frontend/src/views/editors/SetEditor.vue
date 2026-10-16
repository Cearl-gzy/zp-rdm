<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { message, Modal } from 'ant-design-vue'
import { PlusOutlined, ReloadOutlined, EditOutlined, DeleteOutlined, SearchOutlined } from '@ant-design/icons-vue'
import { SetGet, SetAdd, SetRemove, SetUpdate } from '../../../wailsjs/go/service/ValueService'
import type { ScanItems } from '../../types'

const props = defineProps<{ connId: string; db: number; keyName: string; total: number }>()

const { t } = useI18n()

const items = ref<string[]>([])
const cursor = ref(0)
const total = ref(props.total)
const loading = ref(false)
const pattern = ref('')

async function load(reset = false) {
  if (loading.value) return
  loading.value = true
  try {
    if (reset) { items.value = []; cursor.value = 0 }
    const res = (await SetGet(props.connId, props.db, props.keyName, cursor.value, pattern.value.trim(), 200)) as unknown as ScanItems
    cursor.value = Number(res.cursor)
    total.value = res.total
    const exist = new Set(items.value)
    for (const m of (res.items as string[]) || []) {
      if (!exist.has(m)) items.value.push(m)
    }
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    loading.value = false
  }
}

onMounted(() => load(true))

const editVisible = ref(false)
const editMode = ref<'add' | 'edit'>('add')
const editValue = ref('')
const original = ref('')
const saving = ref(false)

function openAdd() {
  editMode.value = 'add'
  editValue.value = ''
  editVisible.value = true
}

function openEdit(member: string) {
  editMode.value = 'edit'
  original.value = member
  editValue.value = member
  editVisible.value = true
}

async function save() {
  if (!editValue.value) { message.warning(t('editors.memberRequired')); return }
  saving.value = true
  try {
    if (editMode.value === 'add') {
      await SetAdd(props.connId, props.db, props.keyName, editValue.value)
    } else if (editValue.value !== original.value) {
      await SetUpdate(props.connId, props.db, props.keyName, original.value, editValue.value)
    }
    message.success(t('editors.saved'))
    editVisible.value = false
    load(true)
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    saving.value = false
  }
}

function remove(member: string) {
  Modal.confirm({
    title: t('editors.deleteMember'),
    content: t('editors.deleteMemberContent', { member }),
    okText: t('common.delete'), okButtonProps: { danger: true }, cancelText: t('common.cancel'),
    async onOk() {
      try {
        await SetRemove(props.connId, props.db, props.keyName, [member])
        items.value = items.value.filter(m => m !== member)
        total.value--
        message.success(t('editors.deleted'))
      } catch (e: any) {
        message.error(String(e?.message ?? e))
      }
    },
  })
}

const tableData = computed(() => items.value.map(m => ({ member: m })))

const columns = computed(() => [
  { title: t('editors.member'), dataIndex: 'member', key: 'member' },
  { title: t('common.actions'), key: 'ops', width: 90 },
])
</script>

<template>
  <div class="coll-editor">
    <div class="ce-toolbar">
      <a-input
        v-model:value="pattern"
        size="small"
        :placeholder="t('editors.patternMember')"
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
        {{ t('editors.addMember') }}
      </a-button>
      <span class="text-3" style="font-size: 11px; margin-left: auto">{{ t('editors.totalMembers', { n: total }) }}</span>
    </div>

    <div class="ce-table">
      <a-table
        :columns="columns"
        :data-source="tableData"
        :pagination="false"
        size="small"
        row-key="member"
        :scroll="{ y: '100%' }"
        :loading="loading && !items.length"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'member'">
            <span class="mono cell-value" :title="record.member">{{ record.member }}</span>
          </template>
          <template v-else-if="column.key === 'ops'">
            <a-button size="small" type="text" @click="openEdit(record.member)"><template #icon><EditOutlined /></template></a-button>
            <a-button size="small" type="text" danger @click="remove(record.member)"><template #icon><DeleteOutlined /></template></a-button>
          </template>
        </template>
      </a-table>
    </div>

    <div class="ce-footer" v-if="cursor !== 0">
      <a-button size="small" type="link" :loading="loading" @click="load(false)">{{ t('editors.loadMoreLoaded', { n: items.length }) }}</a-button>
    </div>

    <a-modal
      v-model:open="editVisible"
      :title="editMode === 'add' ? t('editors.addMember') : t('editors.editMember')"
      width="480px"
      :confirm-loading="saving"
      :ok-text="t('common.save')" :cancel-text="t('common.cancel')"
      @ok="save"
    >
      <a-textarea v-model:value="editValue" :rows="5" class="mono" />
    </a-modal>
  </div>
</template>

<style scoped>
.coll-editor { height: 100%; display: flex; flex-direction: column; gap: 8px; }
.ce-toolbar { display: flex; gap: 6px; align-items: center; flex-shrink: 0; }
.ce-table { flex: 1; min-height: 0; overflow: hidden; }
.ce-table :deep(.ant-table), .ce-table :deep(.ant-table-container), .ce-table :deep(.ant-table-body) { height: 100%; }
.ce-footer { flex-shrink: 0; text-align: center; }
.cell-value { color: var(--text-2); user-select: text; word-break: break-all; white-space: pre-wrap; }
</style>
