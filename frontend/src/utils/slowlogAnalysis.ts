// 慢日志聚合分析（纯函数，供 SlowLog 分析模式与 vitest 使用）
import type { SlowLogEntry } from '../types'

// Redis 复合命令的一级词表：这些命令的命令名取前两个词（CONFIG GET、XINFO GROUPS …）
const TWO_WORD = new Set([
  'ACL', 'CLIENT', 'CLUSTER', 'COMMAND', 'CONFIG', 'DEBUG', 'FAILOVER', 'FUNCTION',
  'LATENCY', 'MEMORY', 'MODULE', 'OBJECT', 'PUBSUB', 'SCRIPT', 'SLOWLOG', 'XGROUP', 'XINFO',
])

// cmdName 提取命令名（大写）；复合命令取前两词
export function cmdName(args: string[]): string {
  if (!args.length) return '?'
  const first = args[0].toUpperCase()
  if (TWO_WORD.has(first) && args.length > 1) return `${first} ${args[1].toUpperCase()}`
  return first
}

export interface CmdStat {
  cmd: string
  count: number
  totalUs: number
  avgUs: number
  maxUs: number
  lastTs: number // 最近一次发生（秒级时间戳）
  share: number // 总耗时占比 0-1
}

// aggregate 按命令名聚合：次数 / 总耗时 / 平均 / 峰值 / 最近发生，按总耗时降序
export function aggregate(entries: SlowLogEntry[]): CmdStat[] {
  const map = new Map<string, CmdStat>()
  let grand = 0
  for (const e of entries) {
    const cmd = cmdName(e.args)
    grand += e.durationUs
    let row = map.get(cmd)
    if (!row) {
      row = { cmd, count: 0, totalUs: 0, avgUs: 0, maxUs: 0, lastTs: 0, share: 0 }
      map.set(cmd, row)
    }
    row.count++
    row.totalUs += e.durationUs
    if (e.durationUs > row.maxUs) row.maxUs = e.durationUs
    if (e.timestamp > row.lastTs) row.lastTs = e.timestamp
  }
  const rows = [...map.values()]
  for (const r of rows) {
    r.avgUs = Math.round(r.totalUs / r.count)
    r.share = grand > 0 ? r.totalUs / grand : 0
  }
  return rows.sort((a, b) => b.totalUs - a.totalUs)
}

export interface SlowSummary {
  total: number
  avgUs: number
  p50Us: number
  p95Us: number
  maxUs: number
}

// percentile 最近秩法（nearest-rank），n 取值 0-100；空数组返回 0
export function percentile(sorted: number[], n: number): number {
  if (!sorted.length) return 0
  const rank = Math.max(1, Math.ceil((n / 100) * sorted.length))
  return sorted[Math.min(rank, sorted.length) - 1]
}

// summarize 全局概览：总数 / 平均 / P50 / P95 / 峰值
export function summarize(entries: SlowLogEntry[]): SlowSummary {
  if (!entries.length) return { total: 0, avgUs: 0, p50Us: 0, p95Us: 0, maxUs: 0 }
  const durs = entries.map((e) => e.durationUs).sort((a, b) => a - b)
  const total = durs.reduce((s, v) => s + v, 0)
  return {
    total: entries.length,
    avgUs: Math.round(total / entries.length),
    p50Us: percentile(durs, 50),
    p95Us: percentile(durs, 95),
    maxUs: durs[durs.length - 1],
  }
}
