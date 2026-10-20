<script setup lang="ts">
import { ref, computed, nextTick, onMounted, onBeforeUnmount, watch } from 'vue'
import { message, Modal } from 'ant-design-vue'
import {
  ClearOutlined, EnterOutlined, PlayCircleOutlined, StopOutlined,
  PauseOutlined, CaretRightOutlined,
} from '@ant-design/icons-vue'
import { ExecCommand } from '../../wailsjs/go/service/CliService'
import { SuggestKeys } from '../../wailsjs/go/service/KeyService'
import { StartMonitor, StopMonitor } from '../../wailsjs/go/service/ServerService'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import { useAppStore } from '../stores/app'
import { REDIS_COMMANDS, REDIS_SUBCOMMANDS } from '../utils/redisCommands'
import { useReadonly } from '../utils/readonly'
import type { CliResult } from '../types'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ connId: string; db: number }>()
const app = useAppStore()
const { t } = useI18n()
const ro = useReadonly(() => props.connId)

interface Entry {
  id: number
  cmd: string
  result: CliResult
  time: string
}

const entries = ref<Entry[]>([])
const input = ref('')
const historyIdx = ref(-1)
const running = ref(false)
const outputRef = ref<HTMLElement>()
const inputRef = ref<HTMLInputElement>()
let seq = 0

// ---------- Tab 自动补全（命令 / 子命令 / 键名） ----------
const cands = ref<string[]>([])
const candIdx = ref(0)
const candOpen = ref(false)
let enterPicked = false // Enter 用于选中候选时，拦截随后的 keyup.enter 触发执行

interface Tok { start: number; prefix: string; index: number }

// 光标所在 token：start 为 token 起始下标，index 为第几个参数（0 = 命令名）
function currentToken(): Tok {
  const text = input.value
  const cursor = inputRef.value?.selectionStart ?? text.length
  const before = text.slice(0, cursor)
  const start = before.lastIndexOf(' ') + 1
  const index = before.slice(0, start).trim().split(/\s+/).filter(Boolean).length
  return { start, prefix: before.slice(start), index }
}

function closeCands() {
  candOpen.value = false
  cands.value = []
  candIdx.value = 0
}

async function complete() {
  const tok = currentToken()
  const cmdName = input.value.trim().split(/\s+/)[0]?.toUpperCase() ?? ''
  let list: string[] = []
  if (tok.index === 0) {
    const p = tok.prefix.toUpperCase()
    list = REDIS_COMMANDS.filter(c => c.startsWith(p) && c !== p)
  } else if (tok.index === 1 && REDIS_SUBCOMMANDS[cmdName]) {
    const p = tok.prefix.toUpperCase()
    list = REDIS_SUBCOMMANDS[cmdName].filter(s => s.startsWith(p) && s !== p)
  } else {
    // 其余参数位按键名补全（异步，需防过期响应）
    try {
      list = await SuggestKeys(props.connId, props.db, tok.prefix)
    } catch {
      list = []
    }
    const now = currentToken()
    if (now.start !== tok.start || now.prefix !== tok.prefix) return // 期间输入已变化
    list = list.filter(k => k !== tok.prefix)
  }
  if (!list.length) { closeCands(); return }
  if (list.length === 1) {
    cands.value = list
    applyCand(0)
    return
  }
  cands.value = list
  candIdx.value = 0
  candOpen.value = true
}

function applyCand(i: number) {
  const c = cands.value[i]
  if (c === undefined) return
  const tok = currentToken()
  const text = input.value
  const cursor = inputRef.value?.selectionStart ?? text.length
  let end = text.indexOf(' ', cursor)
  if (end === -1) end = text.length
  const head = text.slice(0, tok.start)
  const tail = text.slice(end)
  input.value = head + c + (tail.startsWith(' ') ? tail : ' ' + tail)
  const pos = head.length + c.length + 1
  closeCands()
  nextTick(() => {
    inputRef.value?.setSelectionRange(pos, pos)
    inputRef.value?.focus()
  })
}

function parseArgs(cmd: string): string[] {
  const args: string[] = []
  const re = /"([^"]*)"|'([^']*)'|(\S+)/g
  let m: RegExpExecArray | null
  while ((m = re.exec(cmd))) {
    args.push(m[1] ?? m[2] ?? m[3])
  }
  return args
}

function fmtTime(): string {
  const d = new Date()
  const p = (n: number) => String(n).padStart(2, '0')
  return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
}

// ---------- 危险命令护栏 ----------
function dangerOf(cmd: string): string | null {
  const c = cmd.trim()
  if (/^flushall\b/i.test(c)) return t('cli.danger.flushall')
  if (/^flushdb\b/i.test(c)) return t('cli.danger.flushdb')
  if (/^shutdown\b/i.test(c)) return t('cli.danger.shutdown')
  if (/^debug\b/i.test(c)) return t('cli.danger.debug')
  if (/^config\s+(set|resetstat|rewrite)\b/i.test(c)) return t('cli.danger.config')
  if (/^keys\b/i.test(c)) return t('cli.danger.keys')
  return null
}

function exec() {
  const cmd = input.value.trim()
  if (!cmd || running.value) return
  const danger = dangerOf(cmd)
  if (danger) {
    Modal.confirm({
      title: t('cli.danger.title'),
      content: t('cli.danger.content', { reason: danger, cmd }),
      okText: t('cli.danger.confirm'),
      okButtonProps: { danger: true },
      cancelText: t('common.cancel'),
      onOk: () => doExec(cmd),
    })
    return
  }
  doExec(cmd)
}

async function doExec(cmd: string) {
  const args = parseArgs(cmd)
  app.pushCliHistory(cmd)
  historyIdx.value = -1
  input.value = ''
  running.value = true
  try {
    const result = (await ExecCommand(props.connId, props.db, args)) as unknown as CliResult
    entries.value.push({ id: ++seq, cmd, result, time: fmtTime() })
  } catch (e: any) {
    entries.value.push({
      id: ++seq, cmd, time: fmtTime(),
      result: { ok: false, type: 'error', value: String(e?.message ?? e), costMs: 0 },
    })
  } finally {
    running.value = false
    await nextTick()
    outputRef.value?.scrollTo({ top: outputRef.value.scrollHeight })
    inputRef.value?.focus()
  }
}

function onKeydown(e: KeyboardEvent) {
  // 候选面板打开时优先处理导航/选择键
  if (candOpen.value) {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      candIdx.value = (candIdx.value + 1) % cands.value.length
      return
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault()
      candIdx.value = (candIdx.value - 1 + cands.value.length) % cands.value.length
      return
    }
    if (e.key === 'Enter' || e.key === 'Tab') {
      e.preventDefault()
      if (e.key === 'Enter') enterPicked = true
      applyCand(candIdx.value)
      return
    }
    if (e.key === 'Escape') {
      e.preventDefault()
      closeCands()
      return
    }
    closeCands() // 其他按键关闭面板，继续走正常逻辑
  }
  if (e.key === 'Tab') {
    e.preventDefault()
    complete()
    return
  }
  const history = app.cliHistory
  if (e.key === 'ArrowUp') {
    e.preventDefault()
    if (!history.length) return
    if (historyIdx.value === -1) historyIdx.value = history.length - 1
    else if (historyIdx.value > 0) historyIdx.value--
    input.value = history[historyIdx.value]
  } else if (e.key === 'ArrowDown') {
    e.preventDefault()
    if (historyIdx.value === -1) return
    if (historyIdx.value < history.length - 1) {
      historyIdx.value++
      input.value = history[historyIdx.value]
    } else {
      historyIdx.value = -1
      input.value = ''
    }
  }
}

// Enter 选中补全候选后，随后的 keyup.enter 不应再触发执行
function onEnterKeyup() {
  if (enterPicked) { enterPicked = false; return }
  exec()
}

function renderValue(val: any, depth = 0): string {
  if (val === null || val === undefined) return '(nil)'
  if (Array.isArray(val)) {
    if (!val.length) return '(empty array)'
    return val.map((v, i) => `${i + 1}) ${renderValue(v, depth + 1)}`).join('\n')
  }
  if (typeof val === 'object') return JSON.stringify(val)
  return String(val)
}

function resultClass(r: CliResult): string {
  if (!r.ok) return 'res-error'
  if (r.type === 'nil') return 'res-nil'
  if (r.type === 'integer') return 'res-int'
  return 'res-ok'
}

// ---------- MONITOR 模式 ----------
const mode = ref<'cmd' | 'monitor'>('cmd')
const monOn = ref(false)
const monPaused = ref(false)
const monFilter = ref('')
const monLines = ref<string[]>([])
const monOutputRef = ref<HTMLElement>()
const monEvent = computed(() => `monitor:${props.connId}:${props.db}`)
const MAX_MON_LINES = 500

const filteredMonLines = computed(() => {
  const f = monFilter.value.trim().toLowerCase()
  if (!f) return monLines.value
  return monLines.value.filter(l => l.toLowerCase().includes(f))
})

async function startMonitor() {
  try {
    await StartMonitor(props.connId, props.db)
    EventsOn(monEvent.value, (line: string) => {
      if (monPaused.value) return
      monLines.value.push(line)
      if (monLines.value.length > MAX_MON_LINES) {
        monLines.value.splice(0, monLines.value.length - MAX_MON_LINES)
      }
      nextTick(() => monOutputRef.value?.scrollTo({ top: monOutputRef.value.scrollHeight }))
    })
    monOn.value = true
  } catch (e: any) {
    message.error(String(e?.message ?? e))
    mode.value = 'cmd'
  }
}

function stopMonitor() {
  if (!monOn.value) return
  EventsOff(monEvent.value)
  StopMonitor(props.connId, props.db).catch(() => {})
  monOn.value = false
  monPaused.value = false
}

watch(mode, v => {
  if (v === 'monitor') startMonitor()
  else stopMonitor()
})
// 切换数据库 / 组件卸载时停止
watch(() => props.db, () => { if (mode.value === 'monitor') { stopMonitor(); startMonitor() } })
onBeforeUnmount(stopMonitor)

onMounted(() => inputRef.value?.focus())
</script>

<template>
  <div class="cli" @click="mode === 'cmd' && inputRef?.focus()">
    <div class="cli-toolbar">
      <a-segmented v-model:value="mode" size="small"
        :options="[{ value: 'cmd', label: t('cli.modeCmd') }, { value: 'monitor', label: 'MONITOR' }]" />
      <span v-if="mode === 'cmd'" class="text-3" style="font-size: 11px">
        {{ t('cli.cmdHint') }}
      </span>
      <div class="toolbar-actions">
        <template v-if="mode === 'monitor' && monOn">
          <a-tooltip :title="monPaused ? t('cli.resume') : t('cli.pause')">
            <a-button size="small" type="text" @click.stop="monPaused = !monPaused">
              <template #icon><PauseOutlined v-if="!monPaused" /><CaretRightOutlined v-else /></template>
            </a-button>
          </a-tooltip>
          <a-button size="small" type="text" @click.stop="monLines = []">
            <template #icon><ClearOutlined /></template>
          </a-button>
        </template>
        <a-button v-if="mode === 'cmd'" size="small" type="text" @click.stop="entries = []">
          <template #icon><ClearOutlined /></template>
          {{ t('cli.clear') }}
        </a-button>
      </div>
    </div>

    <!-- 命令模式 -->
    <template v-if="mode === 'cmd'">
      <div ref="outputRef" class="cli-output" @click.stop>
        <div class="cli-welcome mono">
          {{ t('cli.welcome', { db }) }}
        </div>
        <div v-for="e in entries" :key="e.id" class="cli-entry term-line">
          <div class="cli-cmd mono">
            <span class="prompt">db{{ db }}&gt;</span> {{ e.cmd }}
            <span class="cmd-time">{{ e.time }} · {{ e.result.costMs }}ms</span>
          </div>
          <pre class="cli-result mono" :class="resultClass(e.result)">{{ renderValue(e.result.value) }}</pre>
        </div>
      </div>

      <div class="cli-input-wrap">
        <div v-if="candOpen" class="cand-list mono">
          <div
            v-for="(c, i) in cands"
            :key="c"
            class="cand-item"
            :class="{ active: i === candIdx }"
            @mousedown.prevent="applyCand(i)"
            @mouseenter="candIdx = i"
          >{{ c }}</div>
          <div class="cand-hint">{{ t('cli.candHint') }}</div>
        </div>
        <div class="cli-input-row">
          <a-tooltip v-if="ro" :title="t('common.readonlyTip')">
            <span class="ro-badge mono">RO</span>
          </a-tooltip>
          <span class="prompt mono">db{{ db }}&gt;</span>
          <input
            ref="inputRef"
            v-model="input"
            class="cli-input mono"
            :placeholder="t('cli.inputPlaceholder')"
            spellcheck="false"
            @keydown="onKeydown"
            @keyup.enter="onEnterKeyup"
          />
          <a-button size="small" type="primary" :loading="running" @click="exec">
            <template #icon><EnterOutlined /></template>
          </a-button>
        </div>
      </div>
    </template>

    <!-- MONITOR 模式 -->
    <template v-else>
      <div class="mon-filter-row">
        <span class="mon-status" :class="{ on: monOn }">{{ monOn ? t('cli.monOn') : t('cli.monOff') }}</span>
        <input
          v-model="monFilter"
          class="mon-filter mono"
          :placeholder="t('cli.filterPlaceholder')"
          spellcheck="false"
          @click.stop
        />
        <span class="text-3" style="font-size: 11px">{{ t('cli.monCount', { filtered: filteredMonLines.length, total: monLines.length }) }}</span>
      </div>
      <div ref="monOutputRef" class="cli-output mon-output" @click.stop>
        <div v-if="!monLines.length" class="cli-welcome mono">
          {{ t('cli.monWelcome') }}
        </div>
        <div v-for="(l, i) in filteredMonLines" :key="i" class="mon-line mono">{{ l }}</div>
        <div v-if="monPaused && monLines.length" class="mon-paused mono">{{ t('cli.monPaused') }}</div>
      </div>
      <div class="mon-footer">
        <a-button v-if="!monOn" size="small" type="primary" @click="startMonitor">
          <template #icon><PlayCircleOutlined /></template>
          {{ t('cli.monStart') }}
        </a-button>
        <a-button v-else size="small" danger @click="stopMonitor">
          <template #icon><StopOutlined /></template>
          {{ t('cli.monStop') }}
        </a-button>
      </div>
    </template>
  </div>
</template>

<style scoped>
.cli { height: 100%; display: flex; flex-direction: column; }
.cli-toolbar {
  display: flex; align-items: center; gap: 12px;
  padding: 7px 16px;
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
}
.toolbar-actions { margin-left: auto; display: flex; align-items: center; gap: 4px; }
.cli-output {
  flex: 1;
  overflow-y: auto;
  padding: 14px 18px;
  user-select: text;
  background: var(--bg-inset);
}
.cli-welcome { color: var(--text-3); font-size: 11.5px; margin-bottom: 14px; }
.cli-entry { margin-bottom: 14px; }
.cli-cmd { font-size: 13px; font-weight: 600; letter-spacing: 0.1px; }
.cmd-time { font-size: 10px; color: var(--text-3); font-weight: 400; margin-left: 10px; }
.prompt { color: var(--accent); font-weight: 700; }
.ro-badge {
  font-size: 9.5px; font-weight: 700; line-height: 1.5;
  color: #f5a623; border: 1px solid #f5a623; border-radius: 3px;
  padding: 0 3px; margin-right: -6px; flex-shrink: 0;
}
.cli-result {
  margin: 5px 0 0;
  padding: 9px 13px;
  background: var(--bg-elevated);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  font-size: 12.5px;
  line-height: 1.7;
  white-space: pre-wrap;
  word-break: break-all;
  user-select: text;
}
.res-ok { color: var(--text-1); border-left: 2px solid var(--success); }
.res-int { color: var(--info); border-left: 2px solid var(--info); }
.res-nil { color: var(--text-3); border-left: 2px solid var(--text-3); }
.res-error { color: #ff8a80; border-left: 2px solid var(--accent); }
.cli-input-wrap { position: relative; flex-shrink: 0; }
.cand-list {
  position: absolute;
  bottom: 100%;
  left: 12px;
  min-width: 220px;
  max-width: 60%;
  max-height: 220px;
  overflow-y: auto;
  background: var(--bg-elevated);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  box-shadow: 0 -4px 16px rgba(0, 0, 0, 0.25);
  z-index: 20;
  padding: 4px;
}
.cand-item {
  padding: 3px 10px;
  font-size: 12.5px;
  color: var(--text-2);
  border-radius: var(--radius-sm);
  cursor: pointer;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.cand-item.active { background: var(--accent-soft); color: var(--text-1); }
.cand-hint {
  padding: 3px 10px;
  font-size: 10px;
  color: var(--text-3);
  border-top: 1px solid var(--border);
  margin-top: 4px;
}
.cli-input-row {
  display: flex; align-items: center; gap: 12px;
  padding: 12px 18px;
  border-top: 1px solid var(--border);
  flex-shrink: 0;
  background: var(--bg-panel);
}
.cli-input {
  flex: 1;
  background: transparent;
  border: none;
  outline: none;
  color: var(--text-1);
  font-size: 13.5px;
  user-select: text;
  caret-color: var(--accent);
}
.cli-input::placeholder { color: var(--text-3); }

/* MONITOR */
.mon-filter-row {
  display: flex; align-items: center; gap: 10px;
  padding: 8px 16px;
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
}
.mon-status {
  font-size: 11px; font-weight: 600;
  color: var(--text-3);
  display: inline-flex; align-items: center; gap: 5px;
}
.mon-status::before {
  content: ''; width: 7px; height: 7px; border-radius: 50%;
  background: var(--text-3);
}
.mon-status.on { color: var(--success); }
.mon-status.on::before {
  background: var(--success);
  box-shadow: 0 0 6px rgba(63, 157, 99, 0.7);
  animation: mon-blink 1.6s ease-in-out infinite;
}
@keyframes mon-blink { 0%, 100% { opacity: 1; } 50% { opacity: 0.35; } }
.mon-filter {
  flex: 1;
  background: var(--bg-inset);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  outline: none;
  color: var(--text-1);
  font-size: 12px;
  padding: 4px 10px;
  transition: border-color 0.15s;
}
.mon-filter:focus { border-color: var(--accent); }
.mon-output { padding: 10px 18px; }
.mon-line {
  font-size: 12px;
  line-height: 1.8;
  color: var(--text-2);
  white-space: pre-wrap;
  word-break: break-all;
  border-bottom: 1px dashed var(--border);
  padding: 1px 0;
}
.mon-line:hover { background: var(--bg-hover); }
.mon-paused {
  margin-top: 10px;
  font-size: 11px;
  color: var(--warning, #d9a03f);
}
.mon-footer {
  display: flex; justify-content: center;
  padding: 10px;
  border-top: 1px solid var(--border);
  flex-shrink: 0;
  background: var(--bg-panel);
}
</style>
