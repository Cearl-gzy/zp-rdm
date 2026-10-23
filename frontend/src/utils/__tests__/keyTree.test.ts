import { describe, it, expect } from 'vitest'
import { buildKeyTree } from '../keyTree'
import type { KeyInfo } from '../../types'

const k = (key: string, type = 'string'): KeyInfo => ({ key, type, ttl: -1, size: 10 })

describe('buildKeyTree', () => {
  it('无分隔符的键全部是根级叶子', () => {
    const tree = buildKeyTree([k('a'), k('b')], ':')
    expect(tree.length).toBe(2)
    expect(tree.every(n => n.isLeaf)).toBe(true)
  })

  it('按分隔符聚合成目录并统计叶子数', () => {
    const tree = buildKeyTree([k('user:1'), k('user:2'), k('cache:a')], ':')
    expect(tree.length).toBe(2)
    const user = tree.find(n => n.key === 'user')!
    expect(user.isLeaf).toBe(false)
    expect(user.count).toBe(2)
    expect(user.children!.length).toBe(2)
    expect(user.children![0]).toMatchObject({ title: '1', key: 'user:1', isLeaf: true, type: 'string' })
  })

  it('多级嵌套', () => {
    const tree = buildKeyTree([k('a:b:c:1')], ':')
    const a = tree[0]
    expect(a.key).toBe('a')
    expect(a.count).toBe(1)
    const ab = a.children![0]
    expect(ab.key).toBe('a:b')
    const abc = ab.children![0]
    expect(abc.key).toBe('a:b:c')
    expect(abc.children![0]).toMatchObject({ key: 'a:b:c:1', isLeaf: true })
  })

  it('目录与同名叶子可共存（两个 key 相同的节点，一叶一目录）', () => {
    const tree = buildKeyTree([k('user'), k('user:1')], ':')
    expect(tree.length).toBe(2)
    const sameKey = tree.filter(n => n.key === 'user')
    expect(sameKey.length).toBe(2)
    expect(sameKey.filter(n => n.isLeaf).length).toBe(1)
    const dir = sameKey.find(n => !n.isLeaf)!
    expect(dir.children![0]).toMatchObject({ key: 'user:1', isLeaf: true })
  })

  it('兄弟目录间排序（键名序）', () => {
    const tree = buildKeyTree([k('z:1'), k('a:1')], ':')
    expect(tree.map(n => n.key)).toEqual(['a', 'z'])
  })

  it('自定义分隔符', () => {
    const tree = buildKeyTree([k('user/1'), k('user/2')], '/')
    expect(tree.length).toBe(1)
    expect(tree[0].count).toBe(2)
  })

  it('叶子保留 ttl/size 元数据', () => {
    const key: KeyInfo = { key: 'a:1', type: 'hash', ttl: 300, size: 1024 }
    const tree = buildKeyTree([key], ':')
    const leaf = tree[0].children![0]
    expect(leaf).toMatchObject({ type: 'hash', ttl: 300, size: 1024, count: 1 })
  })

  it('空列表', () => {
    expect(buildKeyTree([], ':')).toEqual([])
  })
})
