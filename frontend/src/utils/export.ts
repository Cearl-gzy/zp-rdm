import { message } from 'ant-design-vue'
import { PickSaveFile, SaveTextFile } from '../../wailsjs/go/main/App'

// 分析报告导出：前端把当前视图数据序列化为 JSON / 分区 CSV，经保存对话框落盘。

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

// exportReport 弹保存对话框并把内容写盘；用户取消（空路径）时静默返回
export async function exportReport(defaultName: string, content: string, doneText: string): Promise<void> {
  const path = await PickSaveFile(doneText, defaultName)
  if (!path) return
  try {
    await SaveTextFile(path, content)
    message.success(doneText + ': ' + path)
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  }
}
