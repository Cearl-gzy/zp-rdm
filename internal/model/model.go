package model

// SSHConfig SSH 隧道配置
type SSHConfig struct {
	Enabled    bool   `json:"enabled"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Username   string `json:"username"`
	AuthType   string `json:"authType"` // password | key
	Password   string `json:"password"`
	PrivateKey string `json:"privateKey"` // 私钥路径或私钥内容
	Passphrase string `json:"passphrase"`
}

// ConnectionProfile 连接配置
type ConnectionProfile struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Group    string    `json:"group"`
	Host     string    `json:"host"`
	Port     int       `json:"port"`
	Username string    `json:"username"`
	Password string    `json:"password"`
	UseTLS   bool      `json:"useTls"`
	// TLSSkipVerify 跳过服务端证书校验；nil（旧配置）按 true 处理，保持历史行为
	TLSSkipVerify *bool  `json:"tlsSkipVerify"`
	TLSCaCert     string `json:"tlsCaCert"`     // 自定义 CA 证书 PEM 内容（可选）
	TLSClientCert string `json:"tlsClientCert"` // mTLS 客户端证书 PEM（可选）
	TLSClientKey  string `json:"tlsClientKey"`  // mTLS 客户端私钥 PEM（可选，敏感，加密落盘）
	DB            int       `json:"db"`
	SSH           SSHConfig `json:"ssh"`
	Color         string    `json:"color"`
	Cluster       bool      `json:"cluster"` // Redis Cluster 模式（仅 db 0，多节点）
}

// ConnState 连接打开后的状态
type ConnState struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Group   string   `json:"group"`
	Color   string   `json:"color"`
	DBCount int      `json:"dbCount"`
	Version string   `json:"version"`
	Mode    string   `json:"mode"`
	Modules []string `json:"modules"` // 已加载模块名（MODULE LIST），如 ReJSON-RL / search
}

// KeyInfo 键信息
type KeyInfo struct {
	Key  string `json:"key"`
	Type string `json:"type"`
	TTL  int64  `json:"ttl"`
	Size int64  `json:"size"` // 字节，-1 表示未知
}

// ScanRequest 扫描请求
type ScanRequest struct {
	ConnID  string `json:"connId"`
	DB      int    `json:"db"`
	Cursor  uint64 `json:"cursor"`
	Pattern string `json:"pattern"`
	Count   int64  `json:"count"`
	Type    string `json:"type"` // 类型过滤，空为全部
}

// ScanResult 扫描结果
type ScanResult struct {
	Cursor uint64    `json:"cursor"`
	Keys   []KeyInfo `json:"keys"`
	Total  int64     `json:"total"` // dbsize（估算）
}

// ValueDetail 值详情（集合类型仅含概览，明细分页获取）
type ValueDetail struct {
	Key       string `json:"key"`
	Type      string `json:"type"`
	TTL       int64  `json:"ttl"`
	Size      int64  `json:"size"`
	Encoding  string `json:"encoding"`
	StrValue  string `json:"strValue"`
	Truncated bool   `json:"truncated"`
	Binary    bool   `json:"binary"`
	Length    int64  `json:"length"`
}

// HashItem hash 字段
type HashItem struct {
	Field string `json:"field"`
	Value string `json:"value"`
}

// DumpedKey 逻辑导出条目（DUMP 序列化，全类型/二进制无损）
type DumpedKey struct {
	Key  string `json:"key"`
	Type string `json:"type"`
	TTL  int64  `json:"ttl"` // 毫秒，-1 永久
	Dump string `json:"dump"`
}

// TopKey 大 key 排行条目
type TopKey struct {
	Key  string `json:"key"`
	Type string `json:"type"`
	Size int64  `json:"size"`
	TTL  int64  `json:"ttl"`
}

// TTLBucket 过期时间分布桶
type TTLBucket struct {
	Label string `json:"label"`
	Count int64  `json:"count"`
}

// PrefixStat 前缀聚合统计
type PrefixStat struct {
	Prefix string `json:"prefix"`
	Count  int64  `json:"count"`
	Size   int64  `json:"size"`
}

// KeyAnalysis 键空间分析结果
type KeyAnalysis struct {
	TotalKeys   int64        `json:"totalKeys"`
	TotalMemory int64        `json:"totalMemory"`
	Scanned     int64        `json:"scanned"`
	Truncated   bool         `json:"truncated"` // 达到扫描上限
	TopKeys     []TopKey     `json:"topKeys"`
	TTLBuckets  []TTLBucket  `json:"ttlBuckets"`
	Prefixes    []PrefixStat `json:"prefixes"`
}

// ListItem list 元素
type ListItem struct {
	Index int64  `json:"index"`
	Value string `json:"value"`
}

// ZSetItem zset 成员
type ZSetItem struct {
	Member string  `json:"member"`
	Score  float64 `json:"score"`
}

// ScanItems 带游标的集合元素
type ScanItems struct {
	Cursor uint64      `json:"cursor"`
	Total  int64       `json:"total"`
	Items  interface{} `json:"items"`
}

// StreamItem stream 条目
type StreamItem struct {
	ID     string            `json:"id"`
	Fields map[string]string `json:"fields"`
}

// StreamGroup stream 消费者组概览（XINFO GROUPS）
type StreamGroup struct {
	Name            string `json:"name"`
	Consumers       int64  `json:"consumers"`
	Pending         int64  `json:"pending"`
	LastDeliveredID string `json:"lastDeliveredId"`
	EntriesRead     int64  `json:"entriesRead"`
	Lag             int64  `json:"lag"`
}

// StreamConsumer 组内消费者（XINFO CONSUMERS）
type StreamConsumer struct {
	Name    string `json:"name"`
	Pending int64  `json:"pending"`
	IdleMs  int64  `json:"idleMs"`
}

// StreamPendingEntry 待确认（pending）条目
type StreamPendingEntry struct {
	ID            string `json:"id"`
	Consumer      string `json:"consumer"`
	IdleMs        int64  `json:"idleMs"`
	DeliveryCount int64  `json:"deliveryCount"`
}

// ClusterNode 集群节点（CLUSTER NODES 解析）
type ClusterNode struct {
	ID        string   `json:"id"`
	Addr      string   `json:"addr"`
	Role      string   `json:"role"` // master | slave
	MasterID  string   `json:"masterId"`
	LinkState string   `json:"linkState"`
	Slots     []string `json:"slots"` // 如 ["0-5460", "10000"]
	SlotCount int      `json:"slotCount"`
	Myself    bool     `json:"myself"`
}

// SlowLogEntry 慢日志条目
type SlowLogEntry struct {
	ID         int64    `json:"id"`
	Timestamp  int64    `json:"timestamp"`
	DurationUs int64    `json:"durationUs"`
	Args       []string `json:"args"`
	ClientAddr string   `json:"clientAddr"`
	ClientName string   `json:"clientName"`
}

// ClientConnInfo 客户端连接
type ClientConnInfo struct {
	ID    int64  `json:"id"`
	Addr  string `json:"addr"`
	Name  string `json:"name"`
	Age   int64  `json:"age"`
	Idle  int64  `json:"idle"`
	DB    int    `json:"db"`
	Cmd   string `json:"cmd"`
	Flags string `json:"flags"`
}

// PubSubMessage 推送消息
type PubSubMessage struct {
	Channel string `json:"channel"`
	Pattern string `json:"pattern"`
	Payload string `json:"payload"`
	Time    int64  `json:"time"`
	Kind    string `json:"kind"` // message | pmessage | subscribe | unsubscribe
	Count   int64  `json:"count"`
}

// CliResult CLI 执行结果
type CliResult struct {
	OK     bool        `json:"ok"`
	Type   string      `json:"type"` // status | error | integer | bulk | array | nil
	Value  interface{} `json:"value"`
	CostMs int64       `json:"costMs"`
}
