package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// VLINKS 邻居图与批量导入测试（miniredis 不支持 V* 命令 → requireVector 探测跳过，真实 Redis 8 双验证）

func TestVectorLinks(t *testing.T) {
	s, _, connID := newValueService(t)
	requireVector(t, s, connID)
	key := "vl:links"
	for _, e := range []struct {
		name string
		v    []float64
	}{
		{"a", []float64{1, 0}}, {"b", []float64{0.9, 0.1}},
		{"c", []float64{0, 1}}, {"d", []float64{0.1, 0.9}},
		{"e", []float64{0.5, 0.5}},
	} {
		if _, err := s.VectorAdd(connID, 0, key, e.name, e.v, "", ""); err != nil {
			t.Fatal(err)
		}
	}

	layers, err := s.VectorLinks(connID, 0, key, "a")
	if err != nil {
		t.Fatal(err)
	}
	// 找到 L0 层（不假定返回顺序）
	if len(layers) == 0 {
		t.Fatalf("VLINKS 层为空: %+v", layers)
	}
	nb := layers[0].Neighbors
	for i := range layers {
		if layers[i].Layer == 0 {
			nb = layers[i].Neighbors
			break
		}
	}
	if len(nb) == 0 {
		t.Fatalf("VLINKS L0 无邻居: %+v", layers)
	}
	// 层内按相似度降序
	for i := 1; i < len(nb); i++ {
		if nb[i].Score > nb[i-1].Score {
			t.Fatalf("邻居未按相似度降序: %+v", nb)
		}
	}
	// b 与 a 最接近，应排第一
	if nb[0].Element != "b" {
		t.Fatalf("最近邻居 = %s, want b（%+v）", nb[0].Element, nb)
	}
	for _, n := range nb {
		if n.Score <= 0 || n.Score > 1 {
			t.Fatalf("分数越界: %+v", n)
		}
	}

	// 不存在的成员
	if _, err := s.VectorLinks(connID, 0, key, "ghost"); err == nil {
		t.Fatal("ghost 成员应报错")
	}
}

func TestVectorImportJSONL(t *testing.T) {
	s, _, connID := newValueService(t)
	requireVector(t, s, connID)
	key := "vl:imp-jsonl"

	content := strings.Join([]string{
		`{"element":"a","vector":[1,0],"attrs":{"city":"bj"}}`,
		`{"id":"b","values":[0.9,0.1]}`,                    // 别名 id/values
		`{"element":"c","embedding":[0,1],"attrs":"{}"}`,   // 别名 embedding + 字符串 attrs
		`{"element":"bad1","vector":"oops"}`,               // 坏向量
		`{"vector":[1,1]}`,                                 // 缺 element
		`not json`,                                         // 坏 JSON
		`{"element":"bad2","vector":[1,0],"attrs":123}`,    // attrs 类型错
		``,
	}, "\n")
	path := filepath.Join(t.TempDir(), "in.jsonl")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := s.VectorImport(connID, 0, key, path, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 3 || res.Failed != 4 || res.Total != 7 {
		t.Fatalf("导入结果 = %+v, want added=3 failed=4 total=7", res)
	}
	if len(res.Errors) != 4 {
		t.Fatalf("错误列表 = %+v", res.Errors)
	}
	// 属性随导入写入
	attrs, err := s.VectorGetAttr(connID, 0, key, "a")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(attrs, "city") {
		t.Fatalf("attrs = %q, want 含 city", attrs)
	}
	if n, _ := s.VectorInfo(connID, 0, key); n["size"] != "3" {
		t.Fatalf("VINFO size = %v, want 3", n["size"])
	}
}

func TestVectorImportCSV(t *testing.T) {
	s, _, connID := newValueService(t)
	requireVector(t, s, connID)

	// 带表头 + attrs 列
	key1 := "vl:imp-csv1"
	csv1 := "element,x,y,attrs\na,1,0,\"{\"\"n\"\":1}\"\nb,0,1,\n"
	p1 := filepath.Join(t.TempDir(), "in.csv")
	if err := os.WriteFile(p1, []byte(csv1), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := s.VectorImport(connID, 0, key1, p1, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 2 || res.Failed != 0 {
		t.Fatalf("带表头 CSV = %+v, want added=2", res)
	}
	attrs, _ := s.VectorGetAttr(connID, 0, key1, "a")
	if !strings.Contains(attrs, `"n":1`) {
		t.Fatalf("attrs = %q", attrs)
	}

	// 无表头纯向量
	key2 := "vl:imp-csv2"
	csv2 := "e1,0.1,0.2,0.3\ne2,0.4,0.5,0.6\nbad,xx,0.2,0.3\n"
	p2 := filepath.Join(t.TempDir(), "in2.csv")
	if err := os.WriteFile(p2, []byte(csv2), 0o644); err != nil {
		t.Fatal(err)
	}
	res2, err := s.VectorImport(connID, 0, key2, p2, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if res2.Added != 2 || res2.Failed != 1 {
		t.Fatalf("无表头 CSV = %+v, want added=2 failed=1", res2)
	}
}

func TestVectorImportQuantAdapt(t *testing.T) {
	s, _, connID := newValueService(t)
	requireVector(t, s, connID)
	key := "vl:imp-quant"
	// 先以 NOQUANT 建键
	if _, err := s.VectorAdd(connID, 0, key, "seed", []float64{1, 0}, "", "NOQUANT"); err != nil {
		t.Fatal(err)
	}
	// 导入时不带 quant：应按 VINFO 自动适配 NOQUANT，不得报 quantization mismatch
	p := filepath.Join(t.TempDir(), "q.jsonl")
	if err := os.WriteFile(p, []byte(`{"element":"x","vector":[0,1]}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := s.VectorImport(connID, 0, key, p, "jsonl", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 1 {
		t.Fatalf("量化适配导入 = %+v, want added=1", res)
	}
}

func TestVectorImportValidation(t *testing.T) {
	s, _, connID := newValueService(t)
	requireVector(t, s, connID)

	if _, err := s.VectorImport(connID, 0, "k", filepath.Join(t.TempDir(), "missing.jsonl"), "", ""); err == nil {
		t.Fatal("不存在的文件应报错")
	}
	p := filepath.Join(t.TempDir(), "x.txt")
	if err := os.WriteFile(p, []byte("whatever"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.VectorImport(connID, 0, "k", p, "", ""); err == nil || !strings.Contains(err.Error(), "无法识别") {
		t.Fatalf("未知扩展名 err = %v", err)
	}
	// 全部行解析失败：返回结果而非报错（错误在列表中）
	bad := filepath.Join(t.TempDir(), "bad.jsonl")
	if err := os.WriteFile(bad, []byte("garbage\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := s.VectorImport(connID, 0, "vl:none", bad, "jsonl", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 0 || res.Failed != 1 {
		t.Fatalf("全坏行 = %+v", res)
	}
}
