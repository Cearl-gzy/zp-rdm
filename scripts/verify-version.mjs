// 防呆：git tag 必须与 wails.json 中的 productVersion 一致（三平台构建 job 共用）
import { readFileSync } from 'node:fs'

const tag = (process.env.GITHUB_REF_NAME || '').replace(/^v/, '')
if (!tag) {
  console.error('GITHUB_REF_NAME 未设置，无法校验版本号')
  process.exit(1)
}
const cfg = JSON.parse(readFileSync('wails.json', 'utf8'))
if (cfg.info.productVersion !== tag) {
  console.error(`tag ${tag} != wails.json productVersion ${cfg.info.productVersion}，请先同步版本号`)
  process.exit(1)
}
console.log(`version check ok: ${tag}`)
