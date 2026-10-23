import { message } from 'ant-design-vue'
import { PickSaveFile, SaveTextFile } from '../../wailsjs/go/main/App'

// 分析报告导出：前端把当前视图数据序列化为 JSON / 分区 CSV，经保存对话框落盘。
// CSV 序列化纯函数在 ./csv（可单测），这里 re-export 保持既有引用不变
export { csvCell, csvSection } from './csv'

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
