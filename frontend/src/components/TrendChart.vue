<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  data: number[]
  color?: string
  height?: number
  format?: (v: number) => string
  max?: number
}>(), {
  color: '#e04536',
  height: 56,
})

const W = 220

const points = computed(() => {
  const d = props.data
  if (!d.length) return ''
  const max = props.max ?? Math.max(...d, 1)
  const min = Math.min(...d, 0)
  const range = max - min || 1
  const step = d.length > 1 ? W / (d.length - 1) : 0
  return d.map((v, i) => `${(i * step).toFixed(1)},${(props.height - 4 - ((v - min) / range) * (props.height - 10)).toFixed(1)}`).join(' ')
})

const areaPoints = computed(() => {
  if (!points.value) return ''
  return `0,${props.height} ${points.value} ${W},${props.height}`
})

const current = computed(() => props.data.length ? props.data[props.data.length - 1] : 0)
const peak = computed(() => props.data.length ? Math.max(...props.data) : 0)

const gid = Math.random().toString(36).slice(2, 8)
</script>

<template>
  <div class="trend">
    <div class="trend-values">
      <span class="trend-current mono">{{ format ? format(current) : current }}</span>
      <span class="trend-peak mono">峰值 {{ format ? format(peak) : peak }}</span>
    </div>
    <svg :viewBox="`0 0 ${W} ${height}`" preserveAspectRatio="none" class="trend-svg">
      <defs>
        <linearGradient :id="`tg-${gid}`" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" :stop-color="color" stop-opacity="0.35" />
          <stop offset="100%" :stop-color="color" stop-opacity="0.02" />
        </linearGradient>
      </defs>
      <polygon v-if="areaPoints" :points="areaPoints" :fill="`url(#tg-${gid})`" />
      <polyline
        v-if="points"
        :points="points"
        fill="none"
        :stroke="color"
        stroke-width="1.6"
        stroke-linejoin="round"
        stroke-linecap="round"
      />
    </svg>
  </div>
</template>

<style scoped>
.trend { position: relative; width: 100%; }
.trend-values {
  display: flex; justify-content: space-between; align-items: baseline;
  margin-bottom: 4px;
}
.trend-current { font-size: 18px; font-weight: 700; }
.trend-peak { font-size: 10px; color: var(--text-3); }
.trend-svg { width: 100%; display: block; }
</style>
