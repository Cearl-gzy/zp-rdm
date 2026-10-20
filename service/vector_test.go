package service

import (
	"strings"
	"testing"
)

// Vector Set 集成测试。
// miniredis 不支持向量命令，仅真实 Redis 8（CI service redis:8）运行；
// 其余环境探测失败后整体跳过。

func requireVector(t *testing.T, s *ValueService, connID string) {
	t.Helper()
	if _, err := s.VectorAdd(connID, 0, "v:probe", "p1", []float64{0, 0}, "", ""); err != nil {
		if strings.Contains(err.Error(), "unknown command") || strings.Contains(err.Error(), "ERR unknown") {
			t.Skipf("server does not support Vector Set (need Redis 8): %v", err)
		}
		t.Fatalf("probe vadd: %v", err)
	}
	if ok, err := s.VectorRemove(connID, 0, "v:probe", "p1"); err != nil || !ok {
		t.Skipf("vector probe cleanup failed: %v", err)
	}
}

func TestVectorLifecycle(t *testing.T) {
	s, _, connID := newValueService(t)
	requireVector(t, s, connID)
	key := "v:life"

	// VADD（带属性 + 量化选项建键）
	if ok, err := s.VectorAdd(connID, 0, key, "a", []float64{1, 0, 0}, `{"tag":"x"}`, "NOQUANT"); err != nil || !ok {
		t.Fatalf("add a: ok=%v err=%v", ok, err)
	}
	if _, err := s.VectorAdd(connID, 0, key, "b", []float64{0, 1, 0}, `{"tag":"y"}`, ""); err != nil {
		t.Fatalf("add b: %v", err)
	}
	if _, err := s.VectorAdd(connID, 0, key, "c", []float64{0.9, 0.1, 0}, "", ""); err != nil {
		t.Fatalf("add c: %v", err)
	}

	// VCARD 经 GetValue 反映到 Length
	d, err := s.GetValue(connID, 0, key)
	if err != nil {
		t.Fatal(err)
	}
	if d.Type != "vectorset" || d.Length != 3 {
		t.Fatalf("GetValue = %+v", d)
	}

	// VINFO
	info, err := s.VectorInfo(connID, 0, key)
	if err != nil {
		t.Fatal(err)
	}
	// NOQUANT 建键 → VINFO quant-type 为 f32
	if info["quant-type"] != "f32" {
		t.Fatalf("quant-type = %q (VINFO=%+v)", info["quant-type"], info)
	}
	if info["vector-dim"] != "3" {
		t.Fatalf("vector-dim = %q (VINFO=%+v)", info["vector-dim"], info)
	}

	// VSIM 按元素：a 自己应排第一，分数 1.0
	res, err := s.VectorSim(connID, 0, key, "a", nil, 3, 0, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 3 || res[0].Element != "a" || res[0].Score < 0.9999 {
		t.Fatalf("vsim by ele = %+v", res)
	}
	// c 与 a 同向，应排在 b 之前
	if res[1].Element != "c" {
		t.Fatalf("vsim order = %+v", res)
	}

	// VSIM 按原始向量 + COUNT
	res2, err := s.VectorSim(connID, 0, key, "", []float64{0, 1, 0}, 1, 0, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res2) != 1 || res2[0].Element != "b" {
		t.Fatalf("vsim by values = %+v", res2)
	}

	// FILTER 属性过滤
	res3, err := s.VectorSim(connID, 0, key, "a", nil, 3, 0, `.tag == "y"`, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res3) != 1 || res3[0].Element != "b" {
		t.Fatalf("vsim filter = %+v", res3)
	}

	// VEMB
	emb, err := s.VectorEmb(connID, 0, key, "a")
	if err != nil {
		t.Fatal(err)
	}
	if len(emb) != 3 || emb[0] != 1 {
		t.Fatalf("vemb = %v", emb)
	}

	// 属性读写
	attr, err := s.VectorGetAttr(connID, 0, key, "a")
	if err != nil || attr == "" {
		t.Fatalf("getattr = %q err=%v", attr, err)
	}
	if err := s.VectorSetAttr(connID, 0, key, "a", `{"tag":"x2"}`); err != nil {
		t.Fatal(err)
	}
	attr2, _ := s.VectorGetAttr(connID, 0, key, "a")
	if !strings.Contains(attr2, "x2") {
		t.Fatalf("setattr then getattr = %q", attr2)
	}

	// VRANDMEMBER 采样
	members, err := s.VectorSample(connID, 0, key, 10)
	if err != nil || len(members) != 3 {
		t.Fatalf("sample = %v err=%v", members, err)
	}

	// VREM
	if ok, err := s.VectorRemove(connID, 0, key, "b"); err != nil || !ok {
		t.Fatalf("remove: ok=%v err=%v", ok, err)
	}
	d2, _ := s.GetValue(connID, 0, key)
	if d2.Length != 2 {
		t.Fatalf("after remove length = %d", d2.Length)
	}
}

func TestVectorValidation(t *testing.T) {
	s, _, connID := newValueService(t)
	requireVector(t, s, connID)

	// 非法 JSON 属性在前端命令发出前拦截
	if _, err := s.VectorAdd(connID, 0, "v:bad", "e", []float64{1}, `{oops`, ""); err == nil {
		t.Fatal("expected attr json error")
	}
	if _, err := s.VectorAdd(connID, 0, "v:bad", "", []float64{1}, "", ""); err == nil {
		t.Fatal("expected empty element error")
	}
	if _, err := s.VectorAdd(connID, 0, "v:bad", "e", nil, "", ""); err == nil {
		t.Fatal("expected empty vector error")
	}
	if _, err := s.VectorSim(connID, 0, "v:bad", "", nil, 5, 0, "", false); err == nil {
		t.Fatal("expected sim without input error")
	}
	if err := s.VectorSetAttr(connID, 0, "v:bad", "e", `not-json`); err == nil {
		t.Fatal("expected setattr json error")
	}
}

func TestVectorCreateKey(t *testing.T) {
	s, _, connID := newValueService(t)
	requireVector(t, s, connID)

	if err := s.CreateKey(connID, 0, "v:created", "vectorset", "[0.1, 0.2]", "item1", 0, 0); err != nil {
		t.Fatal(err)
	}
	d, err := s.GetValue(connID, 0, "v:created")
	if err != nil {
		t.Fatal(err)
	}
	if d.Type != "vectorset" || d.Length != 1 {
		t.Fatalf("created = %+v", d)
	}
	emb, _ := s.VectorEmb(connID, 0, "v:created", "item1")
	if len(emb) != 2 {
		t.Fatalf("created emb = %v", emb)
	}

	// 非 JSON 数组值报错
	if err := s.CreateKey(connID, 0, "v:bad2", "vectorset", "hello", "e1", 0, 0); err == nil {
		t.Fatal("expected create error for non-array value")
	}
}
