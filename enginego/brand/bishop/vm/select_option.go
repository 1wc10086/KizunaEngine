package vm

type SelectOption struct {
	Index     int
	MessageId int
	LabelId   int
	Value     int
	Kind      int
}

// SetSelectOptions 恢复存档里保存的选择肢。Save/Load 会把 FETCH 出来的
// 选项一起写进扩展段：读档时 PC 已经过了 SELECT，若不恢复选项，
// 玩家会看到一句对白后直接跳过选择（満淫電車2 的自动存档正好在选项前）。
func (m *BsxMachine) SetSelectOptions(options []SelectOption) {
	m.selectOptions = append(m.selectOptions[:0], options...)
	m.options = m.options[:0]
}
