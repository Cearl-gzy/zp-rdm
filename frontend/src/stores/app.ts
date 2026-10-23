import { defineStore } from 'pinia'
import { ref, watch } from 'vue'
import { GetSettings, SaveFrontendState, SaveCliHistory, SaveCliFavorites, SaveWorkspace, Version } from '../../wailsjs/go/main/App'

export interface ConnWorkspace {
  view?: string
  selectedKey?: string
  pattern?: string
  viewMode?: string
  delimiter?: string
  expandedKeys?: string[]
}

export interface Workspace {
  open: string[]
  active: string
  conns: Record<string, ConnWorkspace>
}

const emptyWorkspace = (): Workspace => ({ open: [], active: '', conns: {} })

export const useAppStore = defineStore('app', () => {
  // localStorage 作为首屏防闪烁的即时提示，后端设置加载后以后端为准
  const dark = ref(localStorage.getItem('zp-rdm-theme') !== 'light')
  const sidebarWidth = ref(236)
  const lastDb = ref<Record<string, number>>({})
  const version = ref('')
  const settingsReady = ref(false)

  watch(dark, v => {
    localStorage.setItem('zp-rdm-theme', v ? 'dark' : 'light')
    document.documentElement.classList.toggle('light', !v)
  }, { immediate: true })

  function toggleTheme() { dark.value = !dark.value; persist() }

  function setSidebarWidth(w: number) {
    sidebarWidth.value = Math.min(480, Math.max(160, Math.round(w)))
    persist()
  }

  function setLastDb(connId: string, db: number) {
    lastDb.value = { ...lastDb.value, [connId]: db }
    persist()
  }

  // 防抖落盘
  let timer: ReturnType<typeof setTimeout> | null = null
  function persist() {
    if (!settingsReady.value) return
    if (timer) clearTimeout(timer)
    timer = setTimeout(() => {
      SaveFrontendState(dark.value ? 'dark' : 'light', sidebarWidth.value, { ...lastDb.value })
        .catch(() => {})
    }, 600)
  }

  async function initSettings() {
    try {
      const s = await GetSettings()
      if (s.theme === 'light') dark.value = false
      else if (s.theme === 'dark') dark.value = true
      sidebarWidth.value = s.sidebarWidth || 236
      lastDb.value = (s.lastDb as Record<string, number>) ?? {}
      cliHistory.value = ((s.cliHistory as string[]) ?? []).slice(-200)
      cliFavorites.value = ((s.cliFavorites as string[]) ?? []).slice(0, 100)
      const w = s.workspace
      if (w) {
        workspace.value = {
          open: w.open ?? [],
          active: w.active ?? '',
          conns: (w.conns ?? {}) as Record<string, ConnWorkspace>,
        }
      }
    } catch { /* 设置读取失败时使用默认值 */ }
    settingsReady.value = true
    try { version.value = await Version() } catch { /* 忽略 */ }
  }

  // CLI 命令历史（全局共享，上限 200，防抖落盘）
  const cliHistory = ref<string[]>([])
  let hTimer: ReturnType<typeof setTimeout> | null = null
  function pushCliHistory(cmd: string) {
    const h = cliHistory.value
    if (h[h.length - 1] !== cmd) h.push(cmd) // 连续重复不记录
    if (h.length > 200) h.splice(0, h.length - 200)
    if (!settingsReady.value) return
    if (hTimer) clearTimeout(hTimer)
    hTimer = setTimeout(() => {
      SaveCliHistory([...cliHistory.value]).catch(() => {})
    }, 800)
  }

  // CLI 收藏命令（全局共享，上限 100，防抖落盘）
  const cliFavorites = ref<string[]>([])
  let fTimer: ReturnType<typeof setTimeout> | null = null
  function persistFavorites() {
    if (!settingsReady.value) return
    if (fTimer) clearTimeout(fTimer)
    fTimer = setTimeout(() => {
      SaveCliFavorites([...cliFavorites.value]).catch(() => {})
    }, 600)
  }
  function toggleCliFavorite(cmd: string) {
    const c = cmd.trim()
    if (!c) return
    const i = cliFavorites.value.indexOf(c)
    if (i >= 0) cliFavorites.value.splice(i, 1)
    else cliFavorites.value.push(c)
    persistFavorites()
  }
  function removeCliFavorite(cmd: string) {
    const i = cliFavorites.value.indexOf(cmd)
    if (i >= 0) { cliFavorites.value.splice(i, 1); persistFavorites() }
  }

  // 工作区会话（打开的连接标签 + 各连接视图状态，防抖落盘）
  const workspace = ref<Workspace>(emptyWorkspace())
  let wTimer: ReturnType<typeof setTimeout> | null = null
  function persistWorkspace() {
    if (!settingsReady.value) return
    if (wTimer) clearTimeout(wTimer)
    wTimer = setTimeout(() => {
      SaveWorkspace(JSON.parse(JSON.stringify(workspace.value))).catch(() => {})
    }, 600)
  }
  function setOpenTabs(ids: string[], active: string) {
    workspace.value.open = ids
    workspace.value.active = active
    persistWorkspace()
  }
  function patchConnWorkspace(connId: string, patch: Partial<ConnWorkspace>) {
    workspace.value.conns[connId] = { ...workspace.value.conns[connId], ...patch }
    persistWorkspace()
  }

  // 命令面板（Ctrl+K）
  const paletteOpen = ref(false)
  function togglePalette(v?: boolean) {
    paletteOpen.value = v ?? !paletteOpen.value
  }

  // 侧栏分组折叠状态（纯 UI 状态，localStorage 持久化）
  const COLLAPSED_KEY = 'zp-rdm-collapsed-groups'
  const collapsedGroups = ref<Set<string>>(new Set(
    JSON.parse(localStorage.getItem(COLLAPSED_KEY) || '[]') as string[],
  ))
  function toggleGroupCollapsed(name: string) {
    const s = new Set(collapsedGroups.value)
    if (s.has(name)) s.delete(name)
    else s.add(name)
    collapsedGroups.value = s
    localStorage.setItem(COLLAPSED_KEY, JSON.stringify([...s]))
  }

  return {
    dark, toggleTheme,
    sidebarWidth, setSidebarWidth,
    lastDb, setLastDb,
    version, initSettings,
    paletteOpen, togglePalette,
    cliHistory, pushCliHistory,
    cliFavorites, toggleCliFavorite, removeCliFavorite,
    workspace, setOpenTabs, patchConnWorkspace,
    collapsedGroups, toggleGroupCollapsed,
  }
})
