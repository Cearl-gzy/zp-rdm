import { describe, it, expect } from 'vitest'
import { envColor, envBarStyle } from '../env'

describe('envColor', () => {
  it('已知环境', () => {
    expect(envColor('prod')).toBe('#e04536')
    expect(envColor('test')).toBe('#f5a623')
    expect(envColor('dev')).toBe('#3fbf6b')
  })
  it('未知或空环境返回空串', () => {
    expect(envColor('staging')).toBe('')
    expect(envColor('')).toBe('')
    expect(envColor(undefined)).toBe('')
  })
})

describe('envBarStyle', () => {
  it('有环境色时给出背景样式', () => {
    expect(envBarStyle('prod')).toEqual({ background: '#e04536' })
  })
  it('无环境色时透明', () => {
    expect(envBarStyle(undefined)).toEqual({})
  })
})
