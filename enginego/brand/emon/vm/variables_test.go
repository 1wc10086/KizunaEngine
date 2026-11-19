package vm

import (
	"strings"
	"testing"

	"kizuna/enginego/brand/emon/script"
)

func TestFullwidthNameAssignments(t *testing.T) {
	// 真实脚本：#SetNumber(３Ｐイベント|=0x0001,...) 与
	// #IfJump(放課後４Ｐ追加,(３Ｐイベント&0x0222)==0x0022,...)
	// 计数变量在 ASinitgame 里有 =0 初始化后才会 +=（和原引擎一致：
	// 未定义变量是字符串，加法走字符串拼接）。
	m := newTestMachine()
	runBody(t, m, "fw", "#SetNumber(３Ｐイベント=0)\n#SetNumber(妊娠ポイント=0)\n#SetNumber(３Ｐイベント|=0x0222)\n#SetNumber(イベントフラグ|=0x0800)\n#SetNumber(妊娠ポイント+=1)\n#End()\n")
	if got := m.Context().Variables.Lookup("３Ｐイベント").AsInt(); got != 0x222 {
		t.Errorf("３Ｐイベント = %#x, want 0x222", got)
	}
	if got := m.Context().Variables.Lookup("イベントフラグ").AsInt(); got != 0x800 {
		t.Errorf("イベントフラグ = %#x, want 0x800", got)
	}
	if got := m.Context().Variables.Lookup("妊娠ポイント").AsInt(); got != 1 {
		t.Errorf("妊娠ポイント = %d, want 1", got)
	}
	if got := m.condition("(３Ｐイベント&0x0222)==0x0222"); !got {
		t.Error("(３Ｐイベント&0x0222)==0x0222 = false, want true")
	}
}

func TestVolumeFloatAssignments(t *testing.T) {
	// 真实脚本 ADVsetting.txt：#SetSysNumber(効果音VOLUME=0.5) 等，
	// 浮点等式/比较必须保留小数语义（见 expression_test.go 的 0.5>0）。
	m := newTestMachine()
	runBody(t, m, "vol", "#SetSysNumber(効果音VOLUME=0.5)\n#Number(SEVOLUME=1.0)\n#End()\n")
	if got := m.Context().SystemVariables.Lookup("効果音VOLUME").AsString(); got != "0.5" {
		t.Errorf("効果音VOLUME = %q, want 0.5", got)
	}
	// SEVOLUME 是原引擎用 sub_418640 注册到系统变量表的设置变量，
	// #Number 赋值会写进系统变量（重启后随 emesys.dat 恢复）。
	if got := m.Lookup("SEVOLUME").AsString(); got != "1.0" {
		t.Errorf("SEVOLUME = %q, want 1.0", got)
	}
	if got := m.condition("効果音VOLUME>0"); !got {
		t.Error("効果音VOLUME>0 = false, want true (存为小数字符串时要按文本数值解析)")
	}
	// 小数存为字符串时与数值比较：原引擎 float 0.5 与 int 0 比较是数值比较，
	// 此处经 AsInt 截断为 0（音频路径另有 toVolume255 解析小数串）。
}

func TestSetOnCalListNumberMatchesIntOne(t *testing.T) {
	// 原引擎以「求值结果为整数 1」为命中条件（sub_41FD40 的 v17[6]==1），
	// 不是泛真值：模板返回真值 2 时必须继续向后找。
	m := newTestMachine()
	runBody(t, m, "cal",
		"#SetOnCalListNumber(r,K,K==2,5,9)\n"+
			"#SetOnCalListNumber(s,K,K==2,5,2)\n"+
			"#SetOnCalListNumber(u,K,K==\"b\",a,b)\n"+
			"#End()\n")
	if got := m.Context().Variables.Lookup("r").AsInt(); got != -1 {
		t.Errorf("r = %d, want -1 (无命中)", got)
	}
	if got := m.Context().Variables.Lookup("s").AsInt(); got != 1 {
		t.Errorf("s = %d, want 1", got)
	}
	if got := m.Context().Variables.Lookup("u").AsInt(); got != 1 {
		t.Errorf("u = %d, want 1 (字符串键要加引号再求值)", got)
	}
}

func TestSetOnListStrAndRandom(t *testing.T) {
	m := newTestMachine()
	runBody(t, m, "list", "#SetOnListStr(RNumberStr,RNumber,brand01.ogg,brand02.ogg)\n#RandomNumber(RNumber,2)\n#End()\n")
	if got := m.Context().Variables.Lookup("RNumberStr").AsString(); got != "brand01.ogg" {
		t.Errorf("RNumberStr = %q, want brand01.ogg", got)
	}
	rn := m.Context().Variables.Lookup("RNumber").AsInt()
	if rn < 0 || rn >= 2 {
		t.Errorf("RNumber = %d, want 0<=RNumber<2", rn)
	}
}

func TestCompoundBitwiseWithTilde(t *testing.T) {
	// 真实脚本：#SetNumber(イベントフラグ&=~0x0040) 与
	// #SetRunAction(ボイス１ボタン選択,NUMBER,VoiceFlag=(VoiceFlag&~0x0001)|((~VoiceFlag)&0x0001))
	m := newTestMachine()
	runBody(t, m, "bits",
		"#SetNumber(イベントフラグ=0x0060)\n"+
			"#SetNumber(イベントフラグ&=~0x0040)\n"+
			"#SetNumber(VoiceFlag=0xffffffff)\n"+
			"#SetNumber(VoiceFlag=(VoiceFlag&~0x0001)|((~VoiceFlag)&0x0001))\n"+
			"#End()\n")
	if got := m.Context().Variables.Lookup("イベントフラグ").AsInt(); got != 0x20 {
		t.Errorf("イベントフラグ = %#x, want 0x20", got)
	}
	if got := m.Context().Variables.Lookup("VoiceFlag").AsInt(); got != -2 {
		t.Errorf("VoiceFlag = %#x, want 0xfffffffe", got)
	}
	if got := m.Context().SystemVariables.Lookup("VoiceFlag").AsInt(); got != 0 {
		t.Logf("VoiceFlag in system store = %d (info only, local is authoritative here)", got)
	}
}

func TestIfNumberRealForms(t *testing.T) {
	// 真实脚本里的 IfNumber 形态：多条件 + 赋值混合。
	m := newTestMachine()
	runBody(t, m, "ifnum",
		"#SetNumber(調教ポイント=0)\n"+
			"#SetNumber(調教回数=0)\n"+
			"#IfNumber(調教ポイント+=1,調教回数==3)\n"+
			"#IfNumber(調教ポイント+=1,調教回数<1)\n"+
			"#End()\n")
	if got := m.Context().Variables.Lookup("調教ポイント").AsInt(); got != 1 {
		t.Errorf("調教ポイント = %d, want 1（只有第二条条件满足时才加）", got)
	}
}

func TestSetOnCalListNumberTemplateQuotesLikeOriginal(t *testing.T) {
	// 真实宏 GetCGFileNumber：
	//   #SetOnCalListNumber(変数１,LISTDATA,引数１=="LISTDATA"+".bmp",OP-01a,...)
	// 原引擎 sub_41FD10 只给"整串 ASCII 数字"加引号；OP-01a 必须原样替换，
	// 否则模板变成 引数１==""OP-01a""+".bmp"，求值失败，CG 编号恒为 -1。
	m := newTestMachine()
	m.SetVariable("引数１", script.EmonValueFromString("OP-02b.bmp"))
	m.SetVariable("LISTDATA", script.EmonValueFromString("OP-02b"))
	runBody(t, m, "calq",
		"#SetOnCalListNumber(r,LISTDATA,引数１==\"LISTDATA\"+\".bmp\",OP-01a,OP-02b)\n"+
			"#End()\n")
	if got := m.Context().Variables.Lookup("r").AsInt(); got != 1 {
		t.Errorf("r = %d, want 1 (OP-02b.bmp 命中第二项)", got)
	}

	// 纯数字候选按原引擎加引号，才能与模板里的字符串键比较。
	m2 := newTestMachine()
	runBody(t, m2, "caln",
		"#SetOnCalListNumber(s,N,N==\"2\",1,2)\n"+
			"#End()\n")
	if got := m2.Context().Variables.Lookup("s").AsInt(); got != 1 {
		t.Errorf("s = %d, want 1 (数字候选要加引号)", got)
	}
}

// captureSaves 记录最近一次写入，用于断言 Save 的字段顺序。
type captureSaves struct {
	fakeSaves
	last *EmonSaveData
}

func (c *captureSaves) Write(data *EmonSaveData) bool {
	c.last = data
	return true
}

func TestSaveFieldsPreserveArgumentOrder(t *testing.T) {
	// 真实宏 ゲームセーブ：
	//   #Save(slot,変数１,変数３,REALTIME_TYPEA,変数５,THUMBNAIL,...)
	// 读档界面 #CheckSaveData(slot,変数１,変数３,変数４,変数５,...)
	// 第 3 个字段（REALTIME_TYPEA）必须落在 変数４ 的位置，不能被排到最前面。
	m := newTestMachine()
	cap := &captureSaves{}
	m.context.Saves = cap
	runBody(t, m, "saveorder",
		"#SetNumber(変数１=甲,変数３=乙,変数５=丙)\n"+
			"#Save(2,変数１,変数３,REALTIME_TYPEA,変数５,THUMBNAIL,サムネイル画像,0,0,1,1)\n"+
			"#End()\n")
	if cap.last == nil {
		t.Fatal("Save 没有写入数据")
	}
	fields := cap.last.Fields
	if len(fields) != 4 {
		t.Fatalf("fields = %d, want 4 (%v)", len(fields), fields)
	}
	if got := fields[0].AsString(); got != "甲" {
		t.Errorf("fields[0] = %q, want 甲", got)
	}
	if got := fields[1].AsString(); got != "乙" {
		t.Errorf("fields[1] = %q, want 乙", got)
	}
	if got := fields[2].AsString(); !strings.Contains(got, "-") || !strings.Contains(got, ":") {
		t.Errorf("fields[2] = %q, want REALTIME_TYPEA 的日期时刻", got)
	}
	if got := fields[3].AsString(); got != "丙" {
		t.Errorf("fields[3] = %q, want 丙", got)
	}
	if cap.last.Comment != "丙" {
		t.Errorf("comment = %q, want 丙", cap.last.Comment)
	}
}

func TestStoredIdentifierComparesAsString(t *testing.T) {
	// 真实脚本 #SetNumber(コメント=オープニング) 会把不带引号的单词存成
	// identifier；原引擎只有字符串类型，读取时它就是 "オープニング"。
	// 若原样进入表达式求值，変数１==0 会被 tryGetInteger 当成 0==0，
	// 存档页就会把已有存档的槽位显示成空槽。
	m := newTestMachine()
	m.SetVariable("変数１", script.EmonValueFromIdentifier("オープニング"))
	if got := m.Lookup("変数１").AsString(); got != "オープニング" {
		t.Fatalf("lookup = %q, want オープニング", got)
	}
	if m.condition("変数１==0") {
		t.Error(`"オープニング"==0 = true, want false`)
	}
	if !m.condition("変数１!=0") {
		t.Error(`"オープニング"!=0 = false, want true`)
	}
}
