<script setup lang="ts">
import { ref, computed, onMounted, reactive } from 'vue'
import { useI18n } from 'vue-i18n'
import { message, Modal } from 'ant-design-vue'
import { ReloadOutlined, PlusOutlined } from '@ant-design/icons-vue'
import { AclUsers, AclGetUser, AclSetUser, AclDelUser } from '../../wailsjs/go/service/ServerService'
import type { AclUserInfo } from '../types'

const props = defineProps<{ connId: string; db: number }>()
const { t } = useI18n()

const users = ref<AclUserInfo[]>([])
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    const names = ((await AclUsers(props.connId, props.db)) as unknown as string[]) || []
    const infos = await Promise.all(
      names.map(n => AclGetUser(props.connId, props.db, n) as unknown as Promise<AclUserInfo>)
    )
    users.value = infos
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    loading.value = false
  }
}

onMounted(load)

// ---------- 编辑抽屉 ----------
const drawerOpen = ref(false)
const isNew = ref(false)
const saving = ref(false)
const form = reactive({
  name: '',
  enabled: true,
  nopass: false,
  password: '',
  resetpass: false,
  keys: '',
  commands: '',
  channels: '',
  raw: '',
})

function openNew() {
  isNew.value = true
  Object.assign(form, {
    name: '', enabled: true, nopass: false, password: '',
    resetpass: false, keys: '~*', commands: '+@all', channels: '&*', raw: '',
  })
  drawerOpen.value = true
}

function openEdit(u: AclUserInfo) {
  isNew.value = false
  Object.assign(form, {
    name: u.name, enabled: u.enabled, nopass: u.noPass, password: '',
    resetpass: false, keys: u.keys, commands: u.commands, channels: u.channels, raw: '',
  })
  drawerOpen.value = true
}

function splitTokens(s: string): string[] {
  return s.split(/\s+/).map(x => x.trim()).filter(Boolean)
}

// 最终规则预览：键/命令/频道先 reset 再逐条授予，保证界面语义 = 最终状态
const rules = computed(() => {
  const out: string[] = []
  out.push(form.enabled ? 'on' : 'off')
  if (form.nopass) out.push('nopass')
  if (form.resetpass) out.push('resetpass')
  if (!form.nopass && form.password) out.push('>' + form.password)
  out.push('resetkeys')
  for (const k of splitTokens(form.keys)) out.push(k.startsWith('~') ? k : '~' + k)
  out.push('nocommands')
  for (const c of splitTokens(form.commands)) out.push(c)
  out.push('resetchannels')
  for (const ch of splitTokens(form.channels)) out.push(ch.startsWith('&') ? ch : '&' + ch)
  for (const r of splitTokens(form.raw)) out.push(r)
  return out
})

function doSave() {
  saving.value = true
  AclSetUser(props.connId, props.db, form.name.trim(), rules.value)
    .then(() => {
      message.success(t('serverPanel.aclSaved', { name: form.name.trim() }))
      drawerOpen.value = false
      load()
    })
    .catch((e: any) => message.error(String(e?.message ?? e)))
    .finally(() => { saving.value = false })
}

function onSave() {
  if (!form.name.trim()) {
    message.warning(t('serverPanel.aclNameRequired'))
    return
  }
  if (form.name.trim() === 'default') {
    Modal.confirm({
      title: 'default',
      content: t('serverPanel.aclDefaultTip'),
      okText: t('serverPanel.aclEditDefaultOk'),
      okButtonProps: { danger: true },
      cancelText: t('common.cancel'),
      onOk: doSave,
    })
    return
  }
  doSave()
}

function onDelete(u: AclUserInfo) {
  Modal.confirm({
    title: t('serverPanel.aclDeleteTitle'),
    content: t('serverPanel.aclDeleteContent', { name: u.name }),
    okText: t('serverPanel.aclDeleteOk'),
    okButtonProps: { danger: true },
    cancelText: t('common.cancel'),
    async onOk() {
      try {
        await AclDelUser(props.connId, props.db, u.name)
        message.success(t('serverPanel.aclDeleted', { name: u.name }))
        load()
      } catch (e: any) {
        message.error(String(e?.message ?? e))
      }
    },
  })
}

const columns = computed(() => [
  { title: t('serverPanel.aclColName'), dataIndex: 'name', key: 'name', width: 150 },
  { title: t('serverPanel.aclColStatus'), key: 'status', width: 120 },
  { title: t('serverPanel.aclColKeys'), dataIndex: 'keys', key: 'keys', width: 160 },
  { title: t('serverPanel.aclColCommands'), dataIndex: 'commands', key: 'commands' },
  { title: t('serverPanel.aclColChannels'), dataIndex: 'channels', key: 'channels', width: 110 },
  { title: t('serverPanel.aclColPasswords'), key: 'passwords', width: 80 },
  { title: t('serverPanel.aclColActions'), key: 'actions', width: 120 },
])
</script>

<template>
  <div class="acl-panel">
    <div class="acl-toolbar">
      <a-button size="small" :loading="loading" @click="load">
        <template #icon><ReloadOutlined /></template>
        {{ t('serverPanel.aclRefresh') }}
      </a-button>
      <a-button size="small" type="primary" @click="openNew">
        <template #icon><PlusOutlined /></template>
        {{ t('serverPanel.aclNew') }}
      </a-button>
      <span class="text-3" style="font-size: 11px; margin-left: auto">{{ t('serverPanel.aclNeedRedis6') }}</span>
    </div>

    <div class="acl-table">
      <a-table
        :columns="columns"
        :data-source="users"
        :pagination="{ pageSize: 20, size: 'small', showLessItems: true }"
        size="small"
        row-key="name"
        :loading="loading"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'name'">
            <span class="mono" style="font-weight: 600">{{ record.name }}</span>
          </template>
          <template v-else-if="column.key === 'status'">
            <a-tag :color="record.enabled ? 'green' : 'default'" style="margin-right: 4px">
              {{ record.enabled ? t('serverPanel.aclOn') : t('serverPanel.aclOff') }}
            </a-tag>
            <a-tag v-if="record.noPass" color="orange">{{ t('serverPanel.aclNopass') }}</a-tag>
          </template>
          <template v-else-if="column.key === 'keys'">
            <span class="mono text-2">{{ record.keys }}</span>
          </template>
          <template v-else-if="column.key === 'commands'">
            <span class="mono text-2" :title="record.commands">{{ record.commands }}</span>
          </template>
          <template v-else-if="column.key === 'channels'">
            <span class="mono text-2">{{ record.channels }}</span>
          </template>
          <template v-else-if="column.key === 'passwords'">
            <span class="text-3">{{ t('serverPanel.aclPwCount', { n: record.passwords }) }}</span>
          </template>
          <template v-else-if="column.key === 'actions'">
            <a-button size="small" type="link" style="padding: 0 8px 0 0" @click="openEdit(record)">
              {{ t('serverPanel.aclEdit') }}
            </a-button>
            <a-button
              v-if="record.name !== 'default'"
              size="small" type="link" danger style="padding: 0"
              @click="onDelete(record)"
            >
              {{ t('serverPanel.aclDelete') }}
            </a-button>
          </template>
        </template>
      </a-table>
    </div>

    <a-drawer
      :open="drawerOpen"
      :title="isNew ? t('serverPanel.aclNewTitle') : t('serverPanel.aclEditTitle', { name: form.name })"
      width="440"
      @close="drawerOpen = false"
    >
      <a-form layout="vertical" size="small">
        <a-form-item :label="t('serverPanel.aclNameLabel')" required>
          <a-input v-model:value="form.name" :disabled="!isNew" />
        </a-form-item>
        <a-form-item>
          <a-checkbox v-model:checked="form.enabled">{{ t('serverPanel.aclEnabledLabel') }}</a-checkbox>
        </a-form-item>
        <a-form-item>
          <a-checkbox v-model:checked="form.nopass">{{ t('serverPanel.aclNopassLabel') }}</a-checkbox>
          <a-checkbox v-model:checked="form.resetpass" :disabled="form.nopass" style="margin-left: 12px">
            {{ t('serverPanel.aclResetpassLabel') }}
          </a-checkbox>
        </a-form-item>
        <a-form-item v-if="!form.nopass" :label="t('serverPanel.aclPasswordLabel')">
          <a-input-password v-model:value="form.password" autocomplete="new-password" />
        </a-form-item>
        <a-form-item :label="t('serverPanel.aclKeysLabel')">
          <a-input v-model:value="form.keys" :placeholder="t('serverPanel.aclKeysPlaceholder')" class="mono-input" />
        </a-form-item>
        <a-form-item :label="t('serverPanel.aclCommandsLabel')">
          <a-textarea v-model:value="form.commands" :rows="2" :placeholder="t('serverPanel.aclCommandsPlaceholder')" class="mono-input" />
        </a-form-item>
        <a-form-item :label="t('serverPanel.aclChannelsLabel')">
          <a-input v-model:value="form.channels" :placeholder="t('serverPanel.aclChannelsPlaceholder')" class="mono-input" />
        </a-form-item>
        <a-form-item :label="t('serverPanel.aclRawLabel')">
          <a-input v-model:value="form.raw" :placeholder="t('serverPanel.aclRawPlaceholder')" class="mono-input" />
        </a-form-item>
        <a-form-item :label="t('serverPanel.aclRulesPreview')">
          <div class="rules-preview mono">{{ rules.join(' ') }}</div>
        </a-form-item>
        <div class="drawer-actions">
          <a-button style="margin-right: 8px" @click="drawerOpen = false">{{ t('common.cancel') }}</a-button>
          <a-button type="primary" :loading="saving" @click="onSave">{{ t('common.save') }}</a-button>
        </div>
      </a-form>
    </a-drawer>
  </div>
</template>

<style scoped>
.acl-panel { height: 100%; display: flex; flex-direction: column; }
.acl-toolbar {
  display: flex; align-items: center; gap: 8px;
  padding: 8px 16px;
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
}
.acl-table { flex: 1; overflow: auto; padding: 10px 16px; }
.rules-preview {
  background: var(--bg-hover);
  border-radius: 4px;
  padding: 8px 10px;
  font-size: 11.5px;
  word-break: break-all;
  user-select: text;
  color: var(--text-2);
}
.drawer-actions { display: flex; justify-content: flex-end; }
</style>
