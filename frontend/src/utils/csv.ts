// CSV 序列化（纯函数，便于单测；导出走 utils/export.ts）

// csvCell CSV 单元格转义（含逗号/引号/换行时加引号，内嵌引号双写）
export function csvCell(v: string | number): string {
  const s = String(v)
  if (/[",\n\r]/.test(s)) return '"' + s.replace(/"/g, '""') + '"'
  return s
}

// csvSection 一个分区：标题行 + 表头 + 数据行
export function csvSection(title: string, headers: (string | number)[], rows: (string | number)[][]): string {
  const lines = [`# ${title}`, headers.map(csvCell).join(',')]
  for (const r of rows) lines.push(r.map(csvCell).join(','))
  return lines.join('\n')
}
