import { describe, it, expect } from 'vitest'
import { cmdName, aggregate, summarize, percentile } from '../slowlogAnalysis'
import type { SlowLogEntry } from '../../types'

const entry = (id: number, durationUs: number, args: string[], timestamp = 1000 + id): SlowLogEntry => ({
  id, timestamp, durationUs, clientAddr: '127.0.0.1:5000', clientName: '', args,
})

describe('cmdName', () => {
  it('单词命令大写取首词', () => {
    expect(cmdName(['get', 'k'])).toBe('GET')
    expect(cmdName(['SET', 'k', 'v'])).toBe('SET')
  })
  it('复合命令取前两词', () => {
    expect(cmdName(['config', 'get', 'maxmemory'])).toBe('CONFIG GET')
    expect(cmdName(['XINFO', 'groups', 's'])).toBe('XINFO GROUPS')
    expect(cmdName(['slowlog', 'get'])).toBe('SLOWLOG GET')
  })
  it('复合词表命中但缺第二词时只取首词', () => {
    expect(cmdName(['CONFIG'])).toBe('CONFIG')
  })
  it('空参数兜底', () => {
    expect(cmdName([])).toBe('?')
  })
})

describe('aggregate', () => {
  it('按命令聚合并按总耗时降序', () => {
    const rows = aggregate([
      entry(1, 100, ['GET', 'a']),
      entry(2, 300, ['get', 'b']),
      entry(3, 200, ['SET', 'a', '1']),
    ])
    expect(rows.map((r) => r.cmd)).toEqual(['GET', 'SET'])
    const g = rows[0]
    expect(g.count).toBe(2)
    expect(g.totalUs).toBe(400)
    expect(g.avgUs).toBe(200)
    expect(g.maxUs).toBe(300)
    expect(g.lastTs).toBe(1002)
  })
  it('share 为总耗时占比', () => {
    const rows = aggregate([entry(1, 300, ['GET', 'a']), entry(2, 100, ['SET', 'a', '1'])])
    expect(rows[0].share).toBeCloseTo(0.75)
    expect(rows[1].share).toBeCloseTo(0.25)
  })
  it('空输入返回空数组', () => {
    expect(aggregate([])).toEqual([])
  })
})

describe('percentile', () => {
  it('最近秩法', () => {
    const sorted = [10, 20, 30, 40, 50, 60, 70, 80, 90, 100]
    expect(percentile(sorted, 50)).toBe(50)
    expect(percentile(sorted, 95)).toBe(100)
    expect(percentile(sorted, 5)).toBe(10)
  })
  it('空数组返回 0', () => {
    expect(percentile([], 95)).toBe(0)
  })
  it('单元素', () => {
    expect(percentile([42], 50)).toBe(42)
    expect(percentile([42], 95)).toBe(42)
  })
})

describe('summarize', () => {
  it('总数/平均/分位/峰值', () => {
    const entries = Array.from({ length: 20 }, (_, i) => entry(i, (i + 1) * 10, ['GET', 'k']))
    const s = summarize(entries)
    expect(s.total).toBe(20)
    expect(s.avgUs).toBe(105)
    expect(s.p50Us).toBe(100)
    expect(s.p95Us).toBe(190)
    expect(s.maxUs).toBe(200)
  })
  it('空输入全零', () => {
    expect(summarize([])).toEqual({ total: 0, avgUs: 0, p50Us: 0, p95Us: 0, maxUs: 0 })
  })
})
