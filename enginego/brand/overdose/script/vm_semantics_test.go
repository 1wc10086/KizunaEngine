package script

import (
	"encoding/binary"
	"testing"
)

// 这些用例锁的是三个"看起来只是细节、实际会重播整段剧本"的语义：
//   - 节点 19 是二元加法/字符串拼接（动态 label "talk"+1 → "talk1"）；
//   - 0x0a 查不到 label 是空调用，绝不能回落到偏移 0；
//   - 0x0b 是同脚本跳转，不压帧（0x08 结束的是外层 0x0a 帧）。
// 没有真实游戏目录的 CI 也能跑，避免以后再次退化。

func tnode(t NodeType, payload ...byte) ArgNode {
	switch t {
	case NodeString:
		n := ArgNode{Type: t, Str: string(payload)}
		return n
	case NodeInt:
		return ArgNode{Type: t, Int: int32(binary.LittleEndian.Uint32(payload))}
	case NodeBool:
		return ArgNode{Type: t, Bool: payload[0] != 0}
	case NodeVarRef:
		return ArgNode{Type: t, Namespace: binary.LittleEndian.Uint16(payload), Index: binary.LittleEndian.Uint16(payload[2:])}
	}
	return ArgNode{Type: t}
}

func encodeNode(a ArgNode) []byte {
	var p []byte
	switch a.Type {
	case NodeString:
		p = append([]byte(a.Str), 0)
	case NodeInt:
		p = make([]byte, 4)
		binary.LittleEndian.PutUint32(p, uint32(a.Int))
	case NodeBool:
		if a.Bool {
			p = []byte{1}
		} else {
			p = []byte{0}
		}
	case NodeVarRef:
		p = make([]byte, 4)
		binary.LittleEndian.PutUint16(p, a.Namespace)
		binary.LittleEndian.PutUint16(p[2:], a.Index)
	}
	return append([]byte{byte(2 + len(p)), byte(a.Type)}, p...)
}

func encodeInstr(op uint16, field uint32, args ...ArgNode) []byte {
	var body []byte
	for _, a := range args {
		body = append(body, encodeNode(a)...)
	}
	head := make([]byte, 12)
	binary.LittleEndian.PutUint16(head, uint16(12+len(body)))
	binary.LittleEndian.PutUint16(head[2:], op)
	binary.LittleEndian.PutUint32(head[4:], field)
	binary.LittleEndian.PutUint16(head[8:], uint16(len(args)))
	return append(head, body...)
}

func intNode(v int32) ArgNode  { return tnode(NodeInt, byte(v), byte(v>>8), byte(v>>16), byte(v>>24)) }
func strNode(s string) ArgNode { return tnode(NodeString, []byte(s)...) }
func varNode(ns, idx uint16) ArgNode {
	return tnode(NodeVarRef, byte(ns), byte(ns>>8), byte(idx), byte(idx>>8))
}

// TestNode19AddsAndConcats 是 sop4 动态 label 与 0x10 自增的合成式。
func TestNode19AddsAndConcats(t *testing.T) {
	vs := &VariableStore{}
	_ = vs.SetInt(1, 50, 1)

	vals, err := EvalArgs([]ArgNode{
		strNode("talk"), varNode(1, 50), {Type: NodeAddAssign}, {Type: NodeEnd},
	}, vs)
	if err != nil {
		t.Fatal(err)
	}
	if len(vals) != 1 || vals[0].AsString() != "talk1" {
		t.Fatalf("label 拼接 = %#v, want talk1", vals)
	}

	vals, err = EvalArgs([]ArgNode{
		intNode(50), varNode(1, 50), intNode(1), {Type: NodeAddAssign},
		{Type: NodeBool}, {Type: NodeEnd}, {Type: NodeEnd},
	}, vs)
	if err != nil {
		t.Fatal(err)
	}
	if len(vals) != 3 || vals[1].AsInt() != 2 {
		t.Fatalf("自增表达式 = %#v, want [50 2 false]", vals)
	}
}

func collectDialogue(vm *VM, h EventHandler) []string {
	var out []string
	handler := EventHandlerFunc(func(e Event) error {
		if e.Kind == EventDialogue && e.Text != "" {
			out = append(out, e.Text)
		}
		return h.HandleEvent(e)
	})
	if _, err := vm.Step(4096, handler); err != nil {
		return append(out, "ERR:"+err.Error())
	}
	return out
}

// TestCallScriptIndexedLabel 复刻 sop4 的 `0x0a "t","talk",var(1,50),19`。
func TestCallScriptIndexedLabel(t *testing.T) {
	// 布局：入口用 0x0a 调 t.talk+1；talk1 打印 "hit"；talk2/其他打印 "wrong"。
	var stream []byte
	stream = append(stream, encodeInstr(0x0a, 0, strNode("t"), strNode("talk"), varNode(1, 50), ArgNode{Type: NodeAddAssign}, ArgNode{Type: NodeEnd})...)
	stream = append(stream, encodeInstr(0x1f, 0, strNode(""), strNode("root-resume"), ArgNode{Type: NodeEnd})...)
	stream = append(stream, encodeInstr(0x00, 0xffffffff)...)
	stream = append(stream, encodeInstr(0x02, 1, strNode("talk1"))...)
	stream = append(stream, encodeInstr(0x1f, 0, strNode(""), strNode("hit"), ArgNode{Type: NodeEnd})...)
	stream = append(stream, encodeInstr(0x08, 0, intNode(0))...)
	stream = append(stream, encodeInstr(0x02, 2, strNode("talk2"))...)
	stream = append(stream, encodeInstr(0x1f, 0, strNode(""), strNode("wrong"), ArgNode{Type: NodeEnd})...)
	stream = append(stream, encodeInstr(0x08, 0, intNode(0))...)
	scr, err := Parse("t", stream)
	if err != nil {
		t.Fatal(err)
	}
	vm := New(func(name string) (*Script, error) { return scr, nil })
	if err := vm.Start("t"); err != nil {
		t.Fatal(err)
	}
	_ = vm.Variables().SetInt(1, 50, 1)
	got := collectDialogue(vm, EventHandlerFunc(func(Event) error { return nil }))
	if len(got) != 1 || got[0] != "hit" {
		t.Fatalf("indexed label 结果 = %v, want [hit]（回落到 0 会先播 root-resume）", got)
	}
}

// TestMissingCallLabelIsNoop：0x0a 查不到 label 时不能把 PC 打回脚本头。
func TestMissingCallLabelIsNoop(t *testing.T) {
	var stream []byte
	stream = append(stream, encodeInstr(0x0a, 0, strNode("t"), strNode("nope"), ArgNode{Type: NodeEnd})...)
	stream = append(stream, encodeInstr(0x1f, 0, strNode(""), strNode("after"), ArgNode{Type: NodeEnd})...)
	stream = append(stream, encodeInstr(0x08, 0, intNode(0))...)
	scr, _ := Parse("t", stream)
	vm := New(func(string) (*Script, error) { return scr, nil })
	if err := vm.Start("t"); err != nil {
		t.Fatal(err)
	}
	got := collectDialogue(vm, EventHandlerFunc(func(Event) error { return nil }))
	if len(got) != 1 || got[0] != "after" {
		t.Fatalf("缺失 label 结果 = %v, want [after]", got)
	}
}

// TestGosubIsJump：0x0b 不压帧，子段 0x08 结束当前帧，gosub 之后的代码不应执行。
func TestGosubIsJump(t *testing.T) {
	var stream []byte
	stream = append(stream, encodeInstr(0x0b, 0, strNode("sub"), ArgNode{Type: NodeEnd})...)
	stream = append(stream, encodeInstr(0x1f, 0, strNode(""), strNode("after"), ArgNode{Type: NodeEnd})...)
	stream = append(stream, encodeInstr(0x00, 0xffffffff)...)
	stream = append(stream, encodeInstr(0x02, 1, strNode("sub"))...)
	stream = append(stream, encodeInstr(0x1f, 0, strNode(""), strNode("sub-body"), ArgNode{Type: NodeEnd})...)
	stream = append(stream, encodeInstr(0x08, 0, intNode(0))...)
	scr, _ := Parse("t", stream)
	vm := New(func(string) (*Script, error) { return scr, nil })
	if err := vm.Start("t"); err != nil {
		t.Fatal(err)
	}
	got := collectDialogue(vm, EventHandlerFunc(func(Event) error { return nil }))
	if len(got) != 1 || got[0] != "sub-body" {
		t.Fatalf("gosub 结果 = %v, want [sub-body]（压帧会多播 after）", got)
	}
	// collectDialogue 停在 "sub-body" 的对话停点，再推进一步让 0x08 执行。
	if err := vm.Advance(); err != nil {
		t.Fatal(err)
	}
	if !vm.Done() {
		t.Fatal("0x08 应结束根帧")
	}
}
