# ZP Redis Manager

轻量、精美、功能完善的 Redis 桌面管理工具。基于 Wails v2 + Vue 3 + TypeScript + Ant Design Vue。

![platform](https://img.shields.io/badge/platform-Windows-blue)

## 功能

- **数据浏览** — 键树虚拟滚动（十万级键无压力）、全类型编辑器（String/Hash/List/Set/ZSet/Stream）、二进制值自动检测 + Hex 编辑、大值懒加载
- **键分析** — 大 key 排行（MEMORY USAGE TOP 20）、TTL 分布、按前缀内存聚合
- **监控** — OPS/内存/客户端/区间命中率实时趋势图、INFO 概览、客户端管理
- **慢日志 / 发布订阅 / CLI** — SLOWLOG 查看清空、Pub/Sub 消息流、全命令 CLI（历史持久化 + 危险命令确认 + MONITOR 实时命令流）
- **导入导出** — 选中键导出 JSON/CSV、整库逻辑导出（DUMP/RESTORE 无损含 TTL）、跨连接复制键
- **工程化** — 连接密码 Windows DPAPI 加密存储、窗口/主题/侧栏/历史全量持久化、命令面板（Ctrl+K）、自动更新检查

## 快捷键

| 键 | 功能 |
|---|---|
| `Ctrl+K` | 命令面板 |
| `Ctrl+N` | 新建连接 |
| `Ctrl+W` | 关闭当前标签 |
| `F5` / `Ctrl+R` | 刷新当前视图 |

## 开发

```bash
# 环境：Go 1.21+ / Node 20+ / Wails CLI v2.16
go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0

wails dev          # 开发模式（前端热更新）
wails build        # 构建 build/bin/zp-rdm.exe
wails build -nsis  # 构建 NSIS 安装包（需先安装 NSIS）
go test ./...      # 运行测试（miniredis，无需真实 Redis）
```

集成测试（DUMP/RESTORE 等 miniredis 不支持的命令）需要真实 Redis：

```bash
ZP_RDM_TEST_REDIS_ADDR=localhost:6379 go test ./...
```

## 发布

push 一个 `v*` tag 即触发 GitHub Actions：全量测试 → NSIS 打包 → 自动创建 Release（含更新清单 `latest.json`）。

tag 版本号必须与 `wails.json` 的 `info.productVersion` 一致（CI 会校验）。

## 数据与安全

- 配置目录：`%AppData%/zp-rdm/`（`connections.json` + `settings.json`）
- 连接密码、SSH 私钥/密码/口令使用 **Windows DPAPI** 加密落盘（用户+机器绑定），内存中保持明文
- 旧版明文配置启动时自动迁移为密文
- 连接导出文件为**明文**（可移植性设计），请妥善保管导出文件
