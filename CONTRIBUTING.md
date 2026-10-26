# 贡献指南 / Contributing

感谢你的兴趣！中英文 Issue 与 PR 都欢迎。
Thanks for your interest — issues and PRs in Chinese or English are both welcome.

## 开发环境

- Go 1.21+ / Node 20+ / Wails CLI v2.16
- Windows 开发机建议使用 WSL2 跑测试用 Redis（见下）

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0
wails dev     # 开发模式（前端热更新）
wails build   # 生产构建
```

## 测试

提交 PR 前请确保：

```bash
go test ./...               # 单元测试（miniredis，无需真实 Redis）
gofmt -l .                  # 必须无输出
cd frontend && npx vue-tsc --noEmit   # 改了 frontend/ 时
cd frontend && npm test               # 前端单测（vitest，改了 frontend/ 时）
```

有真实 Redis 时跑集成测试（CI 会跑）：

```bash
ZP_RDM_TEST_REDIS_ADDR=127.0.0.1:6379 go test ./...   # DUMP/RESTORE、INFO 等
ZP_RDM_TEST_STACK_ADDR=127.0.0.1:6380 go test ./...   # JSON/Search/TS/Bloom 模块
```

### WSL2 Redis Stack 测试实例（Windows 开发推荐）

项目维护者在 WSL2 中运行 Redis Stack 8（端口 6380）作为模块测试实例。参考 `README.md` 开发章节；实例不随 WSL 重启存活，需在 WSL 内用脚本拉起。

## 代码约定

- **Wails 绑定方法不能用变参**（`...string`）：Wails v2 运行时分发会报 reflect 错误且前端 Promise 悬挂。集合参数一律用切片（曾有 P0 事故：`GetInfo`）。
- **AntDV 组件以 `options` prop 为准**：如 `a-segmented` 不支持默认插槽子组件，静默不渲染。
- 前端文案走 i18n，四种语言（zh-CN / zh-TW / en / ja）键必须同步补齐；键完整性与占位符一致性由 vitest 用例 `src/i18n/__tests__/parity.test.ts` 校验（`cd frontend && npm test`，CI 也会跑）。
- 后端 service 方法默认带 `connID, db` 两参数，通过 `redisx.Manager` 取连接。
- 提交信息格式：`feat: / fix: / chore: / docs: 中文简述`（参考 git log）。

## 发布流程（仅维护者）

push `v*` tag 触发 CI 三平台构建与 Release；随后 `scripts/update-channels.sh vX.Y.Z` 同步 scoop/homebrew/winget 渠道。详见 README「发布」章节。
