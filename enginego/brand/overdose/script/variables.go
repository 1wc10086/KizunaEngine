package script

import (
	"sort"
)

// VariableStore 按 §5.2 提供 6 个命名空间，每组恰 100 项。
// 约定即护栏：超出 100 立即报错而非扩容——原引擎是定长数组，
// 脚本若故意溢出就是坏引用；扩成无界 map 会把越界写进存档，
// 后续 session 的 JSON 里就多出一批无名条目，污染观赏解锁判定。

const (
	varSlots  = 100
	varStrLen = 128
)

type VarKind uint8

const (
	VarBool VarKind = iota
	VarInt
	VarString
)

var namesOf = [6]VarKind{VarBool, VarInt, VarString, VarBool, VarInt, VarString}

type VariableStore struct {
	bools  [2][varSlots]bool
	ints   [2][varSlots]int32
	strs   [2][varSlots][varStrLen]byte
	hasStr [2][varSlots]bool
	strLen [2][varSlots]int
}

func kindOf(ns uint16) (VarKind, bool) {
	if int(ns) >= len(namesOf) {
		return 0, false
	}
	return namesOf[ns], true
}

// 检查：ns 决定 global/local 槽（0..2 global，3..5 local），
// 访问错 kind 不静默换槽——这是防兜底写串的故意拒绝。
func (vs *VariableStore) slot(ns uint16) (int, bool) {
	k, ok := kindOf(ns)
	if !ok {
		return 0, false
	}
	if k == VarBool {
		return 0, true
	}
	if k == VarInt {
		return 1, true
	}
	return 2, true
}

func (vs *VariableStore) group(ns uint16) int {
	if ns < 3 {
		return 0
	}
	return 1
}

func (vs *VariableStore) check(ns, idx uint16) error {
	if _, ok := kindOf(ns); !ok {
		return errUnknownNamespace
	}
	if int(idx) >= varSlots {
		return errVarOutOfRange
	}
	return nil
}

func (vs *VariableStore) GetBool(ns, idx uint16) (bool, error) {
	if err := vs.check(ns, idx); err != nil {
		return false, err
	}
	k, _ := kindOf(ns)
	if k != VarBool {
		return false, errWrongVarKind
	}
	return vs.bools[vs.group(ns)][idx], nil
}

func (vs *VariableStore) SetBool(ns, idx uint16, v bool) error {
	if err := vs.check(ns, idx); err != nil {
		return err
	}
	k, _ := kindOf(ns)
	if k != VarBool {
		return errWrongVarKind
	}
	vs.bools[vs.group(ns)][idx] = v
	return nil
}

func (vs *VariableStore) GetInt(ns, idx uint16) (int32, error) {
	if err := vs.check(ns, idx); err != nil {
		return 0, err
	}
	k, _ := kindOf(ns)
	if k != VarInt {
		return 0, errWrongVarKind
	}
	return vs.ints[vs.group(ns)][idx], nil
}

func (vs *VariableStore) SetInt(ns, idx uint16, v int32) error {
	if err := vs.check(ns, idx); err != nil {
		return err
	}
	k, _ := kindOf(ns)
	if k != VarInt {
		return errWrongVarKind
	}
	vs.ints[vs.group(ns)][idx] = v
	return nil
}

func (vs *VariableStore) GetString(ns, idx uint16) (string, error) {
	if err := vs.check(ns, idx); err != nil {
		return "", err
	}
	k, _ := kindOf(ns)
	if k != VarString {
		return "", errWrongVarKind
	}
	g := vs.group(ns)
	n := vs.strLen[g][idx]
	return string(vs.strs[g][idx][:n]), nil
}

func (vs *VariableStore) SetString(ns, idx uint16, s string) error {
	if err := vs.check(ns, idx); err != nil {
		return err
	}
	k, _ := kindOf(ns)
	if k != VarString {
		return errWrongVarKind
	}
	g := vs.group(ns)
	b := []byte(s)
	if len(b) >= varStrLen {
		b = b[:varStrLen-1] // 原引擎 char[128] 留零位，超长截断不另起空间
	}
	clear(vs.strs[g][idx][:])
	copy(vs.strs[g][idx][:], b)
	vs.strLen[g][idx] = len(b)
	vs.hasStr[g][idx] = true
	return nil
}

// GetVar 把引用展开成当前值：变量存的原始形态决定，
// 字符串槽即使从未写过也定为 ""，避免 GetVar 确认存在性时分叉。
func (vs *VariableStore) GetVar(ns, idx uint16) (Value, error) {
	if err := vs.check(ns, idx); err != nil {
		return Value{}, err
	}
	kind, _ := kindOf(ns)
	switch kind {
	case VarBool:
		b, _ := vs.GetBool(ns, idx)
		if b {
			return Value{Num: 1}, nil
		}
		return Value{Num: 0}, nil
	case VarInt:
		n, _ := vs.GetInt(ns, idx)
		return Value{Num: n}, nil
	default:
		s, _ := vs.GetString(ns, idx)
		return StrValue(s), nil
	}
}

// SetVarValue 与 GetVar 对称：字符串槽收任何类型的 Display，
// 使其余命名空间也与 0x11 的按类型收字面量? 原 0x10 只处理 int 需要强制转，
// 这里通过按目标 namespace 的 kind 做一次适配，必要时在 JS 侧已把类型收窄。
func (vs *VariableStore) SetVarValue(ns, idx uint16, v Value) error {
	if err := vs.check(ns, idx); err != nil {
		return err
	}
	k, _ := kindOf(ns)
	switch k {
	case VarBool:
		return vs.SetBool(ns, idx, v.AsBool())
	case VarInt:
		return vs.SetInt(ns, idx, v.AsInt())
	default:
		return vs.SetString(ns, idx, v.Display())
	}
}

// Snapshot 输出全量 600 槽（供存档）。写成数组而非 map：
// 槽号在脚本里就是绝对下标，map 会把顺序在恢复时打乱、产生难以排查的漂移。
func (vs *VariableStore) Snapshot() map[namespacedInt]string {
	out := make(map[namespacedInt]string)
	for ns := uint16(0); ns < 6; ns++ {
		kind, _ := kindOf(ns)
		for i := uint16(0); i < varSlots; i++ {
			switch kind {
			case VarBool:
				b, _ := vs.GetBool(ns, i)
				if b {
					out[namespacedInt{ns, i}] = "1"
				}
			case VarInt:
				n, _ := vs.GetInt(ns, i)
				if n != 0 {
					out[namespacedInt{ns, i}] = itoa32(n)
				}
			default:
				s, _ := vs.GetString(ns, i)
				if s != "" {
					out[namespacedInt{ns, i}] = s
				}
			}
		}
	}
	return out
}

func (vs *VariableStore) Restore(m map[namespacedInt]string) {
	for key, val := range m {
		kind, ok := kindOf(key.Ns)
		if !ok || int(key.Index) >= varSlots {
			continue
		}
		switch kind {
		case VarBool:
			_ = vs.SetBool(key.Ns, key.Index, val == "1")
		case VarInt:
			_ = vs.SetInt(key.Ns, key.Index, parseLooseInt(val))
		default:
			_ = vs.SetString(key.Ns, key.Index, val)
		}
	}
}

type namespacedInt struct {
	Ns    uint16
	Index uint16
}

var (
	errUnknownNamespace = errSimple("未知命名空间（合法 0..5）")
	errVarOutOfRange    = errSimple("变量下标越界（合法 0..99）")
	errWrongVarKind     = errSimple("变量读写类型与空间不匹配")
)

type errSimple string

func (e errSimple) Error() string { return string(e) }

func itoa32(v int32) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	u := uint32(v)
	if neg {
		u = uint32(-v)
	}
	var b [12]byte
	i := len(b)
	for u > 0 {
		i--
		b[i] = byte('0' + u%10)
		u /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func (vs *VariableStore) Clone() *VariableStore {
	c := &VariableStore{}
	*c = *vs
	return c
}

func (vs *VariableStore) Dump() []dumpEntry {
	var out []dumpEntry
	for ns := uint16(0); ns < 6; ns++ {
		kind, _ := kindOf(ns)
		for i := uint16(0); i < varSlots; i++ {
			v, err := vs.GetVar(ns, i)
			if err != nil {
				continue
			}
			// 只 dump 非默认（默认就是全零/空串在存档里省略）
			if kind == VarInt && v.Num == 0 {
				continue
			}
			if kind == VarBool && v.Num == 0 {
				continue
			}
			if kind == VarString && v.Display() == "" {
				continue
			}
			out = append(out, dumpEntry{Ns: ns, Index: i, Value: v})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Ns != out[j].Ns {
			return out[i].Ns < out[j].Ns
		}
		return out[i].Index < out[j].Index
	})
	return out
}

type dumpEntry struct {
	Ns    uint16
	Index uint16
	Value Value
}
