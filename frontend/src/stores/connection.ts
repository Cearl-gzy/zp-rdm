import { defineStore } from 'pinia'
import { ref, computed, watch } from 'vue'
import { message } from 'ant-design-vue'
import {
  ListConnections, SaveConnection, DeleteConnection, DuplicateConnection,
  OpenConnection, CloseConnection, TestConnection, OpenStates,
  ExportConnections, ImportConnections, SetConnectionGroup,
} from '../../wailsjs/go/service/ConnectionService'
import { useAppStore } from './app'
import type { ConnectionProfile, ConnState } from '../types'

export interface OpenTab {
  id: string
  state: ConnState
}

export const useConnectionStore = defineStore('connection', () => {
  const profiles = ref<ConnectionProfile[]>([])
  const openTabs = ref<OpenTab[]>([])
  const activeTab = ref<string>('')
  const opening = ref<string>('') // 正在打开的连接 id
  const currentDb = ref<Record<string, number>>({})

  const groups = computed(() => {
    const map = new Map<string, ConnectionProfile[]>()
    for (const p of profiles.value) {
      const g = p.group || '默认分组'
      if (!map.has(g)) map.set(g, [])
      map.get(g)!.push(p)
    }
    return [...map.entries()].map(([name, items]) => ({ name, items }))
  })

  // 已使用的分组名（空分组=默认分组，不列出；供表单补全 / 移动分组菜单）
  const groupNames = computed(() => {
    const set = new Set<string>()
    for (const p of profiles.value) if (p.group) set.add(p.group)
    return [...set].sort((a, b) => a.localeCompare(b))
  })

  const openIds = computed(() => new Set(openTabs.value.map(t => t.id)))

  async function loadProfiles() {
    profiles.value = (await ListConnections()) as unknown as ConnectionProfile[]
    // 恢复已打开状态（应用重启后为空，这里保持一致性）
    const states = (await OpenStates()) as unknown as ConnState[]
    for (const st of states || []) {
      if (!openIds.value.has(st.id)) openTabs.value.push({ id: st.id, state: st })
    }
  }

  async function save(p: ConnectionProfile) {
    const saved = (await SaveConnection(p as any)) as unknown as ConnectionProfile
    // 后端会关闭已打开的连接，前端同步关闭标签页避免状态不一致
    if (openIds.value.has(saved.id)) closeTabLocal(saved.id)
    await loadProfiles()
    return saved
  }

  async function remove(id: string) {
    await DeleteConnection(id)
    closeTabLocal(id)
    await loadProfiles()
  }

  async function duplicate(id: string) {
    await DuplicateConnection(id)
    await loadProfiles()
  }

  async function test(p: ConnectionProfile) {
    await TestConnection(p as any)
  }

  async function open(id: string) {
    if (openIds.value.has(id)) { activeTab.value = id; return }
    opening.value = id
    try {
      const st = (await OpenConnection(id)) as unknown as ConnState
      openTabs.value.push({ id, state: st })
      activeTab.value = id
      if (currentDb.value[id] === undefined) {
        // 优先恢复上次使用的 db，其次用连接配置的默认 db
        const app = useAppStore()
        const prof = profiles.value.find(p => p.id === id)
        currentDb.value[id] = app.lastDb[id] ?? prof?.db ?? 0
      }
    } finally {
      opening.value = ''
    }
  }

  async function close(id: string) {
    await CloseConnection(id)
    closeTabLocal(id)
  }

  function closeTabLocal(id: string) {
    const idx = openTabs.value.findIndex(t => t.id === id)
    if (idx >= 0) {
      openTabs.value.splice(idx, 1)
      if (activeTab.value === id) {
        activeTab.value = openTabs.value[Math.min(idx, openTabs.value.length - 1)]?.id ?? ''
      }
    }
  }

  function setDb(id: string, db: number) {
    currentDb.value[id] = db
    useAppStore().setLastDb(id, db)
  }

  // ---------- 工作区会话 ----------
  // 标签增删 / 激活切换 → 持久化到 settings
  watch(() => openTabs.value.map(t => t.id).join(',') + '|' + activeTab.value, () => {
    useAppStore().setOpenTabs(openTabs.value.map(t => t.id), activeTab.value)
  })

  // 启动时恢复上次打开的连接标签（逐个打开，失败的跳过并提示）
  async function restoreWorkspace() {
    const w = useAppStore().workspace
    if (!w.open.length) return
    const failed: string[] = []
    for (const id of w.open) {
      const prof = profiles.value.find(p => p.id === id)
      if (!prof || openIds.value.has(id)) continue
      try {
        await open(id)
      } catch {
        failed.push(prof.name || id)
      }
    }
    if (w.active && openIds.value.has(w.active)) activeTab.value = w.active
    if (failed.length) message.warning(`连接失败，已跳过：${failed.join('、')}`)
  }

  // 仅更新分组（后端不关闭已打开的连接），本地同步 profiles
  async function setGroup(id: string, group: string) {
    await SetConnectionGroup(id, group)
    const p = profiles.value.find(x => x.id === id)
    if (p) p.group = group
  }

  // 分组重命名：该组（含默认分组，即 group 为空）下所有连接改到新分组
  async function renameGroup(oldName: string, newName: string) {
    const targets = profiles.value.filter(p => (p.group || '默认分组') === oldName)
    for (const p of targets) await SetConnectionGroup(p.id, newName)
    await loadProfiles()
  }

  async function exportTo(path: string) { await ExportConnections(path) }
  async function importFrom(path: string) {
    const n = await ImportConnections(path)
    await loadProfiles()
    return n
  }

  return {
    profiles, openTabs, activeTab, opening, currentDb,
    groups, groupNames, openIds,
    loadProfiles, save, remove, duplicate, test, open, close, setDb, restoreWorkspace,
    setGroup, renameGroup, exportTo, importFrom,
  }
})
