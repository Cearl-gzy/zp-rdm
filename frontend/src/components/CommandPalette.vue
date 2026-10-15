<script setup lang="ts">
import { ref, computed, watch, nextTick, onBeforeUnmount } from 'vue'
import { message } from 'ant-design-vue'
import {
  SearchOutlined, DatabaseOutlined, PlusOutlined, SwapOutlined,
  ImportOutlined, ExportOutlined, CloseOutlined, LinkOutlined,
} from '@ant-design/icons-vue'
import { useAppStore } from '../stores/app'
import { useConnectionStore } from '../stores/connection'
import { emitAppEvent, AppEvents } from '../utils/events'

interface PaletteItem {
  id: string
  group: string
  title: string
  sub?: string
  icon: any
  kbd?: string
  run: () => void | Promise<void>
}

const app = useAppStore()
const conn = useConnectionStore()

const keyword = ref('')
const activeIndex = ref(0)
const inputRef = ref()

// 子序列模糊匹配，返回匹配分数（越小越好），不匹配返回 -1
function fuzzyScore(text: string, query: string): number {
  const t = text.toLowerCase()
  const q = query.toLowerCase()
  if (!q) return 0
  let ti = 0
  let score = 0
  let lastHit = -2
  for (const ch of q) {
    const i = t.indexOf(ch, ti)
    if (i < 0) return -1
    score += i - ti
    if (i !== lastHit + 1) score += 2 // 不连续惩罚
    lastHit = i
    ti = i + 1
  }
  return score
}

const allItems = computed<PaletteItem[]>(() => {
  const items: PaletteItem[] = []

  for (const tab of conn.openTabs) {
    const prof = conn.profiles.find(p => p.id === tab.id)
    items.push({
      id: `tab:${tab.id}`,
      group: '切换标签',
      title: tab.state.name,
      sub: prof ? `${prof.host}:${prof.port} · 已打开` : '已打开',
      icon: LinkOutlined,
      run: () => { conn.activeTab = tab.id },
    })
  }
  for (const p of conn.profiles) {
    if (conn.openIds.has(p.id)) continue
    items.push({
      id: `conn:${p.id}`,
      group: '打开连接',
      title: p.name || `${p.host}:${p.port}`,
      sub: `${p.host}:${p.port}${p.ssh?.enabled ? ' · ssh' : ''}`,
      icon: DatabaseOutlined,
      run: async () => {
        try { await conn.open(p.id) } catch (e: any) { message.error(String(e?.message ?? e)) }
      },
    })
  }

  const actions: PaletteItem[] = [
    { id: 'act:new', group: '操作', title: '新建连接', icon: PlusOutlined, kbd: 'Ctrl N', run: () => emitAppEvent(AppEvents.NewConnection) },
    { id: 'act:import', group: '操作', title: '导入连接', icon: ImportOutlined, run: () => emitAppEvent(AppEvents.ImportConnections) },
    { id: 'act:export', group: '操作', title: '导出连接', icon: ExportOutlined, run: () => emitAppEvent(AppEvents.ExportConnections) },
    { id: 'act:theme', group: '操作', title: app.dark ? '切换为浅色主题' : '切换为深色主题', icon: SwapOutlined, run: () => app.toggleTheme() },
  ]
  if (conn.activeTab) {
    actions.push({
      id: 'act:close', group: '操作', title: '关闭当前标签', icon: CloseOutlined, kbd: 'Ctrl W',
      run: () => { if (conn.activeTab) conn.close(conn.activeTab) },
    })
  }
  return [...items, ...actions]
})

const filtered = computed<PaletteItem[]>(() => {
  const kw = keyword.value.trim()
  if (!kw) return allItems.value
  return allItems.value
    .map(it => {
      const s1 = fuzzyScore(it.title, kw)
      const s2 = it.sub ? fuzzyScore(it.sub, kw) : -1
      const s = s1 < 0 ? s2 : s2 < 0 ? s1 + 2 : Math.min(s1 + 2, s2 + 4)
      return { it, s }
    })
    .filter(x => x.s >= 0)
    .sort((a, b) => a.s - b.s)
    .map(x => x.it)
})

// 分组展示（保持 filtered 顺序）
const groupedList = computed(() => {
  const out: { group: string; items: PaletteItem[] }[] = []
  for (const it of filtered.value) {
    const g = out.find(o => o.group === it.group)
    if (g) g.items.push(it)
    else out.push({ group: it.group, items: [it] })
  }
  return out
})

watch(() => app.paletteOpen, async v => {
  if (v) {
    keyword.value = ''
    activeIndex.value = 0
    await nextTick()
    inputRef.value?.focus()
  }
})
watch(filtered, () => {
  if (activeIndex.value >= filtered.value.length) activeIndex.value = 0
})

function close() { app.togglePalette(false) }

async function execute(item: PaletteItem) {
  close()
  await item.run()
}

function onKeydown(e: KeyboardEvent) {
  const n = filtered.value.length
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    activeIndex.value = n ? (activeIndex.value + 1) % n : 0
    scrollActiveIntoView()
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    activeIndex.value = n ? (activeIndex.value - 1 + n) % n : 0
    scrollActiveIntoView()
  } else if (e.key === 'Enter') {
    e.preventDefault()
    const it = filtered.value[activeIndex.value]
    if (it) execute(it)
  } else if (e.key === 'Escape') {
    e.preventDefault()
    close()
  }
}

const listRef = ref<HTMLElement>()
function scrollActiveIntoView() {
  nextTick(() => {
    listRef.value?.querySelector('.cp-item.active')?.scrollIntoView({ block: 'nearest' })
  })
}

// 面板打开时全局键（输入框外的 Esc 等）
function globalKey(e: KeyboardEvent) {
  if (e.key === 'Escape') { e.preventDefault(); close() }
}
watch(() => app.paletteOpen, v => {
  if (v) window.addEventListener('keydown', globalKey, true)
  else window.removeEventListener('keydown', globalKey, true)
})
onBeforeUnmount(() => window.removeEventListener('keydown', globalKey, true))

function itemIndex(item: PaletteItem): number {
  return filtered.value.indexOf(item)
}
</script>

<template>
  <Teleport to="body">
    <transition name="cp">
      <div v-if="app.paletteOpen" class="cp-backdrop" @mousedown.self="close">
        <div class="cp-panel">
          <div class="cp-input-row">
            <SearchOutlined class="cp-search-icon" />
            <input
              ref="inputRef"
              v-model="keyword"
              class="cp-input"
              placeholder="搜索连接、标签页或操作…"
              spellcheck="false"
              @keydown="onKeydown"
            />
            <span class="kbd">esc</span>
          </div>

          <div ref="listRef" class="cp-list">
            <template v-if="filtered.length">
              <div v-for="g in groupedList" :key="g.group" class="cp-group">
                <div class="cp-group-label">{{ g.group }}</div>
                <div
                  v-for="item in g.items"
                  :key="item.id"
                  class="cp-item"
                  :class="{ active: itemIndex(item) === activeIndex }"
                  @mouseenter="activeIndex = itemIndex(item)"
                  @click="execute(item)"
                >
                  <span class="cp-item-icon"><component :is="item.icon" /></span>
                  <span class="cp-item-title">{{ item.title }}</span>
                  <span v-if="item.sub" class="cp-item-sub num">{{ item.sub }}</span>
                  <span v-if="item.kbd" class="kbd">{{ item.kbd }}</span>
                </div>
              </div>
            </template>
            <div v-else class="cp-empty">没有匹配的结果</div>
          </div>

          <div class="cp-footer">
            <span class="cp-hint"><span class="kbd">↑↓</span> 导航</span>
            <span class="cp-hint"><span class="kbd">↵</span> 执行</span>
            <span class="cp-hint"><span class="kbd">esc</span> 关闭</span>
          </div>
        </div>
      </div>
    </transition>
  </Teleport>
</template>

<style scoped>
.cp-backdrop {
  position: fixed; inset: 0; z-index: 1000;
  background: rgba(4, 6, 10, 0.45);
  backdrop-filter: blur(3px);
  display: flex; justify-content: center;
  padding-top: 14vh;
}
html.light .cp-backdrop { background: rgba(28, 34, 48, 0.25); }

.cp-panel {
  width: 520px; max-width: calc(100vw - 48px);
  height: fit-content;
  max-height: 60vh;
  display: flex; flex-direction: column;
  background: var(--bg-elevated);
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-pop);
  overflow: hidden;
}

.cp-input-row {
  display: flex; align-items: center; gap: 10px;
  padding: 13px 16px;
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
}
.cp-search-icon { color: var(--text-3); font-size: 14px; }
.cp-input {
  flex: 1; min-width: 0;
  border: none; outline: none; background: transparent;
  color: var(--text-1);
  font-size: 14px;
}
.cp-input::placeholder { color: var(--text-3); }

.cp-list {
  flex: 1; overflow-y: auto;
  padding: 6px;
}
.cp-group-label {
  font-size: 10px; font-weight: 600;
  letter-spacing: 0.8px; text-transform: uppercase;
  color: var(--text-3);
  padding: 8px 10px 4px;
}
.cp-item {
  display: flex; align-items: center; gap: 10px;
  padding: 8px 10px;
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: background 0.08s;
}
.cp-item.active { background: var(--accent-soft); }
.cp-item-icon {
  width: 24px; height: 24px;
  display: inline-flex; align-items: center; justify-content: center;
  border-radius: var(--radius-sm);
  background: var(--bg-hover);
  color: var(--text-2);
  font-size: 12px;
  flex-shrink: 0;
}
.cp-item.active .cp-item-icon { color: var(--accent); background: rgba(224, 69, 54, 0.12); }
.cp-item-title { font-size: 13px; font-weight: 500; }
.cp-item-sub {
  flex: 1; min-width: 0;
  font-size: 11px; color: var(--text-3);
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  text-align: right;
}
.cp-item.active .cp-item-sub { color: var(--text-2); }

.cp-empty {
  padding: 36px 0;
  text-align: center;
  color: var(--text-3);
  font-size: 12.5px;
}

.cp-footer {
  display: flex; gap: 16px;
  padding: 8px 16px;
  border-top: 1px solid var(--border);
  flex-shrink: 0;
}
.cp-hint {
  display: inline-flex; align-items: center; gap: 5px;
  font-size: 10.5px; color: var(--text-3);
}

/* 过渡动画 */
.cp-enter-active, .cp-leave-active { transition: opacity 0.16s var(--ease); }
.cp-enter-active .cp-panel, .cp-leave-active .cp-panel { transition: transform 0.16s var(--ease), opacity 0.16s var(--ease); }
.cp-enter-from, .cp-leave-to { opacity: 0; }
.cp-enter-from .cp-panel { transform: translateY(-8px) scale(0.98); opacity: 0; }
.cp-leave-to .cp-panel { transform: translateY(-6px) scale(0.99); opacity: 0; }
</style>
