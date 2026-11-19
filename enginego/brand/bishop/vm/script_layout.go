package vm

type ScriptLayoutKind int

const (
	LayoutModern ScriptLayoutKind = iota
	LayoutClassic
	LayoutAncient
)

const ScriptLayoutTableGroupSize = 0x10

type ScriptLayout struct {
	kind                       ScriptLayoutKind
	codeOffsetField            int
	systemParameterTableOffset int
	systemParameterCountField  int
	messageCountField          int
	choiceSlotCountField       int
	groups                     [6]int
}

var (
	// BSXScript 3.x 的系统参数表在头部内联存放：0x28 是条目数，0xA8 起
	// 是 (key u16, kind u16, index i32) 数组，正好到 codeOffset（0x100/0x110）。
	// 旧实现把它标成 -1，导致 ResolveSlot 永远失败；OHIE 的
	// [status2 screen] register=10..13 和侵蝕/淫獄的系统键都依赖它。
	LayoutModernValue  = ScriptLayout{kind: LayoutModern, codeOffsetField: 0x2C, systemParameterTableOffset: 0xA8, systemParameterCountField: 0x28, messageCountField: -1, choiceSlotCountField: -1, groups: [6]int{0x38, 0x48, 0x58, 0x68, 0x88, 0x98}}
	LayoutClassicValue = ScriptLayout{kind: LayoutClassic, codeOffsetField: 0x28, systemParameterTableOffset: 0x94, systemParameterCountField: 0x24, messageCountField: 0x30, choiceSlotCountField: 0x18, groups: [6]int{0x34, 0x44, 0x54, 0x64, 0x74, 0x84}}
	LayoutAncientValue = ScriptLayout{kind: LayoutAncient, codeOffsetField: 0x24, systemParameterTableOffset: 0x90, systemParameterCountField: 0x20, messageCountField: 0x2C, choiceSlotCountField: -1, groups: [6]int{0x30, 0x40, 0x50, 0x60, 0x70, 0x80}}
)

func AllScriptLayouts() []ScriptLayout {
	return []ScriptLayout{LayoutModernValue, LayoutClassicValue, LayoutAncientValue}
}

func (l ScriptLayout) Kind() ScriptLayoutKind {
	return l.kind
}

func (l ScriptLayout) CodeOffsetField() int {
	return l.codeOffsetField
}

func (l ScriptLayout) SystemParameterTableOffset() int {
	return l.systemParameterTableOffset
}

func (l ScriptLayout) SystemParameterCountField() int {
	return l.systemParameterCountField
}

func (l ScriptLayout) MessageCountField() int {
	return l.messageCountField
}

func (l ScriptLayout) ChoiceSlotCountField() int {
	return l.choiceSlotCountField
}

func (l ScriptLayout) CodeSizeField() int {
	return l.codeOffsetField + 4
}

func (l ScriptLayout) RequiredHeaderSize() int {
	return l.CodeSizeField() + 4
}

func (l ScriptLayout) LabelGroup() int {
	return l.groups[0]
}

func (l ScriptLayout) NameGroup() int {
	return l.groups[1]
}

func (l ScriptLayout) VariableGroup() int {
	return l.groups[2]
}

func (l ScriptLayout) PropertyGroup() int {
	return l.groups[3]
}

func (l ScriptLayout) CharacterGroup() int {
	return l.groups[4]
}

func (l ScriptLayout) MessageGroup() int {
	return l.groups[5]
}

func (l ScriptLayout) GroupStart(index int) int {
	if index >= 0 && index < len(l.groups) {
		return l.groups[index]
	}
	return -1
}

func LayoutForKind(kind ScriptLayoutKind) ScriptLayout {
	switch kind {
	case LayoutClassic:
		return LayoutClassicValue
	case LayoutAncient:
		return LayoutAncientValue
	default:
		return LayoutModernValue
	}
}
