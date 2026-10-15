import { defineStore } from 'pinia'
import { ref, watch } from 'vue'
import { GetSettings, SaveFrontendState, SaveCliHistory, Version } from '../../wailsjs/go/main/App'

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

  // 命令面板（Ctrl+K）
  const paletteOpen = ref(false)
  function togglePalette(v?: boolean) {
    paletteOpen.value = v ?? !paletteOpen.value
  }

  return {
    dark, toggleTheme,
    sidebarWidth, setSidebarWidth,
    lastDb, setLastDb,
    version, initSettings,
    paletteOpen, togglePalette,
    cliHistory, pushCliHistory,
  }
})
