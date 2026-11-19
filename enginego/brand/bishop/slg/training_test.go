package slg

import "testing"

// trainingHost 建一份"刚排完一天训练"的宿主：早紀 是训练对象，
// 第一个窗口是"愛撫"，play_status.next_time = 2 表示它已排好待结算。
//
// 数值与作品 CSV 的列顺序保持一致（角色是列、参数是行），
// 这样 dbView 的转置一旦写错，下面的期望值会立刻对不上。
func trainingHost(t *testing.T) *fakeHost {
	t.Helper()
	host := newFakeHost()
	host.db = loadDB(t)
	entry := map[string]any{
		"行為名":   "愛撫",
		"index": float64(0),
		"系統":    "快楽",
		"従順度":   float64(10),
		"調教受容度": float64(5),
		"口感度":   float64(3),
		"胸感度":   float64(4),
		"性器感度":  float64(5),
		"尻感度":   float64(6),
		"肌感度":   float64(7),
		"BT値":   float64(20),
		"気力":    float64(-10),
		"ログ加算":  "膣挿入回数,膣出し回数",
		"label": "L01_001",
	}
	SetRecipe(host.private, []map[string]any{entry})
	SetPlayStatus(host.private, 1, 2)
	return host
}

// TestResultTraining 钉住结算数值。
//
// 期望值按 training.lua:896-962 逐条推：
//   - 従順度/調教受容度 不乘倍率；
//   - 五个感受度乘倍率（BT 90 + 20 会推满，且アジト補正没禁用 BT）；
//   - BT値 夹在 100，気力 夹在 [-30,100]；
//   - #ID_TC 按行为计数，#ID_LOG 按系统 + 追加日志计数。
func TestResultTraining(t *testing.T) {
	cases := []struct {
		name       string
		prepare    func(host *fakeHost)
		want       int32
		wantParams map[string]int32
	}{
		{
			name: "double-rate-when-bt-fills",
			want: 1,
			wantParams: map[string]int32{
				"従順度":   40, // 30 + 10，不翻倍
				"調教受容度": 15, // 10 + 5
				"口感度":   6,  // 0 + 3*2
				"胸感度":   16, // 8 + 4*2
				"性器感度":  10, // 0 + 5*2
				"尻感度":   12, // 0 + 6*2
				"肌感度":   19, // 5 + 7*2
				"BT値":   100,
				"気力":    90,
			},
		},
		{
			name: "single-rate-when-bt-disabled",
			prepare: func(host *fakeHost) {
				// アジト補正 bit0 = 禁用 BT：即使 BT 会推满也不翻倍。
				// ResultTraining 读的是 @ID_TERCOR（var_db 的映射名），
				// 必须写同一张表；scriptdb 会把同一 CSV 再装一份以表头
				// 命名的表（アジト補正），写到那上面读不到。
				state := host.db.RowIndex("ID_TERCOR", "状態")
				host.db.Set("ID_TERCOR", state, host.db.ColumnNames("ID_TERCOR")[0], 1)
			},
			want: 1,
			wantParams: map[string]int32{
				"口感度": 3,
				"胸感度": 12,
				"BT値": 100,
			},
		},
		{
			name: "slave-keeps-obedience",
			prepare: func(host *fakeHost) {
				host.db.Set("ID_BP", 0, "早紀", 100) // 調教進行度
			},
			want: 1,
			wantParams: map[string]int32{
				"従順度":   30, // 奴隶化后不加
				"調教受容度": 10, // 同上
				"口感度":   6,  // 感受度照常
			},
		},
		{
			name: "no-recipe-yet",
			prepare: func(host *fakeHost) {
				SetPlayStatus(host.private, 1, 1)
			},
			want: 0,
			wantParams: map[string]int32{
				"従順度": 30,
			},
		},
		{
			name: "replay-does-not-apply",
			prepare: func(host *fakeHost) {
				host.properties["#scene_replay"] = 1
			},
			want: 0,
			wantParams: map[string]int32{
				"従順度": 30,
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			host := trainingHost(t)
			if testCase.prepare != nil {
				testCase.prepare(host)
			}
			Translate(host, "ResultTraining()")
			// TRANSLATE 无返回值；结果标志写在 #result（1 = 已结算）。
			if got := host.properties["#result"]; got != testCase.want {
				t.Fatalf("ResultTraining() = %d, want %d", got, testCase.want)
			}
			for column, want := range testCase.wantParams {
				if got := host.db.GetNamed("ID_BP", column, "早紀"); got != want {
					t.Fatalf("%s = %d, want %d", column, got, want)
				}
			}
		})
	}
}

// TestResultTrainingLogs 单独验证计数表被写到"行为"和"系统"两处。
func TestResultTrainingLogs(t *testing.T) {
	host := trainingHost(t)
	Translate(host, "ResultTraining()")

	if got := host.db.GetNamed("ID_TC", "愛撫", "早紀"); got != 4 {
		t.Fatalf("愛撫 count = %d, want 4", got)
	}
	if got := host.db.GetNamed("ID_LOG", "総調教回数", "早紀"); got != 6 {
		t.Fatalf("総調教回数 = %d, want 6", got)
	}
	if got := host.db.GetNamed("ID_LOG", "快楽系調教回数", "早紀"); got != 1 {
		t.Fatalf("快楽系調教回数 = %d, want 1", got)
	}
	if got := host.db.GetNamed("ID_LOG", "膣挿入回数", "早紀"); got != 1 {
		t.Fatalf("膣挿入回数 = %d, want 1", got)
	}
	if got := host.db.GetNamed("ID_LOG", "膣出し回数", "早紀"); got != 1 {
		t.Fatalf("膣出し回数 = %d, want 1", got)
	}
}

// TestNextTraining 钉住"逐个窗口跳标签"的游标推进。
func TestNextTraining(t *testing.T) {
	host := trainingHost(t)
	// 第一个窗口已结算；补一个空窗口和一个第二行为，验证跳空能力。
	entry := map[string]any{"label": "L02_001", "index": float64(1)}
	SetRecipe(host.private, []map[string]any{
		map[string]any{"label": "L01_001"},
		nil,
		entry,
	})
	SetPlayStatus(host.private, 1, 2)
	host.labels["L02_001"] = true

	Translate(host, "NextTraining('#result')")
	if host.properties["#result"] != 0 {
		t.Fatalf("#result = %d, want 0 (label found)", host.properties["#result"])
	}
	if got := number(host.private["play_status"].(map[string]any)["next_time"]); got != 4 {
		t.Fatalf("next_time = %v, want 4", got)
	}

	// 没有下一个窗口了：返回 1 并把游标清 0。
	Translate(host, "NextTraining('#result')")
	if host.properties["#result"] != 1 {
		t.Fatalf("#result = %d, want 1 (no recipe left)", host.properties["#result"])
	}
	if got := number(host.private["play_status"].(map[string]any)["next_time"]); got != 0 {
		t.Fatalf("next_time = %v, want 0", got)
	}
}

// TestBeginTimeSnapshot 验证快照与取消。
func TestBeginTimeSnapshot(t *testing.T) {
	host := trainingHost(t)
	// 快照必须发生在训练之前：CancelDayResult 恢复的就是这一天开始时的值。
	Translate(host, "InitBeginTimeParameters()")

	snapshot, ok := host.private["begin-time parameters"].(map[string]any)
	if !ok {
		t.Fatal("begin-time parameters missing")
	}
	first := snapshot["1"].(map[string]any)
	if got := number(first["従順度"]); got != 30 {
		t.Fatalf("snapshot 従順度 = %v, want 30", got)
	}
	if got := number(first["BT値"]); got != 0 {
		// BT値 不在快照参数表里；出现它说明参数列表被改错了。
		t.Fatalf("snapshot should not contain BT値, got %v", got)
	}

	// 结算后取消，参数应回到快照值（従順度 回到 30）。
	Translate(host, "ResultTraining()")
	if got := host.db.GetNamed("ID_BP", "従順度", "早紀"); got != 40 {
		t.Fatalf("after training 従順度 = %d, want 40", got)
	}
	Translate(host, "CancelDayResult()")
	if got := host.db.GetNamed("ID_BP", "従順度", "早紀"); got != 30 {
		t.Fatalf("after cancel 従順度 = %d, want 30", got)
	}
}

// TestAddCorrectionValue 验证アジト補正只加成两项，且奴隶化后不生效。
func TestAddCorrectionValue(t *testing.T) {
	host := trainingHost(t)
	Translate(host, "AddCorrectionValue( '早紀' )")
	if got := host.db.GetNamed("ID_BP", "従順度", "早紀"); got != 35 {
		t.Fatalf("従順度 = %d, want 35", got)
	}
	if got := host.db.GetNamed("ID_BP", "調教受容度", "早紀"); got != 17 {
		t.Fatalf("調教受容度 = %d, want 7", got)
	}

	host.db.Set("ID_BP", 0, "早紀", 100)
	Translate(host, "AddCorrectionValue( '早紀' )")
	if got := host.db.GetNamed("ID_BP", "従順度", "早紀"); got != 35 {
		t.Fatalf("slave 従順度 = %d, want 35 (unchanged)", got)
	}
}

// TestIsAnimetingReturnsZeroWhileWaiting 是"动画没播完却被放行"的回归测试。
func TestIsAnimetingReturnsZeroWhileWaiting(t *testing.T) {
	host := newFakeHost()
	host.db = loadDB(t)
	host.animations["#result"] = 0
	if got := Tail(host, "is animeted #result , 1"); got != 0 {
		t.Fatalf("waiting is animeted = %d, want 0", got)
	}
	host.animations["#result"] = 1
	if got := Tail(host, "is animeted #result , 1"); got != 1 {
		t.Fatalf("finished is animeted = %d, want 1", got)
	}
}

// TestStoreRestore 保证训练状态能整体 JSON 往返。
func TestStoreRestore(t *testing.T) {
	host := trainingHost(t)
	data := Store(host.private, host.common)
	if len(data) == 0 {
		t.Fatal("Store returned no data")
	}
	private := map[string]any{}
	common := map[string]any{}
	Restore(data, private, common)
	status, ok := private["play_status"].(map[string]any)
	if !ok {
		t.Fatal("play_status lost")
	}
	if got := number(status["next_time"]); got != 2 {
		t.Fatalf("restored next_time = %v, want 2", got)
	}
	if recipe, ok := private["play_recipe"].([]any); !ok || len(recipe) != recipeSlots {
		t.Fatalf("restored recipe = %v", private["play_recipe"])
	}
}
