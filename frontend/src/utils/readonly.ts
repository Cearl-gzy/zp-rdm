import { computed, type ComputedRef } from 'vue'
import { useConnectionStore } from '../stores/connection'

// 当前连接是否只读（来自 ConnState.readonly）。
// 后端 guard hook 是最终拦截层；这里只做 UI 层的按钮禁用提示。
export function useReadonly(connId: () => string): ComputedRef<boolean> {
  const conn = useConnectionStore()
  return computed(() => !!conn.openTabs.find(t => t.id === connId())?.state?.readonly)
}
