package lua

import (
	"strconv"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"

	"kizuna/engine/brand/bishop/data"
	"kizuna/engine/brand/bishop/data/scriptdb"
)

var slgCharacters = []string{"早紀", "ありさ", "美香子"}

// register 注册所有宿主全局函数，并修正 Lua 5.1 与游戏脚本的差异。
// setGlobals 把一组函数注册成全局名；gopher-lua 的 SetFuncs 只接受表。
func setGlobals(L *lua.LState, funcs map[string]lua.LGFunction) {
	for name, fn := range funcs {
		L.SetGlobal(name, L.NewFunction(fn))
	}
}

func (r *Runtime) register() {
	L := r.L
	r.registerDIB()
	r.registerBit32()

	setGlobals(L, map[string]lua.LGFunction{
		"LoadExScript":             r.loadExScript,
		"OpenCSVDB":                r.openCSVDB,
		"GetDB":                    r.getDB,
		"SetDB":                    r.setDB,
		"AddDB":                    r.addDB,
		"GetDBItemName":            r.getDBItemName,
		"GetScenarioValue":         r.getScenarioValue,
		"SetScenarioValue":         r.setScenarioValue,
		"ScenarioLabelCall":        r.scenarioLabelCall,
		"GetAchievementID":         r.getAchievementID,
		"GetAchievementInfo":       r.getAchievementInfo,
		"CheckAchievement":         r.checkAchievement,
		"UnlockAchievement":        r.unlockAchievement,
		"GetMuseumAchievementRate": r.getMuseumAchievementRate,
		"GetCFGKey":                r.getCFGKey,
		"GetCFGString":             r.getCFGString,
		"SetSysButtonShortcut":     func(*lua.LState) int { return 0 },
		"PushKey":                  r.pushKey,
		"IsHitDecisionKey":         r.isHitDecisionKey,
		"OutputData":               r.outputData,
		"InputData":                r.inputData,
	})

	L.SetGlobal("print", L.NewFunction(func(L *lua.LState) int {
		parts := make([]string, 0, L.GetTop())
		for index := 1; index <= L.GetTop(); index++ {
			parts = append(parts, L.Get(index).String())
		}
		r.host.Notice("lua: " + strings.Join(parts, " "))
		return 0
	}))

	// day_result 用 os.clock 做演出计时；Lua 5.1 的 os.clock 是 CPU 时间，
	// 在手机上几乎不变，动画会瞬间跳完。改成运行时创建以来的墙钟秒数。
	if osTable, ok := L.GetGlobal("os").(*lua.LTable); ok {
		L.SetField(osTable, "clock", L.NewFunction(func(L *lua.LState) int {
			L.Push(lua.LNumber(time.Since(r.clock).Seconds()))
			return 1
		}))
	}
}

// registerBit32 提供 Lua 5.2 的 bit32 库；gopher-lua 是 5.1，没有它。
func (r *Runtime) registerBit32() {
	L := r.L
	table := L.NewTable()
	L.SetField(table, "band", L.NewFunction(func(L *lua.LState) int {
		result := ^uint32(0)
		for index := 1; index <= L.GetTop(); index++ {
			result &= uint32(int64(L.CheckNumber(index)))
		}
		L.Push(lua.LNumber(result))
		return 1
	}))
	L.SetField(table, "bor", L.NewFunction(func(L *lua.LState) int {
		var result uint32
		for index := 1; index <= L.GetTop(); index++ {
			result |= uint32(int64(L.CheckNumber(index)))
		}
		L.Push(lua.LNumber(result))
		return 1
	}))
	L.SetField(table, "bxor", L.NewFunction(func(L *lua.LState) int {
		var result uint32
		for index := 1; index <= L.GetTop(); index++ {
			result ^= uint32(int64(L.CheckNumber(index)))
		}
		L.Push(lua.LNumber(result))
		return 1
	}))
	L.SetField(table, "bnot", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LNumber(^uint32(int64(L.CheckNumber(1)))))
		return 1
	}))
	L.SetField(table, "lshift", L.NewFunction(func(L *lua.LState) int {
		value := uint32(int64(L.CheckNumber(1)))
		shift := L.CheckInt(2) & 31
		L.Push(lua.LNumber(value << shift))
		return 1
	}))
	L.SetField(table, "rshift", L.NewFunction(func(L *lua.LState) int {
		value := uint32(int64(L.CheckNumber(1)))
		shift := L.CheckInt(2) & 31
		L.Push(lua.LNumber(value >> shift))
		return 1
	}))
	L.SetField(table, "btest", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LBool(uint32(int64(L.CheckNumber(1)))&uint32(int64(L.CheckNumber(2))) != 0))
		return 1
	}))
	L.SetGlobal("bit32", table)
}

// ------------------------------------------------------------------ 脚本加载

func (r *Runtime) loadExScript(L *lua.LState) int {
	r.LoadScript(L.CheckString(1))
	return 0
}

// LoadScript 加载一个 ex script；重复调用只执行一次。
//
// boot.lua 用它加载 ScrollBar/训练/领地/日结果；引擎还会按
// [custom info windows] 额外加载 cs1/cs2/cs3/mdt。
func (r *Runtime) LoadScript(name string) bool {
	if r == nil || r.L == nil || name == "" {
		return false
	}
	if r.loaded[name] {
		return true
	}
	r.loaded[name] = true
	for _, candidate := range []string{name + ".lua", name, strings.ToLower(name) + ".lua"} {
		source, ok := r.host.Script(candidate)
		if !ok {
			continue
		}
		if err := r.L.DoString(string(source)); err != nil {
			r.host.Notice("lua: ex script '" + candidate + "' failed: " + err.Error())
		}
		return true
	}
	r.host.Notice("lua: ex script '" + name + "' not found")
	return false
}

// ------------------------------------------------------------------ CSV/DB

func (r *Runtime) openCSVDB(L *lua.LState) int {
	name := L.CheckString(1)
	rows := r.host.CSV(name)
	result := L.NewTable()
	if len(rows) == 0 {
		L.Push(result)
		return 1
	}
	if numericRow(rows[0]) {
		// ter_grid_map 没有表头，是纯二维数组；保持 1 基下标。
		for rowIndex, row := range rows {
			sub := L.NewTable()
			for column, cell := range row {
				sub.RawSetInt(column+1, csvValue(L, cell))
			}
			result.RawSetInt(rowIndex+1, sub)
		}
		L.Push(result)
		return 1
	}
	header := rows[0]
	for rowIndex, row := range rows[1:] {
		entry := L.NewTable()
		for column := 0; column < len(header); column++ {
			key := strings.TrimSpace(header[column])
			if key == "" || key == "}" {
				continue
			}
			if strings.HasSuffix(key, "{") {
				// ter_objs 的 param={,1..8,} 折叠成一个数组字段。
				key = strings.TrimSuffix(key, "{")
				array := L.NewTable()
				position := 1
				for next := column + 1; next < len(header) && strings.TrimSpace(header[next]) != "}"; next++ {
					if next < len(row) {
						array.RawSetInt(position, csvValue(L, row[next]))
					}
					position++
				}
				entry.RawSetString(key, array)
				for column+1 < len(header) && strings.TrimSpace(header[column+1]) != "}" {
					column++
				}
				column++
				continue
			}
			if column < len(row) {
				entry.RawSetString(key, csvValue(L, row[column]))
			}
		}
		result.RawSetInt(rowIndex+1, entry)
	}
	L.Push(result)
	return 1
}

func numericRow(row []string) bool {
	if len(row) == 0 {
		return false
	}
	for _, cell := range row {
		if _, err := strconv.ParseFloat(strings.TrimSpace(cell), 64); err != nil {
			return false
		}
	}
	return true
}

func csvValue(L *lua.LState, text string) lua.LValue {
	text = strings.TrimSpace(text)
	if text == "" {
		return lua.LNil
	}
	if strings.EqualFold(text, "TRUE") {
		return lua.LTrue
	}
	if strings.EqualFold(text, "FALSE") {
		return lua.LFalse
	}
	if number, err := strconv.ParseFloat(text, 64); err == nil {
		return lua.LNumber(number)
	}
	return lua.LString(text)
}

func (r *Runtime) getDB(L *lua.LState) int {
	db := r.host.DB()
	if db == nil {
		L.Push(lua.LNumber(0))
		return 1
	}
	name := L.CheckString(1)
	row, column, ok := dbLocate(L, db, name, 2, 3)
	if !ok {
		L.Push(lua.LNumber(0))
		return 1
	}
	L.Push(lua.LNumber(db.Get(name, row, column)))
	return 1
}

func (r *Runtime) setDB(L *lua.LState) int {
	db := r.host.DB()
	if db == nil {
		return 0
	}
	name := L.CheckString(1)
	row, column, ok := dbLocate(L, db, name, 2, 3)
	if !ok {
		return 0
	}
	db.Set(name, row, column, int32(L.CheckNumber(4)))
	return 0
}

func (r *Runtime) addDB(L *lua.LState) int {
	db := r.host.DB()
	if db == nil {
		return 0
	}
	name := L.CheckString(1)
	row, column, ok := dbLocate(L, db, name, 2, 3)
	if !ok {
		return 0
	}
	delta := int32(L.CheckNumber(4))
	minValue := int32(L.OptNumber(5, -2147483648))
	maxValue := int32(L.OptNumber(6, 2147483647))
	db.Add(name, row, column, delta, minValue, maxValue)
	return 0
}

// dbLocate 把 Lua 的 (角色, 参数) 转成 scriptdb 的 (行, 列)。
//
// CSV 是"参数为行、角色为列"，Lua 是 GetDB("#ID_BP", 角色, 参数)；
// 第二个参数是数字时按角色下标/training 行号解释。
func dbLocate(L *lua.LState, db *scriptdb.DB, name string, whoIndex, whatIndex int) (int, string, bool) {
	columns := db.ColumnNames(name)
	if len(columns) == 0 {
		return 0, "", false
	}
	column := columns[0]
	switch value := L.Get(whoIndex).(type) {
	case lua.LString:
		candidate := strings.TrimSpace(string(value))
		for _, columnName := range columns {
			if columnName == candidate {
				column = candidate
				break
			}
		}
	case lua.LNumber:
		index := int(value)
		if index >= 0 && index < len(slgCharacters) {
			for _, columnName := range columns {
				if columnName == slgCharacters[index] {
					column = columnName
					break
				}
			}
		}
	}
	row := -1
	switch value := L.Get(whatIndex).(type) {
	case lua.LNumber:
		row = int(value)
	case lua.LString:
		row = db.RowIndex(name, string(value))
	}
	if row < 0 {
		row = db.RowCount(name)
	}
	return row, column, true
}

func (r *Runtime) getDBItemName(L *lua.LState) int {
	db := r.host.DB()
	if db == nil {
		L.Push(lua.LNil)
		return 1
	}
	name := L.CheckString(1)
	index := L.CheckInt(3)
	rowName := db.RowName(name, index)
	if rowName == "" {
		L.Push(lua.LNil)
		return 1
	}
	L.Push(lua.LString(rowName))
	return 1
}

// ------------------------------------------------------------------ 剧本变量

func (r *Runtime) getScenarioValue(L *lua.LState) int {
	name := L.CheckString(1)
	L.Push(lua.LNumber(r.host.Property(name)))
	return 1
}

func (r *Runtime) setScenarioValue(L *lua.LState) int {
	name := L.CheckString(1)
	r.host.SetProperty(name, int32(L.CheckNumber(2)))
	return 0
}

func (r *Runtime) scenarioLabelCall(L *lua.LState) int {
	label := L.CheckString(1)
	r.pendingLabel = label
	return 0
}

// ------------------------------------------------------------------ 成就/鉴赏

func (r *Runtime) getAchievementID(L *lua.LState) int {
	table := r.host.Achievements()
	if table == nil {
		L.Push(lua.LNil)
		return 1
	}
	index := L.CheckInt(1)
	entries := table.Entries()
	if index < 0 || index >= len(entries) {
		L.Push(lua.LNil)
		return 1
	}
	L.Push(lua.LString(entries[index].ID))
	return 1
}

func (r *Runtime) getAchievementInfo(L *lua.LState) int {
	table := r.host.Achievements()
	id := L.CheckString(1)
	entry := data.Achievement{ID: id}
	if table != nil {
		if value, ok := table.Get(id); ok {
			entry = value
		}
	}
	result := L.NewTable()
	L.SetField(result, "id", lua.LString(entry.ID))
	L.SetField(result, "name", lua.LString(entry.Name))
	L.SetField(result, "notes", lua.LString(entry.Notes))
	L.SetField(result, "secret", lua.LBool(entry.Secret))
	L.SetField(result, "unlock", lua.LBool(r.host.AchievementUnlocked(id)))
	L.Push(result)
	return 1
}

func (r *Runtime) checkAchievement(L *lua.LState) int {
	L.Push(lua.LBool(r.host.AchievementUnlocked(L.CheckString(1))))
	return 1
}

func (r *Runtime) unlockAchievement(L *lua.LState) int {
	r.host.UnlockAchievement(L.CheckString(1))
	return 0
}

func (r *Runtime) getMuseumAchievementRate(L *lua.LState) int {
	L.Push(lua.LNumber(r.host.GalleryRate(L.CheckString(1))))
	return 1
}

// ------------------------------------------------------------------ 配置/输入

func (r *Runtime) getCFGKey(L *lua.LState) int {
	L.Push(lua.LString(L.CheckString(1)))
	return 1
}

func (r *Runtime) getCFGString(L *lua.LState) int {
	section := L.CheckString(1)
	key := L.CheckString(2)
	L.Push(lua.LString(r.host.Config(section, key)))
	return 1
}

func (r *Runtime) pushKey(L *lua.LState) int {
	name := strings.ToUpper(L.CheckString(1))
	switch name {
	case "A":
		r.pendingKey = keyA
	case "B":
		r.pendingKey = keyB
	case "WU":
		r.pendingKey = keyWU
	case "WD":
		r.pendingKey = keyWD
	}
	return 0
}

func (r *Runtime) isHitDecisionKey(L *lua.LState) int {
	L.Push(lua.LBool(r.decision))
	return 1
}

// ------------------------------------------------------------------ 数据拷贝

func (r *Runtime) outputData(L *lua.LState) int {
	out := L.CheckTable(1)
	source := L.CheckTable(2)
	copyTable(out, source)
	return 0
}

func (r *Runtime) inputData(L *lua.LState) int {
	source := L.CheckTable(1)
	destination := L.CheckTable(2)
	copyTable(destination, source)
	return 0
}

func copyTable(destination, source *lua.LTable) {
	if destination == nil || source == nil {
		return
	}
	source.ForEach(func(key lua.LValue, value lua.LValue) {
		destination.RawSet(key, deepCopy(value))
	})
}

func deepCopy(value lua.LValue) lua.LValue {
	table, ok := value.(*lua.LTable)
	if !ok {
		return value
	}
	copy := lua.LTable{}
	table.ForEach(func(key lua.LValue, child lua.LValue) {
		copy.RawSet(key, deepCopy(child))
	})
	return &copy
}

func tableToGo(table *lua.LTable) any {
	if table == nil {
		return nil
	}
	count := 0
	isArray := true
	table.ForEach(func(key lua.LValue, _ lua.LValue) {
		count++
		number, ok := key.(lua.LNumber)
		if !ok || int(number) != count {
			isArray = false
		}
	})
	if count == 0 {
		return map[string]any{}
	}
	if isArray {
		array := make([]any, 0, count)
		for index := 1; index <= count; index++ {
			array = append(array, valueToGo(table.RawGetInt(index)))
		}
		return array
	}
	result := make(map[string]any, count)
	table.ForEach(func(key lua.LValue, value lua.LValue) {
		result[key.String()] = valueToGo(value)
	})
	return result
}

func valueToGo(value lua.LValue) any {
	switch typed := value.(type) {
	case *lua.LTable:
		return tableToGo(typed)
	case lua.LNumber:
		return float64(typed)
	case lua.LString:
		return string(typed)
	case lua.LBool:
		return bool(typed)
	default:
		return nil
	}
}

func goToTable(value any) *lua.LTable {
	table := &lua.LTable{}
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			table.RawSetString(key, goToValue(child))
		}
	case []any:
		for index, child := range typed {
			table.RawSetInt(index+1, goToValue(child))
		}
	}
	return table
}

func goToValue(value any) lua.LValue {
	switch typed := value.(type) {
	case nil:
		return lua.LNil
	case map[string]any, []any:
		return goToTable(typed)
	case float64:
		return lua.LNumber(typed)
	case float32:
		return lua.LNumber(typed)
	case int:
		return lua.LNumber(typed)
	case int32:
		return lua.LNumber(typed)
	case int64:
		return lua.LNumber(typed)
	case string:
		return lua.LString(typed)
	case bool:
		return lua.LBool(typed)
	default:
		return lua.LNil
	}
}

func mergeMap(destination map[string]any, source any) {
	if destination == nil {
		return
	}
	switch typed := source.(type) {
	case map[string]any:
		for key, value := range typed {
			destination[key] = value
		}
	}
}

// callName 从 `Name(args)` 里取函数名。
func callName(statement string) string {
	statement = strings.TrimSpace(statement)
	open := strings.IndexByte(statement, '(')
	if open <= 0 || !strings.HasSuffix(statement, ")") {
		return ""
	}
	return strings.TrimSpace(statement[:open])
}
