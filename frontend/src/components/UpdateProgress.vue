<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import { formatBytes } from '../utils/format'

// 更新下载进度（静态 Modal content 无 app 上下文，文案由父组件以 props 传入）
const props = defineProps<{
  verifyingText: string
}>()
const emit = defineEmits<{ done: [] }>()

const downloaded = ref(0)
const total = ref(-1)

const percent = computed(() => {
  if (total.value <= 0) return 0
  return Math.min(100, Math.round((downloaded.value / total.value) * 100))
})
const sizeText = computed(() => {
  if (total.value <= 0) return formatBytes(downloaded.value)
  return `${formatBytes(downloaded.value)} / ${formatBytes(total.value)}`
})

onMounted(() => {
  EventsOn('update:download', (p: { downloaded: number; total: number; done: boolean }) => {
    downloaded.value = p.downloaded
    total.value = p.total
    if (p.done) emit('done')
  })
})
onBeforeUnmount(() => EventsOff('update:download'))
</script>

<template>
  <div class="upd">
    <a-progress
      :percent="total > 0 ? percent : 100"
      status="active"
      :show-info="total > 0"
    />
    <div class="upd-size num">{{ sizeText }}</div>
    <div v-if="percent >= 100 && total > 0" class="upd-verifying">{{ props.verifyingText }}</div>
  </div>
</template>

<style scoped>
.upd { padding: 8px 0 4px; }
.upd-size { font-size: 11px; color: var(--text-3); margin-top: 4px; text-align: right; }
.upd-verifying { font-size: 11px; color: var(--text-2); margin-top: 6px; }
</style>
