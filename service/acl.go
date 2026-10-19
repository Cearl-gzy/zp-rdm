package service

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"zp-rdm/internal/model"
)

// ACL 用户管理（Redis >= 6.0）。集群模式不支持（ACL 不经集群总线传播，
// 逐节点操作请走 CLI）；哨兵模式作用在 master 上，可用。

// errClusterUnsupported 集群模式不支持的服务器管理操作
var errClusterUnsupported = errors.New("集群模式暂不支持该操作（配置/ACL 不跨节点同步，请在 CLI 中逐节点操作）")

// AclUsers 列出全部用户名（ACL USERS）
func (s *ServerService) AclUsers(connID string, db int) ([]string, error) {
	if s.manager.IsCluster(connID) {
		return nil, errClusterUnsupported
	}
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	res, err := c.Do(s.ctx, "ACL", "USERS").Result()
	if err != nil {
		return nil, err
	}
	list, ok := res.([]interface{})
	if !ok {
		return nil, fmt.Errorf("ACL USERS 返回格式异常: %T", res)
	}
	out := make([]string, 0, len(list))
	for _, it := range list {
		if name, ok := it.(string); ok {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out, nil
}

// AclGetUser 读取单个用户（ACL GETUSER），RESP2 扁平数组 / RESP3 map 双协议解析
func (s *ServerService) AclGetUser(connID string, db int, name string) (model.AclUserInfo, error) {
	if s.manager.IsCluster(connID) {
		return model.AclUserInfo{}, errClusterUnsupported
	}
	c, err := s.cli(connID, db)
	if err != nil {
		return model.AclUserInfo{}, err
	}
	res, err := c.Do(s.ctx, "ACL", "GETUSER", name).Result()
	if err != nil {
		return model.AclUserInfo{}, err
	}
	kv, err := respToStringMap(res)
	if err != nil {
		return model.AclUserInfo{}, fmt.Errorf("ACL GETUSER 解析失败: %w", err)
	}
	info := model.AclUserInfo{Name: name}
	for _, f := range respStringList(kv["flags"]) {
		switch f {
		case "on":
			info.Enabled = true
		case "nopass":
			info.NoPass = true
		}
	}
	info.Passwords = len(respStringList(kv["passwords"]))
	info.Commands, _ = kv["commands"].(string)
	info.Keys, _ = kv["keys"].(string)
	info.Channels, _ = kv["channels"].(string)
	return info, nil
}

// AclSetUser 应用规则（ACL SETUSER name rules...）。rules 至少一条。
func (s *ServerService) AclSetUser(connID string, db int, name string, rules []string) error {
	if s.manager.IsCluster(connID) {
		return errClusterUnsupported
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("用户名不能为空")
	}
	var clean []string
	for _, r := range rules {
		if r = strings.TrimSpace(r); r != "" {
			clean = append(clean, r)
		}
	}
	if len(clean) == 0 {
		return errors.New("至少提供一条 ACL 规则")
	}
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	args := make([]interface{}, 0, len(clean)+3)
	args = append(args, "ACL", "SETUSER", name)
	for _, r := range clean {
		args = append(args, r)
	}
	return c.Do(s.ctx, args...).Err()
}

// AclDelUser 删除用户（ACL DELUSER）。删除 default 会被 Redis 拒绝（错误透传）。
func (s *ServerService) AclDelUser(connID string, db int, name string) error {
	if s.manager.IsCluster(connID) {
		return errClusterUnsupported
	}
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	return c.Do(s.ctx, "ACL", "DELUSER", name).Err()
}

// respToStringMap 把 RESP2 扁平 kv 数组或 RESP3 map 归一成 map[string]interface{}
func respToStringMap(res interface{}) (map[string]interface{}, error) {
	out := map[string]interface{}{}
	switch v := res.(type) {
	case map[interface{}]interface{}: // RESP3
		for k, val := range v {
			ks, ok := k.(string)
			if !ok {
				continue
			}
			out[ks] = val
		}
	case []interface{}: // RESP2 扁平数组 k1 v1 k2 v2 …
		for i := 0; i+1 < len(v); i += 2 {
			ks, ok := v[i].(string)
			if !ok {
				continue
			}
			out[ks] = v[i+1]
		}
	default:
		return nil, fmt.Errorf("未知返回类型 %T", res)
	}
	return out, nil
}

func respStringList(v interface{}) []string {
	list, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, it := range list {
		if s, ok := it.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
