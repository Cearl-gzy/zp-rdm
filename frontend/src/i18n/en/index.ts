// Aggregates all locale modules (import.meta.glob auto-load; adding a module = adding a file)
// Note: TS does not allow type arguments on import.meta.xxx, cast instead
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
