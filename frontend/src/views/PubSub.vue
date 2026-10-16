<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, nextTick, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { message } from 'ant-design-vue'
import { PlusOutlined, DeleteOutlined, SendOutlined, ClearOutlined } from '@ant-design/icons-vue'
import { Subscribe, Unsubscribe, UnsubscribeAll, Publish, PubSubChannels } from '../../wailsjs/go/service/ServerService'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import type { PubSubMessage } from '../types'

const props = defineProps<{ connId: string; db: number }>()

const { t } = useI18n()

const eventName = computed(() => `pubsub:${props.connId}:${props.db}`)

const channel = ref('')
const isPattern = ref(false)
const subscribed = ref<{ channel: string; pattern: boolean }[]>([])
const activeChannels = ref<string[]>([])

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
  try {
    activeChannels.value = ((await PubSubChannels(props.connId, props.db, '')) as unknown as string[]) || []
  } catch { /* 忽略 */ }
}

onMounted(() => {
  EventsOn(eventName.value, async (msg: PubSubMessage) => {
    messages.value.push(msg)
    if (messages.value.length > 500) messages.value.shift()
    await nextTick()
    logRef.value?.scrollTo({ top: logRef.value.scrollHeight })
  })
  loadActive()
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
  <div class="pubsub">
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
          {{ t('pubsub.publishMessage') }}
          <a-button size="small" type="link" style="margin-left: auto" @click="loadActive">{{ t('pubsub.activeChannels') }}</a-button>
        </div>
        <div class="active-channels" v-if="activeChannels.length">
          <a-tag
            v-for="c in activeChannels"
            :key="c"
            class="mono active-ch"
            @click="pubChannel = c"
          >{{ c }}</a-tag>
        </div>
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
</template>

<style scoped>
.pubsub { height: 100%; display: flex; gap: 10px; padding: 12px 18px; }
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
.active-channels { margin-bottom: 10px; display: flex; flex-wrap: wrap; gap: 4px; }
.active-ch { cursor: pointer; transition: border-color 0.12s, color 0.12s; }
.active-ch:hover { border-color: var(--accent) !important; color: var(--accent) !important; }
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
</style>
