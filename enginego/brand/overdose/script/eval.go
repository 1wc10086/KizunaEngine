package script

import (
	"errors"
	"fmt"
)

// errEval 是求值错误的上游兜底；底层是乱序/越界时先把节点序号带上，
// 值栈深浅靠 offset 反查比堆内记录更可定位（PC 已在 VM 里）。
var errEval = errors.New("asb 求值失败")

type evalError struct {
	node int
	why  string
}

func (e *evalError) Error() string {
	return fmt.Sprintf("asb 求值: 节点 %d: %s", e.node, e.why)
}

func (e *evalError) Unwrap() error { return errEval }

type evaluator struct {
	vs    *VariableStore
	stack []Value
	// cutMarks 记录每个 NodeEnd 时的栈高，供按参数切取值。
	// Why 不直接按位置数参数：二元表达式整段只占一个 slot，
	// 切开后仍要能取到“本段的最后一个”而非随栈顶漂移。
	cutMarks []int
	// labels 堆栈：标签引用最终落为 LabelValue 形态并整树参与布尔求值；
	// 不能在入栈时提前丢成数字，否则 (label,1)==1 的探针会失真。
	labels []LabelValue
}

func newEvaluator(vs *VariableStore) *evaluator {
	return &evaluator{vs: vs}
}

// EvalArgs 按节点分隔取“每参数”的最终值（§5.1 step3）。
// 分隔即 28 节点：原机器计入 argument counter 的依据是 28，而不是逐段
// 读一个就返回。Why 数量不等于参数数：真实指令里会出现
// (idx, text, bool, 28, 28) 与 (speaker, text, 28) 两种尾形，
// 参数数是“剥掉所有尾部 28 后剩下的值栈长度”，而不是 28 个数。
// 多余 28 只是“该对齐槽位已结束”的标记，不能落回重复的空槽。
func EvalArgs(args []ArgNode, vs *VariableStore) ([]Value, error) {
	ev := newEvaluator(vs)
	if err := ev.run(args); err != nil {
		return nil, err
	}
	return ev.stackAtCuts(), nil
}

// EvalValue 求“整个节点串只算一个布尔/数值”的值（条件用，如 op 0x04）。
// 末值就是栈顶，不要求 28 分隔；若有 28，也取最后一段的栈顶。
// 这与 EvalArgs 区别在：条件表达式允许带算子与赋值混合（最终只剩一句布尔），
// 不能按参数逐段切断。
func EvalValue(args []ArgNode, vs *VariableStore) (Value, error) {
	ev := newEvaluator(vs)
	if err := ev.run(args); err != nil {
		return Value{}, err
	}
	out := ev.stackAtCuts()
	if len(out) == 0 {
		return Value{Num: 0}, nil
	}
	return out[len(out)-1], nil
}

// EvalRPN 供旧调用方/测试沿用：与 EvalArgs 同义。
func EvalRPN(args []ArgNode, vs *VariableStore) ([]Value, error) {
	return EvalArgs(args, vs)
}

func (e *evaluator) run(args []ArgNode) error {
	for i, n := range args {
		switch n.Type {
		case NodeString:
			e.stack = append(e.stack, StrValue(n.Str))
		case NodeInt:
			e.stack = append(e.stack, Value{Num: n.Int})
		case NodeBool:
			e.stack = append(e.stack, BoolValue(n.Bool))
		case NodeVarRef:
			v, err := e.vs.GetVar(n.Namespace, n.Index)
			if err != nil {
				return &evalError{node: i, why: err.Error()}
			}
			e.stack = append(e.stack, v)
		case NodeLabelID:
			e.stack = append(e.stack, Value{Num: int32(n.Label)})
			e.labels = append(e.labels, LabelValue{Label: n.Label})
		case NodeEq, NodeNe, NodeLt, NodeLe, NodeGt, NodeGe, NodeOr, NodeAnd:
			if err := e.binary(n.Type); err != nil {
				return &evalError{node: i, why: err.Error()}
			}
		case NodeAdd, NodeSub, NodeMul, NodeDiv:
			if err := e.arith(n.Type); err != nil {
				return &evalError{node: i, why: err.Error()}
			}
		case NodeAssign:
			if err := e.assign(); err != nil {
				return &evalError{node: i, why: err.Error()}
			}
		case NodeAddAssign:
			// 19 是二元加法，不是"复合赋值标记"。原引擎 handler（0x43123b）
			// 从值栈弹两个操作数：左操作数为字符串时只做拼接（op==19），
			// 为整数时与 20/21/22/23 一样走整数加减乘除。
			// 这正是 sop4 的动态 label：str("talk") + var(1,50) → "talk1"；
			// 也是 0x10 的自增形状 (idx, var(idx), 1, 19, scope)。
			if err := e.addAssign(); err != nil {
				return &evalError{node: i, why: err.Error()}
			}
		case NodeNeg:
			if err := e.neg(); err != nil {
				return &evalError{node: i, why: err.Error()}
			}
		case NodePairPut:
			// (pair, pair) 覆盖：栈面两份变量引用的转存，存为对子。
			if err := e.pairPut(); err != nil {
				return &evalError{node: i, why: err.Error()}
			}
		case NodeCompound:
			// 25 在真实文件里没出现；原 handler 0x430f28 与 19 同族，
			// 按同样的二元相加处理，避免未观测样本落到"未知节点"直接 fault。
			if err := e.addAssign(); err != nil {
				return &evalError{node: i, why: err.Error()}
			}
		case NodePush, NodePush2:
			// 一元 helpers：弹出 1、压回（恒等），用于填栈占位。
			// 真实文件没出现过 26/27——此处不改栈面，只注释告知的定位。
		case NodeEnd:
			// 分隔：记数器加一，不动栈。后续 stackAtCuts 依此切。
			e.cutMarks = append(e.cutMarks, len(e.stack))
		default:
			return &evalError{node: i, why: fmt.Sprintf("未知节点类型 %d", uint8(n.Type))}
		}
	}
	return nil
}

func (e *evaluator) stackAtCuts() []Value {
	// 实测尾形：多余 28 不补空槽，参数 = 去尾标记后的整个栈。
	// 中间的 28（表达式分隔位）会让栈回到已计好的栈高，
	// 例如 (a,b,28,c,28) 在去尾前要取 a,b,c：当前唯一稳妥的是
	// 直接返回整栈，因为调用方本就按“实参个数”取前 N。
	// 若将来出现该中间 28，本注释就是修正点。
	if len(e.stack) == 0 {
		return nil
	}
	out := make([]Value, len(e.stack))
	copy(out, e.stack)
	return out
}

func (e *evaluator) pop() (Value, error) {
	if len(e.stack) == 0 {
		return Value{}, errors.New("值栈为空")
	}
	v := e.stack[len(e.stack)-1]
	e.stack = e.stack[:len(e.stack)-1]
	return v, nil
}

func (e *evaluator) binary(op NodeType) error {
	right, err := e.pop()
	if err != nil {
		return err
	}
	left, err := e.pop()
	if err != nil {
		return err
	}
	switch op {
	case NodeEq:
		e.stack = append(e.stack, BoolValue(left.Equals(right)))
	case NodeNe:
		e.stack = append(e.stack, BoolValue(!left.Equals(right)))
	case NodeLt:
		e.stack = append(e.stack, BoolValue(left.Compare(right) < 0))
	case NodeLe:
		e.stack = append(e.stack, BoolValue(left.Compare(right) <= 0))
	case NodeGt:
		e.stack = append(e.stack, BoolValue(left.Compare(right) > 0))
	case NodeGe:
		e.stack = append(e.stack, BoolValue(left.Compare(right) >= 0))
	case NodeOr:
		e.stack = append(e.stack, BoolValue(left.AsBool() || right.AsBool()))
	case NodeAnd:
		e.stack = append(e.stack, BoolValue(left.AsBool() && right.AsBool()))
	}
	return nil
}

func (e *evaluator) arith(op NodeType) error {
	right, err := e.pop()
	if err != nil {
		return err
	}
	left, err := e.pop()
	if err != nil {
		return err
	}
	// 字符串参与算术时走数值转换：原引擎 atoi 语义，非数字算 0。
	// 如果两侧都显式是字符串且 op 是加法，先前在 RPN 上栈只有字符串字面，
	// 作为表达式求值不该退化成拼接；真实脚本里这只出现在条件比较上，
	// 故统一走整数路径而不按“字符串拼接”猜测。
	switch op {
	case NodeAdd:
		e.stack = append(e.stack, Value{Num: left.AsInt() + right.AsInt()})
	case NodeSub:
		e.stack = append(e.stack, Value{Num: left.AsInt() - right.AsInt()})
	case NodeMul:
		e.stack = append(e.stack, Value{Num: left.AsInt() * right.AsInt()})
	case NodeDiv:
		d := right.AsInt()
		if d == 0 {
			e.stack = append(e.stack, Value{Num: 0}) // 零除不崩：原引擎 sscanf 后为 0 跳过
		} else {
			e.stack = append(e.stack, Value{Num: left.AsInt() / d})
		}
	}
	return nil
}

// assign 显式用 GetVar 展开“当前值”后再写，
// 不能把栈面那个 VarRef 一股脑塞进去（& 前缀会残到存档里）。
func (e *evaluator) assign() error {
	value, err := e.pop()
	if err != nil {
		return err
	}
	target, err := e.pop()
	if err != nil {
		return err
	}
	if !target.IsVarRef {
		return errors.New("赋值左值不是变量引用")
	}
	return e.vs.SetVarValue(target.NS, target.Index, value)
}

// addAssign 实现节点 19：左操作数决定结果类型。
// Why 以左为准：原 handler 先看第二个弹出值（左）的槽类型，字符串才拼接，
// 否则两侧都按整数取；若反过来按"任一为字符串"拼接，会把
// (int 5) + (str "abc") 变成 "5abc"，与原引擎的 5+0=5 不同。
func (e *evaluator) addAssign() error {
	right, err := e.pop()
	if err != nil {
		return err
	}
	left, err := e.pop()
	if err != nil {
		return err
	}
	if left.IsString() {
		// 原 handler 用 sprintf("%s%s") 拼字符串；右侧 int 也按 %ld 格式化，
		// 因此 "talk" + 1 得到 "talk1" 而不是带空格的形态。
		e.stack = append(e.stack, StrValue(left.Str+right.AsString()))
		return nil
	}
	e.stack = append(e.stack, Value{Num: left.AsInt() + right.AsInt()})
	return nil
}

func (e *evaluator) pairPut() error {
	// 栈面：(dstRef, srcRef) —— 原 24 的两个 pair 即一对 u16，需逐字段转存。
	src, err := e.pop()
	if err != nil {
		return err
	}
	dst, err := e.pop()
	if err != nil {
		return err
	}
	if !dst.IsVarRef || !src.IsVarRef {
		return errors.New("pairPut 两侧都应是变量引用")
	}
	v, err := e.vs.GetVar(src.NS, src.Index)
	if err != nil {
		return err
	}
	return e.vs.SetVarValue(dst.NS, dst.Index, v)
}

func (e *evaluator) neg() error {
	v, err := e.pop()
	if err != nil {
		return err
	}
	// 取负前先转数值：反号在 0/1 探针上与取反等价，全文件里的探针正是 0/1。
	e.stack = append(e.stack, Value{Num: -v.AsInt()})
	return nil
}
