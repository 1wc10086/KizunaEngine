// Package scriptdb implements the Bishop "variable database" tables used by the
// newer SLG titles (BSXScript 3.3 + embedded Lua).
//
// 原引擎把作品数值放在 define.bsa 的 CSV 里：var_db 描述"逻辑表名 -> CSV 文件"，
// 每个 CSV 的第一行是列名、第一列是行名，其余单元格是整数。剧本与 Lua 通过
// `#ID_BP[0,'従順度']` / `@ID_TERCOR['状態']` 这类写法读写它们，`#` 是
// PrivateData（每个存档一份），`@` 是 CommonData（跨周目共享）。
//
// 这里刻意保持"按名字查询"而不是编号：CSV 在作品之间列顺序不同，剧本却用
// 列名寻址；一旦按编号缓存，换作品或打汉化补丁补列就会读错列。
package scriptdb

import (
	"strconv"
	"strings"
)

// Reader 读取游戏资源（BSA 条目或磁盘文件）。返回 false 表示资源不存在。
type Reader func(name string) ([]byte, bool)

// DB 是一组按作品配置加载的表。
type DB struct {
	tables map[string]*table
	// raw 缓存原始 CSV（OpenCSVDB 需要保留字符串列，例如 ter_objs 的
	// `param={,1,2,...}` 不是数字表）。
	raw map[string][][]string
	// mapped 记录 CSV 文件名 -> 表名，便于同一文件被多个名字引用。
	mapped map[string]string
}

type table struct {
	columns []string
	rows    []string
	index   map[string]int
	values  [][]int32
	// dynamic 表没有 CSV 骨架（例如 @アジト補正 由 Lua SetDB 即时创建），
	// 行/列都按调用时出现的名字懒创建。
	dynamic bool
}

// Load 解析 var_db 并加载它引用的所有 CSV。
//
// var_db.db 不是严格 JSON：作品文件里 missing comma 是常见现象（特別授業3SLG
// 的 `"ID_STC": "..." "ID_LOG": ...` 就少了逗号），用 JSON 解码会整份失败，
// 因此只扫描 `"key": "value"` 对。读取失败的表不会阻止其它表：剧本里缺失的
// 数值按 0 处理，比整局游戏起不来好。
func Load(read Reader) *DB {
	db := &DB{
		tables: make(map[string]*table),
		raw:    make(map[string][][]string),
		mapped: make(map[string]string),
	}
	if read == nil {
		return db
	}
	if data, ok := read("var_db"); ok {
		for _, pair := range scanPairs(string(data)) {
			db.loadMapped(pair.value, pair.key, read)
		}
	}
	// var_territory_correction.csv 的第一行第一格就是它自己的逻辑名
	// （アジト補正），var_db 没有登记它；territory.lua 直接按这个名字读写。
	if data, ok := read("var_territory_correction"); ok {
		rows := ParseCSV(data)
		if len(rows) > 0 && len(rows[0]) > 0 {
			name := strings.TrimSpace(rows[0][0])
			if name != "" {
				db.install(name, rows)
				db.raw["var_territory_correction"] = rows
			}
		}
	}
	return db
}

type pair struct{ key, value string }

// scanPairs 从 var_db 的宽松文本里取出所有 `"键": "值"`。
func scanPairs(text string) []pair {
	var out []pair
	for index := 0; index < len(text); index++ {
		if text[index] != '"' {
			continue
		}
		key, next, ok := quoted(text, index)
		if !ok {
			break
		}
		index = next - 1
		colon := strings.IndexByte(text[next:], ':')
		if colon < 0 {
			break
		}
		valueStart := next + colon + 1
		for valueStart < len(text) && (text[valueStart] == ' ' || text[valueStart] == '\t' || text[valueStart] == '\r' || text[valueStart] == '\n') {
			valueStart++
		}
		if valueStart >= len(text) || text[valueStart] != '"' {
			continue
		}
		value, after, ok := quoted(text, valueStart)
		if !ok {
			break
		}
		out = append(out, pair{key: key, value: value})
		index = after - 1
	}
	return out
}

// quoted 读取从 quote 位置开始的 JSON 字符串，返回内容和结束后的下标。
func quoted(text string, quote int) (string, int, bool) {
	var builder strings.Builder
	for index := quote + 1; index < len(text); index++ {
		switch text[index] {
		case '\\':
			if index+1 < len(text) {
				index++
				builder.WriteByte(text[index])
			}
		case '"':
			return builder.String(), index + 1, true
		default:
			builder.WriteByte(text[index])
		}
	}
	return "", len(text), false
}

func (d *DB) loadMapped(resource, name string, read Reader) {
	if resource == "" || name == "" {
		return
	}
	if rows, ok := d.raw[resource]; ok {
		d.install(name, rows)
		return
	}
	data, ok := read(resource)
	if !ok {
		return
	}
	rows := ParseCSV(data)
	d.raw[resource] = rows
	d.mapped[resource] = name
	d.install(name, rows)
}

func (d *DB) install(name string, rows [][]string) {
	if len(rows) < 2 || len(rows[0]) < 2 {
		return
	}
	if existing, ok := d.tables[name]; ok && !existing.dynamic {
		return
	}
	columns := make([]string, 0, len(rows[0])-1)
	for _, cell := range rows[0][1:] {
		columns = append(columns, normalize(cell))
	}
	table := &table{
		columns: columns,
		index:   make(map[string]int),
	}
	for rowIndex, row := range rows[1:] {
		if len(row) == 0 {
			continue
		}
		rowName := normalize(row[0])
		if rowName == "" {
			continue
		}
		values := make([]int32, len(columns))
		for column := 0; column < len(columns) && column+1 < len(row); column++ {
			values[column] = parseInt(row[column+1])
		}
		table.index[rowName] = len(table.rows)
		table.rows = append(table.rows, rowName)
		table.values = append(table.values, values)
		_ = rowIndex
	}
	d.tables[name] = table
}

// ------------------------------------------------------------------ 查询

// Get 按行号、列名读取。行号是 CSV 的数据行（不含表头）。
func (d *DB) Get(db string, row int, column string) int32 {
	table := d.table(db)
	if table == nil {
		return 0
	}
	return table.get(row, normalize(column))
}

// GetNamed 按行名、列名读取（Lua 的 GetDB("#ID_BP", "早紀", "従順度")）。
func (d *DB) GetNamed(db, row, column string) int32 {
	table := d.table(db)
	if table == nil {
		return 0
	}
	index, ok := table.index[normalize(row)]
	if !ok {
		return 0
	}
	return table.get(index, normalize(column))
}

// Set 写入一个值；未知表/列/行按需创建，越界行也按需扩展。
func (d *DB) Set(db string, row int, column string, value int32) {
	table := d.ensureTable(db)
	if table == nil {
		return
	}
	table.set(row, normalize(column), value)
}

// Add 把 delta 加到单元格并夹在 [min,max] 内，返回写入后的值。
func (d *DB) Add(db string, row int, column string, delta, minValue, maxValue int32) int32 {
	table := d.ensureTable(db)
	if table == nil {
		return 0
	}
	current := table.get(row, normalize(column)) + delta
	if current < minValue {
		current = minValue
	}
	if current > maxValue {
		current = maxValue
	}
	table.set(row, normalize(column), current)
	return current
}

// RowCount 返回数据行数。
func (d *DB) RowCount(db string) int {
	table := d.table(db)
	if table == nil {
		return 0
	}
	return len(table.rows)
}

// RowName 返回数据行的原始名字（Lua 表格键）。
func (d *DB) RowName(db string, row int) string {
	table := d.table(db)
	if table == nil || row < 0 || row >= len(table.rows) {
		return ""
	}
	return table.rows[row]
}

// ColumnNames 返回列名（已按原样去除引号/空白）。
func (d *DB) ColumnNames(db string) []string {
	table := d.table(db)
	if table == nil {
		return nil
	}
	out := make([]string, len(table.columns))
	copy(out, table.columns)
	return out
}

// RowIndex 按行名查行号，不存在返回 -1。
func (d *DB) RowIndex(db, row string) int {
	table := d.table(db)
	if table == nil {
		return -1
	}
	if index, ok := table.index[normalize(row)]; ok {
		return index
	}
	return -1
}

// CSV 返回原始 CSV（包含表头），供 OpenCSVDB 使用。
func (d *DB) CSV(resource string) [][]string {
	if rows, ok := d.raw[resource]; ok {
		return rows
	}
	return nil
}

func (d *DB) table(name string) *table {
	return d.tables[normalizeName(name)]
}

func (d *DB) ensureTable(name string) *table {
	key := normalizeName(name)
	if table, ok := d.tables[key]; ok {
		return table
	}
	table := &table{dynamic: true, index: make(map[string]int)}
	d.tables[key] = table
	return table
}

func (t *table) get(row int, column string) int32 {
	if t == nil || row < 0 {
		return 0
	}
	if row >= len(t.values) {
		return 0
	}
	columnIndex := t.column(column)
	if columnIndex < 0 || columnIndex >= len(t.values[row]) {
		return 0
	}
	return t.values[row][columnIndex]
}

func (t *table) set(row int, column string, value int32) {
	if t == nil || row < 0 {
		return
	}
	for row >= len(t.values) {
		t.rows = append(t.rows, strconv.Itoa(len(t.rows)))
		t.values = append(t.values, make([]int32, len(t.columns)))
	}
	columnIndex := t.column(column)
	if columnIndex < 0 {
		t.columns = append(t.columns, column)
		columnIndex = len(t.columns) - 1
		for index := range t.values {
			t.values[index] = append(t.values[index], 0)
		}
	}
	if columnIndex >= len(t.values[row]) {
		gap := columnIndex - len(t.values[row]) + 1
		for index := range t.values {
			t.values[index] = append(t.values[index], make([]int32, gap)...)
		}
	}
	t.values[row][columnIndex] = value
}

func (t *table) column(name string) int {
	for index, column := range t.columns {
		if column == name {
			return index
		}
	}
	return -1
}

// ------------------------------------------------------------------ 持久化

// Snapshot 是可直接 JSON 序列化的存档视图：db -> 行名 -> 列名 -> 值。
//
// 存"名字"而不是下标，是为了让 CSVs 增删行/列后旧存档仍能对齐；原引擎的
// 存档本来就是按逻辑表写出的，这里保持同一种稳定性。
func (d *DB) Snapshot() map[string]map[string]map[string]int32 {
	out := make(map[string]map[string]map[string]int32, len(d.tables))
	for name, table := range d.tables {
		rows := make(map[string]map[string]int32, len(table.values))
		for rowIndex, values := range table.values {
			rowName := table.rowName(rowIndex)
			cells := make(map[string]int32, len(values))
			for columnIndex, value := range values {
				if columnIndex >= len(table.columns) {
					continue
				}
				cells[table.columns[columnIndex]] = value
			}
			rows[rowName] = cells
		}
		out[name] = rows
	}
	return out
}

// Restore 把 Snapshot 写回当前表；缺失的列/行会按需创建。
func (d *DB) Restore(snapshot map[string]map[string]map[string]int32) {
	for name, rows := range snapshot {
		table := d.ensureTable(name)
		for rowName, cells := range rows {
			rowIndex, ok := table.index[rowName]
			if !ok {
				rowIndex = len(table.rows)
				table.rows = append(table.rows, rowName)
				table.index[rowName] = rowIndex
				table.values = append(table.values, make([]int32, len(table.columns)))
			}
			for column, value := range cells {
				table.set(rowIndex, normalize(column), value)
			}
		}
	}
}

func (t *table) rowName(index int) string {
	if index >= 0 && index < len(t.rows) {
		return t.rows[index]
	}
	return strconv.Itoa(index)
}

// ------------------------------------------------------------------ 工具

func normalizeName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "#")
	name = strings.TrimPrefix(name, "@")
	return name
}

func normalize(text string) string {
	return strings.TrimSpace(text)
}

func parseInt(text string) int32 {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	if value, err := strconv.ParseInt(text, 0, 32); err == nil {
		return int32(value)
	}
	if value, err := strconv.ParseFloat(text, 64); err == nil {
		return int32(value)
	}
	return 0
}

// ParseCSV 解析作品使用的 RFC4180 变体：双引号包裹、"" 转义、
// 允许 LF/CRLF、允许空单元格。原文件里有跨列的空值（`,,`），
// 所以不能跳过空行。
func ParseCSV(data []byte) [][]string {
	text := strings.TrimPrefix(string(data), "\ufeff")
	var rows [][]string
	var row []string
	var cell strings.Builder
	quotedCell := false
	flushCell := func() {
		row = append(row, cell.String())
		cell.Reset()
	}
	flushRow := func() {
		flushCell()
		rows = append(rows, row)
		row = nil
	}
	for index := 0; index < len(text); index++ {
		character := text[index]
		switch {
		case quotedCell:
			if character == '"' {
				if index+1 < len(text) && text[index+1] == '"' {
					cell.WriteByte('"')
					index++
					continue
				}
				quotedCell = false
				continue
			}
			cell.WriteByte(character)
		case character == '"' && cell.Len() == 0:
			quotedCell = true
		case character == ',':
			flushCell()
		case character == '\n':
			flushRow()
		case character == '\r':
			// CRLF 的 CR 直接丢弃；孤立 CR 按换行处理。
			if index+1 >= len(text) || text[index+1] != '\n' {
				flushRow()
			}
		default:
			cell.WriteByte(character)
		}
	}
	if cell.Len() > 0 || len(row) > 0 {
		flushRow()
	}
	return rows
}

// TableCount 返回已加载的表数量（调试日志/状态画面页数判断用）。
func (d *DB) TableCount() int {
	if d == nil {
		return 0
	}
	return len(d.tables)
}
