<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, nextTick, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { message } from 'ant-design-vue'
import {
  PlusOutlined, DeleteOutlined, SendOutlined, ClearOutlined, ReloadOutlined,
  PauseOutlined, CaretRightOutlined, BellOutlined,
} from '@ant-design/icons-vue'
import {
  Subscribe, Unsubscribe, UnsubscribeAll, Publish, PubSubInsight, GetConfig, SetConfig,
} from '../../wailsjs/go/service/ServerService'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import type { PubSubMessage, PubSubInsight as Insight } from '../types'

const props = defineProps<{ connId: string; db: number }>()

const { t } = useI18n()

const eventName = computed(() => `pubsub:${props.connId}:${props.db}`)

// 模式：频道消息 / 键空间事件（__keyevent@db__:* 本质是 PSUBSCRIBE）
const mode = ref<'channel' | 'keyspace'>('channel')

const channel = ref('')
const isPattern = ref(false)
const subscribed = ref<{ channel: string; pattern: boolean }[]>([])
const insight = ref<Insight>({ channels: [], shard: [] })
const insightLoading = ref(false)

const pubChannel = ref('')
const pubMessage = ref('')

const messages = ref<PubSubMessage[]>([])
const logRef = ref<HTMLElement>()

async function subscribe() {
  const ch = channel.value.trim()
  if (!ch) return
  try {
    await Subscribe(props.connId, props.db, ch, isPattern.value)
    subscribed.value.push({ channel: ch, pattern: isPattern.value })
    channel.value = ''
    message.success(t('pubsub.subscribed'))
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  }
}

async function unsubscribe(ch: string) {
  try {
    await Unsubscribe(props.connId, props.db, ch)
    subscribed.value = subscribed.value.filter(s => s.channel !== ch)
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  }
}

async function publish() {
  if (!pubChannel.value) { message.warning(t('pubsub.channelRequired')); return }
  try {
    const n = await Publish(props.connId, props.db, pubChannel.value, pubMessage.value)
    message.success(t('pubsub.publishedTo', { n }))
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  }
}

async function loadActive() {
  insightLoading.value = true
  try {
    insight.value = ((await PubSubInsight(props.connId, props.db, '')) as unknown as Insight) || { channels: [], shard: [] }
  } catch { /* 忽略 */ } finally {
    insightLoading.value = false
  }
}

// 点击洞察频道直接订阅（已订阅则忽略）
async function subscribeChannel(ch: string) {
  if (subscribed.value.some(s => s.channel === ch)) return
  try {
    await Subscribe(props.connId, props.db, ch, false)
    subscribed.value.push({ channel: ch, pattern: false })
    message.success(t('pubsub.subscribed'))
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  }
}

// ---------- 键空间事件 ----------
// notify-keyspace-events 标志位（A 为 g$lshzxetdnm 别名，展示时展开）
const KS_FLAG_DEFS = [
  { flag: 'K', label: 'K — keyspace' },
  { flag: 'E', label: 'E — keyevent' },
  { flag: 'g', label: 'g — generic' },
  { flag: '$', label: '$ — string' },
  { flag: 'l', label: 'l — list' },
  { flag: 's', label: 's — set' },
  { flag: 'h', label: 'h — hash' },
  { flag: 'z', label: 'z — zset' },
  { flag: 'x', label: 'x — expired' },
  { flag: 'e', label: 'e — evicted' },
  { flag: 't', label: 't — stream' },
  { flag: 'd', label: 'd — module' },
  { flag: 'n', label: 'n — new key' },
  { flag: 'm', label: 'm — key-miss' },
]
const KS_ALIAS_A = 'g$lshzxetd' // A 不含 n（new key）与 m（key-miss）

const ksSet = ref<Set<string>>(new Set())
const ksConfigLoaded = ref(false)
const ksConfigReadable = ref(true)
const ksSaving = ref(false)
const ksSubscribed = ref(false)
const ksPaused = ref(false)
const ksFilter = ref('')
const ksTypeFilter = ref<string | undefined>(undefined)

interface KsEvent { time: number; key: string; event: string }
const ksEvents = ref<KsEvent[]>([])

const ksFlagsStr = computed(() =>
  KS_FLAG_DEFS.map(d => d.flag).filter(f => ksSet.value.has(f)).join('')
)
const ksPattern = computed(() => `__keyevent@${props.db}__:*`)

function ksSetFromFlags(flags: string) {
  const s = new Set<string>()
  for (const ch of flags) {
    if (ch === 'A') for (const a of KS_ALIAS_A) s.add(a)
    else s.add(ch)
  }
  ksSet.value = s
}

function applyKsPreset(flags: string) {
  ksSetFromFlags(flags)
  saveKsConfig()
}

async function loadKsConfig() {
  try {
    const cfg = await GetConfig(props.connId, props.db, 'notify-keyspace-events')
    ksConfigReadable.value = true
    ksSetFromFlags(cfg['notify-keyspace-events'] ?? '')
  } catch {
    ksConfigReadable.value = false // CONFIG 被禁用的托管实例：仍可手动订阅
  } finally {
    ksConfigLoaded.value = true
  }
}

async function saveKsConfig() {
  ksSaving.value = true
  try {
    await SetConfig(props.connId, props.db, 'notify-keyspace-events', ksFlagsStr.value)
    message.success(t('pubsub.ksSaved', { flags: ksFlagsStr.value || '∅' }))
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    ksSaving.value = false
  }
}

async function toggleKsSubscribe() {
  try {
    if (ksSubscribed.value) {
      await Unsubscribe(props.connId, props.db, ksPattern.value)
      ksSubscribed.value = false
    } else {
      await Subscribe(props.connId, props.db, ksPattern.value, true)
      ksSubscribed.value = true
      message.success(t('pubsub.subscribed'))
    }
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  }
}

// 切库时跟随重订阅（通知标志为服务端全局，无需重载）
watch(() => props.db, async (_newDb, oldDb) => {
  if (!ksSubscribed.value) return
  try {
    await Unsubscribe(props.connId, oldDb, `__keyevent@${oldDb}__:*`)
    await Subscribe(props.connId, props.db, ksPattern.value, true)
  } catch { /* 忽略 */ }
})

const ksTypes = computed(() => {
  const set = new Set<string>()
  for (const e of ksEvents.value) set.add(e.event)
  return [...set].sort()
})

const ksFiltered = computed(() => {
  const kw = ksFilter.value.trim().toLowerCase()
  return ksEvents.value.filter(e =>
    (!kw || e.key.toLowerCase().includes(kw)) &&
    (!ksTypeFilter.value || e.event === ksTypeFilter.value)
  )
})

function ksEventColor(ev: string): string {
  if (['expired', 'evicted', 'del', 'unlink'].includes(ev)) return 'red'
  if (ev.startsWith('expire') || ev === 'persist') return 'orange'
  if (['rename_from', 'rename_to', 'move_from', 'move_to', 'copy_to', 'restore'].includes(ev)) return 'purple'
  return 'blue'
}

onMounted(() => {
  EventsOn(eventName.value, async (msg: PubSubMessage) => {
    // 键空间事件与普通频道消息分流。
    // keyevent 频道：频道名带事件、payload 为键名；keyspace 频道：频道名带键名、payload 为事件
    if (msg.channel.startsWith('__keyevent@') || msg.channel.startsWith('__keyspace@')) {
      if (ksPaused.value) return
      const idx = msg.channel.indexOf('__', 2)
      const suffix = idx >= 0 ? msg.channel.slice(idx + 2) : msg.channel
      const isKeyevent = msg.channel.startsWith('__keyevent@')
      const key = isKeyevent ? msg.payload : suffix
      const ev = isKeyevent ? suffix : msg.payload
      ksEvents.value.push({ time: msg.time, key, event: ev })
      if (ksEvents.value.length > 500) ksEvents.value.shift()
      return
    }
    messages.value.push(msg)
    if (messages.value.length > 500) messages.value.shift()
    await nextTick()
    logRef.value?.scrollTo({ top: logRef.value.scrollHeight })
  })
  loadActive()
  loadKsConfig()
})

onBeforeUnmount(async () => {
  EventsOff(eventName.value)
  try { await UnsubscribeAll(props.connId, props.db) } catch { /* 忽略 */ }
})

function fmtMsgTime(ms: number): string {
  const d = new Date(ms)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
}
</script>

<template>
  <div class="pubsub-wrap">
    <div class="ps-modebar">
      <a-segmented
        v-model:value="mode"
        size="small"
        :options="[
          { value: 'channel', label: t('pubsub.modeChannel') },
          { value: 'keyspace', label: t('pubsub.modeKeyspace') },
        ]"
      />
      <span v-if="mode === 'keyspace'" class="text-3" style="font-size: 11px">
        {{ t('pubsub.ksPatternTip', { pattern: ksPattern }) }}
      </span>
    </div>

    <!-- ================= 频道消息 ================= -->
    <div v-show="mode === 'channel'" class="pubsub">
      <div class="ps-left">
        <div class="ps-section">
          <div class="ps-title">{{ t('pubsub.subscribeChannels') }}</div>
          <div class="sub-form">
            <a-input v-model:value="channel" size="small" :placeholder="t('pubsub.channelPlaceholder')" class="mono" @press-enter="subscribe" />
            <a-checkbox v-model:checked="isPattern">{{ t('pubsub.pattern') }}</a-checkbox>
            <a-button size="small" type="primary" @click="subscribe">
              <template #icon><PlusOutlined /></template>
            </a-button>
          </div>
          <div class="sub-list">
            <a-empty v-if="!subscribed.length" :description="t('pubsub.noSubscriptions')" :image-style="{ height: '36px' }" />
            <div v-for="s in subscribed" :key="s.channel" class="sub-item">
              <a-tag v-if="s.pattern" color="purple">P</a-tag>
              <span class="mono sub-ch">{{ s.channel }}</span>
              <a-button size="small" type="text" danger @click="unsubscribe(s.channel)">
                <template #icon><DeleteOutlined /></template>
              </a-button>
            </div>
          </div>
        </div>

        <div class="ps-section">
          <div class="ps-title">
            {{ t('pubsub.insight') }}
            <a-button size="small" type="text" style="margin-left: auto" :loading="insightLoading" @click="loadActive">
              <template #icon><ReloadOutlined /></template>
            </a-button>
          </div>
          <a-empty
            v-if="!insight.channels.length && !insight.shard.length"
            :description="t('pubsub.noActiveChannels')"
            :image-style="{ height: '36px' }"
          />
          <div class="ch-list">
            <div v-for="c in insight.channels" :key="c.name" class="ch-item">
              <span class="mono ch-name" :title="c.name">{{ c.name }}</span>
              <span class="ch-subs text-3">{{ t('pubsub.subs', { n: c.subs }) }}</span>
              <a-tooltip :title="t('pubsub.subAction')">
                <a-button size="small" type="text" @click="subscribeChannel(c.name)">
                  <template #icon><PlusOutlined /></template>
                </a-button>
              </a-tooltip>
              <a-tooltip :title="t('pubsub.pubAction')">
                <a-button size="small" type="text" @click="pubChannel = c.name">
                  <template #icon><SendOutlined /></template>
                </a-button>
              </a-tooltip>
            </div>
          </div>
          <template v-if="insight.shard.length">
            <div class="shard-title text-3">{{ t('pubsub.shard') }}</div>
            <div class="ch-list">
              <div v-for="c in insight.shard" :key="c.name" class="ch-item">
                <span class="mono ch-name" :title="c.name">{{ c.name }}</span>
                <span class="ch-subs text-3">{{ t('pubsub.subs', { n: c.subs }) }}</span>
              </div>
            </div>
            <div class="shard-tip text-3">{{ t('pubsub.shardTip') }}</div>
          </template>
        </div>

        <div class="ps-section">
          <div class="ps-title">{{ t('pubsub.publishMessage') }}</div>
          <a-input v-model:value="pubChannel" size="small" :placeholder="t('pubsub.publishChannelPlaceholder')" class="mono" style="margin-bottom: 8px" />
          <a-textarea v-model:value="pubMessage" :rows="4" :placeholder="t('pubsub.messagePlaceholder')" class="mono" />
          <a-button size="small" type="primary" block style="margin-top: 8px" @click="publish">
            <template #icon><SendOutlined /></template>
            {{ t('pubsub.publish') }}
          </a-button>
        </div>
      </div>

      <div class="ps-right">
        <div class="ps-title">
          {{ t('pubsub.messageLog', { n: messages.length }) }}
          <a-button size="small" type="text" style="margin-left: auto" @click="messages = []">
            <template #icon><ClearOutlined /></template>
            {{ t('pubsub.clear') }}
          </a-button>
        </div>
        <div ref="logRef" class="msg-log">
          <a-empty v-if="!messages.length" :description="t('pubsub.waitingForMessages')" style="margin-top: 80px" />
          <div v-for="(m, i) in messages" :key="i" class="msg-item term-line">
            <div class="msg-head">
              <span class="msg-time mono">{{ fmtMsgTime(m.time) }}</span>
              <a-tag :color="m.kind === 'pmessage' ? 'purple' : 'red'" class="mono">{{ m.channel }}</a-tag>
              <span v-if="m.pattern" class="text-3 mono" style="font-size: 10px">via {{ m.pattern }}</span>
            </div>
            <pre class="msg-payload mono">{{ m.payload }}</pre>
          </div>
        </div>
      </div>
    </div>

    <!-- ================= 键空间事件 ================= -->
    <div v-show="mode === 'keyspace'" class="pubsub">
      <div class="ps-left">
        <div class="ps-section">
          <div class="ps-title">{{ t('pubsub.ksNotifyConfig') }}</div>
          <a-alert
            v-if="ksConfigLoaded && !ksConfigReadable"
            type="warning"
            show-icon
            :message="t('pubsub.ksConfigUnreadable')"
            style="margin-bottom: 10px"
          />
          <template v-if="ksConfigReadable">
            <div class="ks-presets">
              <a-button size="small" @click="applyKsPreset('')">{{ t('pubsub.ksPresetOff') }}</a-button>
              <a-button size="small" @click="applyKsPreset('KEA')">KEA</a-button>
              <a-button size="small" @click="applyKsPreset('Ex')">Ex</a-button>
              <a-button size="small" @click="applyKsPreset('KEx')">KEx</a-button>
            </div>
            <div class="ks-flags">
              <a-checkbox
                v-for="d in KS_FLAG_DEFS"
                :key="d.flag"
                :checked="ksSet.has(d.flag)"
                class="mono ks-flag"
                @change="(e: any) => { const s = new Set(ksSet); e.target.checked ? s.add(d.flag) : s.delete(d.flag); ksSet = s }"
              >{{ d.label }}</a-checkbox>
            </div>
            <div class="ks-flags-str mono">{{ ksFlagsStr || t('pubsub.ksFlagsEmpty') }}</div>
            <a-button size="small" type="primary" block :loading="ksSaving" @click="saveKsConfig">
              {{ t('pubsub.ksSave') }}
            </a-button>
          </template>
        </div>

        <div class="ps-section">
          <div class="ps-title">{{ t('pubsub.ksSubscribe') }}</div>
          <div class="ks-sub-row">
            <span class="mono ks-pattern">{{ ksPattern }}</span>
            <a-button
              size="small"
              :type="ksSubscribed ? 'default' : 'primary'"
              :danger="ksSubscribed"
              @click="toggleKsSubscribe"
            >
              <template #icon><BellOutlined /></template>
              {{ ksSubscribed ? t('pubsub.ksUnsubscribe') : t('pubsub.ksSubscribeBtn') }}
            </a-button>
          </div>
          <div class="text-3" style="font-size: 10.5px; margin-top: 8px">{{ t('pubsub.ksSubTip') }}</div>
        </div>
      </div>

      <div class="ps-right">
        <div class="ps-title">
          {{ t('pubsub.ksEventLog', { n: ksFiltered.length }) }}
          <div class="ks-log-actions">
            <a-input v-model:value="ksFilter" size="small" allow-clear :placeholder="t('pubsub.ksFilterKey')" style="width: 130px" />
            <a-select
              v-model:value="ksTypeFilter"
              size="small"
              allow-clear
              :placeholder="t('pubsub.ksFilterType')"
              style="width: 110px"
              :options="ksTypes.map(x => ({ value: x, label: x }))"
            />
            <a-button size="small" type="text" @click="ksPaused = !ksPaused">
              <template #icon><component :is="ksPaused ? CaretRightOutlined : PauseOutlined" /></template>
            </a-button>
            <a-button size="small" type="text" @click="ksEvents = []">
              <template #icon><ClearOutlined /></template>
            </a-button>
          </div>
        </div>
        <div class="msg-log">
          <a-empty v-if="!ksFiltered.length" :description="t('pubsub.ksWaiting')" style="margin-top: 80px" />
          <div v-for="(e, i) in ksFiltered" :key="i" class="ks-item term-line">
            <span class="msg-time mono">{{ fmtMsgTime(e.time) }}</span>
            <a-tag :color="ksEventColor(e.event)" class="mono ks-ev">{{ e.event }}</a-tag>
            <span class="mono ks-key" :title="e.key">{{ e.key }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.pubsub-wrap { height: 100%; display: flex; flex-direction: column; padding: 12px 18px; gap: 10px; }
.ps-modebar { display: flex; align-items: center; gap: 12px; flex-shrink: 0; }
.pubsub { flex: 1; min-height: 0; display: flex; gap: 10px; }
.ps-left {
  width: 320px; flex-shrink: 0;
  display: flex; flex-direction: column; gap: 10px;
  overflow-y: auto;
}
.ps-section {
  background: var(--bg-panel);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  padding: 14px 16px;
}
.ps-title { font-size: 12.5px; font-weight: 600; margin-bottom: 12px; display: flex; align-items: center; }
.sub-form { display: flex; gap: 8px; align-items: center; margin-bottom: 10px; }
.sub-list { display: flex; flex-direction: column; gap: 4px; }
.sub-item {
  display: flex; align-items: center; gap: 6px;
  padding: 5px 9px;
  background: var(--bg-inset);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
}
.sub-ch { flex: 1; font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ch-list { display: flex; flex-direction: column; gap: 4px; }
.ch-item {
  display: flex; align-items: center; gap: 6px;
  padding: 4px 6px 4px 9px;
  background: var(--bg-inset);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
}
.ch-name { flex: 1; min-width: 0; font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ch-subs { font-size: 10.5px; flex-shrink: 0; }
.shard-title { font-size: 11px; font-weight: 600; margin: 10px 0 6px; }
.shard-tip { font-size: 10.5px; margin-top: 6px; }
.ps-right {
  flex: 1; min-width: 0;
  display: flex; flex-direction: column;
  background: var(--bg-panel);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  padding: 14px 16px;
}
.msg-log { flex: 1; overflow-y: auto; user-select: text; }
.msg-item {
  padding: 9px 4px;
  border-bottom: 1px solid var(--border);
}
.msg-item:last-child { border-bottom: none; }
.msg-head { display: flex; align-items: center; gap: 8px; margin-bottom: 5px; }
.msg-time { font-size: 10.5px; color: var(--text-3); }
.msg-payload {
  margin: 0;
  font-size: 12px;
  color: var(--text-2);
  white-space: pre-wrap;
  word-break: break-all;
  user-select: text;
  line-height: 1.6;
}

/* ---------- 键空间事件 ---------- */
.ks-presets { display: flex; gap: 6px; margin-bottom: 10px; flex-wrap: wrap; }
.ks-flags { display: grid; grid-template-columns: 1fr 1fr; gap: 2px 8px; margin-bottom: 8px; }
.ks-flag { font-size: 11px; margin-inline-start: 0 !important; }
.ks-flags-str {
  font-size: 12px;
  color: var(--accent);
  background: var(--bg-inset);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 5px 9px;
  margin-bottom: 10px;
  min-height: 28px;
}
.ks-sub-row { display: flex; align-items: center; gap: 8px; }
.ks-pattern { flex: 1; font-size: 11.5px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ks-log-actions { margin-left: auto; display: flex; align-items: center; gap: 6px; }
.ks-item {
  display: flex; align-items: center; gap: 8px;
  padding: 6px 4px;
  border-bottom: 1px solid var(--border);
}
.ks-item:last-child { border-bottom: none; }
.ks-ev { margin-inline-end: 0; min-width: 64px; text-align: center; }
.ks-key { font-size: 12px; color: var(--text-1); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
