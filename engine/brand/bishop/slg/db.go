package slg

import "kizuna/engine/brand/bishop/data/scriptdb"

// dbView 把剧本的下标语义翻译成 scriptdb 的行/列寻址。
//
// 两套坐标是转置的：剧本 `$#ID_BP[0,'BT値']` 的第一个下标是角色
// （早紀/ありさ/美香子），第二个是参数名；而 CSV 里角色是列、参数是行
// （var_base_param.csv 的列头就是三位角色）。scriptdb 按 CSV 原样暴露
// Get(行号,列名)，所以这里统一转置一次，让上层代码和原 Lua 的
// GetDB(db, 角色, 参数) 写法一一对应。
//
// 第二个下标写成数字时（`$#ID_TC[ch-1, 37]`、InitBeginTimeCounters 的
// 0..100 扫描），数字就是 CSV 的数据行号：训练/日志表的行顺序在作品里
// 与行为编号一致（已用 var_training.csv 与行为表的 index 对照验证）。
//
// 一维写法 `$@ID_HASOBJS['重り']` 把行名直接放在第一个下标，列固定取
// 第一列（这些共享表的表头通常只有"共通"一列）。
type dbView struct{ db *scriptdb.DB }

// columnFor 取角色下标对应的列名。表里没有该角色列时退回第一列：
// 有的表只有一列（@アジト補正 / @ID_HASOBJS 的"共通"），按第一列读
// 才是原意，而不是当成缺失。
func (v dbView) columnFor(table string, index int32) string {
	columns := v.db.ColumnNames(table)
	if len(columns) == 0 {
		return ""
	}
	if index >= 0 && int(index) < len(characters) {
		name := characters[index]
		for _, column := range columns {
			if column == name {
				return column
			}
		}
	}
	return columns[0]
}

// rowFor 取参数名对应的数据行号；数字参数按行号直接用。
func (v dbView) rowFor(table, column string) int {
	if value, ok := parseInteger(column); ok {
		return int(value)
	}
	return v.db.RowIndex(table, column)
}

// locate 把 dbRef 落成 (行号, 列名)。行名不存在时返回 -1：
// 读按 0 处理，写则追加到表尾（由调用方决定）。
func (v dbView) locate(ref dbRef) (int, string) {
	if ref.oneDimensional() {
		column := v.columnFor(ref.table, -1)
		if column == "" {
			return -1, ""
		}
		return v.db.RowIndex(ref.table, ref.rowName), column
	}
	return v.rowFor(ref.table, ref.column), v.columnFor(ref.table, ref.index)
}

func (v dbView) get(ref dbRef) int32 {
	row, column := v.locate(ref)
	if row < 0 || column == "" {
		return 0
	}
	return v.db.Get(ref.table, row, column)
}

func (v dbView) set(ref dbRef, value int32) {
	row, column := v.locate(ref)
	if column == "" {
		return
	}
	if row < 0 {
		row = v.db.RowCount(ref.table)
	}
	v.db.Set(ref.table, row, column, value)
}

// add 加减并夹在 [min,max] 内，返回写入后的值。
func (v dbView) add(ref dbRef, delta, min, max int32) int32 {
	row, column := v.locate(ref)
	if column == "" {
		return 0
	}
	if row < 0 {
		row = v.db.RowCount(ref.table)
	}
	return v.db.Add(ref.table, row, column, delta, min, max)
}

func (v dbView) rows(table string) int { return v.db.RowCount(table) }
