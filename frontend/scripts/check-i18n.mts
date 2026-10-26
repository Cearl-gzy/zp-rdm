// i18n 完整性检查：所有语言包与基准（zh-CN）键集合必须一致
// 用法：node scripts/check-i18n.mts（在 frontend/ 下运行；CI 质量门之一）
import { readdirSync } from 'node:fs'
import { pathToFileURL } from 'node:url'
import path from 'node:path'

const i18nDir = path.resolve('src/i18n')
const locales = readdirSync(i18nDir, { withFileTypes: true })
  .filter((d) => d.isDirectory())
  .map((d) => d.name)
  .sort()

async function loadLocale(dir: string): Promise<Record<string, unknown>> {
  const merged: Record<string, unknown> = {}
  for (const f of readdirSync(dir).filter((f) => f.endsWith('.ts') && f !== 'index.ts')) {
    const mod = await import(pathToFileURL(path.join(dir, f)).href)
    Object.assign(merged, mod.default)
  }
  return merged
}

function flatten(obj: Record<string, unknown>, prefix = ''): string[] {
  const keys: string[] = []
  for (const [k, v] of Object.entries(obj)) {
    const key = prefix ? `${prefix}.${k}` : k
    if (v && typeof v === 'object' && !Array.isArray(v)) {
      keys.push(...flatten(v as Record<string, unknown>, key))
    } else {
      keys.push(key)
    }
  }
  return keys
}

const base = flatten(await loadLocale(path.join(i18nDir, 'zh-CN')))
const baseSet = new Set(base)
let failed = false
console.log(`base zh-CN: ${base.length} keys`)
for (const loc of locales) {
  if (loc === 'zh-CN') continue
  const keys = flatten(await loadLocale(path.join(i18nDir, loc)))
  const keySet = new Set(keys)
  const missing = base.filter((k) => !keySet.has(k))
  const extra = keys.filter((k) => !baseSet.has(k))
  console.log(`${loc}: ${keys.length} keys, missing=${missing.length}, extra=${extra.length}`)
  if (missing.length) console.log('  missing:', missing.join(', '))
  if (extra.length) console.log('  extra:', extra.join(', '))
  if (missing.length || extra.length) failed = true
}
if (failed) process.exit(1)
console.log('i18n OK')
