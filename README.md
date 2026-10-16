# ZP Redis Manager

轻量、精美、功能完善的 Redis 桌面管理工具。

基于 **Wails v2 + Vue 3 + TypeScript + Ant Design Vue**，单文件 exe，无运行时依赖，启动即连。

![platform](https://img.shields.io/badge/platform-Windows%20%7C%20macOS%20%7C%20Linux-blue)
![version](https://img.shields.io/badge/version-1.8.0-red)

---

## 安装

从 [Releases](https://github.com/Cearl-gzy/zp-rdm/releases/latest) 下载对应平台：

| 平台 | 文件 | 说明 |
|---|---|---|
| Windows | `zp-rdm-amd64-installer.exe` | NSIS 安装包（推荐）；`zp-rdm.exe` 为便携版 |
| macOS | `zp-rdm-darwin-universal.zip` | Universal 二进制（Intel + Apple Silicon） |
| Linux | `zp-rdm-linux-amd64.tar.gz` | 需要 GTK3 + WebKit2GTK 运行时 |

**macOS**：应用未签名，首次打开需绕过 Gatekeeper：

```bash
unzip zp-rdm-darwin-universal.zip
xattr -cr zp-rdm.app   # 移除隔离属性
open zp-rdm.app
```

**Linux**： Debian/Ubuntu 安装运行时依赖后即可启动：

```bash
sudo apt install libwebkit2gtk-4.1-0
tar -xzf zp-rdm-linux-amd64.tar.gz && ./zp-rdm
```

> 🔐 跨平台加密：连接密码在三大平台均加密落盘——Windows DPAPI、macOS 钥匙串、Linux Secret Service（密钥环不可用时回退 0600 权限的本地密钥文件）。

---

## 功能总览

| 模块 | 能力 |
|---|---|
| 数据浏览 | 键树虚拟滚动、全类型编辑器、二进制 Hex、大值懒加载、批量 TTL/移动 DB |
| 键分析 | TOP Keys、TTL 分布、前缀内存聚合 |
| 监控 | OPS / 内存 / 客户端 / 命中率实时趋势、INFO 概览、客户端管理 |
| 运维 | 慢日志、发布订阅、全命令 CLI（Tab 自动补全）、MONITOR 实时命令流 |
| 模块 | RedisJSON 编辑器（文本/树形、路径级写入）、RediSearch 查询面板 |
| 数据迁移 | 选中键导出、整库导出、跨实例导入、跨连接复制 |
| 集群 | Redis Cluster 连接、全节点聚合扫描、集群节点视图、hash slot 查询 |
| 工程化 | 全平台密码加密、TLS/mTLS 连接、工作区会话恢复、SSH 隧道、命令面板、自动更新 |

---

## 功能详解

### 🌳 数据浏览

- **键树虚拟滚动** — 十万级键无压力。按分隔符（`:` `/` `.` `_` `-`）自动聚合成目录树，目录显示键数统计；树/列表双视图一键切换
- **全类型编辑器** — String / Hash / List / Set / ZSet / Stream 均有专属编辑器，支持字段级增删改、内联编辑；Stream 额外支持**消费者组管理**（组/消费者增删、pending 待确认列表、逐条 ACK）
- **CodeMirror 编辑器** — String 值支持 JSON 语法高亮、格式化、压缩
- **二进制自动检测** — 含不可见字符的值自动切 Hex 视图，十六进制直接编辑，乱码不再困扰
- **大值懒加载** — 超过 1MB 的 String 默认只加载前 1MB 预览，一键加载全量，避免界面卡死
- **TTL 一目了然** — 键树上直接显示剩余 TTL，临期键（< 5 分钟）高亮警示
- **类型过滤 + 模式匹配** — `user:*` 模式搜索与类型筛选可叠加，SCAN 分页加载不阻塞

### 📊 键分析

- **TOP Keys 排行** — `MEMORY USAGE` 采样统计前 20 大键，类型、大小、元素数一表看清
- **TTL 分布** — 永久 / <1h / <24h / <7d / <30d / ≥30d 六档柱状分布，快速发现过期策略问题
- **前缀聚合** — 按前缀（可自定义分隔符）聚合键数与内存占用，定位"哪个业务吃了内存"

### 📈 监控大盘

- **四路实时趋势**（2 秒采样，SVG 迷你图）：
  - **OPS** — 每秒命令数
  - **内存** — used_memory 变化曲线
  - **客户端连接** — 连接数波动
  - **区间命中率** — 每个采样窗口的 keyspace hits/(hits+misses)，比累计命中率更灵敏
- **INFO 全景** — 服务器版本、运行模式、运行时长、键空间各 DB 分布
- **客户端管理** — `CLIENT LIST` 查看所有连接（地址/空闲时长/最后命令），可一键 KILL

### 🖥️ CLI 终端

- **全命令支持** — 任意 Redis 命令直接执行，结果按类型着色（状态/整数/数组/错误）
- **历史持久化** — 命令历史跨会话保留（上限 200 条），↑↓ 翻阅，连续重复自动去重
- **危险命令护栏** — `FLUSHALL` / `FLUSHDB` / `SHUTDOWN` / `DEBUG` / `CONFIG SET` / `KEYS` 执行前弹确认框，防手滑
- **MONITOR 模式** — 实时命令流监听（独立连接，不占用连接池），支持暂停、关键字过滤、500 行滚动缓冲，闪烁指示灯明确监听状态

### 🧩 Redis 模块（Redis Stack）

- **RedisJSON 编辑器** — `ReJSON-RL` 键自动识别：文本模式（语法高亮/格式化）+ 树形模式（结构浏览）；树节点悬停即可**路径级编辑**（`JSON.SET key $.path`，局部写入不动 TTL）；模块加载时新建键可选 json 类型
- **RediSearch 查询面板** — 独立"搜索"视图：索引下拉（FT._LIST）+ schema 字段展示（名称/类型/JSONPath）+ 完整查询语法（`@field:value`、`@age:[18 30]`、`@city:{北京}`），结果分页直达数据视图

### 🌐 Redis Cluster

- **集群连接** — 连接配置勾选「集群模式」即用 ClusterClient 接入（任一节点地址即可，自动发现拓扑），仅 db 0；TLS/SSH 隧道与单机配置完全复用
- **全节点聚合扫描** — 键浏览自动聚合全部 master 节点（服务端维护各节点 SCAN 游标，逐节点直连，规避客户端拓扑竞态漏扫）；键数为全集群 DBSIZE 汇总
- **集群视图** — 独立「集群」面板：cluster_state / slots_assigned / 节点数 / 键数概览，节点表展示角色（主/从）、slot 区间与数量、链路状态、当前节点标记
- **hash slot 查询** — 面板内输入任意键即算 `CLUSTER KEYSLOT`，写脚本/排查热点槽必备
- **跨槽兼容** — 批量删除自动拆分为逐键路由，绕开集群 CROSSSLOT 限制
- **已知限制**：键分析、MONITOR、CLI 自动补全仅覆盖单节点；RENAME 跨槽会报错（Redis 集群语义）

### 🐌 慢日志 & 发布订阅

- **SLOWLOG** — 慢查询列表（耗时/命令/时间/客户端），阈值说明，一键清空
- **Pub/Sub** — 任意频道订阅，消息流实时滚动，支持模式订阅（psubscribe）

### 📦 数据迁移

- **选中键导出** — 勾选后导出 JSON（DUMP 序列化，全类型无损含二进制与 TTL）或 CSV（表格查看）
- **整库导出** — JSONL 流式导出（每行一键），带实时进度，大库导出期间界面可操作
- **跨实例导入** — RESTORE REPLACE 导入到任意连接/DB，同名键覆盖前确认
- **跨连接复制** — 值面板一键把当前键复制到另一个连接的指定 DB，支持重命名与 TTL 保留

### 🔐 安全与工程化

- **全平台密码加密** — 连接密码、SSH 私钥/密码/口令加密落盘（用户+机器绑定，拷走配置文件也解不开）：Windows DPAPI / macOS 钥匙串 / Linux Secret Service（AES-256-GCM 自包含密文，主密钥存系统密钥环）；旧版明文配置启动时自动迁移
- **TLS/SSL 连接** — 支持跳过校验（自签场景）、自定义 CA 证书严格校验、双向认证（mTLS 客户端证书/私钥），私钥随敏感字段加密落盘
- **SSH 隧道** — 连接支持 SSH 跳板（密码/私钥/口令三种认证）
- **工作区会话恢复** — 重启后自动恢复：打开的连接标签、激活标签、每连接的视图（数据/监控/CLI…）、选中的键、匹配模式、树/列表视图、键分隔符、上次使用的 DB
- **窗口状态记忆** — 尺寸、位置、最大化状态、侧栏宽度（可拖拽 160–480px）、深浅主题全部持久化
- **命令面板** — `Ctrl+K` 唤起，模糊搜索连接/视图/操作，全键盘流
- **自动更新检查** — 关于面板一键检查新版本，命中后直达 GitHub Release
- **精致 UI** — 无边框窗口 + 自定义标题栏、深色/浅色双主题、禁用右键菜单、禁用文本选择（终端区除外）

---

## 快捷键

| 键 | 功能 |
|---|---|
| `Ctrl+K` | 命令面板 |
| `Ctrl+N` | 新建连接 |
| `Ctrl+W` | 关闭当前标签 |
| `F5` / `Ctrl+R` | 刷新当前视图 |
| `↑` / `↓` | CLI 中翻阅命令历史 |

---

## 技术栈

- **后端**：Go 1.21+ / Wails v2.16 / go-redis v9 / DPAPI (CryptoProtectData)
- **前端**：Vue 3 / TypeScript / Pinia / Ant Design Vue 4 / CodeMirror 6
- **测试**：miniredis（内存 Redis 桩）+ 真实 Redis 集成测试（CI service container）
- **发布**：GitHub Actions 三平台构建（Windows NSIS / macOS universal app / Linux tar.gz → Release + 多平台更新清单）

## 开发

```bash
# 环境：Go 1.21+ / Node 20+ / Wails CLI v2.16
go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0

wails dev          # 开发模式（前端热更新）
wails build        # 构建 build/bin/zp-rdm.exe
wails build -nsis  # 构建 NSIS 安装包（需先 winget install NSIS.NSIS）
go test ./...      # 单元测试（miniredis，无需真实 Redis）
```

集成测试（DUMP/RESTORE 等 miniredis 不支持的命令）需要真实 Redis：

```bash
ZP_RDM_TEST_REDIS_ADDR=localhost:6379 go test ./...
```

## 发布

push 一个 `v*` tag 即触发 GitHub Actions：全量测试 → 三平台并行构建（Windows NSIS + 便携 exe / macOS universal zip / Linux tar.gz）→ 自动创建 Release（含更新清单 `latest.json`，按平台匹配下载地址）。

> tag 版本号必须与 `wails.json` 的 `info.productVersion` 一致（CI 会校验，不一致直接失败）。

## 数据与安全

- 配置目录：Windows `%AppData%/zp-rdm/`，macOS/Linux `~/.config/zp-rdm/`（`connections.json` + `settings.json`）
- 连接密码、SSH 私钥/密码/口令、TLS 客户端私钥：全平台加密落盘（Windows **DPAPI** / macOS 钥匙串 / Linux Secret Service，主密钥不可用时回退 0600 密钥文件），内存中保持明文
- 旧版明文配置启动时自动迁移为密文
- ⚠️ 连接**导出文件为明文**（可移植性设计），请妥善保管导出文件
- ⚠️ CLI 命令历史以明文保存在 `settings.json`，包含执行过的完整命令
