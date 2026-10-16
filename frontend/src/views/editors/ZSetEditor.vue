<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { message, Modal } from 'ant-design-vue'
import { PlusOutlined, ReloadOutlined, EditOutlined, DeleteOutlined, ArrowUpOutlined, ArrowDownOutlined } from '@ant-design/icons-vue'
import { ZSetGet, ZSetAdd, ZSetRemove, ZSetUpdate } from '../../../wailsjs/go/service/ValueService'
import type { ZSetItem, ScanItems } from '../../types'

const props = defineProps<{ connId: string; db: number; keyName: string; total: number }>()

const { t } = useI18n()

const items = ref<ZSetItem[]>([])
const total = ref(props.total)
const page = ref(1)
const pageSize = ref(100)
const rev = ref(false)
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    const res = (await ZSetGet(props.connId, props.db, props.keyName, page.value, pageSize.value, rev.value)) as unknown as ScanItems
    total.value = res.total
    items.value = (res.items as ZSetItem[]) || []
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    loading.value = false
  }
}

onMounted(load)

function toggleOrder() {
  rev.value = !rev.value
  page.value = 1
  load()
}

const editVisible = ref(false)
const editMode = ref<'add' | 'edit'>('add')
const editMember = ref('')
const editScore = ref(0)
const original = ref('')
const saving = ref(false)

function openAdd() {
  editMode.value = 'add'
  editMember.value = ''
  editScore.value = 0
  editVisible.value = true
}

function openEdit(item: ZSetItem) {
  editMode.value = 'edit'
  original.value = item.member
  editMember.value = item.member
  editScore.value = item.score
  editVisible.value = true
}

async function save() {
  if (!editMember.value) { message.warning(t('editors.memberRequired')); return }
  saving.value = true
  try {
    if (editMode.value === 'add') {
      await ZSetAdd(props.connId, props.db, props.keyName, editMember.value, editScore.value)
    } else {
      await ZSetUpdate(props.connId, props.db, props.keyName, original.value, editMember.value, editScore.value)
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

function remove(item: ZSetItem) {
  Modal.confirm({
    title: t('editors.deleteMember'),
    content: t('editors.deleteMemberContent', { member: item.member }),
    okText: t('common.delete'), okButtonProps: { danger: true }, cancelText: t('common.cancel'),
    async onOk() {
      try {
        await ZSetRemove(props.connId, props.db, props.keyName, [item.member])
        message.success(t('editors.deleted'))
        load()
      } catch (e: any) {
        message.error(String(e?.message ?? e))
      }
    },
  })
}

const columns = computed(() => [
  { title: t('editors.member'), dataIndex: 'member', key: 'member' },
  { title: t('editors.score'), dataIndex: 'score', key: 'score', width: 140 },
  { title: t('common.actions'), key: 'ops', width: 90 },
])
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
        {{ rev ? t('editors.desc') : t('editors.asc') }}
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
        :data-source="items"
        :pagination="false"
        size="small"
        row-key="member"
        :scroll="{ y: '100%' }"
        :loading="loading"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'member'">
            <span class="mono cell-value" :title="record.member">{{ record.member }}</span>
          </template>
          <template v-else-if="column.key === 'score'">
            <span class="mono score">{{ record.score }}</span>
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
      :title="editMode === 'add' ? t('editors.addMember') : t('editors.editMember')"
      width="440px"
      :confirm-loading="saving"
      :ok-text="t('common.save')" :cancel-text="t('common.cancel')"
      @ok="save"
    >
      <a-form layout="vertical" size="small">
        <a-form-item :label="t('editors.member')" required>
          <a-input v-model:value="editMember" class="mono" />
        </a-form-item>
        <a-form-item :label="t('editors.score')">
          <a-input-number v-model:value="editScore" style="width: 100%" :step="1" />
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
.score { color: #2dd4bf; font-weight: 600; }
</style>
