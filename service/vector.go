package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"zp-rdm/internal/model"
)

// Redis 8 Vector Set 操作。全部走裸 Do()：go-redis 对向量命令的封装版本
// 要求高且各小版本签名变动频繁，裸命令直观且行为稳定。
// 参考：https://redis.io/docs/latest/develop/data-types/vector-sets/

// VectorInfo VINFO：量化类型、维度、元素数、图参数等概览（键值对数组 → map）
func (s *ValueService) VectorInfo(connID string, db int, key string) (map[string]string, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	res, err := c.Do(s.ctx, "VINFO", key).Result()
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, 12)
	switch v := res.(type) {
	case map[interface{}]interface{}:
		// go-redis 将 VINFO 的 map 回复解析为 map
		for k, val := range v {
			out[fmt.Sprint(k)] = fmt.Sprint(val)
		}
	case []interface{}:
		for i := 0; i+1 < len(v); i += 2 {
			out[fmt.Sprint(v[i])] = fmt.Sprint(v[i+1])
		}
	default:
		return nil, fmt.Errorf("VINFO 返回格式异常: %T", res)
	}
	return out, nil
}

// VectorSample VRANDMEMBER 随机采样成员名（向量集合无 SCAN 类遍历命令）
func (s *ValueService) VectorSample(connID string, db int, key string, count int64) ([]string, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	return c.Do(s.ctx, "VRANDMEMBER", key, count).StringSlice()
}

// VectorSim VSIM 相似度查询。
// ele 非空按成员查询（VSIM key ELE ele）；否则按原始向量（VSIM key VALUES n v...）。
// k 返回条数（COUNT），ef 探索因子（0=默认），filter 属性过滤表达式（可空），
// truth 为真强制线性精确搜索（慢但无近似误差）。
func (s *ValueService) VectorSim(connID string, db int, key, ele string, values []float64, k, ef int64, filter string, truth bool) ([]model.VectorSimEntry, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	args := []interface{}{"VSIM", key}
	if ele != "" {
		args = append(args, "ELE", ele)
	} else {
		if len(values) == 0 {
			return nil, errors.New("请提供查询元素或向量")
		}
		args = append(args, "VALUES", len(values))
		for _, v := range values {
			args = append(args, v)
		}
	}
	args = append(args, "WITHSCORES")
	if k > 0 {
		args = append(args, "COUNT", k)
	}
	if ef > 0 {
		args = append(args, "EF", ef)
	}
	if strings.TrimSpace(filter) != "" {
		args = append(args, "FILTER", filter)
	}
	if truth {
		args = append(args, "TRUTH")
	}
	res, err := c.Do(s.ctx, args...).Result()
	if err != nil {
		return nil, err
	}
	// WITHSCORES：go-redis 解析为 map（元素→分数），或扁平数组（RESP2 原样）。
	// map 无序，统一按分数降序重排（最相似在前）。
	var out []model.VectorSimEntry
	switch v := res.(type) {
	case map[interface{}]interface{}:
		out = make([]model.VectorSimEntry, 0, len(v))
		for ele, score := range v {
			out = append(out, model.VectorSimEntry{Element: fmt.Sprint(ele), Score: toFloat(score)})
		}
	case []interface{}:
		out = make([]model.VectorSimEntry, 0, len(v)/2)
		for i := 0; i+1 < len(v); i += 2 {
			out = append(out, model.VectorSimEntry{Element: fmt.Sprint(v[i]), Score: toFloat(v[i+1])})
		}
	default:
		return nil, fmt.Errorf("VSIM 返回格式异常: %T", res)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out, nil
}

// VectorEmb VEMB：成员的近似向量（量化后为去量化值）
func (s *ValueService) VectorEmb(connID string, db int, key, ele string) ([]float64, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	res, err := c.Do(s.ctx, "VEMB", key, ele).Result()
	if err != nil {
		return nil, err
	}
	arr, ok := res.([]interface{})
	if !ok {
		return nil, fmt.Errorf("VEMB 返回格式异常")
	}
	out := make([]float64, 0, len(arr))
	for _, v := range arr {
		out = append(out, toFloat(v))
	}
	return out, nil
}

// VectorAdd VADD：插入/更新成员向量。attrs 为可选 JSON 属性（SETATTR），
// quant 可选 NOQUANT / Q8 / BIN（仅建键时生效）。
func (s *ValueService) VectorAdd(connID string, db int, key, ele string, values []float64, attrs, quant string) (bool, error) {
	if ele == "" {
		return false, errors.New("元素名不能为空")
	}
	if len(values) == 0 {
		return false, errors.New("向量不能为空")
	}
	c, err := s.cli(connID, db)
	if err != nil {
		return false, err
	}
	args := []interface{}{"VADD", key, "VALUES", len(values)}
	for _, v := range values {
		args = append(args, v)
	}
	args = append(args, ele)
	switch strings.ToUpper(quant) {
	case "NOQUANT", "Q8", "BIN":
		args = append(args, strings.ToUpper(quant))
	}
	if strings.TrimSpace(attrs) != "" {
		if !json.Valid([]byte(attrs)) {
			return false, errors.New("属性 JSON 格式不合法")
		}
		args = append(args, "SETATTR", attrs)
	}
	ok, err := c.Do(s.ctx, args...).Bool()
	if err == nil {
		return ok, nil
	}
	// Redis 8.0 行为：集合以 NOQUANT/BIN 建键后，后续 VADD 必须重复相同量化选项，
	// 否则报 "quantization mismatch"。此时读 VINFO 推断既有量化方式重试一次。
	if strings.Contains(err.Error(), "quantization mismatch") && quant == "" {
		// VINFO quant-type 取值：int8（默认）/ f32（NOQUANT）/ bin（BIN）
		if info, ierr := s.VectorInfo(connID, db, key); ierr == nil {
			switch strings.ToLower(info["quant-type"]) {
			case "f32":
				return s.VectorAdd(connID, db, key, ele, values, attrs, "NOQUANT")
			case "bin":
				return s.VectorAdd(connID, db, key, ele, values, attrs, "BIN")
			}
		}
	}
	return false, err
}

// VectorRemove VREM 删除成员
func (s *ValueService) VectorRemove(connID string, db int, key, ele string) (bool, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return false, err
	}
	return c.Do(s.ctx, "VREM", key, ele).Bool()
}

// VectorGetAttr VGETATTR 成员 JSON 属性（无属性返回空串）
func (s *ValueService) VectorGetAttr(connID string, db int, key, ele string) (string, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return "", err
	}
	v, err := c.Do(s.ctx, "VGETATTR", key, ele).Text()
	if err != nil {
		if err.Error() == "redis: nil" {
			return "", nil
		}
		return "", err
	}
	return v, nil
}

// VectorSetAttr VSETATTR 覆盖式设置成员属性（空串清除属性）
func (s *ValueService) VectorSetAttr(connID string, db int, key, ele, attrs string) error {
	if strings.TrimSpace(attrs) != "" && !json.Valid([]byte(attrs)) {
		return errors.New("属性 JSON 格式不合法")
	}
	c, err := s.cli(connID, db)
	if err != nil {
		return err
	}
	// VSETATTR 空 JSON 串表示删除属性
	return c.Do(s.ctx, "VSETATTR", key, ele, attrs).Err()
}

// toFloat RESP 返回值宽松转 float64（int64 / float64 / string / []byte）
func toFloat(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return f
	case []byte:
		f, _ := strconv.ParseFloat(string(n), 64)
		return f
	default:
		f, _ := strconv.ParseFloat(fmt.Sprint(v), 64)
		return f
	}
}
