/**
 * 轻量全局事件总线（基于 window CustomEvent）
 * 用于跨组件触发动作：快捷键 / 命令面板 → Sidebar、KeyBrowser 等
 */
export const AppEvents = {
  NewConnection: 'zp:new-connection',
  ImportConnections: 'zp:import-connections',
  ExportConnections: 'zp:export-connections',
  Refresh: 'zp:refresh',
} as const

export function emitAppEvent(name: string) {
  window.dispatchEvent(new CustomEvent(name))
}

export function onAppEvent(name: string, handler: () => void) {
  const fn = () => handler()
  window.addEventListener(name, fn)
  return () => window.removeEventListener(name, fn)
}
