package slg

import (
	"testing"

	"kizuna/engine/brand/bishop/data/scriptdb"
)

// fakeHost 是表达式/命令层的最小宿主。
//
// 测试只用它验证"语句到宿主调用"的映射，不依赖 session 的加载顺序；
// DB 用 scriptdb 的真实 CSV 解析器建表，避免手写 table 结构体和实现脱节。
type fakeHost struct {
	properties map[string]int32
	db         *scriptdb.DB
	private    map[string]any
	common     map[string]any

	achievements []string
	states       []string
	calls        []string
	rates        map[string]int32
	animations   map[string]int32
	objects      int32
	action       int32
	labels       map[string]bool
}

func newFakeHost() *fakeHost {
	return &fakeHost{
		properties: map[string]int32{},
		private:    map[string]any{},
		common:     map[string]any{},
		rates:      map[string]int32{},
		animations: map[string]int32{},
		labels:     map[string]bool{},
	}
}

func (h *fakeHost) DB() *scriptdb.DB                     { return h.db }
func (h *fakeHost) Property(name string) int32           { return h.properties[name] }
func (h *fakeHost) SetProperty(name string, value int32) { h.properties[name] = value }
func (h *fakeHost) PrivateData() map[string]any          { return h.private }
func (h *fakeHost) CommonData() map[string]any           { return h.common }
func (h *fakeHost) UnlockAchievement(ids ...string)      { h.achievements = append(h.achievements, ids...) }
func (h *fakeHost) EnterState(name string)               { h.states = append(h.states, name) }
func (h *fakeHost) CustomMap() int32                     { return 7 }
func (h *fakeHost) AutoSave()                            { h.calls = append(h.calls, "auto-save") }
func (h *fakeHost) Movie(name string) bool               { h.calls = append(h.calls, "movie:"+name); return true }
func (h *fakeHost) Title() bool                          { return true }
func (h *fakeHost) SetGalleryRate(kind string) int32     { return h.rates[kind] }
func (h *fakeHost) WaitAnimating(name string, value int32) int32 {
	h.calls = append(h.calls, "anim:"+name)
	return h.animations[name]
}
func (h *fakeHost) ActionType(name string) int32 {
	h.calls = append(h.calls, "action:"+name)
	return h.action
}
func (h *fakeHost) GetObjectTest(string) int32 { return h.objects }
func (h *fakeHost) CallLabel(label string) bool {
	h.calls = append(h.calls, "label:"+label)
	return h.labels[label]
}

// loadDB 用真实的 CSV 解析器建表。
//
// 列顺序按作品文件：var_base_param.csv 的表头是"基本パラメータ,早紀,ありさ,美香子"
// （第一列是行名），所以角色是列、参数是行——dbView 的转置正是靠它验证的。
func loadDB(t *testing.T) *scriptdb.DB {
	t.Helper()
	files := map[string]string{
		"var_base_param": `"基本パラメータ","早紀","ありさ","美香子"
"調教進行度",0,0,0
"従順度",30,0,0
"調教受容度",10,0,0
"口感度",0,2,6
"胸感度",8,0,15
"性器感度",0,6,20
"尻感度",0,0,0
"肌感度",5,3,12
"BT値",90,0,0
"気力",100,100,100
"状態",0,0,0
`,
		"var_training": `"調教回数","早紀","ありさ","美香子"
"愛撫",3,0,0
"ローター愛撫",0,0,0
`,
		"var_log": `"調教ログ","早紀","ありさ","美香子"
"まぐろ回数",2,0,0
"総調教回数",5,0,0
"快楽系調教回数",0,0,0
"その他調教回数",0,0,0
"膣挿入回数",0,0,0
"膣出し回数",0,0,0
`,
		"var_territory_correction": `"アジト補正","共通"
"従順度",5
"調教受容度",7
"状態",2
`,
		"var_hasobjs": `オブジェ,共通
重り,3
浣腸器＋洗面器,0
`,
		"var_db": `{
	"common": { "ID_TERCOR": "var_territory_correction", "ID_HASOBJS": "var_hasobjs" },
	"private": { "ID_BP": "var_base_param", "ID_TC": "var_training", "ID_LOG": "var_log" }
}`,
	}
	return scriptdb.Load(func(name string) ([]byte, bool) {
		text, ok := files[name]
		return []byte(text), ok
	})
}

// TestExpressionEvaluation 钉住求值语义。
//
// 用例全部来自作品剧本的原句（注释里标了出处），因为这套语法的坑都在
// "看起来像 C 但优先级不同"的地方：`&` 高于比较、`=` 是赋值不是相等。
func TestExpressionEvaluation(t *testing.T) {
	host := newFakeHost()
	host.db = loadDB(t)
	host.properties["#saki_energy"] = 42
	host.properties["#w"] = 3
	host.animations["#result"] = 1

	cases := []struct {
		name string
		expr string
		want int32
	}{
		{"literal", "7", 7},
		{"hex", "0x1", 1},
		{"negative-zero", "-0", 0},
		{"add-sub", "10 - 4 + 1", 7},
		{"mul-div-precedence", "2 + 3 * 4", 14},
		{"division-truncates", "7 / 2", 3},
		{"division-by-zero", "7 / 0", 0},
		{"modulo-by-zero", "7 % 0", 0},
		{"variable", "#saki_energy + 1", 43},
		{"unknown-variable", "#nope + 5", 5},
		// $#表[角色,'参数']：剧本的行号是角色，CSV 的列才是角色。
		{"db-parameter", "$#ID_BP[0,'胸感度']", 8},
		{"db-second-character", "$#ID_BP[2,'性器感度']", 20},
		{"db-unknown-cell", "$#ID_BP[0,'存在しない']", 0},
		{"db-unknown-table", "$#NOPE[0,'x']", 0},
		// $@表[0,'列']：只有一列的共享表，角色下标退化成第一列。
		{"db-common", "$@ID_TERCOR[0,'状態']", 2},
		// $@表['行名']：一维写法，行名 + 第一列。
		{"db-one-dimensional", "$@ID_HASOBJS['重り']", 3},
		{"db-one-dimensional-missing", "$@ID_HASOBJS['無い']", 0},
		// `( $#ID_BP[0,'BT値'] == 100 ) && ( ... & 0x1 ) == 0` 的原句：
		// BT 是 90、状態 是 2，两边都不成立。
		{"bitwise-inside-compare", "$#ID_BP[0,'BT値'] & 0x2", 2},
		{"and-short-circuit", "( 1 == 2 ) && ( 1 / 0 )", 0},
		{"or", "( 1 == 2 ) || ( 3 > 1 )", 1},
		{"not", "!0", 1},
		{"not-nonzero", "!5", 0},
		{"compare-chain", "2 * 3 == 6", 1},
		{"parentheses", "( 2 + 3 ) * 4", 20},
		{"bit-or", "4 | 1", 5},
		{"bit-and", "6 & 3", 2},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			body, err := parseExpression(testCase.expr)
			if err != nil {
				t.Fatalf("parse %q: %v", testCase.expr, err)
			}
			evaluate := evaluator{host: host}
			if got := body.eval(&evaluate); got != testCase.want {
				t.Fatalf("%q = %d, want %d", testCase.expr, got, testCase.want)
			}
		})
	}
}

// TestTailStatements 钉住 SELECT_TAIL frame[1] 的分派结果。
func TestTailStatements(t *testing.T) {
	cases := []struct {
		name       string
		statement  string
		want       int32
		wantProps  map[string]int32
		wantStates []string
	}{
		{
			name:      "assign-expression",
			statement: "#_w = $#ID_BP[0,'従順度'] * 100 / 255",
			want:      11,
			wantProps: map[string]int32{"#_w": 11},
		},
		{
			name:      "compound-add",
			statement: "$#ID_LOG[0,'まぐろ回数'] += 1",
			want:      3,
		},
		{
			name:      "compound-sub",
			statement: "$#ID_BP[0,'BT値'] -= 10",
			want:      80,
		},
		{
			name:      "bit-or-write",
			statement: "$#ID_BP[0,'状態'] |= 0x1",
			want:      1,
		},
		{
			name: "and-of-compare-and-bit-test",
			// 原句：BT 已经 100 且アジト補正没有禁用 BT 时为 1。
			// 测试表 BT=90、状態=2，两个条件都不成立 -> 0。
			statement: "#saki_BT = ( $#ID_BP[0,'BT値'] == 100 ) && ( $@ID_TERCOR['状態'] & 0x1 ) == 0",
			want:      0,
			wantProps: map[string]int32{"#saki_BT": 0},
		},
		{
			name:       "state-command",
			statement:  "state training",
			want:       1,
			wantStates: []string{"training"},
		},
		{
			name:      "achievement-command",
			statement: "実績解除 ID17",
			want:      1,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			host := newFakeHost()
			host.db = loadDB(t)
			host.rates["cg"] = 66
			host.animations["#result"] = 1
			if got := Tail(host, testCase.statement); got != testCase.want {
				t.Fatalf("Tail(%q) = %d, want %d", testCase.statement, got, testCase.want)
			}
			for name, want := range testCase.wantProps {
				if got := host.properties[name]; got != want {
					t.Fatalf("%s = %d, want %d", name, got, want)
				}
			}
			if len(testCase.wantStates) > 0 {
				if len(host.states) != len(testCase.wantStates) || host.states[0] != testCase.wantStates[0] {
					t.Fatalf("states = %v, want %v", host.states, testCase.wantStates)
				}
			}
		})
	}
}

// TestHostCommands 覆盖只写变量/调用宿主、不返回有用值的命令。
func TestHostCommands(t *testing.T) {
	host := newFakeHost()
	host.db = loadDB(t)
	host.rates["cg"] = 66
	host.animations["#result"] = 0
	host.objects = 12
	host.action = 4

	if got := Tail(host, "達成率 #museum_cg_rate,cg"); got != 1 {
		t.Fatalf("達成率 returned %d", got)
	}
	if host.properties["#museum_cg_rate"] != 66 {
		t.Fatalf("rate = %d", host.properties["#museum_cg_rate"])
	}

	// 动画还没到目标值：结果必须是 0，脚本才会原地重试。
	if got := Tail(host, "is animeted #result , 1"); got != 0 {
		t.Fatalf("is animeted (not ready) = %d, want 0", got)
	}
	host.animations["#result"] = 1
	if got := Tail(host, "is animeted #result , 1"); got != 1 {
		t.Fatalf("is animeted (ready) = %d, want 1", got)
	}

	if got := Tail(host, "custom map"); got != 1 {
		t.Fatalf("custom map = %d", got)
	}
	if host.properties["#result"] != 7 {
		t.Fatalf("custom map result = %d", host.properties["#result"])
	}

	Translate(host, "GetObjectTest( '#item' )")
	if host.properties["#result"] != 12 {
		t.Fatalf("GetObjectTest result = %d", host.properties["#result"])
	}

	Translate(host, "CheckAndUnlockAchievement( 'ID64' )")
	// ID64 要「等身大フィギュア」，测试表里没有这一行 -> 不解锁。
	if len(host.achievements) != 0 {
		t.Fatalf("achievements = %v", host.achievements)
	}
}

// TestCommandCoverage 覆盖剩余命令词的识别边界。
//
// 这些命令都是"前缀 + 参数"的写法，最容易出现的问题是把别的命令
// 当成自己的前缀（`state` 匹配到 `state day_result` 之外的串），
// 所以逐条验证宿主是否真的收到调用。
func TestCommandCoverage(t *testing.T) {
	host := newFakeHost()
	host.db = loadDB(t)
	host.action = 4
	host.rates["scene"] = 100

	if got, ok := newDispatcher(host).command("ActionType1"); !ok || got != 1 {
		t.Fatalf("ActionType1 = %d/%v", got, ok)
	}
	if host.properties["#result"] != 4 {
		t.Fatalf("ActionType result = %d", host.properties["#result"])
	}

	if _, ok := newDispatcher(host).command("state day_result"); !ok {
		t.Fatal("state day_result not recognised")
	}
	if len(host.states) != 1 || host.states[0] != "day_result" {
		t.Fatalf("states = %v", host.states)
	}

	for _, command := range []string{"title", "op", "staffroll", "sync"} {
		if _, ok := newDispatcher(host).command(command); !ok {
			t.Fatalf("%q not recognised", command)
		}
	}

	// 不认识的串必须返回 false，否则 Tail 会把它当命令吞掉。
	for _, command := range []string{"", "state", "custom", "Action", "実績"} {
		if _, ok := newDispatcher(host).command(command); ok {
			t.Fatalf("%q should not be a command", command)
		}
	}
}

// TestAchievementChecks 覆盖能由数值表判定的成就。
func TestAchievementChecks(t *testing.T) {
	host := newFakeHost()
	host.db = loadDB(t)

	// ID64 只看 @ID_HASOBJS 的「等身大フィギュア」；测试表没有这一行。
	Translate(host, "CheckAndUnlockAchievement( 'ID64' )")
	if len(host.achievements) != 0 {
		t.Fatalf("ID64 unlocked without the figure: %v", host.achievements)
	}

	host.db.Set("ID_HASOBJS", host.db.RowCount("ID_HASOBJS"), "共通", 1)
	// ID62 需要 30 种，两行不够。
	Translate(host, "CheckAndUnlockAchievement( 'ID62' )")
	if len(host.achievements) != 0 {
		t.Fatalf("ID62 unlocked too early: %v", host.achievements)
	}
}

// TestTranslateIgnoresUnknownCalls 保证未知 TRANSLATE 不会误伤状态。
func TestTranslateIgnoresUnknownCalls(t *testing.T) {
	host := newFakeHost()
	host.db = loadDB(t)
	Translate(host, "NotAFunction( 'x' )")
	Translate(host, "broken(")
	if len(host.states) != 0 || len(host.achievements) != 0 {
		t.Fatalf("unexpected side effects: %v %v", host.states, host.achievements)
	}
}
