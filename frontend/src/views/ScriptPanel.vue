<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { message, Modal } from 'ant-design-vue'
import {
  ReloadOutlined, ThunderboltOutlined, DeleteOutlined, CopyOutlined,
  PlusOutlined, CodeOutlined, ExclamationCircleOutlined,
} from '@ant-design/icons-vue'
import {
  ScriptList, ScriptLoad, ScriptFlush, ScriptKill,
  FunctionList, FunctionStats, FunctionLoad, FunctionDelete, FunctionKill,
} from '../../wailsjs/go/service/ServerService'
import CodeEditor from '../components/CodeEditor.vue'
import type { FunctionLibrary, FunctionRunStat } from '../types'

// 脚本管理面板：Lua 脚本缓存（SCRIPT）+ Redis Functions（Redis 7+）
const props = defineProps<{ connId: string; db: number }>()
const { t } = useI18n()

const section = ref<'scripts' | 'functions'>('scripts')

// ---------- Lua 脚本缓存 ----------
const scripts = ref<string[]>([])
const scriptsLoading = ref(false)
const scriptListSupported = ref(true) // SCRIPT LIST 需要 Redis 8.2+
const scriptCode = ref('return redis.call("GET", KEYS[1])')
const scriptLoadingBusy = ref(false)

function isUnknownCmd(e: any): boolean {
  return /unknown (sub)?command/i.test(String(e?.message ?? e))
}

async function loadScripts() {
  scriptsLoading.value = true
  try {
    scripts.value = (await ScriptList(props.connId, props.db)) ?? []
    scriptListSupported.value = true
  } catch (e: any) {
    if (isUnknownCmd(e)) {
      scriptListSupported.value = false
      scripts.value = []
    } else {
      message.error(String(e?.message ?? e))
    }
  } finally {
    scriptsLoading.value = false
  }
}

async function loadScript() {
  if (!scriptCode.value.trim()) { message.warning(t('script.codeRequired')); return }
  scriptLoadingBusy.value = true
  try {
    const sha = await ScriptLoad(props.connId, props.db, scriptCode.value)
    message.success(t('script.loaded', { sha: sha.slice(0, 10) + '…' }))
    if (scriptListSupported.value) await loadScripts()
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    scriptLoadingBusy.value = false
  }
}

function copySha(sha: string) {
  navigator.clipboard.writeText(sha).then(() => message.success(t('common.copied')))
}

function flushScripts() {
  Modal.confirm({
    title: t('script.flushTitle'),
    content: t('script.flushContent'),
    okText: t('common.confirm'),
    okButtonProps: { danger: true },
    cancelText: t('common.cancel'),
    async onOk() {
      try {
        await ScriptFlush(props.connId, props.db)
        message.success(t('script.flushed'))
        if (scriptListSupported.value) await loadScripts()
      } catch (e: any) {
        message.error(String(e?.message ?? e))
      }
    },
  })
}

async function killScript() {
  try {
    await ScriptKill(props.connId, props.db)
    message.success(t('script.killed'))
  } catch (e: any) {
    message.warning(String(e?.message ?? e))
  }
}

// ---------- Redis Functions ----------
const libs = ref<FunctionLibrary[]>([])
const libsLoading = ref(false)
const functionsSupported = ref(true) // FUNCTION 需要 Redis 7+
const stat = ref<FunctionRunStat>({ running: false, name: '', command: '', durationMs: 0 })
const loadVisible = ref(false)
const loadCode = ref('#!lua name=mylib\nredis.register_function(\'myfn\', function(keys, args)\n  return redis.call("PING")\nend)')
const loadReplace = ref(false)
const loadBusy = ref(false)
const codeVisible = ref(false)
const codeLib = ref<FunctionLibrary | null>(null)

async function loadFunctions() {
  libsLoading.value = true
  try {
    libs.value = ((await FunctionList(props.connId, props.db)) as unknown as FunctionLibrary[]) ?? []
    functionsSupported.value = true
  } catch (e: any) {
    if (isUnknownCmd(e)) {
      functionsSupported.value = false
      libs.value = []
    } else {
      message.error(String(e?.message ?? e))
    }
  } finally {
    libsLoading.value = false
  }
  try {
    stat.value = (await FunctionStats(props.connId, props.db)) as unknown as FunctionRunStat
  } catch { /* 低版本无 FUNCTION STATS，静默 */ }
}

async function doLoadLibrary() {
  if (!loadCode.value.trim()) { message.warning(t('script.codeRequired')); return }
  loadBusy.value = true
  try {
    const name = await FunctionLoad(props.connId, props.db, loadCode.value, loadReplace.value)
    message.success(t('script.libLoaded', { name }))
    loadVisible.value = false
    await loadFunctions()
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    loadBusy.value = false
  }
}

function deleteLibrary(lib: FunctionLibrary) {
  Modal.confirm({
    title: t('script.deleteLibTitle'),
    content: t('script.deleteLibContent', { name: lib.name }),
    okText: t('common.delete'),
    okButtonProps: { danger: true },
    cancelText: t('common.cancel'),
    async onOk() {
      try {
        await FunctionDelete(props.connId, props.db, lib.name)
        message.success(t('common.deleted'))
        await loadFunctions()
      } catch (e: any) {
        message.error(String(e?.message ?? e))
      }
    },
  })
}

async function killFunction() {
  try {
    await FunctionKill(props.connId, props.db)
    message.success(t('script.killed'))
    stat.value.running = false
  } catch (e: any) {
    message.warning(String(e?.message ?? e))
  }
}

function viewCode(lib: FunctionLibrary) {
  codeLib.value = lib
  codeVisible.value = true
}

onMounted(() => {
  loadScripts()
  loadFunctions()
})
</script>

<template>
  <div class="script-panel">
    <div class="sp-head">
      <a-segmented
        v-model:value="section"
        size="small"
        :options="[
          { value: 'scripts', label: t('script.secScripts') },
          { value: 'functions', label: t('script.secFunctions') },
        ]"
      />
    </div>

    <!-- ================= Lua 脚本缓存 ================= -->
    <div v-show="section === 'scripts'" class="sp-body">
      <a-alert
        v-if="!scriptListSupported"
        type="warning"
        show-icon
        class="sp-gate"
        :message="t('script.listUnsupported')"
      />
      <div class="sp-col">
        <div class="sp-card">
          <div class="sp-title">
            {{ t('script.cachedScripts', { n: scripts.length }) }}
            <div class="sp-title-actions">
              <a-tooltip :title="t('script.killTip')">
                <a-button size="small" type="text" danger @click="killScript">
                  <template #icon><ExclamationCircleOutlined /></template>
                  KILL
                </a-button>
              </a-tooltip>
              <a-tooltip :title="t('script.flushTip')">
                <a-button size="small" type="text" danger @click="flushScripts">
                  <template #icon><DeleteOutlined /></template>
                  FLUSH
                </a-button>
              </a-tooltip>
              <a-button size="small" type="text" :loading="scriptsLoading" @click="loadScripts">
                <template #icon><ReloadOutlined /></template>
              </a-button>
            </div>
          </div>
          <a-empty
            v-if="!scripts.length"
            :description="scriptListSupported ? t('script.noScripts') : t('script.listUnsupportedShort')"
            :image-style="{ height: '40px' }"
          />
          <div v-else class="sha-list">
            <div v-for="sha in scripts" :key="sha" class="sha-item">
              <span class="mono sha-text">{{ sha }}</span>
              <a-tooltip :title="t('common.copy')">
                <a-button size="small" type="text" @click="copySha(sha)">
                  <template #icon><CopyOutlined /></template>
                </a-button>
              </a-tooltip>
            </div>
          </div>
        </div>

        <div class="sp-card sp-card-grow">
          <div class="sp-title">{{ t('script.loadScript') }}</div>
          <div class="editor-box">
            <CodeEditor v-model="scriptCode" lang="text" :placeholder="t('script.scriptPlaceholder')" />
          </div>
          <div class="sp-actions">
            <span class="text-3 sp-hint">{{ t('script.loadHint') }}</span>
            <a-button size="small" type="primary" :loading="scriptLoadingBusy" @click="loadScript">
              <template #icon><ThunderboltOutlined /></template>
              SCRIPT LOAD
            </a-button>
          </div>
        </div>
      </div>
    </div>

    <!-- ================= Redis Functions ================= -->
    <div v-show="section === 'functions'" class="sp-body">
      <a-alert
        v-if="!functionsSupported"
        type="warning"
        show-icon
        class="sp-gate"
        :message="t('script.functionsUnsupported')"
      />
      <div class="sp-col">
        <a-alert
          v-if="stat.running"
          type="error"
          show-icon
          class="sp-gate"
        >
          <template #message>
            <div class="running-row">
              <span>
                {{ t('script.runningFn') }}: <span class="mono">{{ stat.name }}</span>
                <span class="text-3 mono"> {{ stat.command }}</span>
                <span class="text-3"> · {{ stat.durationMs }}ms</span>
              </span>
              <a-button size="small" danger @click="killFunction">FUNCTION KILL</a-button>
            </div>
          </template>
        </a-alert>
        <div class="sp-card sp-card-grow">
          <div class="sp-title">
            {{ t('script.libraries', { n: libs.length }) }}
            <div class="sp-title-actions">
              <a-button size="small" type="primary" ghost :disabled="!functionsSupported" @click="loadVisible = true">
                <template #icon><PlusOutlined /></template>
                {{ t('script.loadLibrary') }}
              </a-button>
              <a-button size="small" type="text" :loading="libsLoading" @click="loadFunctions">
                <template #icon><ReloadOutlined /></template>
              </a-button>
            </div>
          </div>
          <a-empty
            v-if="!libs.length"
            :description="functionsSupported ? t('script.noLibraries') : t('script.functionsUnsupportedShort')"
            :image-style="{ height: '40px' }"
          />
          <a-collapse v-else ghost class="lib-collapse">
            <a-collapse-panel v-for="lib in libs" :key="lib.name">
              <template #header>
                <div class="lib-head">
                  <span class="mono lib-name">{{ lib.name }}</span>
                  <a-tag class="mono" color="blue">{{ lib.engine }}</a-tag>
                  <span class="text-3">{{ t('script.fnCount', { n: lib.functions.length }) }}</span>
                </div>
              </template>
              <template #extra>
                <a-tooltip :title="t('script.viewCode')">
                  <a-button size="small" type="text" @click.stop="viewCode(lib)">
                    <template #icon><CodeOutlined /></template>
                  </a-button>
                </a-tooltip>
                <a-tooltip :title="t('common.delete')">
                  <a-button size="small" type="text" danger @click.stop="deleteLibrary(lib)">
                    <template #icon><DeleteOutlined /></template>
                  </a-button>
                </a-tooltip>
              </template>
              <div class="fn-list">
                <div v-for="fn in lib.functions" :key="fn.name" class="fn-item">
                  <span class="mono fn-name">{{ fn.name }}</span>
                  <a-tag v-for="f in fn.flags" :key="f" class="mono fn-flag">{{ f }}</a-tag>
                  <span v-if="fn.desc" class="text-3 fn-desc">{{ fn.desc }}</span>
                </div>
              </div>
            </a-collapse-panel>
          </a-collapse>
        </div>
      </div>
    </div>

    <!-- 注册函数库 -->
    <a-modal
      v-model:open="loadVisible"
      :title="t('script.loadLibrary')"
      :width="640"
      :ok-text="t('script.loadLibraryOk')"
      :cancel-text="t('common.cancel')"
      :confirm-loading="loadBusy"
      @ok="doLoadLibrary"
    >
      <div class="modal-editor">
        <CodeEditor v-model="loadCode" lang="text" />
      </div>
      <a-checkbox v-model:checked="loadReplace" style="margin-top: 10px">
        REPLACE <span class="text-3">{{ t('script.replaceTip') }}</span>
      </a-checkbox>
    </a-modal>

    <!-- 查看库代码 -->
    <a-drawer
      v-model:open="codeVisible"
      :title="codeLib ? `${codeLib.name} (${codeLib.engine})` : ''"
      :width="560"
      placement="right"
    >
      <div class="drawer-editor">
        <CodeEditor :model-value="codeLib?.code ?? ''" lang="text" readonly />
      </div>
    </a-drawer>
  </div>
</template>

<style scoped>
.script-panel { height: 100%; display: flex; flex-direction: column; padding: 12px 16px; gap: 10px; }
.sp-head { flex-shrink: 0; }
.sp-body { flex: 1; min-height: 0; display: flex; flex-direction: column; gap: 10px; }
.sp-gate { flex-shrink: 0; }
.sp-col { flex: 1; min-height: 0; display: flex; flex-direction: column; gap: 10px; overflow-y: auto; }
.sp-card {
  background: var(--bg-panel);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  padding: 14px 16px;
  flex-shrink: 0;
  display: flex; flex-direction: column;
}
.sp-card-grow { flex: 1; min-height: 220px; }
.sp-title { font-size: 12.5px; font-weight: 600; margin-bottom: 12px; display: flex; align-items: center; }
.sp-title-actions { margin-left: auto; display: flex; align-items: center; gap: 4px; }
.sha-list { display: flex; flex-direction: column; gap: 4px; }
.sha-item {
  display: flex; align-items: center; gap: 6px;
  padding: 5px 9px;
  background: var(--bg-inset);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
}
.sha-text { flex: 1; font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.editor-box {
  flex: 1; min-height: 140px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: var(--bg-inset);
  overflow: hidden;
}
.sp-actions { display: flex; align-items: center; margin-top: 10px; gap: 10px; }
.sp-hint { flex: 1; font-size: 11px; }
.running-row { display: flex; align-items: center; justify-content: space-between; gap: 10px; }
.lib-collapse :deep(.ant-collapse-item) { border-bottom: 1px solid var(--border); }
.lib-head { display: flex; align-items: center; gap: 8px; }
.lib-name { font-size: 12.5px; font-weight: 600; }
.fn-list { display: flex; flex-direction: column; gap: 4px; padding: 2px 0 6px; }
.fn-item {
  display: flex; align-items: center; gap: 6px; flex-wrap: wrap;
  padding: 5px 9px;
  background: var(--bg-inset);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
}
.fn-name { font-size: 12px; }
.fn-flag { font-size: 10px; margin-inline-end: 0; }
.fn-desc { font-size: 11px; }
.modal-editor { height: 260px; border: 1px solid var(--border); border-radius: var(--radius-sm); overflow: hidden; }
.drawer-editor { height: 100%; border: 1px solid var(--border); border-radius: var(--radius-sm); overflow: hidden; }
</style>
