<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { message } from 'ant-design-vue'
import { PlusOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons-vue'
import { BfInfo, BfAdd, BfMExists } from '../../../wailsjs/go/service/BfService'
import { useReadonly } from '../../utils/readonly'
import { formatBytes } from '../../utils/format'
import type { BfInfo as BfInfoT } from '../../types'

const props = defineProps<{ connId: string; db: number; keyName: string; total: number }>()

const { t } = useI18n()
const ro = useReadonly(() => props.connId)

// ---------- 概览（BF.INFO） ----------
const info = ref<BfInfoT | null>(null)

async function loadInfo() {
  try {
    info.value = (await BfInfo(props.connId, props.db, props.keyName)) as unknown as BfInfoT
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  }
}

onMounted(loadInfo)

// 填充率（已插入/容量；容量为 0 时不可用）
const fillPct = computed(() => {
  const i = info.value
  if (!i || i.capacity <= 0) return 0
  return Math.min(100, (i.inserted / i.capacity) * 100)
})
const fillClass = computed(() => (fillPct.value < 70 ? 'fill-good' : fillPct.value < 90 ? 'fill-mid' : 'fill-bad'))

// ---------- 添加元素（BF.ADD） ----------
const addItem = ref('')
const adding = ref(false)

async function doAdd() {
  const item = addItem.value.trim()
  if (!item) {
    message.warning(t('bloom.itemRequired'))
    return
  }
  adding.value = true
  try {
    const added = await BfAdd(props.connId, props.db, props.keyName, item)
    message.success(added ? t('bloom.added') : t('bloom.addedDup'))
    addItem.value = ''
    await loadInfo()
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    adding.value = false
  }
}

// ---------- 批量存在性检查（BF.MEXISTS） ----------
const checkInput = ref('')
const checking = ref(false)
const checkResults = ref<{ item: string; exists: boolean }[]>([])

async function doCheck() {
  const items = checkInput.value.split('\n').map(s => s.trim()).filter(Boolean)
  if (!items.length) {
    message.warning(t('bloom.checkRequired'))
    return
  }
  checking.value = true
  try {
    const res = await BfMExists(props.connId, props.db, props.keyName, items)
    checkResults.value = items.map((item, i) => ({ item, exists: !!res[i] }))
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    checking.value = false
  }
}
</script>

<template>
  <div class="bf-editor">
    <div class="bf-toolbar">
      <div class="bf-cards">
        <div class="bf-card">
          <div class="card-value num">{{ info?.inserted ?? '—' }}</div>
          <div class="card-label">{{ t('bloom.inserted') }}</div>
        </div>
        <div class="bf-card">
          <div class="card-value num">{{ info?.capacity || '—' }}</div>
          <div class="card-label">{{ t('bloom.capacity') }}</div>
        </div>
        <div class="bf-card">
          <div class="card-value num">{{ info ? formatBytes(info.size) : '—' }}</div>
          <div class="card-label">{{ t('bloom.sizeBytes') }}</div>
        </div>
        <div class="bf-card">
          <div class="card-value num">{{ info?.filters ?? '—' }}</div>
          <div class="card-label">{{ t('bloom.filters') }}</div>
        </div>
      </div>
      <a-tooltip :title="t('common.refresh')">
        <button class="icon-btn" @click="loadInfo"><ReloadOutlined /></button>
      </a-tooltip>
    </div>

    <div v-if="info && info.capacity > 0" class="bf-fill">
      <div class="fill-bar">
        <div class="fill-inner" :class="fillClass" :style="{ width: fillPct + '%' }" />
      </div>
      <span class="fill-text num">{{ fillPct.toFixed(1) }}%</span>
      <span class="text-3 fill-tip">{{ t('bloom.fillTip') }}</span>
    </div>

    <div class="bf-note">
      <span class="text-3">{{ t('bloom.semanticsTip') }}</span>
    </div>

    <div v-if="!ro" class="bf-add">
      <a-input
        v-model:value="addItem" size="small" class="mono"
        :placeholder="t('bloom.itemPlaceholder')" @press-enter="doAdd"
      />
      <a-button type="primary" size="small" :loading="adding" @click="doAdd">
        <template v-if="!adding" #icon><PlusOutlined /></template>
        {{ t('bloom.add') }}
      </a-button>
    </div>

    <div class="bf-check">
      <div class="check-title">{{ t('bloom.checkTitle') }}</div>
      <a-textarea
        v-model:value="checkInput" :rows="4" class="mono"
        :placeholder="t('bloom.checkPlaceholder')"
      />
      <a-button size="small" :loading="checking" style="margin-top: 6px" @click="doCheck">
        <template v-if="!checking" #icon><SearchOutlined /></template>
        {{ t('bloom.check') }}
      </a-button>
    </div>

    <div v-if="checkResults.length" class="bf-results">
      <div v-for="r in checkResults" :key="r.item" class="result-row">
        <span class="result-item mono">{{ r.item }}</span>
        <span class="result-chip" :class="r.exists ? 'chip-yes' : 'chip-no'">
          {{ r.exists ? t('bloom.maybeExists') : t('bloom.definitelyNot') }}
        </span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.bf-editor { display: flex; flex-direction: column; gap: 12px; padding: 12px; height: 100%; overflow-y: auto; }
.bf-toolbar { display: flex; align-items: flex-start; gap: 8px; }
.bf-cards { display: flex; gap: 8px; flex-wrap: wrap; flex: 1; }
.bf-card {
  min-width: 100px; padding: 8px 12px;
  border: 1px solid var(--border); border-radius: var(--radius-md);
}
.card-value { font-size: 16px; font-weight: 600; color: var(--text-1); }
.card-label { font-size: 10.5px; color: var(--text-3); margin-top: 2px; }

.icon-btn {
  display: inline-flex; align-items: center; justify-content: center;
  width: 26px; height: 26px; border: none; background: transparent;
  border-radius: var(--radius-sm); color: var(--text-3); cursor: pointer;
  font-size: 13px; transition: background 0.12s, color 0.12s; padding: 0;
}
.icon-btn:hover { background: var(--bg-hover); color: var(--text-1); }

.bf-fill { display: flex; align-items: center; gap: 8px; }
.fill-bar {
  width: 180px; height: 8px; border-radius: 4px;
  background: var(--bg-hover); overflow: hidden;
}
.fill-inner { height: 100%; border-radius: 4px; transition: width 0.2s; }
.fill-good { background: var(--success, #3f9d63); }
.fill-mid { background: #f5a623; }
.fill-bad { background: #ff6b60; }
.fill-text { font-size: 11.5px; font-weight: 600; }
.fill-tip, .bf-note { font-size: 11px; }

.bf-add { display: flex; gap: 8px; }
.bf-add .mono { flex: 1; }

.check-title { font-size: 12px; font-weight: 600; color: var(--text-2); margin-bottom: 6px; }

.bf-results { border: 1px solid var(--border); border-radius: var(--radius-md); overflow: hidden; }
.result-row {
  display: flex; align-items: center; justify-content: space-between; gap: 8px;
  padding: 5px 10px; font-size: 11.5px; border-top: 1px solid var(--border);
}
.result-row:first-child { border-top: none; }
.result-item { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.result-chip { flex-shrink: 0; font-size: 10.5px; padding: 1px 8px; border-radius: 8px; }
.chip-yes { color: #f5a623; background: rgba(245, 166, 35, 0.12); }
.chip-no { color: var(--success, #3f9d63); background: rgba(63, 157, 99, 0.12); }
</style>
