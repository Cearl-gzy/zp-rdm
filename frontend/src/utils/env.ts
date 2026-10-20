// 环境标签着色（prod/test/dev）与只读模式 UI 辅助
export const ENV_COLORS: Record<string, string> = {
  prod: '#e04536', // 生产：红
  test: '#f5a623', // 测试：橙
  dev: '#3fbf6b',  // 开发：绿
}

export function envColor(env?: string): string {
  return ENV_COLORS[env ?? ''] ?? ''
}

// 侧边栏/标签页的环境色条样式（无环境时透明）
export function envBarStyle(env?: string): Record<string, string> {
  const c = envColor(env)
  return c ? { background: c } : {}
}
