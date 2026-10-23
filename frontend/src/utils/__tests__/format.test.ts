import { describe, it, expect } from 'vitest'
import { formatBytes, formatTTL, formatNumber, formatDuration, tryFormatJson, toHex, formatTime } from '../format'

describe('formatBytes', () => {
  it('负数与零', () => {
    expect(formatBytes(-1)).toBe('-')
    expect(formatBytes(0)).toBe('0 B')
  })
  it('字节级', () => {
    expect(formatBytes(512)).toBe('512 B')
    expect(formatBytes(1023)).toBe('1023 B')
  })
  it('KB/MB/GB/TB 进位', () => {
    expect(formatBytes(1024)).toBe('1.0 KB')
    expect(formatBytes(1536)).toBe('1.5 KB')
    expect(formatBytes(1024 * 1024)).toBe('1.0 MB')
    expect(formatBytes(1024 ** 3)).toBe('1.0 GB')
    expect(formatBytes(1024 ** 4)).toBe('1.0 TB')
  })
  it('≥100 取整，<100 保留一位', () => {
    expect(formatBytes(100 * 1024)).toBe('100 KB')
    expect(formatBytes(99.5 * 1024)).toBe('99.5 KB')
  })
  it('TB 封顶不再进位', () => {
    expect(formatBytes(1024 ** 5)).toBe('1024 TB')
  })
})

describe('formatTTL', () => {
  it('特殊值', () => {
    expect(formatTTL(-2)).toBe('不存在')
    expect(formatTTL(-1)).toBe('永久')
    expect(formatTTL(-99)).toBe('永久')
  })
  it('秒/分/时/天', () => {
    expect(formatTTL(59)).toBe('59s')
    expect(formatTTL(60)).toBe('1m 0s')
    expect(formatTTL(125)).toBe('2m 5s')
    expect(formatTTL(3600)).toBe('1h 0m')
    expect(formatTTL(3661)).toBe('1h 1m')
    expect(formatTTL(86400)).toBe('1d 0h')
    expect(formatTTL(90000)).toBe('1d 1h')
  })
})

describe('formatNumber', () => {
  it('分级缩写', () => {
    expect(formatNumber(999)).toBe('999')
    expect(formatNumber(1500)).toBe('1.5K')
    expect(formatNumber(2_500_000)).toBe('2.50M')
    expect(formatNumber(3_000_000_000)).toBe('3.00B')
  })
})

describe('formatDuration', () => {
  it('分/时/天组合', () => {
    expect(formatDuration(125)).toBe('2分5秒')
    expect(formatDuration(3661)).toBe('1小时1分')
    expect(formatDuration(90000)).toBe('1天1小时')
  })
})

describe('tryFormatJson', () => {
  it('非 JSON 直接返回 null', () => {
    expect(tryFormatJson('')).toBeNull()
    expect(tryFormatJson('hello')).toBeNull()
    expect(tryFormatJson('123')).toBeNull()
  })
  it('对象与数组格式化', () => {
    expect(tryFormatJson('{"a":1}')).toBe('{\n  "a": 1\n}')
    expect(tryFormatJson('[1,2]')).toBe('[\n  1,\n  2\n]')
  })
  it('非法 JSON 返回 null', () => {
    expect(tryFormatJson('{broken')).toBeNull()
    expect(tryFormatJson('[1,')).toBeNull()
  })
})

describe('toHex', () => {
  it('ASCII 可打印字符', () => {
    const out = toHex('AB')
    expect(out).toContain('41 42')
    expect(out).toContain('AB')
    expect(out.startsWith('00000000')).toBe(true)
  })
  it('不可见字符显示为点', () => {
    expect(toHex('\x01')).toContain(' .')
  })
  it('UTF-8 多字节', () => {
    // 中 = e4 b8 ad
    expect(toHex('中')).toContain('e4 b8 ad')
  })
  it('超过 16 字节换行且偏移递增', () => {
    const lines = toHex('0123456789abcdefg').split('\n')
    expect(lines.length).toBe(2)
    expect(lines[1].startsWith('00000010')).toBe(true)
  })
})

describe('formatTime', () => {
  it('秒级时间戳格式化', () => {
    const ts = Math.floor(new Date(2026, 9, 23, 8, 5, 3).getTime() / 1000)
    expect(formatTime(ts)).toBe('2026-10-23 08:05:03')
  })
})
