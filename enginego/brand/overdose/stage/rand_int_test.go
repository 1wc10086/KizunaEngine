package stage

import (
	"testing"

	overscript "kizuna/enginego/brand/overdose/script"
)

// rand_int 是 macro.asb 的内联 opcode 0x71：结果 = (int)(rand01*N)，N 来自
// int var 92 的当前值，范围 0..N-1。少了它，所有随机分支恒取预设值，
// 剧情旗标（bool65/bool74）永远不满足，会卡在序盤A 的日常循环。
func TestRandIntVarMatchesEngineRange(t *testing.T) {
	vs := &overscript.VariableStore{}
	_ = vs.SetInt(1, 92, 3)
	seen := map[int32]bool{}
	for i := 0; i < 300; i++ {
		_ = vs.SetInt(1, 92, 3)
		randIntVar(vs, 92)
		v, _ := vs.GetInt(1, 92)
		if v < 0 || v > 2 {
			t.Fatalf("rand(3) = %d, 越界（原 handler cvttsd2si 给 0..N-1）", v)
		}
		seen[v] = true
	}
	if len(seen) < 2 {
		t.Fatalf("rand(3) 只出现 %v，随机源未生效", seen)
	}

	_ = vs.SetInt(1, 92, 0)
	randIntVar(vs, 92)
	if v, _ := vs.GetInt(1, 92); v != 0 {
		t.Fatalf("rand(0) = %d, want 0", v)
	}
}
