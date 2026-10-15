<script setup lang="ts">
import { ref, computed, nextTick, onMounted, onBeforeUnmount, watch } from 'vue'
import { message, Modal } from 'ant-design-vue'
import {
  ClearOutlined, EnterOutlined, PlayCircleOutlined, StopOutlined,
  PauseOutlined, CaretRightOutlined,
} from '@ant-design/icons-vue'
import { ExecCommand } from '../../wailsjs/go/service/CliService'
import { StartMonitor, StopMonitor } from '../../wailsjs/go/service/ServerService'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import { useAppStore } from '../stores/app'
import type { CliResult } from '../types'

const props = defineProps<{ connId: string; db: number }>()
const app = useAppStore()

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
const inputRef = ref<{ focus: () => void }>()
let seq = 0

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
  if (/^flushall\b/i.test(c)) return 'FLUSHALL 将清空整个实例的全部数据'
  if (/^flushdb\b/i.test(c)) return 'FLUSHDB 将清空当前数据库的全部数据'
  if (/^shutdown\b/i.test(c)) return 'SHUTDOWN 将关闭 Redis 服务器'
  if (/^debug\b/i.test(c)) return 'DEBUG 命令可能导致服务器崩溃或长时间阻塞'
  if (/^config\s+(set|resetstat|rewrite)\b/i.test(c)) return '该命令会修改服务器运行配置'
  if (/^keys\b/i.test(c)) return 'KEYS 会全量扫描键空间，生产环境可能长时间阻塞'
  return null
}

function exec() {
  const cmd = input.value.trim()
  if (!cmd || running.value) return
  const danger = dangerOf(cmd)
  if (danger) {
    Modal.confirm({
      title: '危险命令确认',
      content: `${danger}。确定执行「${cmd}」吗？`,
      okText: '仍要执行',
      okButtonProps: { danger: true },
      cancelText: '取消',
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
        :options="[{ value: 'cmd', label: '命令' }, { value: 'monitor', label: 'MONITOR' }]" />
      <span v-if="mode === 'cmd'" class="text-3" style="font-size: 11px">
        支持任意 Redis 命令，↑↓ 翻历史，危险命令需确认
      </span>
      <div class="toolbar-actions">
        <template v-if="mode === 'monitor' && monOn">
          <a-tooltip :title="monPaused ? '继续' : '暂停'">
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
          清屏
        </a-button>
      </div>
    </div>

    <!-- 命令模式 -->
    <template v-if="mode === 'cmd'">
      <div ref="outputRef" class="cli-output" @click.stop>
        <div class="cli-welcome mono">
          ZP Redis CLI · 已连接 db{{ db }} · 输入命令开始，例如 INFO server
        </div>
        <div v-for="e in entries" :key="e.id" class="cli-entry term-line">
          <div class="cli-cmd mono">
            <span class="prompt">db{{ db }}&gt;</span> {{ e.cmd }}
            <span class="cmd-time">{{ e.time }} · {{ e.result.costMs }}ms</span>
          </div>
          <pre class="cli-result mono" :class="resultClass(e.result)">{{ renderValue(e.result.value) }}</pre>
        </div>
      </div>

      <div class="cli-input-row">
        <span class="prompt mono">db{{ db }}&gt;</span>
        <input
          ref="inputRef"
          v-model="input"
          class="cli-input mono"
          placeholder="输入 Redis 命令…"
          spellcheck="false"
          @keydown="onKeydown"
          @keyup.enter="exec"
        />
        <a-button size="small" type="primary" :loading="running" @click="exec">
          <template #icon><EnterOutlined /></template>
        </a-button>
      </div>
    </template>

    <!-- MONITOR 模式 -->
    <template v-else>
      <div class="mon-filter-row">
        <span class="mon-status" :class="{ on: monOn }">{{ monOn ? '监听中' : '未启动' }}</span>
        <input
          v-model="monFilter"
          class="mon-filter mono"
          placeholder="过滤（如 GET、db0、IP）…"
          spellcheck="false"
          @click.stop
        />
        <span class="text-3" style="font-size: 11px">{{ filteredMonLines.length }} / {{ monLines.length }} 条</span>
      </div>
      <div ref="monOutputRef" class="cli-output mon-output" @click.stop>
        <div v-if="!monLines.length" class="cli-welcome mono">
          MONITOR 实时显示服务器执行的每条命令。注意：生产环境开启会有性能损耗。
        </div>
        <div v-for="(l, i) in filteredMonLines" :key="i" class="mon-line mono">{{ l }}</div>
        <div v-if="monPaused && monLines.length" class="mon-paused mono">已暂停（新命令仍在后台丢弃，点击继续恢复）</div>
      </div>
      <div class="mon-footer">
        <a-button v-if="!monOn" size="small" type="primary" @click="startMonitor">
          <template #icon><PlayCircleOutlined /></template>
          开始监听
        </a-button>
        <a-button v-else size="small" danger @click="stopMonitor">
          <template #icon><StopOutlined /></template>
          停止监听
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
