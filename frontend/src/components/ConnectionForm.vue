<script setup lang="ts">
import { ref, watch, reactive, nextTick } from 'vue'
import { message } from 'ant-design-vue'
import { useConnectionStore } from '../stores/connection'
import type { ConnectionProfile } from '../types'

const props = defineProps<{ open: boolean; profile: ConnectionProfile | null }>()
const emit = defineEmits<{ 'update:open': [boolean] }>()

const conn = useConnectionStore()
const saving = ref(false)
const testing = ref(false)

const COLORS = ['#e04536', '#f5a623', '#3fbf6b', '#3b82f6', '#a855f7', '#14b8a6', '#ec4899', '#8b93a3']

function emptyProfile(): ConnectionProfile {
  return {
    id: '', name: '', group: '', host: '127.0.0.1', port: 6379,
    username: '', password: '', useTls: false,
    tlsSkipVerify: true, tlsCaCert: '', tlsClientCert: '', tlsClientKey: '',
    db: 0, color: COLORS[0], cluster: false,
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
    message.success('连接成功')
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    testing.value = false
  }
}

async function onSave() {
  if (!form.host) { message.warning('请填写主机地址'); return }
  saving.value = true
  try {
    await conn.save(JSON.parse(JSON.stringify(form)))
    message.success('已保存')
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
    :title="profile ? '编辑连接' : '新建连接'"
    width="640px"
    :mask-closable="false"
    @update:open="emit('update:open', $event)"
    @cancel="close"
  >
    <a-tabs size="small" @keydown.enter="onEnterKey">
      <a-tab-pane key="general" tab="常规">
        <a-form layout="vertical" size="small">
          <div class="form-row">
            <a-form-item label="连接名称" class="flex-2">
              <a-input ref="nameInput" v-model:value="form.name" placeholder="留空则使用 主机:端口" />
            </a-form-item>
            <a-form-item label="分组" class="flex-1">
              <a-input v-model:value="form.group" placeholder="默认分组" />
            </a-form-item>
          </div>
          <div class="form-row">
            <a-form-item label="主机" required class="flex-2">
              <a-input v-model:value="form.host" placeholder="127.0.0.1" />
            </a-form-item>
            <a-form-item label="端口" required class="flex-1">
              <a-input-number v-model:value="form.port" :min="1" :max="65535" style="width: 100%" />
            </a-form-item>
          </div>
          <div class="form-row">
            <a-form-item label="用户名（ACL，可空）" class="flex-1">
              <a-input v-model:value="form.username" autocomplete="off" />
            </a-form-item>
            <a-form-item label="密码" class="flex-1">
              <a-input-password v-model:value="form.password" autocomplete="new-password" />
            </a-form-item>
          </div>
          <div class="form-row">
            <a-form-item label="默认数据库" class="flex-1">
              <a-input-number v-model:value="form.db" :min="0" :max="255" style="width: 100%" />
            </a-form-item>
            <a-form-item label="标识颜色" class="flex-2">
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
            <a-checkbox v-model:checked="form.useTls">启用 TLS/SSL（详细选项见「TLS」页）</a-checkbox>
            <a-checkbox v-model:checked="form.cluster" style="margin-left: 16px">
              Redis Cluster 集群模式（仅 db 0）
            </a-checkbox>
          </a-form-item>
        </a-form>
      </a-tab-pane>

      <a-tab-pane key="tls" tab="TLS">
        <a-form layout="vertical" size="small">
          <a-form-item>
            <a-switch v-model:checked="form.useTls" />
            <span style="margin-left: 8px">启用 TLS/SSL 加密连接</span>
          </a-form-item>
          <template v-if="form.useTls">
            <a-form-item>
              <a-checkbox v-model:checked="form.tlsSkipVerify">
                跳过服务端证书校验（自签证书场景；关闭后需可信 CA）
              </a-checkbox>
            </a-form-item>
            <a-form-item label="自定义 CA 证书（PEM 内容，可空）">
              <a-textarea
                v-model:value="form.tlsCaCert"
                :rows="4"
                placeholder="-----BEGIN CERTIFICATE-----&#10;…&#10;-----END CERTIFICATE-----"
                class="mono-input"
              />
            </a-form-item>
            <a-divider style="margin: 8px 0 12px">双向认证（mTLS，可选）</a-divider>
            <a-form-item label="客户端证书（PEM 内容）">
              <a-textarea
                v-model:value="form.tlsClientCert"
                :rows="4"
                placeholder="-----BEGIN CERTIFICATE-----&#10;…&#10;-----END CERTIFICATE-----"
                class="mono-input"
              />
            </a-form-item>
            <a-form-item label="客户端私钥（PEM 内容，加密落盘）">
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

      <a-tab-pane key="ssh" tab="SSH 隧道">
        <a-form layout="vertical" size="small">
          <a-form-item>
            <a-switch v-model:checked="form.ssh.enabled" />
            <span style="margin-left: 8px">启用 SSH 隧道</span>
          </a-form-item>
          <template v-if="form.ssh.enabled">
            <div class="form-row">
              <a-form-item label="SSH 主机" class="flex-2">
                <a-input v-model:value="form.ssh.host" />
              </a-form-item>
              <a-form-item label="端口" class="flex-1">
                <a-input-number v-model:value="form.ssh.port" :min="1" :max="65535" style="width: 100%" />
              </a-form-item>
            </div>
            <div class="form-row">
              <a-form-item label="用户名" class="flex-1">
                <a-input v-model:value="form.ssh.username" />
              </a-form-item>
              <a-form-item label="认证方式" class="flex-1">
                <a-radio-group v-model:value="form.ssh.authType">
                  <a-radio-button value="password">密码</a-radio-button>
                  <a-radio-button value="key">私钥</a-radio-button>
                </a-radio-group>
              </a-form-item>
            </div>
            <a-form-item v-if="form.ssh.authType === 'password'" label="SSH 密码">
              <a-input-password v-model:value="form.ssh.password" autocomplete="new-password" />
            </a-form-item>
            <template v-else>
              <a-form-item label="私钥路径或内容">
                <a-textarea v-model:value="form.ssh.privateKey" :rows="3" placeholder="C:\Users\you\.ssh\id_rsa 或粘贴私钥内容" />
              </a-form-item>
              <a-form-item label="私钥口令（可空）">
                <a-input-password v-model:value="form.ssh.passphrase" autocomplete="new-password" />
              </a-form-item>
            </template>
          </template>
        </a-form>
      </a-tab-pane>
    </a-tabs>

    <template #footer>
      <a-button :loading="testing" @click="onTest">测试连接</a-button>
      <a-button @click="close">取消</a-button>
      <a-button type="primary" :loading="saving" @click="onSave">保存</a-button>
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
