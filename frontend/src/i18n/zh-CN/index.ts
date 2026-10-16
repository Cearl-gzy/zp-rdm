// 聚合所有模块语言包（import.meta.glob 自动加载，新增模块只需放文件）
// 注意：TS 不允许在 import.meta.xxx 上写泛型实参，这里用断言
const modules = import.meta.glob('./*.ts', {
  eager: true,
  import: 'default',
}) as Record<string, Record<string, unknown>>

export default Object.assign(
  {},
  ...Object.entries(modules)
    .filter(([p]) => !p.endsWith('index.ts'))
    .map(([, m]) => m),
)
