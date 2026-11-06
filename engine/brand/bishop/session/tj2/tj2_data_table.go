package tj2

import (
	"errors"

	"kizuna/engine/brand/bishop/vm"
	vmtj2 "kizuna/engine/brand/bishop/vm/tj2"
	"kizuna/engine/core"
	"kizuna/engine/core/binutil"
)

const (
	tj2TableHeaderSize      = 28
	tj2TableCountBlock      = 32
	tj2TableNameField       = 8
	tj2TableRecordSize      = 16
	tj2TableFlagInitSize    = 512
	tj2TableFlagInitEntries = 256
	tj2TableFlagAliasNone   = 0xFFFF
	tj2TableFlagAliasGate   = 0xD5
)

var tj2TableMagic = []byte{'E', 'X', 'S', 'C', 'H', 'E', 'D', 0}

type Tj2DataTable struct {
	sceneCount        int
	flagCount         int
	sourceCount       int
	scenarioBuffer    int
	engineVersion     int
	macroBitCount     int
	saveCompatibility int
	usesSimpleWrite   bool
	dialect           vmtj2.Tj2Dialect
	flagAliases       []vmtj2.Tj2FlagAlias
	graphicNames      []string
	trackNumbers      []string
	soundNames        []string
	functionSlots     []vmtj2.Tj2FunctionSlot
	nameRecords       []vmtj2.Tj2NameRecord
	macros            []vmtj2.Tj2Macro
}

var _ vmtj2.Tj2DataTable = (*Tj2DataTable)(nil)

func LoadTj2DataTable(data []byte) (*Tj2DataTable, error) {
	if len(data) < tj2TableHeaderSize {
		return nil, errors.New("tj2 data table is too short")
	}
	for i := range tj2TableMagic {
		if data[i] != tj2TableMagic[i] {
			return nil, errors.New("tj2 data table magic is not EXSCHED")
		}
	}
	tableOffset := int(binutil.U32(data[8:12]))
	if tableOffset != tj2TableHeaderSize {
		return nil, errors.New("tj2 data table offset mismatch")
	}
	engineVersion := int(binutil.U32(data[16:20]))
	flagUnitWidth := int(binutil.U32(data[20:24]))
	saveCompatibility := int(binutil.U32(data[12:16]))
	reserved := binutil.U32(data[24:28])
	if tableOffset < 0 || tableOffset+tj2TableCountBlock > len(data) {
		return nil, errors.New("tj2 data table ends inside the count block")
	}
	counts := data[tableOffset : tableOffset+tj2TableCountBlock]
	sceneCount := int(binutil.U16(counts[0:2]))
	flagCount := int(binutil.U16(counts[2:4]))
	graphicCount := int(binutil.U16(counts[4:6]))
	trackCount := int(binutil.U16(counts[6:8]))
	nameCount := int(binutil.U16(counts[8:10]))
	functionCount := int(binutil.U16(counts[10:12]))
	soundCount := int(binutil.U32(counts[12:16]))
	sourceCount := int(binutil.U16(counts[16:18]))
	groupSwitch := int(binutil.U16(counts[18:20]))
	scenarioBufferSize := binutil.U32(counts[20:24])
	macroBitCount := binutil.U32(counts[28:32])
	if groupSwitch != 0 {
		return nil, errors.New("tj2 data table group switch is not 0")
	}
	bitmapSize := (sceneCount >> 3) + 1
	hasFlagAliases := engineVersion >= tj2TableFlagAliasGate
	flagInit := 0
	if hasFlagAliases {
		flagInit = tj2TableFlagInitSize
	}
	total := int64(tj2TableHeaderSize+tj2TableCountBlock+bitmapSize) +
		int64(tj2TableNameField)*int64(graphicCount) +
		int64(tj2TableNameField)*int64(trackCount) +
		int64(tj2TableNameField)*int64(soundCount) +
		int64(tj2TableRecordSize)*int64(nameCount) +
		int64(tj2TableRecordSize)*int64(functionCount) +
		int64(flagInit)
	if total != int64(len(data)) {
		return nil, errors.New("tj2 data table sections size mismatch")
	}
	offset := int64(tableOffset + tj2TableCountBlock + bitmapSize)
	graphicNames, err := readTj2Names(data, &offset, graphicCount)
	if err != nil {
		return nil, err
	}
	trackNumbers, err := readTj2Names(data, &offset, trackCount)
	if err != nil {
		return nil, err
	}
	soundNames, err := readTj2Names(data, &offset, soundCount)
	if err != nil {
		return nil, err
	}
	nameRecords, err := readTj2NameRecords(data, &offset, nameCount)
	if err != nil {
		return nil, err
	}
	functionSlots, err := readTj2FunctionSlots(data, &offset, functionCount)
	if err != nil {
		return nil, err
	}
	flagAliases, err := readTj2FlagAliases(data, &offset, hasFlagAliases, flagCount)
	if err != nil {
		return nil, err
	}
	if offset != int64(len(data)) {
		return nil, errors.New("tj2 data table sections end mismatch")
	}
	return &Tj2DataTable{
		sceneCount:        sceneCount,
		flagCount:         flagCount,
		sourceCount:       sourceCount,
		scenarioBuffer:    int(scenarioBufferSize),
		engineVersion:     engineVersion,
		macroBitCount:     int(macroBitCount),
		saveCompatibility: saveCompatibility,
		usesSimpleWrite:   reserved == 0,
		dialect:           vmtj2.CreateDialect(engineVersion, flagUnitWidth),
		flagAliases:       flagAliases,
		graphicNames:      graphicNames,
		trackNumbers:      trackNumbers,
		soundNames:        soundNames,
		functionSlots:     functionSlots,
		nameRecords:       nameRecords,
		macros:            buildTj2Macros(nameRecords),
	}, nil
}

func (t *Tj2DataTable) SceneCount() int {
	return t.sceneCount
}

func (t *Tj2DataTable) FlagCount() int {
	return t.flagCount
}

func (t *Tj2DataTable) SourceCount() int {
	return t.sourceCount
}

func (t *Tj2DataTable) ScenarioBufferSize() int {
	return t.scenarioBuffer
}

func (t *Tj2DataTable) EngineVersion() int {
	return t.engineVersion
}

func (t *Tj2DataTable) MacroBitCount() int {
	return t.macroBitCount
}

func (t *Tj2DataTable) SaveCompatibility() int {
	return t.saveCompatibility
}

func (t *Tj2DataTable) UsesSimpleWrite() bool {
	return t.usesSimpleWrite
}

func (t *Tj2DataTable) Dialect() vmtj2.Tj2Dialect {
	return t.dialect
}

func (t *Tj2DataTable) FlagAliases() []vmtj2.Tj2FlagAlias {
	return t.flagAliases
}

func (t *Tj2DataTable) GraphicNames() []string {
	return t.graphicNames
}

func (t *Tj2DataTable) TrackNumbers() []string {
	return t.trackNumbers
}

func (t *Tj2DataTable) SoundNames() []string {
	return t.soundNames
}

func (t *Tj2DataTable) FunctionSlots() []vmtj2.Tj2FunctionSlot {
	return t.functionSlots
}

func (t *Tj2DataTable) NameRecords() []vmtj2.Tj2NameRecord {
	return t.nameRecords
}

func (t *Tj2DataTable) Macros() []vmtj2.Tj2Macro {
	return t.macros
}

func (t *Tj2DataTable) GraphicName(index int) string {
	if index < 0 || index >= len(t.graphicNames) {
		return ""
	}
	return t.graphicNames[index]
}

func (t *Tj2DataTable) TrackName(index int) string {
	if index < 0 || index >= len(t.trackNumbers) {
		return ""
	}
	return t.trackNumbers[index]
}

func (t *Tj2DataTable) SoundName(index int) string {
	if index < 0 || index >= len(t.soundNames) {
		return ""
	}
	return t.soundNames[index]
}

func buildTj2Macros(records []vmtj2.Tj2NameRecord) []vmtj2.Tj2Macro {
	macros := make([]vmtj2.Tj2Macro, 0, len(records))
	for _, record := range records {
		if len(record.First) == 0 && len(record.Second) == 0 {
			continue
		}
		macros = append(macros, vmtj2.Tj2Macro{From: record.First, To: record.Second})
	}
	return macros
}

func readTj2Names(data []byte, offset *int64, count int) ([]string, error) {
	names := make([]string, count)
	for i := range names {
		field, err := readTj2Field(data, offset, tj2TableNameField)
		if err != nil {
			return nil, err
		}
		names[i] = decodeTj2NameField(field)
	}
	return names, nil
}

func readTj2NameRecords(data []byte, offset *int64, count int) ([]vmtj2.Tj2NameRecord, error) {
	records := make([]vmtj2.Tj2NameRecord, count)
	for i := range records {
		field, err := readTj2Field(data, offset, tj2TableRecordSize)
		if err != nil {
			return nil, err
		}
		records[i] = vmtj2.Tj2NameRecord{
			First:  decodeTj2NameField(field[:tj2TableNameField]),
			Second: decodeTj2NameField(field[tj2TableNameField:]),
		}
	}
	return records, nil
}

func readTj2FunctionSlots(data []byte, offset *int64, count int) ([]vmtj2.Tj2FunctionSlot, error) {
	slots := make([]vmtj2.Tj2FunctionSlot, count)
	for i := range slots {
		field, err := readTj2Field(data, offset, tj2TableRecordSize)
		if err != nil {
			return nil, err
		}
		text := decodeTj2NameField(field[:tj2TableNameField])
		slots[i] = vmtj2.Tj2FunctionSlot{Number: tj2Atoi(text), Text: text}
	}
	return slots, nil
}

func readTj2FlagAliases(data []byte, offset *int64, present bool, flagCount int) ([]vmtj2.Tj2FlagAlias, error) {
	if !present {
		return []vmtj2.Tj2FlagAlias{}, nil
	}
	field, err := readTj2Field(data, offset, tj2TableFlagInitSize)
	if err != nil {
		return nil, err
	}
	aliases := make([]vmtj2.Tj2FlagAlias, 0, tj2TableFlagInitEntries)
	// 旧版 EXSCHED（学園 2001）的版本号落在"有别名表"的区间里，但最后
	// 512 字节其实是声音名，不是 256 组 u16 别名：值会出现 0x4F4E 这种
	// 远超 flag 数的目标。真实别名表是稀疏的（绝大多数是 0xFFFF），
	// 声音名表则是 256 条全部非空；两者同时出现时才整块丢弃。
	// 稀疏表即使个别下标超出脚本 flag 数也保留：机器侧的 read/writeFlag
	// 已经做了越界保护，不能因为一个坏条目丢掉整张表。
	outOfRange := 0
	for i := 0; i < tj2TableFlagInitEntries; i++ {
		target := int(binutil.U16(field[i*2:]))
		if target == tj2TableFlagAliasNone || target == i {
			continue
		}
		if target >= flagCount || i >= flagCount {
			outOfRange++
		}
		aliases = append(aliases, vmtj2.Tj2FlagAlias{Source: i, Target: target})
	}
	if outOfRange > 0 && len(aliases) > 32 {
		core.Logf("tj2: flag alias block looks like a sound-name table (%d/%d aliases out of range), ignored", outOfRange, len(aliases))
		return []vmtj2.Tj2FlagAlias{}, nil
	}
	return aliases, nil
}

func readTj2Field(data []byte, offset *int64, size int) ([]byte, error) {
	if *offset+int64(size) > int64(len(data)) {
		return nil, errors.New("tj2 data table ends inside a field")
	}
	field := data[*offset : *offset+int64(size)]
	*offset += int64(size)
	return field, nil
}

func decodeTj2NameField(field []byte) string {
	end := len(field)
	for i, b := range field {
		if b == 0 {
			end = i
			break
		}
	}
	return vm.DecodeText(field[:end], vm.Cp932)
}
