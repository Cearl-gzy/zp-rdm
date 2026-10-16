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

export type InfoMap = Record<string, Record<string, string>>
