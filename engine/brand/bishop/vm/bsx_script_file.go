package vm

import (
	"errors"
	"os"
	"strings"

	"kizuna/engine/brand/bishop/vm/bs"
	"kizuna/engine/core"
	"kizuna/engine/core/binutil"
)

const MinimumScriptHeaderSize = 0x94

const scriptSystemParameterSize = 8

const (
	ancientPropertyKind = 1
	ancientVariableKind = 2
	revVariableKind     = 0x0102
	revPropertyKind     = 0x0103
)

type BsSystemParameter struct {
	Key   int
	Kind  int
	Index int
}

type scriptTable struct {
	listOffset  int
	count       int
	blockOffset int
	blockSize   int
	encoding    ScriptTextEncoding
}

func emptyScriptTable() scriptTable {
	return scriptTable{encoding: Utf16}
}

type BsxScriptFile struct {
	data                    []byte
	layout                  ScriptLayout
	labels                  scriptTable
	names                   scriptTable
	variables               scriptTable
	properties              scriptTable
	characters              scriptTable
	messages                scriptTable
	systemParameters        []BsSystemParameter
	dialect                 ScriptDialect
	bsDialect               bs.BsScriptDialect
	hasBsDialect            bool
	signature               string
	codeOffset              int
	codeSize                int
	bsStackCapacity         int
	bsRegisterCapacity      int
	bsVariableCapacity      int
	bsPropertyCapacity      int
	bsGlobalCapacity        int
	bsFlagCapacity          int
	bsVariableArrayCapacity int
	bsPropertyArrayCapacity int
	bsMessageCapacity       int
	bsChoiceSlotCount       int
}

func LoadBsxScriptFile(path string) (*BsxScriptFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseBsxScriptFile(data)
}

func ParseBsxScriptFile(data []byte) (*BsxScriptFile, error) {
	if len(data) < MinimumScriptHeaderSize {
		return nil, errors.New("script file is too small")
	}
	signature, ok := DetectSignature(data)
	if !ok {
		binutil.NegateBytes(data)
		signature, ok = DetectSignature(data)
		if !ok {
			return nil, errors.New("missing script signature")
		}
	}
	preferred := LayoutKindForSignature(signature)
	script, err := tryCreateBsx(data, signature, LayoutForKind(preferred))
	if err == nil {
		return script, nil
	}
	for _, candidate := range AllScriptLayouts() {
		if candidate.Kind() == preferred {
			continue
		}
		script, err := tryCreateBsx(data, signature, candidate)
		if err == nil {
			core.Logf("script: signature '%s' fell back to %d layout", signature, int(candidate.Kind()))
			return script, nil
		}
	}
	return nil, errors.New("unsupported script layout for '" + signature + "'")
}

func tryCreateBsx(data []byte, signature string, layout ScriptLayout) (*BsxScriptFile, error) {
	if len(data) < layout.RequiredHeaderSize() || len(data) < MinimumScriptHeaderSize {
		return nil, errors.New("script file is too small")
	}
	codeOffset := readInt32AtBytes(data, layout.CodeOffsetField())
	codeSize := readInt32AtBytes(data, layout.CodeSizeField())
	if codeOffset < MinimumScriptHeaderSize || codeSize <= 0 || int64(codeOffset)+int64(codeSize) > int64(len(data)) {
		return nil, errors.New("script code block is out of range")
	}
	for group := 0; group < 6; group++ {
		start := layout.GroupStart(group)
		if start+ScriptLayoutTableGroupSize > len(data) {
			return nil, errors.New("script table group is out of range")
		}
		listOffset := readInt32AtBytes(data, start)
		listSize := readInt32AtBytes(data, start+4)
		blockOffset := readInt32AtBytes(data, start+8)
		blockSize := readInt32AtBytes(data, start+12)
		if !isTableValid(data, listOffset, listSize, blockOffset, blockSize) {
			return nil, errors.New("script table is out of range")
		}
	}
	script := &BsxScriptFile{data: data, layout: layout, signature: signature}
	script.dialect = dialectForSignature(signature, layout)
	script.codeOffset = int(codeOffset)
	script.codeSize = int(codeSize)
	bsDialect, hasBsDialect := detectBsDialect(data, signature, layout)
	script.bsDialect = bsDialect
	script.hasBsDialect = hasBsDialect
	if layout.Kind() == LayoutAncient {
		script.bsStackCapacity = int(readInt32AtBytes(data, 0x10))
		script.bsRegisterCapacity = int(readInt32AtBytes(data, 0x14))
		script.bsVariableCapacity = int(readInt32AtBytes(data, 0x18))
		script.bsPropertyCapacity = int(readInt32AtBytes(data, 0x1C))
	} else if hasBsDialect && bs.RevIsRev(bsDialect) {
		script.bsGlobalCapacity = int(readInt32AtBytes(data, 0x10))
		script.bsFlagCapacity = int(readInt32AtBytes(data, 0x14))
		script.bsVariableArrayCapacity = int(readInt32AtBytes(data, 0x1C))
		script.bsPropertyArrayCapacity = int(readInt32AtBytes(data, 0x20))
	}
	script.labels = script.readTable(layout.LabelGroup())
	script.names = script.readTable(layout.NameGroup())
	script.variables = script.readTable(layout.VariableGroup())
	script.properties = script.readTable(layout.PropertyGroup())
	script.characters = script.readTable(layout.CharacterGroup())
	script.messages = script.readTable(layout.MessageGroup())
	script.systemParameters = script.readSystemParameters(layout)
	if layout.MessageCountField() >= 0 {
		script.bsMessageCapacity = max(0, int(readInt32AtBytes(data, layout.MessageCountField())))
	}
	if layout.ChoiceSlotCountField() >= 0 {
		script.bsChoiceSlotCount = max(0, int(readInt32AtBytes(data, layout.ChoiceSlotCountField())))
	}
	return script, nil
}

// dialectForSignature 决定指令语义（操作码编码），与"头部布局"是两件事。
//
// BSXScript 2.x 的头部仍是旧的 0x28/0x34 布局（比 3.x 整体前移 4 字节），
// 但指令集已经是现代编码：对全脚本按 modernSize 逐条走，能刚好走到代码末尾、
// 所有标签都落在指令边界上（已用 特別授業3SLG 旧版 tj3slg 的 3.19MB 代码验证）。
// 旧实现只看 LayoutKind，把 2.x 当成 BSScript Rev. 的"旧操作码 +1/+3"翻译，
// 于是 MSG 被当成单字节命令、MOVE 变成 OR，指令流很快错位，点 START 后直接
// 撞到 HALT，宿主就弹出播放结束页。
//
// 因此：凡是 BSXScript 签名一律用现代语义；只有 BSScript Rev.（Classic 布局）
// 和 BSScript（Ancient 布局）才走各自的历史操作码集。
func dialectForSignature(signature string, layout ScriptLayout) ScriptDialect {
	if strings.HasPrefix(signature, ScriptSignatureModern) {
		return DialectModern
	}
	if layout.Kind() == LayoutAncient {
		return DialectAncient
	}
	return DialectLegacy
}

func detectBsDialect(data []byte, signature string, layout ScriptLayout) (bs.BsScriptDialect, bool) {
	hinted, hasHinted := bs.RevForVersion(bs.RevSignatureVersion(signature))
	isBsScript := strings.HasPrefix(signature, ScriptSignatureAncient)
	if !isBsScript {
		return bs.BsScriptAncientWide, false
	}
	codeOffset := readInt32AtBytes(data, layout.CodeOffsetField())
	codeSize := readInt32AtBytes(data, layout.CodeSizeField())
	if codeOffset <= 0 || codeSize <= 0 || int64(codeOffset)+int64(codeSize) > int64(len(data)) {
		if hasHinted {
			core.Logf("script: BSScript %d code block is out of range", int(hinted))
			return hinted, true
		}
		core.Logf("script: BSScript code block is out of range, assuming wide")
		return bs.BsScriptAncientWide, true
	}
	code := data[codeOffset : codeOffset+codeSize]
	if detected, ok := bs.TryDetectBsDialect(code, signature); ok {
		core.Logf("script: BSScript dialect %d (exact scan of %d bytes)", int(detected), len(code))
		return detected, true
	}
	labels := readLabelOffsets(data, layout)
	if !hasHinted {
		wide := bs.AlignScore(code, labels, bs.BsScriptAncientWide)
		compact := bs.AlignScore(code, labels, bs.BsScriptAncientCompact)
		core.Logf("script: BSScript dialect fallback wide=%d compact=%d", wide, compact)
		if wide >= compact {
			return bs.BsScriptAncientWide, true
		}
		return bs.BsScriptAncientCompact, true
	}
	best := bs.BestByAlignment(code, labels, true, hinted)
	core.Logf("script: BSScript dialect fallback %d (hint %d, align %d)", int(best), int(hinted), bs.AlignScore(code, labels, best))
	return best, true
}

func readLabelOffsets(data []byte, layout ScriptLayout) map[int]bool {
	result := map[int]bool{}
	listOffset := readInt32AtBytes(data, layout.LabelGroup())
	listSize := readInt32AtBytes(data, layout.LabelGroup()+4)
	if listOffset <= 0 || listSize <= 0 {
		return result
	}
	for index := 0; index < int(listSize)/8; index++ {
		value := readInt32AtBytes(data, int(listOffset)+(index*8))
		if value > 0 {
			result[int(value)] = true
		}
	}
	return result
}

func isTableValid(data []byte, listOffset, listSize, blockOffset, blockSize int32) bool {
	if listOffset == 0 && listSize == 0 && blockOffset == 0 && blockSize == 0 {
		return true
	}
	if listOffset < MinimumScriptHeaderSize || int(listOffset) >= len(data) {
		return false
	}
	if blockOffset < MinimumScriptHeaderSize || int(blockOffset) >= len(data) {
		return false
	}
	if listSize == 0 || blockSize == 0 {
		return true
	}
	if listSize < 0 || blockSize < 0 {
		return false
	}
	return int64(listOffset)+int64(listSize) <= int64(len(data))
}

func (s *BsxScriptFile) Signature() string {
	return s.signature
}

func (s *BsxScriptFile) LayoutKind() ScriptLayoutKind {
	return s.layout.Kind()
}

// IsLegacy 表示"旧操作码语义"，不是"旧头部布局"。
//
// BSXScript 2.x 用旧头部 + 现代操作码，调用方（RestartBoot、剧情入口兜底）
// 关心的是操作码语义，所以必须看 dialect；否则旧版游戏会被当成 BSScript
// 处理，boot/入口行为也跟着走错分支。
func (s *BsxScriptFile) IsLegacy() bool {
	return !s.dialect.IsModern()
}

func (s *BsxScriptFile) Dialect() ScriptDialect {
	return s.dialect
}

func (s *BsxScriptFile) BsDialect() (bs.BsScriptDialect, bool) {
	return s.bsDialect, s.hasBsDialect
}

func (s *BsxScriptFile) IsBsScript() bool {
	return s.hasBsDialect
}

func (s *BsxScriptFile) CodeOffset() int {
	return s.codeOffset
}

func (s *BsxScriptFile) CodeSize() int {
	return s.codeSize
}

func (s *BsxScriptFile) LabelArrayOffset() int {
	return s.labels.listOffset
}

func (s *BsxScriptFile) LabelNamePoolOffset() int {
	return s.labels.blockOffset
}

func (s *BsxScriptFile) LabelCount() int {
	return s.labels.count
}

func (s *BsxScriptFile) NameListOffset() int {
	return s.names.listOffset
}

func (s *BsxScriptFile) NameBlockOffset() int {
	return s.names.blockOffset
}

func (s *BsxScriptFile) NameCount() int {
	return s.names.count
}

func (s *BsxScriptFile) VariableNameListOffset() int {
	return s.variables.listOffset
}

func (s *BsxScriptFile) VariableNameBlockOffset() int {
	return s.variables.blockOffset
}

func (s *BsxScriptFile) VariableNameCount() int {
	return s.variables.count
}

func (s *BsxScriptFile) PropertyNameListOffset() int {
	return s.properties.listOffset
}

func (s *BsxScriptFile) PropertyNameBlockOffset() int {
	return s.properties.blockOffset
}

func (s *BsxScriptFile) PropertyNameCount() int {
	return s.properties.count
}

func (s *BsxScriptFile) CharacterNameListOffset() int {
	return s.characters.listOffset
}

func (s *BsxScriptFile) CharacterNameBlockOffset() int {
	return s.characters.blockOffset
}

func (s *BsxScriptFile) CharacterNameCount() int {
	return s.characters.count
}

func (s *BsxScriptFile) MessageListOffset() int {
	return s.messages.listOffset
}

func (s *BsxScriptFile) MessageBlockOffset() int {
	return s.messages.blockOffset
}

func (s *BsxScriptFile) MessageCount() int {
	return s.messages.count
}

func (s *BsxScriptFile) LabelEncoding() ScriptTextEncoding {
	return s.labels.encoding
}

func (s *BsxScriptFile) NameEncoding() ScriptTextEncoding {
	return s.names.encoding
}

func (s *BsxScriptFile) VariableNameEncoding() ScriptTextEncoding {
	return s.variables.encoding
}

func (s *BsxScriptFile) PropertyEncoding() ScriptTextEncoding {
	return s.properties.encoding
}

func (s *BsxScriptFile) CharacterEncoding() ScriptTextEncoding {
	return s.characters.encoding
}

func (s *BsxScriptFile) MessageEncoding() ScriptTextEncoding {
	return s.messages.encoding
}

func (s *BsxScriptFile) Code() []byte {
	if s.codeOffset < 0 || s.codeSize <= 0 || s.codeOffset+s.codeSize > len(s.data) {
		return nil
	}
	return s.data[s.codeOffset : s.codeOffset+s.codeSize]
}

func (s *BsxScriptFile) BsStackCapacity() int {
	return s.bsStackCapacity
}

func (s *BsxScriptFile) BsRegisterCapacity() int {
	return s.bsRegisterCapacity
}

func (s *BsxScriptFile) BsVariableCapacity() int {
	return s.bsVariableCapacity
}

func (s *BsxScriptFile) BsPropertyCapacity() int {
	return s.bsPropertyCapacity
}

func (s *BsxScriptFile) BsGlobalCapacity() int {
	return s.bsGlobalCapacity
}

func (s *BsxScriptFile) BsFlagCapacity() int {
	return s.bsFlagCapacity
}

func (s *BsxScriptFile) BsVariableArrayCapacity() int {
	return s.bsVariableArrayCapacity
}

func (s *BsxScriptFile) BsPropertyArrayCapacity() int {
	return s.bsPropertyArrayCapacity
}

func (s *BsxScriptFile) BsMessageCapacity() int {
	return s.bsMessageCapacity
}

func (s *BsxScriptFile) BsChoiceSlotCount() int {
	return s.bsChoiceSlotCount
}

func (s *BsxScriptFile) SystemParameters() []BsSystemParameter {
	return s.systemParameters
}

func (s *BsxScriptFile) GetLabelOffset(index int) int {
	if index < 0 || index >= s.labels.count {
		return -1
	}
	return int(readInt32AtBytes(s.data, s.labels.listOffset+(index*8)))
}

func (s *BsxScriptFile) GetLabelNameOffset(index int) int {
	if index < 0 || index >= s.labels.count {
		return -1
	}
	return int(readInt32AtBytes(s.data, s.labels.listOffset+(index*8)+4))
}

func (s *BsxScriptFile) GetLabelName(index int) string {
	offset := s.GetLabelNameOffset(index)
	if offset < 0 {
		return ""
	}
	return s.readBlockString(s.labels, offset)
}

func (s *BsxScriptFile) FindLabel(name string) int {
	for index := 0; index < s.labels.count; index++ {
		if strings.EqualFold(s.GetLabelName(index), name) {
			return s.GetLabelOffset(index)
		}
	}
	return -1
}

func (s *BsxScriptFile) GetEntryPoint() int {
	entry := 0
	if s.labels.count > 0 {
		entry = s.GetLabelOffset(0)
	}
	if entry > 0 {
		return entry
	}
	return 0
}

func (s *BsxScriptFile) GetName(index int) string {
	return s.readTableString(s.names, index)
}

func (s *BsxScriptFile) GetVariableName(index int) string {
	return s.readTableString(s.variables, index)
}

func (s *BsxScriptFile) GetPropertyName(index int) string {
	return s.readTableString(s.properties, index)
}

func (s *BsxScriptFile) GetCharacterName(index int) string {
	return s.readTableString(s.characters, index)
}

func (s *BsxScriptFile) GetMessage(index int) string {
	return s.readTableString(s.messages, index)
}

func (s *BsxScriptFile) ResolveSlot(key int) (bool, int, bool) {
	propertyKind := revPropertyKind
	variableKind := revVariableKind
	if s.layout.Kind() == LayoutAncient {
		propertyKind = ancientPropertyKind
		variableKind = ancientVariableKind
	}
	for _, parameter := range s.systemParameters {
		if parameter.Key != key {
			continue
		}
		isProperty := parameter.Kind == propertyKind
		if isProperty || parameter.Kind == variableKind {
			return isProperty, parameter.Index, true
		}
		break
	}
	return false, -1, false
}

func (s *BsxScriptFile) GetVariableIndex(name string) int {
	if len(name) == 0 {
		return -1
	}
	target := stripPrefix(name)
	for index := 0; index < s.variables.count; index++ {
		candidate := s.GetVariableName(index)
		if len(candidate) > 0 && strings.EqualFold(stripPrefix(candidate), target) {
			return index
		}
	}
	return -1
}

func (s *BsxScriptFile) GetPropertyIndex(name string) int {
	if len(name) == 0 {
		return -1
	}
	target := stripPrefix(name)
	for index := 0; index < s.properties.count; index++ {
		candidate := s.GetPropertyName(index)
		if len(candidate) > 0 && strings.EqualFold(stripPrefix(candidate), target) {
			return index
		}
	}
	return -1
}

func (s *BsxScriptFile) readTable(groupStart int) scriptTable {
	if groupStart <= 0 || groupStart+ScriptLayoutTableGroupSize > len(s.data) {
		return emptyScriptTable()
	}
	listOffset := readInt32AtBytes(s.data, groupStart)
	listSize := readInt32AtBytes(s.data, groupStart+4)
	blockOffset := readInt32AtBytes(s.data, groupStart+8)
	blockSize := readInt32AtBytes(s.data, groupStart+12)
	if listOffset == 0 || blockOffset == 0 || listSize <= 0 || blockSize <= 0 {
		return emptyScriptTable()
	}
	stride := 4
	if groupStart == s.layout.LabelGroup() {
		stride = 8
	}
	block := s.blockOf(int(blockOffset), int(blockSize))
	// 编码选择跟"指令族"走，而不是跟头部布局走：
	// BSXScript 2.x 用旧头部，但名字/消息块和 3.x 一样是 UTF-16LE（个别
	// 汉化版是 UTF-8）。若按 Classic 去猜 CP932/GBK，标签名会解成乱码，
	// FindLabel("prologue") 找不到入口，标题点 START 只能回退到 boot。
	var encoding ScriptTextEncoding
	if strings.HasPrefix(s.signature, ScriptSignatureModern) {
		if IsUtf8Block(block) {
			encoding = Utf8
		} else {
			encoding = Utf16
		}
	} else {
		encoding = DetectTextEncoding(block)
	}
	return scriptTable{
		listOffset:  int(listOffset),
		count:       int(listSize) / stride,
		blockOffset: int(blockOffset),
		blockSize:   int(blockSize),
		encoding:    encoding,
	}
}

func (s *BsxScriptFile) blockOf(offset, size int) []byte {
	if offset < 0 || offset >= len(s.data) {
		return nil
	}
	if size < 0 {
		size = 0
	}
	if size > len(s.data)-offset {
		size = len(s.data) - offset
	}
	return s.data[offset : offset+size]
}

func (s *BsxScriptFile) readTableString(table scriptTable, index int) string {
	if index < 0 || index >= table.count || table.listOffset <= 0 || table.blockOffset <= 0 {
		return ""
	}
	relative := readInt32AtBytes(s.data, table.listOffset+(index*4))
	if relative < 0 {
		return ""
	}
	return s.readBlockString(table, int(relative))
}

func (s *BsxScriptFile) readBlockString(table scriptTable, relative int) string {
	unit := TextUnitSize(table.encoding)
	block := s.blockOf(table.blockOffset, table.blockSize)
	offset := relative * unit
	if offset < 0 || offset >= len(block) {
		return ""
	}
	return ReadString(block, offset, table.encoding)
}

func (s *BsxScriptFile) readInt32(offset int) int32 {
	return readInt32AtBytes(s.data, offset)
}

// readSystemParameters 读取脚本头部的系统参数表。
//
// BSXScript 2.x/3.x 把表**内联**在头部：经典布局是 count@0x24 / 表@0x94，
// 现代布局是 count@0x28 / 表@0xA8，条目 8 字节 (key u16, kind u16, index i32)。
// 旧实现把 0x94/0x90 处的值当"表指针"，OH 的 0x94 实际是第一条记录，读出来
// 的偏移 0x01030000 越界，整张表被判为空，ResolveSlot 全部失败。
//
// 早期 BSScript Rev. 的安全写法是否用指针无法从现有发行版验证，因此这里
// 先按内联解析；只有在条目明显不合法时才把表头值当指针再试一次。
func (s *BsxScriptFile) readSystemParameters(layout ScriptLayout) []BsSystemParameter {
	countField := layout.SystemParameterCountField()
	table := layout.SystemParameterTableOffset()
	if countField < 0 || table < 0 {
		return nil
	}
	count := int(s.readInt32(countField))
	if count <= 0 || count > 4096 {
		return nil
	}
	if result, ok := s.parseSystemParameterTable(table, count); ok {
		return result
	}
	// 兼容假设：表头里存的是偏移。只有内联解析失败才走这条路。
	pointer := int(s.readInt32(table))
	if pointer != table {
		if result, ok := s.parseSystemParameterTable(pointer, count); ok {
			return result
		}
	}
	return nil
}

// parseSystemParameterTable 解析内联系统参数表，并检查条目是否可信。
//
// 可信条件：每条的 kind 只能是 0x0102/0x0103（现代）或 1/2（Ancient），
// key 不重复，且 index 落在对应的属性/变量表范围内。指针或随机数据很难
// 同时满足这些条件。注意 key 不一定递增（侵蝕/淫獄 是 0,1,2,10,1000,100…），
// 所以不能按单调性判断。
func (s *BsxScriptFile) parseSystemParameterTable(table, count int) ([]BsSystemParameter, bool) {
	if table < 0 || count <= 0 {
		return nil, false
	}
	required := count * scriptSystemParameterSize
	if table >= len(s.data) || required <= 0 || table+required > len(s.data) {
		return nil, false
	}
	result := make([]BsSystemParameter, count)
	seen := make(map[int]struct{}, count)
	for index := range result {
		entry := table + (index * scriptSystemParameterSize)
		parameter := BsSystemParameter{
			Key:   int(readUInt16AtBytes(s.data, entry)),
			Kind:  int(readUInt16AtBytes(s.data, entry+2)),
			Index: int(readInt32AtBytes(s.data, entry+4)),
		}
		var capacity int
		switch parameter.Kind {
		case revPropertyKind, ancientPropertyKind:
			capacity = s.properties.count
		case revVariableKind, ancientVariableKind:
			capacity = s.variables.count
		default:
			return nil, false
		}
		if parameter.Index < 0 || parameter.Index >= capacity {
			return nil, false
		}
		if _, duplicate := seen[parameter.Key]; duplicate {
			return nil, false
		}
		seen[parameter.Key] = struct{}{}
		result[index] = parameter
	}
	return result, true
}

func readInt32AtBytes(data []byte, offset int) int32 {
	if offset < 0 || offset+4 > len(data) {
		return 0
	}
	return binutil.I32(data[offset:])
}

func readUInt16AtBytes(data []byte, offset int) uint16 {
	if offset < 0 || offset+2 > len(data) {
		return 0
	}
	return binutil.U16(data[offset:])
}

func stripPrefix(value string) string {
	if len(value) > 0 && (value[0] == '#' || value[0] == '@') {
		return value[1:]
	}
	return value
}
