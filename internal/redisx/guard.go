package redisx

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"zp-rdm/internal/model"
)

// 只读防护与写命令审计（生产安全套件）。
// 设计：在 go-redis 客户端挂 Hook，一处拦截覆盖全部写路径（UI 操作 / CLI / 管道 / 数据迁移），
// 不依赖各 service 记得检查。写命令判定用静态表（核心 @write + 容器命令子命令规则 +
// 常用模块命令），未知命令（未列出的模块命令）默认放行——只读是防手滑护栏，不是安全边界。

// writeCommands 整条命令级拦截表（小写命令名）
var writeCommands = map[string]bool{
	// string / key
	"set": true, "setnx": true, "setex": true, "psetex": true, "mset": true, "msetnx": true,
	"append": true, "incr": true, "incrby": true, "incrbyfloat": true, "decr": true, "decrby": true,
	"setrange": true, "getset": true, "getdel": true, "getex": true,
	"del": true, "unlink": true, "expire": true, "expireat": true, "pexpire": true, "pexpireat": true,
	"persist": true, "rename": true, "renamenx": true, "move": true, "swapdb": true, "copy": true,
	"restore": true, "restore-asking": true, "migrate": true, "sort": true, // SORT 含 STORE 选项，官方标记 @write
	// hash / list / set / zset
	"hset": true, "hsetnx": true, "hmset": true, "hincrby": true, "hincrbyfloat": true, "hdel": true,
	"lpush": true, "rpush": true, "lpushx": true, "rpushx": true, "lset": true, "linsert": true,
	"lrem": true, "ltrim": true, "lpop": true, "rpop": true, "rpoplpush": true, "lmove": true,
	"blpop": true, "brpop": true, "brpoplpush": true, "blmove": true, "lmpop": true, "blmpop": true,
	"sadd": true, "srem": true, "spop": true, "smove": true,
	"sinterstore": true, "sunionstore": true, "sdiffstore": true,
	"zadd": true, "zincrby": true, "zrem": true, "zremrangebyscore": true, "zremrangebyrank": true,
	"zremrangebylex": true, "zpopmin": true, "zpopmax": true, "bzpopmin": true, "bzpopmax": true,
	"zunionstore": true, "zinterstore": true, "zdiffstore": true, "zmpop": true, "bzmpop": true,
	"zrangestore": true,
	// bitmap / hyperloglog / geo
	"setbit": true, "bitop": true, "bitfield": true,
	"pfadd": true, "pfmerge": true,
	"geoadd": true, "geosearchstore": true,
	// stream
	"xadd": true, "xtrim": true, "xdel": true, "xgroup": true, "xack": true,
	"xclaim": true, "xautoclaim": true, "xsetid": true,
	// pub/sub 副作用（不改数据集，但会触发订阅方动作，只读连接一并拦截）
	"publish": true, "spublish": true,
	// 脚本 / 函数（可读回调用 *_RO 变体，已放行）
	"eval": true, "evalsha": true, "fcall": true,
	// 服务端管理
	"flushdb": true, "flushall": true, "save": true, "bgsave": true, "bgrewriteaof": true,
	"shutdown": true, "failover": true, "slaveof": true, "replicaof": true, "debug": true,
	// 常用模块写命令（应用内置 JSON/Search 编辑器必须覆盖）
	"json.set": true, "json.del": true, "json.mset": true, "json.merge": true, "json.forget": true,
	"json.arrappend": true, "json.arrinsert": true, "json.arrpop": true, "json.arrtrim": true,
	"json.clear": true, "json.numincrby": true, "json.nummultby": true, "json.strappend": true,
	"json.toggle": true, "json.debug": true,
	"ft.create": true, "ft.dropindex": true, "ft.alter": true, "ft.add": true, "ft.del": true,
	"ft.dictadd": true, "ft.dictdel": true, "ft.sugadd": true, "ft.sugdel": true, "ft.sugincr": true,
}

// containerWriteSub 容器命令按子命令拦截（命令 → 写子命令表）
var containerWriteSub = map[string]map[string]bool{
	"config":   {"set": true, "rewrite": true, "resetstat": true},
	"acl":      {"setuser": true, "deluser": true, "save": true, "load": true},
	"script":   {"load": true, "flush": true, "kill": true, "debug": true},
	"function": {"load": true, "delete": true, "flush": true, "restore": true, "kill": true},
	"slowlog":  {"reset": true},
	"client":   {"kill": true, "pause": true, "unpause": true},
	"cluster": {
		"meet": true, "forget": true, "replicate": true, "failover": true, "reset": true,
		"setslot": true, "addslots": true, "delslots": true, "flushslots": true,
		"bumpepoch": true, "set-config-epoch": true, "saveconfig": true,
	},
	"memory":  {"purge": true},
	"latency": {"reset": true},
	"module":  {"load": true, "unload": true, "loadex": true},
}

// IsWriteCommand 判定命令是否为写/副作用命令。name 为小写命令名（redis.Cmd.Name() 已小写），
// args 为命令名之后的参数。容器命令按首个参数（子命令）判定。
func IsWriteCommand(name string, args []string) bool {
	if writeCommands[name] {
		return true
	}
	if subs, ok := containerWriteSub[name]; ok {
		if len(args) == 0 {
			return false // 裸容器命令是语法错误，放行由服务端报错
		}
		return subs[strings.ToLower(args[0])]
	}
	return false
}

// cmdStringArgs 提取命令参数（跳过命令名），非字符串参数按 fmt 渲染
func cmdStringArgs(cmd redis.Cmder) []string {
	raw := cmd.Args()
	if len(raw) <= 1 {
		return nil
	}
	out := make([]string, 0, len(raw)-1)
	for _, a := range raw[1:] {
		switch v := a.(type) {
		case string:
			out = append(out, v)
		case []byte:
			out = append(out, string(v))
		default:
			out = append(out, fmt.Sprintf("%v", v))
		}
	}
	return out
}

// ErrReadonly 只读拦截错误（errors.Is 可判；service/UI 可据此给友好提示）
var errReadonlySentinel = fmt.Errorf("只读模式")

// guardHook 只读拦截 + 写命令审计（按连接配置启用）
type guardHook struct {
	connID   string
	connName string
	readonly bool
	audit    bool
}

func newGuardHook(p model.ConnectionProfile) guardHook {
	return guardHook{connID: p.ID, connName: p.Name, readonly: p.Readonly, audit: p.Audit}
}

// attachGuard 按连接配置为客户端挂防护钩子（只读与审计都关闭时零开销不挂载）
func attachGuard(c redis.UniversalClient, p model.ConnectionProfile) {
	if !p.Readonly && !p.Audit {
		return
	}
	c.AddHook(newGuardHook(p))
}

func (h guardHook) DialHook(next redis.DialHook) redis.DialHook { return next }

// check 命令准入检查：写命令且只读 → 拦截；写命令且审计 → 记录。返回拦截错误（nil 放行）
func (h guardHook) check(cmd redis.Cmder) error {
	name := cmd.Name()
	args := cmdStringArgs(cmd)
	if !IsWriteCommand(name, args) {
		return nil
	}
	if h.readonly {
		return fmt.Errorf("%w：连接「%s」已拦截写命令 %s", errReadonlySentinel, h.connName, strings.ToUpper(name))
	}
	if h.audit {
		writeAuditLine(h.connID, h.connName, name, args)
	}
	return nil
}

func (h guardHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if err := h.check(cmd); err != nil {
			cmd.SetErr(err)
			return err
		}
		return next(ctx, cmd)
	}
}

func (h guardHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		for _, cmd := range cmds {
			if err := h.check(cmd); err != nil {
				cmd.SetErr(err)
				return err // 管道含被拦截的写命令 → 整管中止
			}
		}
		return next(ctx, cmds)
	}
}

// ---------- 审计日志 ----------

// AuditDir 审计日志目录（app 启动时设为配置目录/zp-rdm/audit；测试可指向临时目录）
var AuditDir string

// auditEntry 单行审计记录（JSONL，按天分文件）
type auditEntry struct {
	Time string `json:"time"`
	Conn string `json:"conn"`
	ID   string `json:"connId"`
	Cmd  string `json:"cmd"`
	Args string `json:"args"` // 渲染后的完整参数（截断 300 字符）
}

var auditMu sync.Mutex

func auditDirPath() string {
	if AuditDir != "" {
		return AuditDir
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "zp-rdm", "audit")
}

// writeAuditLine 追加一条写命令审计（按天滚动文件；写失败静默，绝不影响命令执行）
func writeAuditLine(connID, connName, cmd string, args []string) {
	dir := auditDirPath()
	if dir == "" {
		return
	}
	rendered := strings.Join(args, " ")
	if len(rendered) > 300 {
		rendered = rendered[:300] + "…"
	}
	b, err := json.Marshal(auditEntry{
		Time: time.Now().Format("2006-01-02 15:04:05.000"),
		Conn: connName, ID: connID,
		Cmd: strings.ToUpper(cmd), Args: rendered,
	})
	if err != nil {
		return
	}
	auditMu.Lock()
	defer auditMu.Unlock()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	path := filepath.Join(dir, "audit-"+time.Now().Format("2006-01-02")+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close() //nolint:errcheck
	_, _ = f.Write(append(b, '\n'))
}
