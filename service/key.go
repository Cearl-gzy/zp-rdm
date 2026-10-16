package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/redis/go-redis/v9"

	"zp-rdm/internal/model"
	"zp-rdm/internal/redisx"
)

// KeyService 键管理
type KeyService struct {
	ctx     context.Context
	manager *redisx.Manager
	scanMu  sync.Mutex
	scans   map[string]*clusterScanState // 集群全节点扫描进度（按 connID）
}

// clusterScanState 集群扫描状态：每个 master 节点一个游标 + 直连客户端。
// 不用 go-redis ForEachMaster 遍历：新客户端首次加载集群状态时偶发漏节点且
// 部分状态会跨调用缓存，导致漏扫。改为从 CLUSTER NODES 拿全量 master 地址，
// 逐节点直连扫描（SCAN 只扫本节点 keyspace，不经槽位路由）。
type clusterScanState struct {
	cursors map[string]uint64        // nodeAddr → SCAN 游标
	masters []string                 // 全量 master 地址（CLUSTER NODES）
	nodes   map[string]*redis.Client // nodeAddr → 直连客户端
}

func (st *clusterScanState) close() {
	for _, c := range st.nodes {
		_ = c.Close()
	}
}

func NewKeyService(m *redisx.Manager) *KeyService {
	return &KeyService{manager: m, scans: make(map[string]*clusterScanState)}
}

func (s *KeyService) Startup(ctx context.Context) { s.ctx = ctx }

func (s *KeyService) cli(connID string, db int) (redis.UniversalClient, error) {
	return s.manager.Get(connID, db)
}

// scanBatch 单轮 SCAN（带 TYPE 选项，老版本退化为扫描后过滤）
func scanBatch(ctx context.Context, c redis.UniversalClient, cursor uint64, pattern string, count int64, typ string) ([]string, uint64, error) {
	if typ != "" {
		batch, next, err := c.ScanType(ctx, cursor, pattern, count, typ).Result()
		if err == nil {
			return batch, next, nil
		}
	}
	batch, next, err := c.Scan(ctx, cursor, pattern, count).Result()
	return batch, next, err
}

// scanStandalone 单机扫描：迭代直到凑够一页或游标归零，最多 20 轮
func scanStandalone(ctx context.Context, c redis.UniversalClient, req model.ScanRequest) ([]string, uint64, error) {
	var keys []string
	cursor := req.Cursor
	for i := 0; i < 20; i++ {
		batch, next, err := scanBatch(ctx, c, cursor, req.Pattern, req.Count, req.Type)
		if err != nil {
			return nil, 0, err
		}
		cursor = next
		keys = append(keys, batch...)
		if cursor == 0 || len(keys) >= int(req.Count) {
			break
		}
	}
	return keys, cursor, nil
}

// scanCluster 集群扫描：聚合所有 master 节点。
// 游标语义对前端保持兼容：0 开始/结束，非 0 继续；各节点游标保存在服务端。
// 每轮对每个未完成节点扫描一次，凑够一页或全部完成时返回。
func (s *KeyService) scanCluster(ctx context.Context, cc *redis.ClusterClient, req model.ScanRequest) ([]string, uint64, error) {
	s.scanMu.Lock()
	if req.Cursor == 0 {
		if old := s.scans[req.ConnID]; old != nil {
			old.close()
		}
		delete(s.scans, req.ConnID) // 新一轮扫描
	}
	st, ok := s.scans[req.ConnID]
	if !ok {
		st = &clusterScanState{
			cursors: make(map[string]uint64),
			nodes:   make(map[string]*redis.Client),
		}
		s.scans[req.ConnID] = st
	}
	masters := st.masters
	s.scanMu.Unlock()

	if masters == nil {
		// 首轮：从 CLUSTER NODES 解析全量 master 地址
		if raw, err := cc.Do(ctx, "CLUSTER", "NODES").Text(); err == nil {
			for _, n := range parseClusterNodes(raw) {
				if n.Role == "master" {
					masters = append(masters, n.Addr)
				}
			}
		}
		if masters == nil {
			masters = []string{} // 解析失败：退化为空（下轮重试），避免误标完成
		}
		s.scanMu.Lock()
		st.masters = masters
		s.scanMu.Unlock()
	}

	var keys []string
	done := len(masters) > 0
	for _, addr := range masters {
		s.scanMu.Lock()
		cur, seen := st.cursors[addr]
		nc, hasClient := st.nodes[addr]
		s.scanMu.Unlock()
		if seen && cur == 0 {
			continue // 该节点已扫完
		}
		if !hasClient {
			// 直连节点客户端：复用集群客户端的认证/TLS/SSH 拨号配置
			opt := cc.Options()
			nc = redis.NewClient(&redis.Options{
				Addr:         addr,
				Username:     opt.Username,
				Password:     opt.Password,
				DialTimeout:  opt.DialTimeout,
				ReadTimeout:  opt.ReadTimeout,
				WriteTimeout: opt.WriteTimeout,
				TLSConfig:    opt.TLSConfig,
				Dialer:       opt.Dialer,
			})
			s.scanMu.Lock()
			st.nodes[addr] = nc
			s.scanMu.Unlock()
		}
		batch, next, err := scanBatch(ctx, nc, cur, req.Pattern, req.Count, req.Type)
		if err != nil {
			return nil, 0, err
		}
		s.scanMu.Lock()
		st.cursors[addr] = next
		s.scanMu.Unlock()
		keys = append(keys, batch...)
		if next != 0 {
			done = false
		}
	}
	if done {
		s.scanMu.Lock()
		st.close()
		delete(s.scans, req.ConnID)
		s.scanMu.Unlock()
		return keys, 0, nil
	}
	return keys, 1, nil
}

// clusterDBSize 集群键数估算：汇总全部 master 节点的 DBSIZE
func clusterDBSize(ctx context.Context, cc *redis.ClusterClient) int64 {
	var total int64
	_ = cc.ForEachMaster(ctx, func(nctx context.Context, nc *redis.Client) error {
		if n, err := nc.DBSize(nctx).Result(); err == nil {
			total += n
		}
		return nil
	})
	return total
}

// ScanKeys 扫描键（游标分页；集群模式聚合全部 master 节点）
func (s *KeyService) ScanKeys(req model.ScanRequest) (*model.ScanResult, error) {
	c, err := s.cli(req.ConnID, req.DB)
	if err != nil {
		return nil, err
	}
	if req.Count <= 0 {
		req.Count = 500
	}
	if req.Pattern == "" {
		req.Pattern = "*"
	}
	ctx := s.ctx
	var keys []string
	var cursor uint64
	if cc, ok := c.(*redis.ClusterClient); ok {
		keys, cursor, err = s.scanCluster(ctx, cc, req)
	} else {
		keys, cursor, err = scanStandalone(ctx, c, req)
	}
	if err != nil {
		return nil, err
	}

	// 管道取 TYPE / TTL / MEMORY USAGE
	pipe := c.Pipeline()
	typeCmds := make([]*redis.StatusCmd, len(keys))
	ttlCmds := make([]*redis.DurationCmd, len(keys))
	memCmds := make([]*redis.Cmd, len(keys))
	for i, k := range keys {
		typeCmds[i] = pipe.Type(ctx, k)
		ttlCmds[i] = pipe.TTL(ctx, k)
		memCmds[i] = pipe.Do(ctx, "MEMORY", "USAGE", k)
	}
	_, _ = pipe.Exec(ctx)

	infos := make([]model.KeyInfo, 0, len(keys))
	for i, k := range keys {
		t := typeCmds[i].Val()
		if t == "" || t == "none" {
			continue
		}
		if req.Type != "" && t != req.Type {
			continue
		}
		size := int64(-1)
		if v, err := memCmds[i].Int64(); err == nil {
			size = v
		}
		infos = append(infos, model.KeyInfo{Key: k, Type: t, TTL: ttlSeconds(ttlCmds[i].Val()), Size: size})
	}

	var total int64
	if cc, ok := c.(*redis.ClusterClient); ok {
		total = clusterDBSize(ctx, cc)
	} else {
		total, _ = c.DBSize(ctx).Result()
	}
	return &model.ScanResult{Cursor: cursor, Keys: infos, Total: total}, nil
}

// DeleteKeys 批量删除
func (s *KeyService) DeleteKeys(connID string, db int, keys []string) (int64, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return 0, err
	}
	if len(keys) == 0 {
		return 0, nil
	}
	if _, ok := c.(*redis.ClusterClient); ok {
		// 集群不允许跨槽多键 DEL，逐键管道（非事务管道按槽自动路由）
		pipe := c.Pipeline()
		cmds := make([]*redis.IntCmd, 0, len(keys))
		for _, k := range keys {
			cmds = append(cmds, pipe.Del(s.ctx, k))
		}
		if _, err := pipe.Exec(s.ctx); err != nil {
			return 0, err
		}
		var n int64
		for _, cmd := range cmds {
			n += cmd.Val()
		}
		return n, nil
	}
	return c.Del(s.ctx, keys...).Result()
}

// RenameKey 重命名
func (s *KeyService) RenameKey(connID string, db int, key, newKey string) error {
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	return c.Rename(s.ctx, key, newKey).Err()
}

// CopyKey 复制键（Redis 6.2+）
func (s *KeyService) CopyKey(connID string, db int, key, newKey string) error {
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	ok, err := c.Copy(s.ctx, key, newKey, 0, false).Result()
	if err != nil {
		return err
	}
	if ok == 0 {
		return redis.Nil
	}
	return nil
}

// SetTTL 设置过期秒数（-1 移除过期）
func (s *KeyService) SetTTL(connID string, db int, key string, seconds int64) error {
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	if seconds < 0 {
		return c.Persist(s.ctx, key).Err()
	}
	return c.Expire(s.ctx, key, durationSeconds(seconds)).Err()
}

// MemoryUsage 键内存占用
func (s *KeyService) MemoryUsage(connID string, db int, key string) (int64, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return -1, err
	}
	v, err := c.Do(s.ctx, "MEMORY", "USAGE", key).Int64()
	if err != nil {
		return -1, nil
	}
	return v, nil
}

// KeyExists 键是否存在
func (s *KeyService) KeyExists(connID string, db int, key string) (bool, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return false, err
	}
	n, err := c.Exists(s.ctx, key).Result()
	return n > 0, err
}

// BatchSetTTL 批量设置 TTL；seconds < 0 表示清除 TTL（PERSIST，与 SetTTL 语义一致）。返回成功数量
func (s *KeyService) BatchSetTTL(connID string, db int, keys []string, seconds int64) (int64, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return 0, err
	}
	pipe := c.Pipeline()
	cmds := make([]*redis.BoolCmd, 0, len(keys))
	for _, k := range keys {
		if seconds < 0 {
			cmds = append(cmds, pipe.Persist(s.ctx, k))
		} else {
			cmds = append(cmds, pipe.Expire(s.ctx, k, durationSeconds(seconds)))
		}
	}
	// 部分键不存在不算失败，按返回的布尔统计
	if _, err = pipe.Exec(s.ctx); err != nil && err != redis.Nil {
		return 0, err
	}
	var n int64
	for _, cmd := range cmds {
		if cmd.Val() {
			n++
		}
	}
	return n, nil
}

// MoveKeys 批量移动键到目标 db（MOVE 语义：目标已存在同名键则跳过该键）。返回成功移动数量
func (s *KeyService) MoveKeys(connID string, db int, keys []string, targetDB int) (int64, error) {
	if targetDB < 0 {
		return 0, fmt.Errorf("目标 db 不能为负数")
	}
	if targetDB == db {
		return 0, fmt.Errorf("目标 db 与当前 db 相同")
	}
	c, err := s.cli(connID, db)
	if err != nil {
		return 0, err
	}
	pipe := c.Pipeline()
	cmds := make([]*redis.BoolCmd, 0, len(keys))
	for _, k := range keys {
		cmds = append(cmds, pipe.Move(s.ctx, k, targetDB))
	}
	if _, err = pipe.Exec(s.ctx); err != nil && err != redis.Nil {
		return 0, err
	}
	var n int64
	for _, cmd := range cmds {
		if cmd.Val() {
			n++
		}
	}
	return n, nil
}

// suggestMax 自动补全返回的最大键数
const suggestMax = 10

// SuggestKeys 用于 CLI 自动补全：返回匹配前缀的键（最多 suggestMax 个，排序去重）
func (s *KeyService) SuggestKeys(connID string, db int, prefix string) ([]string, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	pattern := globEscape(prefix) + "*"
	var cursor uint64
	seen := make(map[string]struct{})
	out := []string{}
	for i := 0; i < suggestMax && len(out) < suggestMax; i++ {
		var batch []string
		batch, cursor, err = c.Scan(s.ctx, cursor, pattern, 100).Result()
		if err != nil {
			return nil, err
		}
		for _, k := range batch {
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			out = append(out, k)
			if len(out) >= suggestMax {
				break
			}
		}
		if cursor == 0 {
			break
		}
	}
	sort.Strings(out)
	return out, nil
}

// globEscape 转义 Redis glob 元字符，避免补全前缀被当作模式
func globEscape(s string) string {
	return strings.NewReplacer(
		`\`, `\\`, `*`, `\*`, `?`, `\?`, `[`, `\[`, `]`, `\]`,
	).Replace(s)
}
