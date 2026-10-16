/**
 * 轻量全局事件总线（基于 window CustomEvent）
 * 用于跨组件触发动作：快捷键 / 命令面板 → Sidebar、KeyBrowser 等
 */
export const AppEvents = {
  NewConnection: 'zp:new-connection',
  ImportConnections: 'zp:import-connections',
  ExportConnections: 'zp:export-connections',
  Refresh: 'zp:refresh',
  // detail: { connId: string; key: string } — 搜索面板等跳转数据视图打开指定键
  OpenKey: 'zp:open-key',
} as const

export function emitAppEvent(name: string, detail?: unknown) {
  window.dispatchEvent(new CustomEvent(name, { detail }))
}

export function onAppEvent<T = unknown>(name: string, handler: (detail: T) => void) {
  const fn = (e: Event) => handler((e as CustomEvent).detail as T)
  window.addEventListener(name, fn)
  return () => window.removeEventListener(name, fn)
}
