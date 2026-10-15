import type { KeyInfo } from '../types'

export interface KeyNode {
  title: string
  key: string        // 叶子为完整 key，目录为前缀
  isLeaf: boolean
  type?: string
  ttl?: number
  size?: number
  count: number      // 目录下的叶子数
  children?: KeyNode[]
}

// 将扁平 key 列表按分隔符构建树
export function buildKeyTree(keys: KeyInfo[], delimiter: string): KeyNode[] {
  const root: KeyNode[] = []
  const dirMap = new Map<string, KeyNode>()

  const sorted = [...keys].sort((a, b) => a.key.localeCompare(b.key))

  for (const k of sorted) {
    const parts = k.key.split(delimiter)
    let siblings = root
    let prefix = ''
    for (let i = 0; i < parts.length; i++) {
      const seg = parts[i]
      const isLast = i === parts.length - 1
      prefix = i === 0 ? seg : prefix + delimiter + seg
      if (isLast) {
        siblings.push({
          title: seg, key: k.key, isLeaf: true,
          type: k.type, ttl: k.ttl, size: k.size, count: 1,
        })
      } else {
        let dir = dirMap.get(prefix)
        if (!dir) {
          dir = { title: seg, key: prefix, isLeaf: false, count: 0, children: [] }
          dirMap.set(prefix, dir)
          siblings.push(dir)
        }
        dir.count++
        siblings = dir.children!
      }
    }
  }
  return root
}

// 仅保留一层目录展开后的平铺（用于目录只有单子节点时压缩显示可扩展，这里保持简单）
export function flattenFolders(nodes: KeyNode[]): KeyNode[] {
  return nodes
}
