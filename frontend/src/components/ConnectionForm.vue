<script setup lang="ts">
import { ref, watch, reactive, nextTick } from 'vue'
import { message } from 'ant-design-vue'
import { useI18n } from 'vue-i18n'
import { useConnectionStore } from '../stores/connection'
import type { ConnectionProfile } from '../types'

const props = defineProps<{ open: boolean; profile: ConnectionProfile | null }>()
const emit = defineEmits<{ 'update:open': [boolean] }>()

const conn = useConnectionStore()
const { t } = useI18n()
const saving = ref(false)
const testing = ref(false)

const COLORS = ['#e04536', '#f5a623', '#3fbf6b', '#3b82f6', '#a855f7', '#14b8a6', '#ec4899', '#8b93a3']

function emptyProfile(): ConnectionProfile {
  return {
    id: '', name: '', group: '', host: '127.0.0.1', port: 6379,
    username: '', password: '', useTls: false,
    tlsSkipVerify: true, tlsCaCert: '', tlsClientCert: '', tlsClientKey: '',
    db: 0, color: COLORS[0], cluster: false,
    sentinel: false, masterName: '', sentinelAddrs: '',
    readonly: false, env: '', audit: false,
    ssh: { enabled: false, host: '', port: 22, username: '', authType: 'password', password: '', privateKey: '', passphrase: '' },
  }
}

const form = reactive<ConnectionProfile>(emptyProfile())
const nameInput = ref()

watch(() => props.open, v => {
  if (v) {
    Object.assign(form, emptyProfile(), props.profile ? JSON.parse(JSON.stringify(props.profile)) : {})
    if (!form.ssh) form.ssh = emptyProfile().ssh
    // 旧配置无该字段：保持历史行为（跳过证书校验）
    if (form.tlsSkipVerify == null) form.tlsSkipVerify = true
    nextTick(() => nameInput.value?.focus())
  }
})

function onEnterKey(e: KeyboardEvent) {
  const tag = (e.target as HTMLElement)?.tagName
  if (tag === 'TEXTAREA') return
  e.preventDefault()
  onSave()
}

function close() { emit('update:open', false) }

async function onTest() {
  testing.value = true
  try {
    await conn.test(form)
    message.success(t('connection.testSuccess'))
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    testing.value = false
  }
}

async function onSave() {
  if (!form.host) { message.warning(t('connection.hostRequired')); return }
  if (form.sentinel && !form.masterName?.trim()) {
    message.warning(t('connection.masterNameRequired'))
    return
  }
  saving.value = true
  try {
    await conn.save(JSON.parse(JSON.stringify(form)))
    message.success(t('connection.saved'))
    close()
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <a-modal
    :open="open"
    :title="profile ? t('connection.editTitle') : t('connection.newTitle')"
    width="640px"
    :mask-closable="false"
    @update:open="emit('update:open', $event)"
    @cancel="close"
  >
    <a-tabs size="small" @keydown.enter="onEnterKey">
      <a-tab-pane key="general" :tab="t('connection.tabGeneral')">
        <a-form layout="vertical" size="small">
          <div class="form-row">
            <a-form-item :label="t('connection.nameLabel')" class="flex-2">
              <a-input ref="nameInput" v-model:value="form.name" :placeholder="t('connection.namePlaceholder')" />
            </a-form-item>
            <a-form-item :label="t('connection.groupLabel')" class="flex-1">
              <a-input v-model:value="form.group" :placeholder="t('connection.groupPlaceholder')" />
            </a-form-item>
          </div>
          <div class="form-row">
            <a-form-item :label="t('connection.hostLabel')" required class="flex-2">
              <a-input v-model:value="form.host" placeholder="127.0.0.1" />
            </a-form-item>
            <a-form-item :label="t('connection.portLabel')" required class="flex-1">
              <a-input-number v-model:value="form.port" :min="1" :max="65535" style="width: 100%" />
            </a-form-item>
          </div>
          <div class="form-row">
            <a-form-item :label="t('connection.usernameLabel')" class="flex-1">
              <a-input v-model:value="form.username" autocomplete="off" />
            </a-form-item>
            <a-form-item :label="t('connection.passwordLabel')" class="flex-1">
              <a-input-password v-model:value="form.password" autocomplete="new-password" />
            </a-form-item>
          </div>
          <div class="form-row">
            <a-form-item :label="t('connection.defaultDb')" class="flex-1">
              <a-input-number v-model:value="form.db" :min="0" :max="255" style="width: 100%" />
            </a-form-item>
            <a-form-item :label="t('connection.colorLabel')" class="flex-2">
              <div class="color-picker">
                <span
                  v-for="c in COLORS"
                  :key="c"
                  class="color-swatch"
                  :class="{ selected: form.color === c }"
                  :style="{ background: c }"
                  @click="form.color = c"
                />
              </div>
            </a-form-item>
          </div>
          <a-form-item>
            <a-checkbox v-model:checked="form.useTls">{{ t('connection.useTls') }}</a-checkbox>
            <a-checkbox
              v-model:checked="form.cluster"
              style="margin-left: 16px"
              :disabled="form.sentinel"
            >
              {{ t('connection.clusterMode') }}
            </a-checkbox>
            <a-checkbox
              v-model:checked="form.sentinel"
              style="margin-left: 16px"
              :disabled="form.cluster"
            >
              {{ t('connection.sentinelMode') }}
            </a-checkbox>
          </a-form-item>
          <div class="form-row">
            <a-form-item :label="t('connection.envLabel')" class="flex-1">
              <a-select v-model:value="form.env" size="small">
                <a-select-option value="">{{ t('connection.envNone') }}</a-select-option>
                <a-select-option value="prod">{{ t('connection.envProd') }}</a-select-option>
                <a-select-option value="test">{{ t('connection.envTest') }}</a-select-option>
                <a-select-option value="dev">{{ t('connection.envDev') }}</a-select-option>
              </a-select>
            </a-form-item>
            <a-form-item :label="t('connection.safetyLabel')" class="flex-2">
              <a-checkbox v-model:checked="form.readonly">{{ t('connection.readonlyLabel') }}</a-checkbox>
              <a-checkbox v-model:checked="form.audit" style="margin-left: 12px">{{ t('connection.auditLabel') }}</a-checkbox>
            </a-form-item>
          </div>
          <template v-if="form.sentinel">
            <a-form-item :label="t('connection.masterNameLabel')" required>
              <a-input v-model:value="form.masterName" placeholder="mymaster" />
            </a-form-item>
            <a-form-item :label="t('connection.sentinelAddrsLabel')">
              <a-textarea
                v-model:value="form.sentinelAddrs"
                :rows="2"
                :placeholder="t('connection.sentinelAddrsPlaceholder')"
                class="mono-input"
              />
            </a-form-item>
          </template>
        </a-form>
      </a-tab-pane>

      <a-tab-pane key="tls" tab="TLS">
        <a-form layout="vertical" size="small">
          <a-form-item>
            <a-switch v-model:checked="form.useTls" />
            <span style="margin-left: 8px">{{ t('connection.tlsEnable') }}</span>
          </a-form-item>
          <template v-if="form.useTls">
            <a-form-item>
              <a-checkbox v-model:checked="form.tlsSkipVerify">
                {{ t('connection.tlsSkipVerify') }}
              </a-checkbox>
            </a-form-item>
            <a-form-item :label="t('connection.caCertLabel')">
              <a-textarea
                v-model:value="form.tlsCaCert"
                :rows="4"
                placeholder="-----BEGIN CERTIFICATE-----&#10;…&#10;-----END CERTIFICATE-----"
                class="mono-input"
              />
            </a-form-item>
            <a-divider style="margin: 8px 0 12px">{{ t('connection.mtlsDivider') }}</a-divider>
            <a-form-item :label="t('connection.clientCertLabel')">
              <a-textarea
                v-model:value="form.tlsClientCert"
                :rows="4"
                placeholder="-----BEGIN CERTIFICATE-----&#10;…&#10;-----END CERTIFICATE-----"
                class="mono-input"
              />
            </a-form-item>
            <a-form-item :label="t('connection.clientKeyLabel')">
              <a-textarea
                v-model:value="form.tlsClientKey"
                :rows="4"
                placeholder="-----BEGIN PRIVATE KEY-----&#10;…&#10;-----END PRIVATE KEY-----"
                class="mono-input"
              />
            </a-form-item>
          </template>
        </a-form>
      </a-tab-pane>

      <a-tab-pane key="ssh" :tab="t('connection.tabSsh')">
        <a-form layout="vertical" size="small">
          <a-form-item>
            <a-switch v-model:checked="form.ssh.enabled" />
            <span style="margin-left: 8px">{{ t('connection.sshEnable') }}</span>
          </a-form-item>
          <template v-if="form.ssh.enabled">
            <div class="form-row">
              <a-form-item :label="t('connection.sshHostLabel')" class="flex-2">
                <a-input v-model:value="form.ssh.host" />
              </a-form-item>
              <a-form-item :label="t('connection.portLabel')" class="flex-1">
                <a-input-number v-model:value="form.ssh.port" :min="1" :max="65535" style="width: 100%" />
              </a-form-item>
            </div>
            <div class="form-row">
              <a-form-item :label="t('connection.sshUsernameLabel')" class="flex-1">
                <a-input v-model:value="form.ssh.username" />
              </a-form-item>
              <a-form-item :label="t('connection.authTypeLabel')" class="flex-1">
                <a-radio-group v-model:value="form.ssh.authType">
                  <a-radio-button value="password">{{ t('connection.authPassword') }}</a-radio-button>
                  <a-radio-button value="key">{{ t('connection.authKey') }}</a-radio-button>
                </a-radio-group>
              </a-form-item>
            </div>
            <a-form-item v-if="form.ssh.authType === 'password'" :label="t('connection.sshPasswordLabel')">
              <a-input-password v-model:value="form.ssh.password" autocomplete="new-password" />
            </a-form-item>
            <template v-else>
              <a-form-item :label="t('connection.privateKeyLabel')">
                <a-textarea v-model:value="form.ssh.privateKey" :rows="3" :placeholder="t('connection.privateKeyPlaceholder')" />
              </a-form-item>
              <a-form-item :label="t('connection.passphraseLabel')">
                <a-input-password v-model:value="form.ssh.passphrase" autocomplete="new-password" />
              </a-form-item>
            </template>
          </template>
        </a-form>
      </a-tab-pane>
    </a-tabs>

    <template #footer>
      <a-button :loading="testing" @click="onTest">{{ t('connection.testConnection') }}</a-button>
      <a-button @click="close">{{ t('common.cancel') }}</a-button>
      <a-button type="primary" :loading="saving" @click="onSave">{{ t('common.save') }}</a-button>
    </template>
  </a-modal>
</template>

<style scoped>
.form-row { display: flex; gap: 12px; }
.flex-1 { flex: 1; }
.flex-2 { flex: 2; }
.color-picker { display: flex; gap: 8px; align-items: center; height: 24px; }
.color-swatch {
  width: 18px; height: 18px; border-radius: 50%;
  cursor: pointer; transition: transform 0.12s;
  border: 2px solid transparent;
}
.color-swatch:hover { transform: scale(1.15); }
.color-swatch.selected {
  border-color: var(--text-1);
  box-shadow: 0 0 0 2px var(--bg-panel), 0 0 0 4px currentColor;
}
.mono-input :deep(textarea), .mono-input {
  font-family: var(--mono, monospace);
  font-size: 11.5px;
}
</style>
