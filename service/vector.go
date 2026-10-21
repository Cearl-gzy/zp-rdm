package service

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"

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

// VectorLinks VLINKS：成员的 HNSW 各层邻居（始终 WITHSCORES；外层下标 = 层号，
// 层内按相似度降序）。用于邻居图可视化：中心成员的每一层邻居环。
func (s *ValueService) VectorLinks(connID string, db int, key, ele string) ([]model.VectorLinkLayer, error) {
	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	res, err := c.Do(s.ctx, "VLINKS", key, ele, "WITHSCORES").Result()
	if err != nil {
		return nil, err
	}
	arr, ok := res.([]interface{})
	if !ok {
		return nil, fmt.Errorf("VLINKS 返回格式异常: %T", res)
	}
	out := make([]model.VectorLinkLayer, 0, len(arr))
	for li, lv := range arr {
		layer := model.VectorLinkLayer{Layer: li, Neighbors: []model.VectorSimEntry{}}
		switch m := lv.(type) {
		case map[interface{}]interface{}:
			// RESP3：每层解析为 map（元素 → 分数）
			for e, score := range m {
				layer.Neighbors = append(layer.Neighbors, model.VectorSimEntry{Element: fmt.Sprint(e), Score: toFloat(score)})
			}
		case []interface{}:
			// RESP2：每层是扁平 kv 数组
			for i := 0; i+1 < len(m); i += 2 {
				layer.Neighbors = append(layer.Neighbors, model.VectorSimEntry{Element: fmt.Sprint(m[i]), Score: toFloat(m[i+1])})
			}
		default:
			return nil, fmt.Errorf("VLINKS 层 %d 格式异常: %T", li, lv)
		}
		sort.Slice(layer.Neighbors, func(i, j int) bool { return layer.Neighbors[i].Score > layer.Neighbors[j].Score })
		out = append(out, layer)
	}
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

// ---------- 批量导入 ----------

const (
	vectorImportBatch     = 200        // 每批管道写入的 VADD 条数
	vectorImportMaxErrors = 20         // 结果中保留的失败原因上限
	vectorImportMaxSize   = 256 << 20  // 导入文件大小上限 256MB
)

// importRow 解析后的一行导入数据（line 为 1 起始行号，错误报告用）
type importRow struct {
	line   int
	ele    string
	values []float64
	attrs  string
}

// VectorImport 批量导入向量。format：jsonl / csv / ""（按扩展名自动）。
//   - JSONL：每行 {"element"|"id", "vector"|"values"|"embedding": [...], "attrs"?（对象或 JSON 串）}
//   - CSV：首列 element、其后各列为向量分量；首行可为表头（第二列非数字即表头），
//     表头末列名为 attrs 时该列解析为属性 JSON。
//
// quant：建键时的量化选项（NOQUANT/Q8/BIN，空 = 默认）；集合已存在时忽略该参数，
// 按其 VINFO 量化方式自动适配（f32→NOQUANT、bin→BIN）。
// 每批进度通过事件 vector-import:{connID}:{db}:{key} 推送 {done,total,added,failed}。
func (s *ValueService) VectorImport(connID string, db int, key, path, format, quant string) (*model.VectorImportResult, error) {
	if strings.TrimSpace(key) == "" {
		return nil, errors.New("键名不能为空")
	}
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		switch {
		case strings.HasSuffix(strings.ToLower(path), ".jsonl"):
			format = "jsonl"
		case strings.HasSuffix(strings.ToLower(path), ".csv"):
			format = "csv"
		default:
			return nil, errors.New("无法识别文件格式，请选择 .jsonl 或 .csv")
		}
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("文件不可读: %v", err)
	}
	if st.Size() > vectorImportMaxSize {
		return nil, fmt.Errorf("文件超过 256MB 上限（%dMB）", st.Size()>>20)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck

	var rows []importRow
	var result = &model.VectorImportResult{Errors: []string{}}
	addErr := func(format string, args ...interface{}) {
		result.Failed++
		if len(result.Errors) < vectorImportMaxErrors {
			result.Errors = append(result.Errors, fmt.Sprintf(format, args...))
		} else {
			result.Truncated = true
		}
	}
	switch format {
	case "jsonl":
		rows, err = parseVectorJSONL(f, addErr)
	case "csv":
		rows, err = parseVectorCSV(f, addErr)
	default:
		return nil, fmt.Errorf("不支持的格式 %q", format)
	}
	if err != nil {
		return nil, err
	}
	result.Total = len(rows) + result.Failed
	if len(rows) == 0 {
		if result.Failed == 0 {
			return nil, errors.New("文件中没有可导入的数据行")
		}
		return result, nil // 全部解析失败，错误已在列表中
	}

	c, err := s.cli(connID, db)
	if err != nil {
		return nil, err
	}
	// 已存在的集合：按 VINFO 量化方式强制适配（与 VectorAdd 的量化重试同源，批量化前置）
	if info, ierr := s.VectorInfo(connID, db, key); ierr == nil {
		switch strings.ToLower(info["quant-type"]) {
		case "f32":
			quant = "NOQUANT"
		case "bin":
			quant = "BIN"
		default:
			quant = ""
		}
	}

	event := fmt.Sprintf("vector-import:%s:%d:%s", connID, db, key)
	done := result.Failed // 解析失败的行也算已处理
	emitProgress := func() {
		emit(s.ctx, event, map[string]int{
			"done": done, "total": result.Total, "added": result.Added, "failed": result.Failed,
		})
	}
	for start := 0; start < len(rows); start += vectorImportBatch {
		end := start + vectorImportBatch
		if end > len(rows) {
			end = len(rows)
		}
		batch := rows[start:end]
		pipe := c.Pipeline()
		cmds := make([]*redis.Cmd, 0, len(batch))
		for _, r := range batch {
			args := []interface{}{"VADD", key, "VALUES", len(r.values)}
			for _, v := range r.values {
				args = append(args, v)
			}
			args = append(args, r.ele)
			if quant != "" {
				args = append(args, quant)
			}
			if r.attrs != "" {
				args = append(args, "SETATTR", r.attrs)
			}
			cmds = append(cmds, pipe.Do(s.ctx, args...))
		}
		_, _ = pipe.Exec(s.ctx) // 错误在逐命令 Err() 上，Exec 仅返回首个
		for i, cmd := range cmds {
			if cerr := cmd.Err(); cerr != nil {
				addErr("行 %d: %v", batch[i].line, cerr)
			} else {
				result.Added++
			}
			done++
		}
		emitProgress()
	}
	return result, nil
}

// parseVectorJSONL 解析 JSONL 导入文件（逐行；坏行计入 addErr 并跳过）
func parseVectorJSONL(f *os.File, addErr func(string, ...interface{})) ([]importRow, error) {
	rows := make([]importRow, 0, 1024)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20) // 单行上限 16MB（高维向量）
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal([]byte(text), &m); err != nil {
			addErr("行 %d: JSON 解析失败", line)
			continue
		}
		ele := jsonStrField(m, "element", "id")
		if ele == "" {
			addErr("行 %d: 缺少 element 字段", line)
			continue
		}
		rawVec, ok := jsonField(m, "vector", "values", "embedding")
		if !ok {
			addErr("行 %d: 缺少 vector 字段", line)
			continue
		}
		var values []float64
		if err := json.Unmarshal(rawVec, &values); err != nil || !validVector(values) {
			addErr("行 %d: vector 必须是非空有限数值数组", line)
			continue
		}
		attrs, err := jsonAttrsField(m)
		if err != nil {
			addErr("行 %d: %v", line, err)
			continue
		}
		rows = append(rows, importRow{line: line, ele: ele, values: values, attrs: attrs})
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("读取文件失败（可能单行过大）: %v", err)
	}
	return rows, nil
}

// parseVectorCSV 解析 CSV 导入文件（首列 element，其后为向量分量；第二列非数字的首行视为表头；
// 表头末列名为 attrs 时该列解析为属性 JSON）
func parseVectorCSV(f *os.File, addErr func(string, ...interface{})) ([]importRow, error) {
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1 // 容忍缺尾列的坏行，自行报错
	recs, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("CSV 解析失败: %v", err)
	}
	if len(recs) == 0 {
		return nil, errors.New("空 CSV 文件")
	}
	start, attrsCol := 0, -1
	if len(recs[0]) >= 2 {
		if _, perr := strconv.ParseFloat(strings.TrimSpace(recs[0][1]), 64); perr != nil {
			start = 1 // 表头行
			if strings.EqualFold(strings.TrimSpace(recs[0][len(recs[0])-1]), "attrs") {
				attrsCol = len(recs[0]) - 1
			}
		}
	}
	rows := make([]importRow, 0, len(recs))
	for i := start; i < len(recs); i++ {
		rec, line := recs[i], i+1
		vecEnd := len(rec)
		if attrsCol >= 0 {
			vecEnd = attrsCol
		}
		if len(rec) < 2 || strings.TrimSpace(rec[0]) == "" {
			addErr("行 %d: 缺少 element 列", line)
			continue
		}
		values := make([]float64, 0, vecEnd-1)
		bad := false
		for _, cell := range rec[1:vecEnd] {
			v, perr := strconv.ParseFloat(strings.TrimSpace(cell), 64)
			if perr != nil || math.IsNaN(v) || math.IsInf(v, 0) {
				addErr("行 %d: 向量分量 %q 不是有限数值", line, cell)
				bad = true
				break
			}
			values = append(values, v)
		}
		if bad || !validVector(values) {
			if !bad {
				addErr("行 %d: 向量为空", line)
			}
			continue
		}
		attrs := ""
		if attrsCol >= 0 && attrsCol < len(rec) {
			attrs = strings.TrimSpace(rec[attrsCol])
			if attrs != "" && !json.Valid([]byte(attrs)) {
				addErr("行 %d: attrs 不是合法 JSON", line)
				continue
			}
		}
		rows = append(rows, importRow{line: line, ele: strings.TrimSpace(rec[0]), values: values, attrs: attrs})
	}
	return rows, nil
}

// validVector 非空且全部为有限数值
func validVector(values []float64) bool {
	if len(values) == 0 {
		return false
	}
	for _, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

// jsonField 从 map 中按候选名取字段
func jsonField(m map[string]json.RawMessage, names ...string) (json.RawMessage, bool) {
	for _, n := range names {
		if v, ok := m[n]; ok {
			return v, true
		}
	}
	return nil, false
}

// jsonStrField 取字符串字段（缺失/非字符串返回空串）
func jsonStrField(m map[string]json.RawMessage, names ...string) string {
	raw, ok := jsonField(m, names...)
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return strings.TrimSpace(s)
}

// jsonAttrsField 取可选 attrs 字段（对象 → 压缩 JSON 串；字符串 → 校验 JSON；数字等 → 报错）
func jsonAttrsField(m map[string]json.RawMessage) (string, error) {
	raw, ok := m["attrs"]
	if !ok || string(raw) == "null" {
		return "", nil
	}
	trimmed := strings.TrimSpace(string(raw))
	switch trimmed[0] {
	case '{':
		return compactJSON(trimmed)
	case '"':
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", errors.New("attrs 字符串非法")
		}
		if strings.TrimSpace(s) == "" {
			return "", nil
		}
		if !json.Valid([]byte(s)) {
			return "", errors.New("attrs 不是合法 JSON")
		}
		return compactJSON(s)
	default:
		return "", errors.New("attrs 必须是对象或 JSON 字符串")
	}
}

func compactJSON(s string) (string, error) {
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(s)); err != nil {
		return "", errors.New("attrs 不是合法 JSON")
	}
	return buf.String(), nil
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
