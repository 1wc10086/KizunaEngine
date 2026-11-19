package script

import (
	"strconv"
	"strings"
)

// Value 是 VM 里唯一的运行时值。Why 单窄：原引擎 parse args 的栈元素
// 本来就是“三选一加一种特殊”（整数/字符串/真值/变量引用解出后亦是这几种），
// 把它们并入一个结构可以让表达式求值只做一次类型分派；
// 分开存会让每个算子都要写两次分支，后面 28 个节点类型极易漏。
type Value struct {
	Num   int32
	Str   string
	IsStr bool
	// IsVarRef 只在“还没解出的引用”时占位；一经求值即展开，
	// 存入变量时绝不再存引用形态（否则查别名会无限跟）。
	NS       uint16
	Index    uint16
	IsVarRef bool
}

// LabelValue 是 label 引用的求值形态：专属类型而非 int，是因为
// 条件里 `(label, 1)==真` 的布尔比较与数值比较不可混（op 0x04 的
// fd==1 探测、macro 里的 label 判断都会走这里），类型放错里标签就
// 会被当成普通整数参与算术，进而把 110 当仓走。
type LabelValue struct {
	Label uint16
}

func IntValue(v int32) Value  { return Value{Num: v} }
func StrValue(s string) Value { return Value{Str: s, IsStr: true} }
func BoolValue(b bool) Value {
	if b {
		return Value{Num: 1}
	}
	return Value{Num: 0}
}
func VarRefValue(ns, idx uint16) Value {
	return Value{NS: ns, Index: idx, IsVarRef: true}
}

func (v Value) IsString() bool { return v.IsStr && !v.IsVarRef }

// AsInt 把任意形态按原引擎语义转整数：字符串为纯数值才解析，
// 其中“非数字字符串算 0”是故意的（emacfg 文本对数值做前缀匹配，
// 非数混入是场景里常见的人物名变量，猜出 0 比整串截断进算术更稳）。
func (v Value) AsInt() int32 {
	if v.IsVarRef || v.IsStr {
		return parseLooseInt(v.Str)
	}
	return v.Num
}

// AsBool 判真假。Why 刻意“字符串非空即真、数值非零即真”，与 emon
// EvaluateCondition 一致：条件里两种形態都流经同一入口，去掉任一会
// 让 (pt(5,1)=="") 这类字符串条件不是空栈就是多栈。
func (v Value) AsBool() bool {
	if v.IsVarRef || v.IsStr {
		return len(v.Str) != 0
	}
	return v.Num != 0
}

func (v Value) AsString() string {
	if v.IsVarRef {
		return ""
	}
	if v.IsStr {
		return v.Str
	}
	return strconv.FormatInt(int64(v.Num), 10)
}

// Display 在有“引用→字符串”格式化的上下文里用的就是它；
// 与 AsString 区别仅在于布尔真值也格式化为 "1" 而非 "true"，
// 这与原 parse args 的 fmt 习惯一致，也是按搜索/比较结果写回变量时的形态。
func (v Value) Display() string { return v.AsString() }

func (v Value) IsNumeric() bool {
	if v.IsVarRef || v.IsStr {
		return false
	}
	return true
}

// Equals / Compare 按实测顺序：两侧都是引用展开后的“类型”优先，
// 引用展开为字符串时若另一侧也是数值文本则回退数值比较，
// 这样不会把 “bg011b” 与 int 0 比较时凭空产生非 0 误差。
func (v Value) Equals(other Value) bool {
	// 布尔的 1 与数值 1 同一数值空间，不设区别。
	if v.IsString() || other.IsString() {
		return v.Display() == other.Display()
	}
	return v.AsInt() == other.AsInt()
}

func (v Value) Compare(other Value) int {
	if v.IsString() || other.IsString() {
		a, b := v.Display(), other.Display()
		if a < b {
			return -1
		}
		if a > b {
			return 1
		}
		return 0
	}
	a, b := v.AsInt(), other.AsInt()
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func parseLooseInt(s string) int32 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	// 带符号的十六进制（宏里有 0x0022 之类的标记位比较）
	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	}
	if len(s) > 2 && (s[:2] == "0x" || s[:2] == "0X") {
		u, err := strconv.ParseUint(s[2:], 16, 32)
		if err != nil {
			return 0
		}
		if neg {
			return -int32(u)
		}
		return int32(u)
	}
	// sscanf("%d") 前缀取整数：尾尾带文字的按其数值前缀，
	// 和原引擎一致；全不是数字则上文已回 0。
	n := 0
	seen := false
	for _, r := range s {
		if r >= '0' && r <= '9' {
			n = n*10 + int(r-'0')
			seen = true
			continue
		}
		if seen {
			break
		}
		return 0
	}
	if !seen {
		return 0
	}
	if neg {
		return -int32(n)
	}
	return int32(n)
}
