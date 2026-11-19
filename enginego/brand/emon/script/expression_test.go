package script

import "testing"

// 以下表达式均取自真实脚本 ASENURE 的剧本（/tmp/emx/scene 解包文本），
// 结论以原引擎伪代码（sub_419250/sub_41A720 等）为准。

func TestExpressionFullwidthNameBeforeAmpersand(t *testing.T) {
	// 真实脚本 #IfJump(放課後４Ｐ追加,(３Ｐイベント&0x0222)==0x0022,調教ポイント>=6)
	// 「３Ｐイベント」以全角数字开头，在 CP932 下首字节是 0x82，原引擎走标识符
	// 分支查变量表；此实现若按全角数字判数值，会把变量名吃成 0。
	store := NewEmonVariableStore()
	store.Set("３Ｐイベント", EmonValueFromInt(0x22))
	if got := EvaluateCondition("(３Ｐイベント&0x0222)==0x0022", store); !got {
		t.Error("(３Ｐイベント&0x0222)==0x0022 = false, want true")
	}

	store.Set("３Ｐイベント", EmonValueFromInt(0))
	if got := EvaluateCondition("(３Ｐイベント&0x0222)==0x0022", store); got {
		t.Error("(３Ｐイベント&0x0222)==0x0022 = true, want false (hiragana or missing var)")
	}
}

func TestExpressionFullwidthDateIsString(t *testing.T) {
	// 真实脚本 #SetNumber(コメント=５日、土曜日)：值是日期字符串，
	// 不能因为首字是全角数字就被解析成整数 0。
	store := NewEmonVariableStore()
	value := Evaluate("５日、土曜日", store)
	if !value.IsInteger() {
		if value.AsString() != "５日、土曜日" {
			t.Errorf("value = %q, want ５日、土曜日", value.AsString())
		}
	} else {
		t.Errorf("value is int %d, want string ５日、土曜日", value.AsInt())
	}
}

func TestExpressionNumericPrefix(t *testing.T) {
	// 原引擎用 sscanf 读数值前缀（"12abc" → 12），Go 曾返回 0。
	store := NewEmonVariableStore()
	if got := Evaluate("12abc", store); got.AsInt() != 12 {
		t.Errorf("12abc = %d, want 12", got.AsInt())
	}
	if got := Evaluate("0x8000", store); got.AsInt() != 0x8000 {
		t.Errorf("0x8000 = %d, want %d", got.AsInt(), 0x8000)
	}
	if got := Evaluate("0xffffffff", store); got.AsInt() != -1 {
		t.Errorf("0xffffffff = %d, want -1 (符号翻转)", got.AsInt())
	}
}

func TestExpressionBitwisePrecedence(t *testing.T) {
	// 原引擎优先级（sub_41A720）：& 与移位同级 > | > ^|。
	// Go 曾把 ^ 排在 | 之上（^ 更紧），表达式结果相反：
	//   1|2^3 → 原引擎 (1|2)^3 = 0，旧实现 1|(2^3) = 1。
	store := NewEmonVariableStore()
	if got := Evaluate("1|2^3", store); got.AsInt() != 0 {
		t.Errorf("1|2^3 = %d, want 0 ((1|2)^3)", got.AsInt())
	}
	// & 与移位同级左结合：1&3<<2 → (1&3)<<2 = 4；若移位更紧则是 1&(3<<2) = 0。
	if got := Evaluate("1&3<<2", store); got.AsInt() != 4 {
		t.Errorf("1&3<<2 = %d, want 4 ((1&3)<<2)", got.AsInt())
	}
	// 比较比位运算更松：1|2==3 → (1|2)==3 = 1。
	if got := EvaluateCondition("1|2==3", store); !got {
		t.Error("1|2==3 = false, want true")
	}
}

func TestExpressionReversedComparisons(t *testing.T) {
	// 原引擎把 "=<"/"=>"/">< 当作 <=/>=/!=（sub_41AF90 二字符表）。
	store := NewEmonVariableStore()
	if got := EvaluateCondition("1=<2", store); !got {
		t.Error("1=<2 = false, want true")
	}
	if got := EvaluateCondition("3=>2", store); !got {
		t.Error("3=>2 = false, want true")
	}
	if got := EvaluateCondition("1><2", store); !got {
		t.Error("1><2 = false, want true")
	}
	if got := EvaluateCondition("2><2", store); got {
		t.Error("2><2 = true, want false")
	}
}

func TestExpressionRealCompare(t *testing.T) {
	// 真实脚本里有浮点音量（#SetSysNumber(効果音VOLUME=0.5)），
	// 比较必须按实数走，不能把 0.5 截成 0。
	store := NewEmonVariableStore()
	if got := EvaluateCondition("0.5>0", store); !got {
		t.Error("0.5>0 = false, want true (旧实现截断为 0)")
	}
	if got := EvaluateCondition("0.5==0", store); got {
		t.Error("0.5==0 = true, want false")
	}
	if got := EvaluateCondition("1.0==1", store); !got {
		t.Error("1.0==1 = false, want true")
	}
}

func TestExpressionRealScriptBits(t *testing.T) {
	// 真实脚本里的标记位与移位表达式。
	store := NewEmonVariableStore()
	store.Set("調教イベント", EmonValueFromInt(0x20))
	if got := EvaluateCondition("(調教イベント&0x0020)!=0", store); !got {
		t.Error("(調教イベント&0x0020)!=0 = false, want true")
	}
	if got := EvaluateCondition("(調教イベント&0x0010)==0", store); !got {
		t.Error("(調教イベント&0x0010)==0 = false, want true")
	}
	// #Number(SelectNumber=0,クリアフラグ|=(1<<ルートフラグ)) 里的移位子句。
	store.Set("ルートフラグ", EmonValueFromInt(3))
	if got := EvaluateCondition("(1<<ルートフラグ)==8", store); !got {
		t.Error("(1<<ルートフラグ)==8 = false, want true")
	}
	// #IfJump(放課後４Ｐ追加,...) 条件里的字符串比较。
	store.Set("RNumberStr", EmonValueFromString("kou-06.txt"))
	if got := EvaluateCondition("(RNumberStr=\"kou-06.txt\")", store); !got {
		t.Error("(RNumberStr=\"kou-06.txt\") = false, want true")
	}
	if got := EvaluateCondition("RNumberStr==0", store); got {
		t.Error("RNumberStr==0 = true, want false (字符串与 0 比较不相等)")
	}
}

func TestDefineSubstitution(t *testing.T) {
	// 真实脚本 ASstaff.txt：
	//   #ReLoadSprite(チップ,#Define(スタッフロールＣＧ枚数,11)
	//   #IfJump(スタッフロール手前,RNumber>=スタッフロールＣＧ枚数)
	// 原引擎在加载期做文本替换（sub_417200/417040），#Define 之后的
	// 所有「スタッフロールＣＧ枚数」都会变成 11。Go 曾把 #Define 当成
	// 普通命令分派，onDefine 又不认「名,值」两参格式，导致该条件恒为真。
	program := Parse("t",
		"#IfJump(スタッフロール手前,RNumber>=スタッフロールＣＧ枚数)\n"+
			"#ReLoadSprite(チップ,#Define(スタッフロールＣＧ枚数,11))\n")
	if len(program.Statements) != 2 {
		t.Fatalf("statements = %d, want 2", len(program.Statements))
	}
	cond := program.Statements[0]
	if got := cond.Argument(1).Raw; got != "RNumber>=11" {
		t.Errorf("cond = %q, want RNumber>=11", got)
	}
	reload := program.Statements[1]
	if got := reload.ArgumentCount(); got != 2 {
		t.Errorf("reload args = %d, want 2 (チップ + 空占位)", got)
	}
	if got := reload.Argument(0).Raw; got != "チップ" {
		t.Errorf("reload arg0 = %q, want チップ", got)
	}
}

func TestStoreCompoundShiftAssign(t *testing.T) {
	// SetFromText 的 <<= / >>= 是原引擎的复合赋值（sub_41AF90 4013116）。
	store := NewEmonVariableStore()
	store.Set("x", EmonValueFromInt(3))
	store.SetFromText("x<<=2")
	if got := store.Lookup("x").AsInt(); got != 12 {
		t.Errorf("x = %d, want 12", got)
	}
	store.SetFromText("x>>=1")
	if got := store.Lookup("x").AsInt(); got != 6 {
		t.Errorf("x = %d, want 6", got)
	}
	store.SetFromText("y=1,x+=4")
	if got := store.Lookup("y").AsInt(); got != 1 {
		t.Errorf("y = %d, want 1", got)
	}
	if got := store.Lookup("x").AsInt(); got != 10 {
		t.Errorf("x = %d, want 10", got)
	}
}
