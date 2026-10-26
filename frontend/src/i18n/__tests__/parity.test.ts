// i18n 完整性检查：所有语言包与基准（zh-CN）键集合必须一致
// 原为独立脚本 scripts/check-i18n.mts；Node 20 无法直接跑 .mts 且不能 import .ts，
// 改为 vitest 用例（vite 原生支持 import.meta.glob），随 npm test / CI 质量门一起跑
import { describe, it, expect } from 'vitest'
import zhCN from '../zh-CN'
import zhTW from '../zh-TW'
import en from '../en'
import ja from '../ja'

type Messages = Record<string, unknown>

function flatten(obj: Messages, prefix = ''): string[] {
  const keys: string[] = []
  for (const [k, v] of Object.entries(obj)) {
    const key = prefix ? `${prefix}.${k}` : k
    if (v && typeof v === 'object' && !Array.isArray(v)) {
      keys.push(...flatten(v as Messages, key))
    } else {
      keys.push(key)
    }
  }
  return keys
}

// 收集文案中的 {placeholder}，校验各语言占位符与基准一致（防翻译丢参数）
// 真实参数是 ASCII 标识符（{key} / {count} 等）；search.queryPlaceholder 里的
// @city:{北京} 是 RediSearch TAG 语法示例（各语言示例值不同），不是插值参数，排除
const PLACEHOLDER_SKIP = new Set(['search.queryPlaceholder'])

function placeholders(obj: Messages, prefix = ''): Map<string, string[]> {
  const map = new Map<string, string[]>()
  for (const [k, v] of Object.entries(obj)) {
    const key = prefix ? `${prefix}.${k}` : k
    if (v && typeof v === 'object' && !Array.isArray(v)) {
      for (const [kk, vv] of placeholders(v as Messages, key)) map.set(kk, vv)
    } else if (typeof v === 'string' && !PLACEHOLDER_SKIP.has(key)) {
      map.set(key, [...v.matchAll(/\{([A-Za-z_][A-Za-z0-9_]*)\}/g)].map((m) => m[1]).sort())
    }
  }
  return map
}

const base = flatten(zhCN as Messages)
const baseSet = new Set(base)
const basePh = placeholders(zhCN as Messages)

describe('i18n 键完整性（基准 zh-CN）', () => {
  it('基准 zh-CN 键数 >= 900', () => {
    expect(base.length).toBeGreaterThanOrEqual(900)
  })

  for (const [name, msgs] of Object.entries({ 'zh-TW': zhTW, en, ja })) {
    it(`${name} 与 zh-CN 键集合一致`, () => {
      const keys = flatten(msgs as Messages)
      const keySet = new Set(keys)
      expect(base.filter((k) => !keySet.has(k)), 'missing').toEqual([])
      expect(keys.filter((k) => !baseSet.has(k)), 'extra').toEqual([])
    })

    it(`${name} 占位符与 zh-CN 一致`, () => {
      const ph = placeholders(msgs as Messages)
      const bad: string[] = []
      for (const [k, v] of basePh) {
        const got = ph.get(k)
        if (!got || got.join(',') !== v.join(',')) bad.push(k)
      }
      expect(bad).toEqual([])
    })
  }
})
