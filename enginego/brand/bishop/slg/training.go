package slg

import (
	"encoding/json"
	"math/rand"
	"strings"
)

// 训练界面（training.lua）的数据模型。
//
// 原实现把两份状态放在 PrivateData（每存档一份、序列化成 JSON）里：
//
//	play_status: {target_no, next_time, play_count, prev_class, prev_play}
//	play_recipe: [false | {行为字段...}]，按"当天训练窗口"下标对齐
//
// 引擎不跑 Lua，所以这里用同样的键名把两份状态存成 map[string]any，
// 由 Store/Restore 走 JSON。字段名保持原样（含中文）是为了让存档能和
// Lua 数据逐项对照排查。所有读取都做类型断言 + 缺省值，旧存档缺字段时
// 按 0/false 处理，不会 panic。

const (
	statusKey   = "play_status"
	recipeKey   = "play_recipe"
	paramsKey   = "begin-time parameters"
	countersKey = "begin-time counters"

	// 表名保留剧本里的前缀（scriptdb 会自己去掉），让调用点和 Lua 一一对应。
	parameterTable  = "#ID_BP"
	counterTable    = "#ID_TC"
	stealthTable    = "#ID_STC"
	logTable        = "#ID_LOG"
	correctionTable = "@アジト補正"
	objectsTable    = "@ID_HASOBJS"
	stateTable      = "@ID_TERCOR"

	// 训练窗口宽度：剧本 #training_time 最大 5，NextTraining 也按 5 格循环。
	recipeSlots = 5
	// counterCandidates 是 InitBeginTimeCounters 的扫描上界（Lua 写死 0..100）。
	counterCandidates = 100
	// objectLimit 是每种アジト物品的持有上限（Lua 用 < 9 判定"还没满"）。
	objectLimit = 9
)

var (
	// 三位角色在 #ID_BP / #ID_TC / #ID_LOG 里按固定顺序占行。
	// 不能排序（汉字序会变），写死才能和 Lua 的 chno = target_no-1 对齐。
	characters = []string{"早紀", "ありさ", "美香子"}
	// beginTimeParams 是 InitBeginTimeParameters 快照的 7 个感受/关系参数；
	// 原 Lua 不含 BT値/気力/状態/残時間。
	beginTimeParams = []string{"口感度", "胸感度", "性器感度", "尻感度", "肌感度", "従順度", "調教受容度"}
	// sensitivityParams 是受"倍率 2"影响的那五个感受度。
	sensitivityParams = []string{"口感度", "胸感度", "性器感度", "尻感度", "肌感度"}
	// logClasses 是"行为系统"到日志列的映射，ResultTraining 每执行一次加 1。
	logClasses = []struct{ class, column string }{
		{"快楽", "快楽系調教回数"},
		{"変態", "変態系調教回数"},
		{"退廃", "退廃系調教回数"},
		{"嗜虐", "嗜虐系調教回数"},
		{"奉仕", "奉仕系調教回数"},
	}
)

// parameterLimit 是训练参数写入的夹取范围，照抄 training.lua:920-934。
// 训练一次不能把参数推出上下限，否则下一轮菜单的可用行为会算错。
type parameterLimit struct{ min, max int32 }

var trainingLimits = map[string]parameterLimit{
	"従順度":   {-50, 255},
	"調教受容度": {-255, 255},
	"口感度":   {0, 999},
	"胸感度":   {0, 999},
	"性器感度":  {0, 999},
	"尻感度":   {0, 999},
	"肌感度":   {0, 999},
	"BT値":   {0, 100},
	"気力":    {-30, 100},
}

// ------------------------------------------------------------------ 持久化

// private 是 Lua PrivateData 的替代品（每周目一份，序列化进槽位存档）。
// 做成 Host 方法而不是字段，是为了让存档层决定挂到哪个槽位。
func (d *dispatcher) private() map[string]any {
	if d.host == nil {
		return nil
	}
	return d.host.PrivateData()
}

// playStatus 读 play_status；缺失时建一份空表并挂回父表，
// 这样后面的"补字段"写回才生效。
func (d *dispatcher) playStatus() map[string]any {
	data := d.private()
	if data == nil {
		return map[string]any{}
	}
	status, _ := data[statusKey].(map[string]any)
	if status == nil {
		status = map[string]any{}
		data[statusKey] = status
	}
	return status
}

// playRecipe 读 play_recipe。原 Lua 用数组 + false 占位，
// 这里保持"下标 i 对应窗口 i+1"的语义；缺格子按 false 处理。
func (d *dispatcher) playRecipe() []any {
	data := d.private()
	if data == nil {
		return nil
	}
	recipe, _ := data[recipeKey].([]any)
	return recipe
}

// number 读 JSON 里的数值。JSON 解码后整数是 float64，
// 但测试或旧存档也可能给 int/int32，统一归一。
func number(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int32:
		return float64(typed)
	case int64:
		return float64(typed)
	}
	return 0
}

func intNumber(value any) int32 { return int32(number(value)) }

// Store 序列化两张 Lua 数据表。失败返回 nil，调用方跳过该段即可。
func Store(private, common map[string]any) []byte {
	payload := struct {
		Private map[string]any `json:"private"`
		Common  map[string]any `json:"common"`
	}{Private: private, Common: common}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return data
}

// Restore 把 Store 的结果写回两张表。缺段保留原值：
// 存档里没有 common 段时不应该把跨周目进度清零。
func Restore(data []byte, private, common map[string]any) {
	if len(data) == 0 {
		return
	}
	var payload struct {
		Private map[string]any `json:"private"`
		Common  map[string]any `json:"common"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return
	}
	merge(private, payload.Private)
	merge(common, payload.Common)
}

func merge(destination, source map[string]any) {
	if destination == nil {
		return
	}
	for key, value := range source {
		destination[key] = value
	}
}

// ------------------------------------------------------------------ 配方

// SetRecipe 安装当天排好的训练计划（Lua 的 PrivateData["play_recipe"]）。
//
// 每个元素是一行的键值表：行為名/系統/各参数增量/ログ加算/label/index。
// 传 nil 表示该窗口空着；不足 5 格时补 false——ResultTraining 与
// NextTraining 都只按 5 格读，长度不齐会让它们读到越界下标。
func SetRecipe(private map[string]any, entries []map[string]any) {
	if private == nil {
		return
	}
	recipe := make([]any, 0, recipeSlots)
	for index := 0; index < recipeSlots; index++ {
		if index < len(entries) && entries[index] != nil {
			recipe = append(recipe, entries[index])
			continue
		}
		recipe = append(recipe, false)
	}
	private[recipeKey] = recipe
}

// SetPlayStatus 写入当前训练状态：
//
//	targetNo  1..3 的训练对象（Lua 的 target_no）
//
// nextTime  下一个要执行的窗口下标（从 1 起；开始前等于 begin_time）
func SetPlayStatus(private map[string]any, targetNo, nextTime int32) {
	if private == nil {
		return
	}
	status, _ := private[statusKey].(map[string]any)
	if status == nil {
		status = map[string]any{}
		private[statusKey] = status
	}
	status["target_no"] = float64(targetNo)
	status["next_time"] = float64(nextTime)
}

// ------------------------------------------------------------------ ResultTraining

// resultTraining 把"上一次排好的训练"结算进 DB。
//
// 关键不变量：
//   - 只在 play_status.next_time >= 2 时结算：next_time 是"下一次要执行的
//     窗口"，< 2 表示还没排过任何行为。
//   - 倍率 2 的条件是"本次训练会把 BT 推满 100 且アジト補正没有禁用 BT"，
//     与 Lua 一致；漏掉 0x1 判定会让禁用 BT 的周目多拿一倍感受度。
//   - 従順度/調教受容度只在調教進行度 < 100 时加，且不乘倍率：
//     奴隶化后不再变化。
func (d *dispatcher) resultTraining() int32 {
	if d.host == nil {
		return 0
	}
	// 回想模式不能推进数值，否则看一遍回想就把本周目的训练结算掉了。
	if d.host.Property("#scene_replay") > 0 {
		return 0
	}
	status := d.playStatus()
	nextTime := intNumber(status["next_time"])
	if nextTime < 2 {
		return 0
	}
	recipe := d.playRecipe()
	position := int(nextTime) - 2
	if position < 0 || position >= len(recipe) {
		return 0
	}
	entry, ok := recipe[position].(map[string]any)
	if !ok {
		return 0
	}
	target := intNumber(status["target_no"]) - 1
	if target < 0 || target >= int32(len(characters)) {
		return 0
	}

	rate := int32(1)
	if d.get(parameterTable, target, "BT値")+intNumber(entry["BT値"]) >= 100 &&
		d.get(stateTable, 0, "状態")&0x1 == 0 {
		rate = 2
	}

	if d.get(parameterTable, target, "調教進行度") < 100 {
		d.addLimit(target, "従順度", intNumber(entry["従順度"]))
		d.addLimit(target, "調教受容度", intNumber(entry["調教受容度"]))
	}
	for _, name := range sensitivityParams {
		d.addLimit(target, name, intNumber(entry[name])*rate)
	}
	d.addLimit(target, "BT値", intNumber(entry["BT値"]))
	d.addLimit(target, "気力", intNumber(entry["気力"]))

	// 実行回数：训练计数表按行为（列名或编号）累计，日志表按系统分类累计。
	d.count(counterTable, target, behaviorColumn(entry))
	d.count(logTable, target, "総調教回数")
	d.addClassLog(target, stringOf(entry["系統"]))
	for _, column := range splitList(stringOf(entry["ログ加算"])) {
		d.count(logTable, target, column)
	}
	return 1
}

// addClassLog 按系统分类累计日志；未知系统落到"その他"，与 Lua 的 if 链一致。
func (d *dispatcher) addClassLog(target int32, class string) {
	for _, known := range logClasses {
		if known.class == class {
			d.count(logTable, target, known.column)
			return
		}
	}
	d.count(logTable, target, "その他調教回数")
}

// behaviorColumn 取行为的训练计数键。
//
// #ID_TC 的列名就是行为名，行是角色；但成就判定（check_achievement.lua）
// 又用 `GetDB("#ID_TC", ch, 37)` 这种数字列读同一份计数。行为表自带 index，
// 用它作键可以让两种写法落到同一格：数字列不存在时 dbView.rowFor 会按
// 行号读，而行顺序在 var_training.csv 里就是 index 顺序（已对照验证）。
func behaviorColumn(entry map[string]any) string {
	index, ok := entry["index"]
	if !ok {
		if name := stringOf(entry["行為名"]); name != "" {
			return name
		}
		return "0"
	}
	return FormatInt(intNumber(index))
}

// ------------------------------------------------------------------ NextTraining

// nextTraining 跳到下一个还没执行的训练行为，并把结果写回目标变量。
//
// 0 表示"找到标签并已请求跳转"，1 表示"没有可跳的窗口"。
// Lua 在第 5 个窗口之后就把 next_time 清 0，所以循环上界固定 5。
func (d *dispatcher) nextTraining(target string) {
	status := d.playStatus()
	nextTime := intNumber(status["next_time"])
	resultValue := int32(1)
	if nextTime > 0 {
		recipe := d.playRecipe()
		for index := nextTime; index <= recipeSlots; index++ {
			position := int(index) - 1
			if position < 0 || position >= len(recipe) {
				continue
			}
			entry, ok := recipe[position].(map[string]any)
			if !ok {
				continue
			}
			label := stringOf(entry["label"])
			// 标签不存在时不推进 next_time：剧本可能在同一窗口重试，
			// 推进了就永远跳过这个行为。
			if label == "" || !d.host.CallLabel(label) {
				continue
			}
			status["next_time"] = float64(index + 1)
			resultValue = 0
			break
		}
	}
	if target != "" {
		d.host.SetProperty(target, resultValue)
	}
	if resultValue != 0 {
		status["next_time"] = float64(0)
	}
}

// ------------------------------------------------------------------ 快照

// initBeginTimeParameters 快照当天开始时的参数。
//
// 快照写进 PrivateData["begin-time parameters"]，按"角色下标 1..3"索引，
// 内部按参数名排。键名带空格与连字符是原存档的写法，不能改。
func (d *dispatcher) initBeginTimeParameters() {
	data := d.private()
	if data == nil {
		return
	}
	snapshot, _ := data[paramsKey].(map[string]any)
	if snapshot == nil {
		snapshot = map[string]any{}
		data[paramsKey] = snapshot
	}
	for index := range characters {
		key := FormatInt(int32(index + 1))
		row, ok := snapshot[key].(map[string]any)
		if !ok {
			row = map[string]any{}
			snapshot[key] = row
		}
		for _, column := range beginTimeParams {
			row[column] = float64(d.get(parameterTable, int32(index), column))
		}
	}
}

// restoreBeginTimeParameters 把快照写回 DB（CancelDayResult 用）。
// 快照缺失时什么都不做：没有"当天起点"就没有可恢复的目标。
func (d *dispatcher) restoreBeginTimeParameters() {
	data := d.private()
	if data == nil {
		return
	}
	snapshot, _ := data[paramsKey].(map[string]any)
	if snapshot == nil {
		return
	}
	for index := range characters {
		row, ok := snapshot[FormatInt(int32(index+1))].(map[string]any)
		if !ok {
			continue
		}
		for _, column := range beginTimeParams {
			ref := dbRef{table: parameterTable, index: int32(index), column: column}
			view := d.evaluate.view()
			if view == nil {
				return
			}
			view.set(ref, intNumber(row[column]))
		}
	}
}

// initBeginTimeCounters 记录当天"尚未解锁"的训练行为编号。
//
// 原 Lua 扫 #ID_TC 的 0..100 号列，值为 -1 的才算（-1 = 尚未解锁）。
// 训练计数表按行为编号建列，数字列通常不存在，读出来是 0（已解锁），
// 因此不会把已有行为误判为不可用。
func (d *dispatcher) initBeginTimeCounters() {
	data := d.private()
	if data == nil {
		return
	}
	counters, _ := data[countersKey].(map[string]any)
	if counters == nil {
		counters = map[string]any{}
		data[countersKey] = counters
	}
	for index := range characters {
		columns := []any{}
		for column := int32(0); column <= counterCandidates; column++ {
			if d.get(counterTable, int32(index), FormatInt(column)) == -1 {
				columns = append(columns, float64(column))
			}
		}
		counters[FormatInt(int32(index+1))] = columns
	}
}

// updateBeginTimeCounters 按快照重新过滤：值不再为负的行为被移除。
// CancelDayResult 用它把"当天已经点过"的行为从候选里剔掉。
func (d *dispatcher) updateBeginTimeCounters() {
	data := d.private()
	if data == nil {
		return
	}
	counters, _ := data[countersKey].(map[string]any)
	if counters == nil {
		return
	}
	for index := range characters {
		key := FormatInt(int32(index + 1))
		previous, _ := counters[key].([]any)
		kept := make([]any, 0, len(previous))
		for _, entry := range previous {
			column := intNumber(entry)
			if d.get(counterTable, int32(index), FormatInt(column)) < 0 {
				kept = append(kept, float64(column))
			}
		}
		counters[key] = kept
	}
}

// addCorrectionValue 把 @アジト補正 的加成加到角色参数上。
//
// 只在調教進行度 < 100 时生效：奴隶化之后アジト補正不再影响角色。
// 名字必须是三位角色之一（剧本传 '早紀' 这类字面量），否则什么也不做。
func (d *dispatcher) addCorrectionValue(name string) {
	row := int32(-1)
	for index, character := range characters {
		if character == name {
			row = int32(index)
			break
		}
	}
	if row < 0 || d.get(parameterTable, row, "調教進行度") >= 100 {
		return
	}
	d.addLimit(row, "従順度", d.get(correctionTable, 0, "従順度"))
	d.addLimit(row, "調教受容度", d.get(correctionTable, 0, "調教受容度"))
}

// ------------------------------------------------------------------ 成就与物件

// checkAndUnlockAchievement 检查并解锁成就。
//
// 只实现能由数值表判定的成就（check_achievement.lua 的 ID50/51/62/63/64）；
// 其余 ID 的条件依赖剧本变量（#combiH、#saki_ev3、鉴赏率达 100 等），
// 由剧本自己维护。没有判定函数时不解锁：宁可少解锁，也不误解锁。
func (d *dispatcher) checkAndUnlockAchievement(ids []string) {
	if d.host == nil {
		return
	}
	var unlocked []string
	for _, id := range ids {
		if d.achievementSatisfied(id) {
			unlocked = append(unlocked, id)
		}
	}
	if len(unlocked) > 0 {
		d.host.UnlockAchievement(unlocked...)
	}
}

func (d *dispatcher) achievementSatisfied(id string) bool {
	switch strings.TrimSpace(id) {
	case "ID50":
		// 調教マスター：所有训练各做过至少一次（37/38 两个 3P 需特殊条件，
		// 原 Lua 直接跳过）。剧本变量那部分（固定训练位）不在这里判。
		for index := int32(0); index <= 48; index++ {
			if index == 37 || index == 38 {
				continue
			}
			if !d.anyCharacterUsed(index) {
				return false
			}
		}
		for index := int32(0); index <= 4; index++ {
			if !d.anyStealthUsed(index) {
				return false
			}
		}
		return true
	case "ID51":
		// 万能奴隷：任一角色做全 47 种训练。
		for character := int32(0); character < int32(len(characters)); character++ {
			count := int32(0)
			for index := int32(0); index <= 48; index++ {
				if index == 37 || index == 38 {
					continue
				}
				if d.get(counterTable, character, FormatInt(index)) > 0 {
					count++
				}
			}
			if count == 47 {
				return true
			}
		}
		return false
	case "ID62":
		// 泥棒教師：アジト物品收集到 30 种。
		return d.collectedObjects() >= 30
	case "ID63":
		// 怪盗教師：アジト物品全部收集。
		rows := d.evaluate.view().rows(objectsTable)
		return rows > 0 && d.collectedObjects() == int32(rows)
	case "ID64":
		// 沙耶の人形：持有「等身大フィギュア」。
		return d.get(objectsTable, 0, "等身大フィギュア") >= 1
	}
	// ID69 依赖鉴赏室达成率，由宿主在 SetGalleryRate 里维护；
	// 剧本可以在达成率到 100 后自行调用解锁。
	return false
}

func (d *dispatcher) anyCharacterUsed(index int32) bool {
	for character := int32(0); character < int32(len(characters)); character++ {
		if d.get(counterTable, character, FormatInt(index)) > 0 {
			return true
		}
	}
	return false
}

func (d *dispatcher) anyStealthUsed(index int32) bool {
	for character := int32(0); character < int32(len(characters)); character++ {
		if d.get(stealthTable, character, FormatInt(index)) > 0 {
			return true
		}
	}
	return false
}

// collectedObjects 统计持有数 > 0 的物品行数。
func (d *dispatcher) collectedObjects() int32 {
	view := d.evaluate.view()
	if view == nil {
		return 0
	}
	count := int32(0)
	for row := 0; row < view.rows(objectsTable); row++ {
		if view.db.Get(objectsTable, row, objectColumn(view)) > 0 {
			count++
		}
	}
	return count
}

// objectColumn 取物件数量表的列名。var_hasobjs.csv 的表头是"共通"，
// 补丁可能改名，所以按"第一列"取，取不到再退回"共通"。
func objectColumn(view *dbView) string {
	if columns := view.db.ColumnNames(objectsTable); len(columns) > 0 {
		return columns[0]
	}
	return "共通"
}

// getObjectTest 抽一个"还没收集满"的アジト物品编号。
//
// 优先交给宿主：完整的物件表在 define.bsa 里（ter_objs），只有宿主能按
// 大小分档随机（原 Lua 的 1/12/32/64 阈值），-1 是它约定的"一个都没有"。
// 宿主返回非负值时以宿主为准；返回 -1 时再退回数值表：随机挑一行数量
// < 9 的物品返回行号，同样一个都没有时 -1（对应 Lua 的 #items == 0）。
func (d *dispatcher) getObjectTest(varName string) int32 {
	if value := d.host.GetObjectTest(varName); value != -1 {
		return value
	}
	view := d.evaluate.view()
	if view == nil {
		return -1
	}
	column := objectColumn(view)
	candidates := make([]int32, 0, view.rows(objectsTable))
	for row := 0; row < view.rows(objectsTable); row++ {
		if view.db.Get(objectsTable, row, column) < objectLimit {
			candidates = append(candidates, int32(row))
		}
	}
	if len(candidates) == 0 {
		return -1
	}
	return candidates[rand.Intn(len(candidates))]
}

// ------------------------------------------------------------------ 工具

// get 读一格；没有 DB 时返回 0（普通 AVG 也会走到训练命令）。
func (d *dispatcher) get(table string, index int32, column string) int32 {
	view := d.evaluate.view()
	if view == nil {
		return 0
	}
	return view.get(dbRef{table: table, index: index, column: column})
}

// addLimit 按 Lua 的夹取范围加值。
func (d *dispatcher) addLimit(index int32, column string, delta int32) int32 {
	view := d.evaluate.view()
	if view == nil {
		return 0
	}
	limit, ok := trainingLimits[column]
	if !ok {
		limit = parameterLimit{min: -32768, max: 32767}
	}
	return view.add(dbRef{table: parameterTable, index: index, column: column}, delta, limit.min, limit.max)
}

// count 是"计次加 1"的简写，上界足够大（计次不封顶）。
func (d *dispatcher) count(table string, index int32, column string) int32 {
	view := d.evaluate.view()
	if view == nil {
		return 0
	}
	return view.add(dbRef{table: table, index: index, column: column}, 1, 0, 1<<30)
}

// splitList 拆逗号分隔的列表；`なし`（无追加效果）返回 nil。
func splitList(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" || text == "なし" {
		return nil
	}
	parts := strings.Split(text, ",")
	out := parts[:0]
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func stringOf(value any) string {
	text, _ := value.(string)
	return text
}
