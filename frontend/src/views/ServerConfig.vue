<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { message, Modal } from 'ant-design-vue'
import { ReloadOutlined, SaveOutlined } from '@ant-design/icons-vue'
import { GetConfig, SetConfig, ConfigRewrite } from '../../wailsjs/go/service/ServerService'
import { useReadonly } from '../utils/readonly'

const props = defineProps<{ connId: string; db: number }>()
const { t, te } = useI18n()
const ro = useReadonly(() => props.connId)

interface CfgRow { param: string; value: string }

const rows = ref<CfgRow[]>([])
const loading = ref(false)
const filter = ref('')

async function load() {
  loading.value = true
  try {
    const m = (await GetConfig(props.connId, props.db, '*')) as unknown as Record<string, string>
    rows.value = Object.keys(m || {})
      .sort()
      .map(k => ({ param: k, value: m[k] }))
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    loading.value = false
  }
}

onMounted(load)

const filtered = computed(() => {
  const q = filter.value.trim().toLowerCase()
  if (!q) return rows.value
  return rows.value.filter(r =>
    r.param.toLowerCase().includes(q) || r.value.toLowerCase().includes(q)
  )
})

// i18n 参数说明键：maxmemory-policy → serverPanel.desc.maxmemory_policy
function descKey(param: string): string {
  return 'serverPanel.desc.' + param.replace(/[-.]/g, '_')
}
function desc(param: string): string {
  const k = descKey(param)
  return te(k) ? t(k) : ''
}

// 高风险参数：修改前二次确认
const DANGER_PARAMS = new Set([
  'requirepass', 'masterauth', 'maxmemory', 'appendonly', 'save',
  'dir', 'dbfilename', 'replicaof', 'slaveof', 'rename-command',
])

const editOpen = ref(false)
const editParam = ref('')
const editValue = ref('')
const saving = ref(false)

function openEdit(row: CfgRow) {
  editParam.value = row.param
  editValue.value = row.value
  editOpen.value = true
}

function doSave() {
  saving.value = true
  SetConfig(props.connId, props.db, editParam.value, editValue.value)
    .then(() => {
      message.success(t('serverPanel.cfgSaved', { param: editParam.value }))
      const row = rows.value.find(r => r.param === editParam.value)
      if (row) row.value = editValue.value
      editOpen.value = false
    })
    .catch((e: any) => message.error(String(e?.message ?? e)))
    .finally(() => { saving.value = false })
}

function onSave() {
  if (DANGER_PARAMS.has(editParam.value)) {
    Modal.confirm({
      title: t('serverPanel.cfgDangerTitle'),
      content: t('serverPanel.cfgDangerContent', { param: editParam.value }),
      okText: t('serverPanel.cfgDangerOk'),
      okButtonProps: { danger: true },
      cancelText: t('common.cancel'),
      onOk: () => { editOpen.value = false; doSave() },
    })
    return
  }
  editOpen.value = false
  doSave()
}

function onRewrite() {
  Modal.confirm({
    title: t('serverPanel.cfgRewriteTitle'),
    content: t('serverPanel.cfgRewriteContent'),
    okText: t('serverPanel.cfgRewriteOk'),
    cancelText: t('common.cancel'),
    async onOk() {
      try {
        await ConfigRewrite(props.connId, props.db)
        message.success(t('serverPanel.cfgRewriteDone'))
      } catch (e: any) {
        message.error(String(e?.message ?? e))
      }
    },
  })
}

const columns = computed(() => [
  { title: t('serverPanel.cfgColParam'), dataIndex: 'param', key: 'param', width: 230 },
  { title: t('serverPanel.cfgColValue'), dataIndex: 'value', key: 'value' },
  { title: t('serverPanel.cfgColDesc'), key: 'desc', width: 320 },
  { title: t('serverPanel.cfgColActions'), key: 'actions', width: 80 },
])
</script>

<template>
  <div class="svr-cfg">
    <div class="cfg-toolbar">
      <a-input
        v-model:value="filter"
        size="small"
        :placeholder="t('serverPanel.cfgSearch')"
        allow-clear
        style="width: 220px"
      />
      <a-button size="small" :loading="loading" @click="load">
        <template #icon><ReloadOutlined /></template>
        {{ t('common.refresh') }}
      </a-button>
      <a-button size="small" :disabled="ro" :title="ro ? t('common.readonlyTip') : ''" @click="onRewrite">
        <template #icon><SaveOutlined /></template>
        {{ t('serverPanel.cfgRewrite') }}
      </a-button>
      <span class="text-3" style="font-size: 11px; margin-left: auto">{{ t('serverPanel.cfgRuntimeTip') }}</span>
    </div>

    <div class="cfg-table">
      <a-table
        :columns="columns"
        :data-source="filtered"
        :pagination="{ pageSize: 30, size: 'small', showLessItems: true }"
        size="small"
        row-key="param"
        :loading="loading"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'param'">
            <span class="mono">{{ record.param }}</span>
          </template>
          <template v-else-if="column.key === 'value'">
            <span class="mono cfg-val" :title="record.value">{{ record.value || '-' }}</span>
          </template>
          <template v-else-if="column.key === 'desc'">
            <span class="text-3" style="font-size: 11.5px">{{ desc(record.param) }}</span>
          </template>
          <template v-else-if="column.key === 'actions'">
            <a-button size="small" type="link" style="padding: 0" :disabled="ro" @click="openEdit(record)">
              {{ t('serverPanel.cfgEdit') }}
            </a-button>
          </template>
          <template v-else-if="column.key === 'empty'">
            {{ t('serverPanel.cfgEmpty') }}
          </template>
        </template>
      </a-table>
    </div>

    <a-modal
      :open="editOpen"
      :title="t('serverPanel.cfgEditTitle')"
      :ok-text="t('common.save')"
      :cancel-text="t('common.cancel')"
      :confirm-loading="saving"
      @ok="onSave"
      @cancel="editOpen = false"
    >
      <a-form layout="vertical" size="small">
        <a-form-item :label="editParam">
          <div v-if="desc(editParam)" class="text-3" style="font-size: 12px; margin-bottom: 6px">
            {{ desc(editParam) }}
          </div>
          <a-textarea v-model:value="editValue" :rows="3" class="mono-input" />
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<style scoped>
.svr-cfg { height: 100%; display: flex; flex-direction: column; }
.cfg-toolbar {
  display: flex; align-items: center; gap: 8px;
  padding: 8px 16px;
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
}
.cfg-table { flex: 1; overflow: auto; padding: 10px 16px; }
.cfg-val { user-select: text; word-break: break-all; white-space: pre-wrap; color: var(--text-2); }
</style>
