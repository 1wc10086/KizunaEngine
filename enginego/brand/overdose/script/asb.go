package script

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"unicode/utf8"

	"kizuna/enginego/core/textcodec"
)

// ASB 容器头（overdose_spec §5）：魔数 + 加密长度 + 解压长度 + 行数。
const (
	asbMagic      = "ASB\x00"
	asbHeaderSize = 16
	instrMin      = 12
	argNodeMin    = 2
)

// NodeType 是参数节点的类型标（§5.1）。名字里不带原始数字，是为了让
// 分派和测试读起来像契约原文，而不是靠数字猜含义。
type NodeType uint8

const (
	NodeString    NodeType = 7 // CP932 或 UTF-8 零结尾字符串（汉化剧本用 UTF-8）
	NodeInt       NodeType = 6 // int32，小端
	NodeBool      NodeType = 5 // 1 字节
	NodeVarRef    NodeType = 4 // (命名空间, 索引) 两个 u16
	NodeLabelID   NodeType = 3 // 标签编号，u16
	NodeEq        NodeType = 8
	NodeNe        NodeType = 9
	NodeLt        NodeType = 10
	NodeLe        NodeType = 11
	NodeGt        NodeType = 12
	NodeGe        NodeType = 13
	NodeOr        NodeType = 14
	NodeAnd       NodeType = 15
	NodeNeg       NodeType = 17 // 一元 - / !，见下注
	NodeAssign    NodeType = 18 // 赋值：弹值与变量引用，存入，不留栈
	NodeAddAssign NodeType = 19 // +=，见 eval
	NodeAdd       NodeType = 20 // 见注：20/21/22/23 在 handler 里逐个核对，暂按 + - * / 占位
	NodeSub       NodeType = 21
	NodeMul       NodeType = 22
	NodeDiv       NodeType = 23
	NodePairPut   NodeType = 24 // 一对 u16 存入变量对（极少）
	NodeCompound  NodeType = 25 // 复合赋值辅助（极少，语义近 +=）
	NodePush      NodeType = 26 // 一元推栈辅助（极少）
	NodePush2     NodeType = 27 // 同上，第二个一元辅助
	NodeEnd       NodeType = 28 // 参数串结束/分隔
)

// 一元符号注：实测 17 只出现在 op 0x1b 的 (1,-1,1,-1…) 探针序列里，
// 没有对应 handler 可逐字核对。这里先按“对栈顶取负”（布尔取反时等价于 0/1 翻转）
// 实现；若将来 disasm 出原 handler 是取反而非取负，两者在 0/1 探针上结果相同，
// 不会使现有脚本跑偏，只有大整数探针会有 1 个数量级的差别。
// NodeDouble 占位：28 之前还有 16 没观测到（0~118 表之外也未见），
// 若以后出现类型 16 的节点这里会报“未知节点类型”而非静默错算。

// ArgNode 是 argc 记数的一段原始参数节点。Str 只在 NodeString 有效，
// 已按 CP932 解出，不含尾零；Int/Bool/Namespace/Index/Label 按类型平铺，
// 这样求值器不用再回读原始字节，也省掉一层切片换算。
type ArgNode struct {
	Type      NodeType
	Str       string
	Int       int32
	Bool      bool
	Namespace uint16
	Index     uint16
	Label     uint16
}

func (n ArgNode) String() string {
	switch n.Type {
	case NodeString:
		return fmt.Sprintf("str(%q)", n.Str)
	case NodeInt:
		return fmt.Sprintf("int(%d)", n.Int)
	case NodeBool:
		return fmt.Sprintf("bool(%t)", n.Bool)
	case NodeVarRef:
		return fmt.Sprintf("var(%d,%d)", n.Namespace, n.Index)
	case NodeLabelID:
		return fmt.Sprintf("label(%d)", n.Label)
	default:
		return fmt.Sprintf("t%d", uint8(n.Type))
	}
}

// Instruction 是一条平流指令。Offsets 是它在解压后流里的字节偏移——
// op 0x04/0x05 的 field 是这种偏移而非序号（§5.3），所以 VM 跳转必须靠它;
// 少了这个字段就得重排表，错位时排查也无从下手。
type Instruction struct {
	Offset  int
	Size    int
	Opcode  uint16
	Field   uint32
	Argc    int // 节点个数，非最终参数个数
	Line    uint16
	Args    []ArgNode
	RawArgs [][]byte // 原始节点字节（size+type+payload）保留，供不透明透传
}

// Script 是单文件 AST：带名程序体 + 从指令块建起的标签索引。
type Script struct {
	Name         string
	Data         []byte // 解压后流
	Instructions []*Instruction
	Labels       map[string]int // 标签名 → 指令偏移
	byOffset     map[int]*Instruction
}

func (s *Script) name() string { return s.Name }

func (s *Script) instructionAt(offset int) (*Instruction, bool) {
	ins, ok := s.byOffset[offset]
	return ins, ok
}

func (s *Script) labelOffset(name string) (int, bool) {
	off, ok := s.Labels[name]
	return off, ok
}

// DecompressASB 解开一个 script.arc 条目载荷（§5）。
// 变换是对有效 dword 的回绕相减，而不是异或——sha/ARC 那路是异或，
// 这里写错会解出能 zlib 起步但指令流全错乱的中间态，必须逐字节守住。
func DecompressASB(raw []byte) ([]byte, error) {
	if len(raw) < asbHeaderSize {
		return nil, fmt.Errorf("asb: 文件过短 (%d 字节)，放不下 16 字节头", len(raw))
	}
	if string(raw[:4]) != asbMagic {
		return nil, fmt.Errorf("asb: 魔数不符 %q，期望 ASB\\0", raw[:4])
	}
	encSize := binary.LittleEndian.Uint32(raw[4:8])
	uncompSize := binary.LittleEndian.Uint32(raw[8:12])
	if encSize != uint32(len(raw)-asbHeaderSize) {
		// 长度字段不一致并不致命走到解压，必须先不拦死；
		// 但明显不对说明文件已不是预期生成口径，继续解只会产生不透明垃圾。
		return nil, fmt.Errorf("asb: 加密长度 %d 与实际 %d 不符", encSize, len(raw)-asbHeaderSize)
	}
	key := uncompSize ^ 0x9e370001
	body := raw[asbHeaderSize:]
	n := len(body) / 4
	plain := make([]byte, 0, len(body))
	buf := make([]byte, n*4)
	for i := 0; i < n; i++ {
		w := binary.LittleEndian.Uint32(body[i*4:])
		binary.LittleEndian.PutUint32(buf[i*4:], w-key)
	}
	plain = append(plain, buf...)
	plain = append(plain, body[n*4:]...) // 尾字节原样保留（变换整 floor(len/4) 个 dword）
	zr, err := zlib.NewReader(bytes.NewReader(plain))
	if err != nil {
		return nil, fmt.Errorf("asb: zlib 起始失败 %w", err)
	}
	defer zr.Close()
	out, err := io.ReadAll(zr)
	if err != nil {
		return nil, fmt.Errorf("asb: zlib 解压失败 %w", err)
	}
	if uncompSize != 0 && uint32(len(out)) != uncompSize {
		// 长度只是校验口径，实际以 zlib 流的完整输出为准；差异记作错误，
		// 因为截断会把跳转表打错位，后面的跳转校验会误报更难定位。
		return nil, fmt.Errorf("asb: 解压长度 %d 与头 %d 不符", len(out), uncompSize)
	}
	return out, nil
}

// Parse 解码解压后流，依次产出指令与标签索引。
// 指令边界只信每条的 size；任何越界都报错终止（调度表里内嵌的偏移必须可信）,
// 绝不猜长度往下一条跳——猜出来的 PC 会静默执行错代码。
func Parse(name string, data []byte) (*Script, error) {
	s := &Script{
		Name:     name,
		Data:     data,
		Labels:   make(map[string]int),
		byOffset: make(map[int]*Instruction),
	}
	for pc := 0; pc < len(data); {
		if pc+instrMin > len(data) {
			return nil, fmt.Errorf("asb %s: 偏移 %#x 处剩余 %d 字节放不下指令头", name, pc, len(data)-pc)
		}
		size := int(binary.LittleEndian.Uint16(data[pc:]))
		if size < instrMin || pc+size > len(data) {
			return nil, fmt.Errorf("asb %s: 偏移 %#x 指令 size %d 越界", name, pc, size)
		}
		opcode := binary.LittleEndian.Uint16(data[pc+2:])
		field := binary.LittleEndian.Uint32(data[pc+4:])
		argc := int(binary.LittleEndian.Uint16(data[pc+8:]))
		line := binary.LittleEndian.Uint16(data[pc+10:])
		args, raw, err := parseArgs(data[pc+12 : pc+size])
		if err != nil {
			return nil, fmt.Errorf("asb %s: 偏移 %#x 节点解析 %w", name, pc, err)
		}
		if len(args) != argc {
			// argc 是权威计数；节点是否恰好填满记录由解析端守住，
			// 若数对不上说明记录内还有非节点填充（应无），但不能放行。
			return nil, fmt.Errorf("asb %s: 偏移 %#x argc %d 与实际节点 %d 不符", name, pc, argc, len(args))
		}
		ins := &Instruction{
			Offset: pc, Size: size, Opcode: opcode, Field: field,
			Argc: argc, Line: line, Args: args, RawArgs: raw,
		}
		s.Instructions = append(s.Instructions, ins)
		s.byOffset[pc] = ins
		if opcode == 0x02 {
			// 标签的真名只有第一条带字符串时才成立（如 main 首条无参只起锚）。
			if len(args) == 1 && args[0].Type == NodeString {
				s.Labels[args[0].Str] = pc
			}
		}
		pc += size
	}
	return s, nil
}

func parseArgs(region []byte) ([]ArgNode, [][]byte, error) {
	var args []ArgNode
	var raws [][]byte
	for pos := 0; pos < len(region); {
		if pos+argNodeMin > len(region) {
			return nil, nil, fmt.Errorf("节点头越界 @%d（剩余 %d）", pos, len(region)-pos)
		}
		sz := int(region[pos])
		typ := NodeType(region[pos+1])
		if sz < argNodeMin || pos+sz > len(region) {
			return nil, nil, fmt.Errorf("节点 size %d 越界 @%d", sz, pos)
		}
		payload := region[pos+2 : pos+sz]
		node := ArgNode{Type: typ}
		switch typ {
		case NodeString:
			node.Str = decodeScriptString(payload)
		case NodeInt:
			if len(payload) != 4 {
				return nil, nil, fmt.Errorf("int 节点 payload 应 4 字节，实 %d", len(payload))
			}
			node.Int = int32(binary.LittleEndian.Uint32(payload))
		case NodeBool:
			if len(payload) != 1 {
				return nil, nil, fmt.Errorf("bool 节点 payload 应 1 字节，实 %d", len(payload))
			}
			node.Bool = payload[0] != 0
		case NodeVarRef:
			if len(payload) != 4 {
				return nil, nil, fmt.Errorf("var-ref payload 应 4 字节，实 %d", len(payload))
			}
			node.Namespace = binary.LittleEndian.Uint16(payload[0:2])
			node.Index = binary.LittleEndian.Uint16(payload[2:4])
		case NodeLabelID:
			if len(payload) != 2 {
				return nil, nil, fmt.Errorf("label payload 应 2 字节，实 %d", len(payload))
			}
			node.Label = binary.LittleEndian.Uint16(payload)
		case NodeEq, NodeNe, NodeLt, NodeLe, NodeGt, NodeGe, NodeOr, NodeAnd,
			NodeAssign, NodeAddAssign, NodeAdd, NodeSub, NodeMul, NodeDiv,
			NodeNeg, NodeCompound, NodePush, NodePush2, NodeEnd, NodePairPut:
			// 无载荷的算子；若带载荷按“observed size 2”守住仍放行，
			// 长尾异常先不拦截（避免误伤异版本文件），只影响该节点。
		default:
			// 未观测类型（0/1/2/16…）：保留原始字节，绝不静默当别的东西算。
			// 求值器一碰到它就报错，解析层只保证“无损保留”。
		}
		args = append(args, node)
		raw := make([]byte, sz)
		copy(raw, region[pos:pos+sz])
		raws = append(raws, raw)
		pos += sz
	}
	return args, raws, nil
}

// decodeScriptString 解字符串节点。原版是 CP932，汉化后的 ASB 一般写成
// UTF-8；两者用“严格 UTF-8 且含非 ASCII”区分：纯 ASCII 两种编码完全一致，
// CP932 的双字节序列几乎必然不是合法 UTF-8，所以不会把日文原句误判成 UTF-8。
// 末尾的 0 必须按字节先切掉，不能先解码再截（多字节字符会被截断）。
func decodeScriptString(payload []byte) string {
	for i, b := range payload {
		if b == 0 {
			payload = payload[:i]
			break
		}
	}
	// 有些编辑器会写 UTF-8 BOM；ASB 的节点长度包含它，不去掉会在对话框
	// 开头显示一个不可见字符，翻译工具也会把它当正文。
	if len(payload) >= 3 && payload[0] == 0xEF && payload[1] == 0xBB && payload[2] == 0xBF {
		payload = payload[3:]
	}
	if utf8.Valid(payload) && hasNonASCII(payload) {
		return string(payload)
	}
	return textcodec.DecodeCP932(payload)
}

func hasNonASCII(b []byte) bool {
	for _, c := range b {
		if c >= 0x80 {
			return true
		}
	}
	return false
}

// Unmarshal 是给 arc 层（Agent B）准备的组合入口：手里只有抽出的加密条目字节时，
// 一句走完“解压 → 解码 → 建索引”。其余地方按需只调其中一段。
func Unmarshal(name string, raw []byte) (*Script, error) {
	data, err := DecompressASB(raw)
	if err != nil {
		return nil, err
	}
	return Parse(name, data)
}
