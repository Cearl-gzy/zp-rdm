# ZP Redis Manager

[简体中文](README.md) | **English**

A lightweight, polished, full-featured Redis desktop manager.

Built with **Wails v2 + Vue 3 + TypeScript + Ant Design Vue**. Single-file exe, no runtime dependencies, connect on launch.

![platform](https://img.shields.io/badge/platform-Windows%20%7C%20macOS%20%7C%20Linux-blue)
![version](https://img.shields.io/badge/version-1.25.0-red)

---

## Installation

Download for your platform from [Releases](https://github.com/Cearl-gzy/zp-rdm/releases/latest):

| Platform | File | Notes |
|---|---|---|
| Windows | `zp-rdm-amd64-installer.exe` | NSIS installer (recommended); `zp-rdm.exe` is the portable build |
| macOS | `zp-rdm-darwin-universal.zip` | Universal binary (Intel + Apple Silicon) |
| Linux | `zp-rdm-linux-amd64.tar.gz` / `.deb` | Requires GTK3 + WebKit2GTK runtime |

Or install via a package manager:

```powershell
# Windows — winget
winget install Cearl-gzy.zp-rdm

# Windows — Scoop
scoop bucket add zp-rdm https://github.com/Cearl-gzy/scoop-bucket
scoop install zp-rdm
```

```bash
# macOS — Homebrew
brew tap Cearl-gzy/tap
brew install --cask zp-rdm

# Debian/Ubuntu — .deb
sudo dpkg -i zp-rdm-linux-amd64.deb   # depends on libwebkit2gtk-4.1-0; run apt --fix-broken install if missing
```

**macOS**: the app is unsigned — bypass Gatekeeper on first launch:

```bash
unzip zp-rdm-darwin-universal.zip
xattr -cr zp-rdm.app   # remove the quarantine attribute
open zp-rdm.app
```

**Linux**: install the runtime dependency on Debian/Ubuntu and you're ready:

```bash
sudo apt install libwebkit2gtk-4.1-0
tar -xzf zp-rdm-linux-amd64.tar.gz && ./zp-rdm
```

> 🔐 Cross-platform encryption: connection passwords are stored encrypted on all three platforms — Windows DPAPI, macOS Keychain, Linux Secret Service (falls back to a 0600-permission local key file when no keyring is available).

---

## Feature Overview

| Module | Capabilities |
|---|---|
| Data browsing | Virtual-scrolled key tree, editors for every type, binary Hex view, lazy loading for large values, batch TTL / move-DB, pattern batch delete |
| Key analysis | TOP Keys, TTL distribution, per-prefix memory aggregation, report export (JSON/CSV) |
| Monitoring | Real-time OPS / memory / clients / hit-rate trends, INFO overview, client management |
| Operations | Slow log, LATENCY event curves, Pub/Sub (channel insights + keyspace event stream), full-command CLI (Tab completion + inline COMMAND DOCS + favorite commands), live MONITOR stream |
| Server admin | Replication topology, CONFIG get/set/REWRITE, ACL user management (Redis 6+), Lua script cache & Redis Functions management, Memory Doctor (MEMORY STATS/DOCTOR) |
| Modules | RedisJSON editor (text/tree, path-level writes), RediSearch query panel, TimeSeries panel, Bloom Filter panel, Redis 8 Vector Set panel |
| Data migration | Export selected keys, full-DB export, cross-instance import, cross-connection copy |
| Production safety | Read-only connection mode (client-side write blocking), environment tag coloring (prod/test/dev), write-command audit log |
| Cluster | Redis Cluster connections, aggregated all-node scanning, cluster node view, hash slot lookup |
| Sentinel | Redis Sentinel connections, automatic failover following, multiple sentinel addresses |
| Engineering | Encrypted passwords on all platforms, TLS/mTLS, workspace session restore, SSH tunneling, command palette, auto-update, four-language UI (简中/繁中/EN/JA) |

---

## Screenshots

| Welcome | Data Browsing (Hash editor) |
|---|---|
| ![Welcome](docs/screenshots/welcome.png) | ![Hash editor](docs/screenshots/main-hash.png) |
| **JSON tree editing** | **Monitoring dashboard** |
| ![JSON tree editing](docs/screenshots/json-tree.png) | ![Monitoring dashboard](docs/screenshots/monitor.png) |
| **Key analysis** | **CLI terminal (Tab completion)** |
| ![Key analysis](docs/screenshots/analysis.png) | ![CLI terminal](docs/screenshots/cli.png) |
| **RediSearch queries** | **TimeSeries panel** |
| ![RediSearch queries](docs/screenshots/search.png) | ![TimeSeries panel](docs/screenshots/ts-panel.png) |
| **Bloom Filter panel** | **Vector Set similarity search** |
| ![Bloom Filter panel](docs/screenshots/bloom.png) | ![Vector Set similarity search](docs/screenshots/vector-vsim.png) |

---

## Feature Details

### 🌳 Data Browsing

- **Virtual-scrolled key tree** — handles hundreds of thousands of keys with ease. Keys auto-group into a directory tree by delimiter (`:` `/` `.` `_` `-`) with per-directory key counts; switch between tree and list views instantly
- **Editors for every type** — dedicated editors for String / Hash / List / Set / ZSet / Stream with field-level add/edit/delete and inline editing. Streams additionally support **consumer group management** (add/remove groups and consumers, pending list, per-entry ACK), **XRANGE range queries**, and **XAUTOCLAIM pending claiming** (hand dead/stuck messages to a consumer in one click)
- **CodeMirror editor** — String values get JSON syntax highlighting, formatting, and minification
- **Automatic binary detection** — values containing non-printable characters switch to Hex view automatically; edit directly in hexadecimal, no more mojibake
- **Lazy loading for large values** — Strings over 1MB preview only the first 1MB by default; load the full value on demand to keep the UI responsive
- **TTL at a glance** — remaining TTL shown right on the key tree; keys expiring soon (< 5 minutes) are highlighted
- **Type filter + pattern matching** — combine `user:*` pattern search with type filters; SCAN-based paged loading never blocks
- **Pattern batch delete** — enter a match pattern from the ⋮ menu (prefilled with the current filter), live key count updates as you type, then SCAN-based batched deletion; scans each master directly in cluster mode, and read-only connections are blocked by the guardrail

### 📊 Key Analysis

- **TOP Keys ranking** — `MEMORY USAGE` sampling ranks the 20 largest keys; type, size, and element count at a glance
- **TTL distribution** — six buckets (permanent / <1h / <24h / <7d / <30d / ≥30d) in a bar chart to spot expiration-policy problems fast
- **Prefix aggregation** — aggregate key count and memory by prefix (custom delimiter) to answer "which business is eating the memory"
- **Big-key scan report** — deep scan with configurable key pattern / sample cap (10k–200k) / TOP N (20–200): per-key `MEMORY USAGE` + element count + TTL, per-type distribution, elapsed time and truncation flags
- **Hot-key statistics** — two modes: ① MONITOR stream sampling (works on any instance; client-side aggregated read/write counts on an independent connection that doesn't disturb the MONITOR view; auto-stops after 300s) ② `OBJECT FREQ` (reads Redis access counters directly under LFU eviction; gives a clear hint on non-LFU policies)
- **Cluster-wide analysis** — in cluster mode, key analysis / big-key reports automatically aggregate all master nodes (direct per-node SCAN; key counts are the cluster-wide DBSIZE sum)
- **Report export** — one-click export of overview analysis / big-key reports / hot-key stats to JSON (full structure) or CSV (sectioned tables), ready for archiving or pasting into tickets

### 📈 Monitoring Dashboard

- **Four real-time trends** (2s sampling, SVG sparklines):
  - **OPS** — commands per second
  - **Memory** — used_memory curve
  - **Clients** — connection count fluctuation
  - **Windowed hit rate** — keyspace hits/(hits+misses) per sampling window, more responsive than the cumulative rate
- **INFO panorama** — server version, run mode, uptime, keyspace distribution across DBs
- **Client management** — view all connections via `CLIENT LIST` (address / idle time / last command), with one-click KILL

### 🖥️ CLI Terminal

- **Full command support** — run any Redis command; results color-coded by type (status/integer/array/error)
- **Inline COMMAND DOCS** — typing a command name (including subcommands like CONFIG GET) pops up the official syntax, a one-line description, the version introduced, and time complexity. Docs come from the server itself (Redis 6.2+), so they always match its version — module commands included
- **Persistent history** — command history survives sessions (cap 200), browse with ↑↓, consecutive duplicates auto-deduped
- **Favorite commands** — star the current input with one click, reopen the favorites panel in the toolbar to refill the input or remove entries; persisted across sessions (cap 100)
- **Dangerous-command guardrails** — `FLUSHALL` / `FLUSHDB` / `SHUTDOWN` / `DEBUG` / `CONFIG SET` / `KEYS` ask for confirmation before executing
- **MONITOR mode** — live command stream on a dedicated connection (doesn't occupy the pool), with pause, keyword filter, a 500-line scrollback buffer, and a blinking indicator showing listener state

### 🧩 Redis Modules (Redis Stack)

- **RedisJSON editor** — `ReJSON-RL` keys auto-detected: text mode (syntax highlighting/formatting) + tree mode (structural browsing); hover a tree node for **path-level editing** (`JSON.SET key $.path` — partial writes that preserve TTL); when the module is loaded, new keys can be created as type json
- **RediSearch query panel** — a dedicated "Search" view: index dropdown (FT._LIST) + schema field display (name/type/JSONPath) + full query syntax (`@field:value`, `@age:[18 30]`, `@city:{Beijing}`), with paged results that jump straight into the data view
- **FT.CREATE index wizard** — build indexes graphically: ON JSON/HASH, multiple key prefixes, field table (JSONPath/alias/type TEXT·TAG·NUMERIC·GEO/SORTABLE), alias auto-filled from the last JSONPath segment; one-click index deletion (optional DD to drop documents too)
- **FT.AGGREGATE panel** — structured aggregation builder: filter + multi-field GROUPBY + reducers (COUNT / COUNT_DISTINCT / SUM / AVG / MIN / MAX, auto-aliased), dynamic-column result table; complex pipelines (LOAD/APPLY/SORTBY) via native CLI commands
- **TimeSeries panel** — `TSDB-TYPE` keys auto-detected: `TS.INFO` overview (samples/memory/retention/chunks + labels and downsampling rules), range presets (all/5m/1h/1d) + aggregation queries (avg/sum/min/max/count/first/last with bucket ms) on an SVG line chart, latest-samples table (last 50, newest first), `TS.ADD` sample append (empty timestamp = server now); when the module is loaded, new keys can be created as type ts
- **Bloom Filter panel** — `MBbloom--` keys auto-detected: `BF.INFO` overview cards (inserted/capacity/bitmap size/sub-filters) + fill-ratio bar (threshold-colored), `BF.ADD` item insert (reports added vs. probably-duplicate), `BF.MEXISTS` batch existence check (one item per line, up to 100, "may exist / definitely not" semantics chips); when the module is loaded, new keys can be created as type bf

### 🧭 Redis 8 Vector Set Panel

- **VSIM similarity search** — query by member (`ELE`) or raw vector (`VALUES`): Top-K, EF exploration factor, `FILTER` attribute expressions (syntax cheat-sheet + bracket/quote pairing validation), `TRUTH` exact mode (linear scan to verify HNSW recall quality); results sorted by similarity with colored score bars, one-click export to CSV / JSON
- **VLINKS neighbor graph** — visualize the HNSW graph inside the member detail drawer: radial layout by layer (L0 outermost), larger nodes = more similar, score-colored; click any neighbor to redraw around it — an intuitive read on vector-index connectivity
- **Bulk vector import** — both JSONL (`{"element","vector":[...],"attrs"?}`, accepting id/values/embedding aliases) and CSV (element first column + component columns, optional header and attrs column); 200-row pipelined writes, live progress bar, quantization auto-adapted from `VINFO`, per-row failure details
- **Member management** — `VADD` to add/remove members (FP32 vector JSON + `SETATTR` JSON attributes + NOQUANT/Q8/BIN quantization options, auto-adapting to the set's existing quantization), `VRANDMEMBER` random sampling for browsing (vector sets have no iteration command)
- **Vector & attribute inspection** — `VEMB` shows approximate vectors (dequantized values when quantized, one-click copy), `VGETATTR`/`VSETATTR` for live JSON attribute editing
- **Overview** — `VINFO` badges for dimensions / quantization type / element count / HNSW parameters, `VCARD` counting; on Redis 8+ connections, new keys can be created as type vectorset (auto-hidden on older versions)

### 🌐 Redis Cluster

- **Cluster connections** — tick "Cluster mode" in the connection profile to connect via ClusterClient (any node address works; topology auto-discovered), db 0 only; TLS/SSH tunneling fully reused from standalone configs
- **Aggregated all-node scanning** — key browsing automatically aggregates all master nodes (the backend keeps per-node SCAN cursors and connects directly, avoiding client-side topology races); key counts are the cluster-wide DBSIZE sum
- **Cluster view** — a dedicated "Cluster" panel: cluster_state / slots_assigned / node count / key count overview; node table with role (master/replica), slot ranges and counts, link state, and current-node marker
- **Hash slot lookup** — type any key in the panel to compute `CLUSTER KEYSLOT`; essential for scripting and hot-slot troubleshooting
- **Cross-slot compatibility** — batch deletes are automatically split into per-key routed operations, sidestepping cluster CROSSSLOT limits
- **Known limitations**: MONITOR and CLI completion cover a single node only; cross-slot RENAME fails (Redis cluster semantics)

### 🛰️ Redis Sentinel

- **Sentinel connections** — tick "Sentinel mode", fill in the master name + sentinel addresses (first host:port, more can be appended); go-redis FailoverClient discovers the master automatically
- **Automatic failover following** — when the master switches, the client reconnects to the new master with no config changes; multi-db support identical to standalone
- **Config reuse** — TLS/mTLS and SSH tunneling fully reused (both sentinel discovery and data connections go through the same TLS/tunnel)
- **Known limitations**: sentinel-side authentication (SentinelUsername/Password) not yet supported; you connect to the master, so sentinel node state is not shown in the monitoring dashboard

### 🐌 Slow Log & Pub/Sub

- **SLOWLOG** — slow query list (duration/command/time/client), threshold explanation, one-click clear
- **LATENCY events** — `LATENCY LATEST` snapshot table (event/last occurrence/last latency/historical peak, graded coloring) + `LATENCY HISTORY` sampling curves (up to 160 points); complements the slow log for pinpointing command/IO/fork spikes
- **Pub/Sub** — subscribe to any channel with a live scrolling message stream; pattern subscriptions (psubscribe) supported
- **Channel insights** — `PUBSUB CHANNELS + NUMSUB` active-channel list with subscriber counts; click a channel to subscribe or set it as the publish target; Redis 7+ shard channels (SHARDCHANNELS) shown alongside
- **Keyspace event stream** — visual `notify-keyspace-events` flag configuration (K/E/g/$/l/s/h/z/x/e/t/d/n/m checkboxes + Off/KEA/Ex presets, saved via CONFIG SET), one-click subscribe to `__keyevent@db__:*`, events colored by type (expired/evicted red, rename purple…), with key filter, event-type filter, and pause

### 🛠️ Server Administration

- **Replication topology** — `INFO replication` visualized: master view lists downstream replicas (address/state/lag seconds/behind-bytes/offset); replica view shows the upstream link (state/last I/O/syncing), plus backlog buffer usage; graded lag coloring
- **Runtime configuration** — searchable `CONFIG GET` table (20+ common parameters with descriptions), graphical `CONFIG SET` (high-risk parameters get a second confirmation), one-click `CONFIG REWRITE` persistence
- **ACL user management** — user list (status/keys/commands/channels/password count), drawer-based rule editing: on/off, nopass, password set/clear, key patterns, command categories, channel patterns + extra raw rules, live final-rule preview; strong confirmation when modifying the default user
- **Scripts & functions** — Lua script cache panel: `SCRIPT LIST` browsing (Redis 8.2+) + `SCRIPT LOAD` online loading (returns sha1) + `FLUSH`/`KILL`; Redis Functions panel (Redis 7+): library list (engine/function count/flags), source viewing, `FUNCTION LOAD [REPLACE]` registration, `FUNCTION STATS` running-function monitoring with `KILL`, library deletion
- **Memory Doctor** — `MEMORY STATS` visualized: overview cards (used/peak/RSS/fragmentation with graded coloring/key count and average bytes per key), dataset-vs-overhead composition bar, overhead breakdown (startup/backlog/client buffers/AOF buffer/hashtable & expiry overhead/per-DB overhead), allocator stats (allocated/active/resident); `MEMORY DOCTOR` diagnosis card (healthy/issues-found marker); one-click `MEMORY PURGE` to return dirty pages; one-click JSON diagnostic report export
- **Known limitations**: not supported in cluster mode (config/ACL don't sync across nodes — use the cluster view for replication topology, or operate per-node via CLI); works on the master in sentinel mode

### 📦 Data Migration

- **Export selected keys** — export checked keys to JSON (DUMP serialization, lossless for all types including binary and TTL) or CSV (tabular view)
- **Full-DB export** — streaming JSONL export (one key per line) with live progress; the UI stays usable during large exports
- **Cross-instance import** — RESTORE REPLACE into any connection/DB, with confirmation before overwriting same-name keys
- **Cross-connection copy** — one click in the value panel copies the current key to a specified DB on another connection, supporting rename and TTL preservation

### 🛡️ Production Safety

- **Read-only connection mode** — tick "Read-only" in the connection profile and the client layer blocks **all write commands** (SET/DEL/EVAL/CONFIG SET/FLUSHDB… including module writes and container write-subcommands): UI editor write buttons are disabled, the CLI rejects writes with "read-only mode", pipelines containing writes abort entirely, and data migration/import is blocked too. Covers every write path (go-redis Hook) — it's an anti-fat-finger guardrail (unknown commands pass by default), not a server-side security boundary; pair it with server-side ACL `+@read`
- **Environment tag coloring** — mark connections as production (red) / test (orange) / dev (green): sidebar items, tabs, and view headers all show the colored badge, so "this is production" is unmistakable with many instances open
- **Write-command audit** — when auditing is enabled on a connection, every write command (with arguments, truncated at 300 chars) is appended to a local JSONL log (`config-dir/zp-rdm/audit/audit-<date>.jsonl`), rotating daily; audit-write failures are silently ignored and never block commands
- **Audit log viewer** — one click in the sidebar footer: filter by day/connection, keyword-search commands and arguments, dangerous commands (FLUSHALL/DEL/EVAL…) highlighted, one-click export of the filtered view to JSONL

### 🔐 Security & Engineering

- **Encrypted passwords on all platforms** — connection passwords, SSH private keys/passwords/passphrases stored encrypted (bound to user+machine — copying the config file elsewhere can't decrypt them): Windows DPAPI / macOS Keychain / Linux Secret Service (AES-256-GCM self-contained ciphertext, master key in the system keyring); legacy plaintext configs auto-migrate on startup
- **TLS/SSL connections** — skip verification (self-signed scenarios), strict verification with a custom CA certificate, and mutual TLS (mTLS client cert/key); private keys are encrypted at rest with the other sensitive fields
- **SSH tunneling** — connections support SSH jump hosts (password / private key / passphrase authentication)
- **Workspace session restore** — after a restart, automatically restore: open connection tabs, the active tab, per-connection view (data/monitor/CLI…), selected key, match pattern, tree/list view, key delimiter, and last-used DB
- **Window state memory** — size, position, maximized state, sidebar width (draggable 160–480px), and dark/light theme all persist
- **Command palette** — `Ctrl+K` to summon; fuzzy-search connections/views/actions, full keyboard flow
- **Auto-update** — check for new versions in the About panel; Windows supports in-app automatic install: download (**official source + public mirrors racing**, with progress and sha256 verification — mirrors can only accelerate, never tamper; slow/dead sources drop out automatically) → exit → one UAC prompt for an elevated silent install to the original directory (custom install locations preserved) → automatic restart into the new version (declining UAC falls back to launching the old version); falls back to the API download channel when github.com is unreachable. macOS / Linux still download via browser (brew / package manager recommended). Note: auto-install targets the NSIS-installed build; a portable exe gets replaced by the newly installed instance
- **Multilingual UI** — one click in the sidebar footer switches between 简体中文 / 繁體中文 / English / 日本語; the choice persists and first launch follows the system language; Ant Design component texts switch in sync (Redis error messages from the server stay in their original language)
- **Connection latency awareness** — open connections show live PING round-trip time in the sidebar (5s refresh, <50ms green / <200ms amber / higher red), network jitter visible at a glance
- **Connection group management** — right-click a group header to rename the whole group (open connections stay online), the connection menu's "Move to Group" offers existing groups or creating a new one, group collapse state survives restarts, and the connection form's group field auto-completes existing group names
- **Polished UI** — frameless window with custom title bar, dark/light themes, right-click menu disabled, text selection disabled (except terminal areas)

---

## Shortcuts

| Key | Action |
|---|---|
| `Ctrl+K` | Command palette |
| `Ctrl+N` | New connection |
| `Ctrl+W` | Close current tab |
| `F5` / `Ctrl+R` | Refresh current view |
| `↑` / `↓` | Browse command history in CLI |

---

## Tech Stack

- **Backend**: Go 1.21+ / Wails v2.16 / go-redis v9 / DPAPI (CryptoProtectData)
- **Frontend**: Vue 3 / TypeScript / Pinia / Ant Design Vue 4 / CodeMirror 6
- **Testing**: miniredis (in-memory Redis stub) + real-Redis integration tests (CI service container)
- **Release**: GitHub Actions three-platform builds (Windows NSIS / macOS universal app / Linux tar.gz → Release + multi-platform update manifest)

## Development

```bash
# Requirements: Go 1.21+ / Node 20+ / Wails CLI v2.16
go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0

wails dev          # dev mode (frontend hot reload)
wails build        # build build/bin/zp-rdm.exe
wails build -nsis  # build the NSIS installer (requires winget install NSIS.NSIS)
go test ./...      # unit tests (miniredis, no real Redis needed)
```

Integration tests (for commands miniredis doesn't support, e.g. DUMP/RESTORE) need a real Redis:

```bash
ZP_RDM_TEST_REDIS_ADDR=localhost:6379 go test ./...
```

RedisJSON / RediSearch / TimeSeries / Bloom integration tests need a module-enabled instance (Redis Stack, or Redis 8 with redisearch + rejson + redisbloom + redistimeseries loaded):

```bash
ZP_RDM_TEST_JSON_ADDR=localhost:6380 go test ./...
# TimeSeries / Bloom can also use a dedicated address (falls back to ZP_RDM_TEST_JSON_ADDR):
ZP_RDM_TEST_STACK_ADDR=localhost:6380 go test ./...
```

> Tests skip automatically when the corresponding env var is unset — the rest of the suite is unaffected.

## Release

Pushing a `v*` tag triggers GitHub Actions: full test suite → parallel three-platform builds (Windows NSIS + portable exe / macOS universal zip / Linux tar.gz + .deb) → automatic Release creation (including the `latest.json` update manifest with per-platform download URLs).

Once the Release assets are ready, run `scripts/update-channels.sh vX.Y.Z` to sync the distribution channels: update the [scoop-bucket](https://github.com/Cearl-gzy/scoop-bucket) and [homebrew-tap](https://github.com/Cearl-gzy/homebrew-tap) manifests (sha256 computed and pushed automatically), and generate winget-pkgs manifests under `packaging/winget/<version>/` (submitted to `manifests/c/Cearl-gzy/zp-rdm/<version>/` in microsoft/winget-pkgs).

> The tag version must match `info.productVersion` in `wails.json` (CI validates this and fails on mismatch).

## Data & Security

- Config directory: Windows `%AppData%/zp-rdm/`, macOS/Linux `~/.config/zp-rdm/` (`connections.json` + `settings.json`)
- Connection passwords, SSH private keys/passwords/passphrases, TLS client private keys: encrypted at rest on all platforms (Windows **DPAPI** / macOS Keychain / Linux Secret Service, falling back to a 0600 key file when no master key is available); kept as plaintext in memory
- Legacy plaintext configs auto-migrate to ciphertext on startup
- ⚠️ Connection **export files are plaintext** (portability by design) — keep them safe
- ⚠️ CLI command history is stored as plaintext in `settings.json`, including full executed commands
