# i18n 迁移规范（ZP RDM）

目标：把 .vue 文件里所有面向用户的中文文案迁移到 vue-i18n。

## 铁律

1. **只改分配给你的 .vue 文件和你自己的语言包模块文件**——不要动 `src/i18n/index.ts`、`src/i18n/zh-CN/index.ts`、`src/i18n/en/index.ts`、`common.ts`、别人的模块文件、wailsjs、后端 Go 代码。
2. 语言包按模块命名空间隔离：每个模块一个 `src/i18n/zh-CN/<module>.ts` 和 `src/i18n/en/<module>.ts`，内容为 `export default { <module>: { ...keys } }`。index.ts 用 `import.meta.glob` 自动加载，**无需注册**。
3. Redis 命令名、语法示例、字段名、协议名不翻译（如 `FT.SEARCH`、`@field:value`、`MEMORY USAGE`、`db0`）。英文版里它们原样保留。
4. 英文用简洁的技术英语，首字母大写规则：按钮/菜单用 Title Case 或 Sentence case 均可，同一模块内保持一致；完整句子用 Sentence case。
5. 不遗漏：模板文本、placeholder、title/tooltip、message.success/error/warning、Modal.confirm 的 title/content/okText/cancelText、空态 description、表头 title、校验提示，全部要迁移。
6. 不翻译：console 日志、注释、CSS、变量名、wailsjs 方法名。代码注释保持中文原样。
7. 后端返回的错误串（`e.message`）原样透传，不包 t()。

## 用法范式

script setup 顶部：

```ts
import { useI18n } from 'vue-i18n'
const { t } = useI18n()
```

模板：

```vue
<a-button>{{ t('common.save') }}</a-button>
<a-input :placeholder="t('keyBrowser.patternPlaceholder')" />
<a-tooltip :title="t('common.refresh')" />
```

带插值（禁止字符串拼接）：

```ts
// zh-CN: deleteN: '删除选中的 {n} 个键？'
// en:    deleteN: 'Delete {n} selected keys?'
message.success(t('keyBrowser.deletedN', { n: count }))
```

复数/数量统一用 `{n}` 插值，不用 vue-i18n 复数管线。

## key 命名

- camelCase，按用途分组可加一层嵌套（如 `create: { title: ..., nameLabel: ... }`）。
- zh-CN 与 en 的 key 必须一一对应。
- 通用词优先用 `common.*`（已有：confirm/cancel/save/delete/edit/add/refresh/search/reset/close/copy/copied/loading/loadMore/actions/success/failed/deleteConfirm/irreversible/empty/total/yes/no/ok/create/name/type/key/value/field/optional/all/none）。common 没有的就在自己模块里定义，不要加进 common.ts。

## 自检

完成后：
1. `grep` 你的 .vue 文件确认没有残留中文（注释除外）：`grep -nP '[\x{4e00}-\x{9fff}]' <file>.vue`——剩下的每一处都该在注释里或是 Redis 示例语法里的中文演示数据（如 `@city:{北京}` 可保留，它是查询语法示例；对应翻译行 en 可改成 `@city:{beijing}`）。
2. 确认 zh-CN/en 两个模块文件 key 完全对称。
3. 不要运行构建/测试（由主会话统一验证）。
