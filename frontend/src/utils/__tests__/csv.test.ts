import { describe, it, expect } from 'vitest'
import { csvCell, csvSection } from '../csv'

describe('csvCell', () => {
  it('普通值原样输出', () => {
    expect(csvCell('abc')).toBe('abc')
    expect(csvCell(42)).toBe('42')
  })
  it('含逗号加引号', () => {
    expect(csvCell('a,b')).toBe('"a,b"')
  })
  it('内嵌引号双写', () => {
    expect(csvCell('say "hi"')).toBe('"say ""hi"""')
  })
  it('含换行加引号', () => {
    expect(csvCell('a\nb')).toBe('"a\nb"')
  })
})

describe('csvSection', () => {
  it('标题 + 表头 + 数据行', () => {
    const out = csvSection('概览', ['键', '数量'], [['user', 3], ['cache', 2]])
    expect(out).toBe('# 概览\n键,数量\nuser,3\ncache,2')
  })
  it('空数据行只有标题与表头', () => {
    expect(csvSection('T', ['a'], [])).toBe('# T\na')
  })
  it('单元格转义贯穿表头与数据', () => {
    const out = csvSection('T', ['a,b'], [['x"y']])
    expect(out).toBe('# T\n"a,b"\n"x""y"')
  })
})
