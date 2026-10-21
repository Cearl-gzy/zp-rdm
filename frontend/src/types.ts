// 与后端 model 对应的 TS 类型（结构化匹配 wails 生成的类）
export interface SSHConfig {
  enabled: boolean
  host: string
  port: number
  username: string
  authType: string
  password: string
  privateKey: string
  passphrase: string
}

export interface ConnectionProfile {
  id: string
  name: string
  group: string
  host: string
  port: number
  username: string
  password: string
  useTls: boolean
  tlsSkipVerify?: boolean | null // 旧配置缺省视为 true（历史行为）
  tlsCaCert: string // 自定义 CA 证书 PEM 内容
  tlsClientCert: string // mTLS 客户端证书 PEM
  tlsClientKey: string // mTLS 客户端私钥 PEM（加密落盘）
  db: number
  ssh: SSHConfig
  color: string
  cluster?: boolean // Redis Cluster 模式
  sentinel?: boolean // Redis Sentinel 模式（host:port 为首个哨兵地址）
  masterName?: string // 哨兵监控的 master 名称
  sentinelAddrs?: string // 额外哨兵地址（每行或逗号分隔 host:port）
  readonly?: boolean // 只读模式：客户端层拦截全部写命令
  env?: string // 环境标签：'' | prod | test | dev（界面着色）
  audit?: boolean // 写命令审计（记录到配置目录 audit/*.jsonl）
}

export interface ConnState {
  id: string
  name: string
  group: string
  color: string
  dbCount: number
  version: string
  mode: string
  modules: string[] // 已加载模块名（如 ReJSON-RL、search）
  readonly?: boolean // 只读模式（界面禁用写操作）
  env?: string // 环境标签（prod/test/dev，界面着色）
}

export interface KeyInfo {
  key: string
  type: string
  ttl: number
  size: number
}

export interface ScanResult {
  cursor: number
  keys: KeyInfo[]
  total: number
}

export interface ValueDetail {
  key: string
  type: string
  ttl: number
  size: number
  encoding: string
  strValue: string
  truncated: boolean
  binary: boolean
  length: number
}

export interface TopKey { key: string; type: string; size: number; ttl: number }
export interface TTLBucket { label: string; count: number }
export interface PrefixStat { prefix: string; count: number; size: number }
export interface KeyAnalysis {
  totalKeys: number
  totalMemory: number
  scanned: number
  truncated: boolean
  topKeys: TopKey[]
  ttlBuckets: TTLBucket[]
  prefixes: PrefixStat[]
}

export interface BigKeyEntry { key: string; type: string; size: number; len: number; ttl: number }
export interface BigKeyTypeStat { type: string; count: number; size: number }
export interface BigKeyReport {
  totalKeys: number
  scanned: number
  elapsedMs: number
  truncated: boolean
  top: BigKeyEntry[]
  types: BigKeyTypeStat[]
}

export interface HotKeyFreqEntry { key: string; type: string; freq: number }
export interface HotKeyFreqReport {
  policy: string
  available: boolean
  scanned: number
  keys: HotKeyFreqEntry[]
}
export interface HotKeyEntry { key: string; hits: number; reads: number; writes: number }
export interface HotKeySampleReport {
  durationSec: number
  totalCmds: number
  uniqueKeys: number
  truncated: boolean
  keys: HotKeyEntry[]
}
export interface HotKeySampleStats {
  running: boolean
  elapsedSec: number
  totalCmds: number
  uniqueKeys: number
}

export interface ReplicaInfo { addr: string; state: string; offset: number; lag: number }
export interface ReplicationInfo {
  role: string
  replicas: ReplicaInfo[]
  offset: number
  backlogActive: boolean
  backlogSize: number
  backlogHistlen: number
  masterAddr: string
  linkStatus: string
  lastIOSeconds: number
  syncInProgress: boolean
}

export interface HashItem { field: string; value: string }
export interface ListItem { index: number; value: string }
export interface ZSetItem { member: string; score: number }
export interface StreamItem { id: string; fields: Record<string, string> }
export interface StreamGroup {
  name: string
  consumers: number
  pending: number
  lastDeliveredId: string
  entriesRead: number
  lag: number
}
export interface StreamConsumer { name: string; pending: number; idleMs: number }
export interface StreamPendingEntry { id: string; consumer: string; idleMs: number; deliveryCount: number }

export interface ClusterNode {
  id: string
  addr: string
  role: string // master | slave
  masterId: string
  linkState: string
  slots: string[]
  slotCount: number
  myself: boolean
}

export interface ScanItems {
  cursor: number
  total: number
  items: any
}

export interface SlowLogEntry {
  id: number
  timestamp: number
  durationUs: number
  args: string[]
  clientAddr: string
  clientName: string
}

export interface ClientConnInfo {
  id: number
  addr: string
  name: string
  age: number
  idle: number
  db: number
  cmd: string
  flags: string
}

export interface PubSubMessage {
  channel: string
  pattern: string
  payload: string
  time: number
  kind: string
  count: number
}

export interface CliResult {
  ok: boolean
  type: string
  value: any
  costMs: number
}

export interface CommandDoc {
  name: string
  summary: string
  since: string
  complexity: string
  syntax: string
}

export interface PubSubChannelInfo {
  name: string
  subs: number
}

export interface PubSubInsight {
  channels: PubSubChannelInfo[]
  shard: PubSubChannelInfo[]
}

export interface LatencyEvent {
  name: string
  timestamp: number
  latestMs: number
  maxMs: number
}

export interface LatencySample {
  timestamp: number
  ms: number
}

export interface VectorSimEntry {
  element: string
  score: number
}

export interface VectorLinkLayer {
  layer: number
  neighbors: VectorSimEntry[]
}

export interface VectorImportResult {
  total: number
  added: number
  failed: number
  errors: string[]
  truncated: boolean
}

export interface FunctionEntry {
  name: string
  desc: string
  flags: string[]
}

export interface FunctionLibrary {
  name: string
  engine: string
  functions: FunctionEntry[]
  code: string
}

export interface FunctionRunStat {
  running: boolean
  name: string
  command: string
  durationMs: number
}

export type InfoMap = Record<string, Record<string, string>>

export interface AclUserInfo {
  name: string
  enabled: boolean
  noPass: boolean
  passwords: number
  commands: string
  keys: string
  channels: string
}

export interface UpdateInfo {
  hasUpdate: boolean
  version: string
  url: string
  sha256: string
  auto: boolean
}

export interface TsSample {
  ts: number
  value: number
}

export interface TsRule {
  destKey: string
  bucketMs: number
  aggregator: string
}

export interface TsInfo {
  totalSamples: number
  memoryUsage: number
  firstTimestamp: number
  lastTimestamp: number
  retentionMs: number
  chunkCount: number
  labels: Record<string, string>
  rules: TsRule[]
}

export interface BfInfo {
  capacity: number
  size: number
  filters: number
  inserted: number
  expansion: number
}
