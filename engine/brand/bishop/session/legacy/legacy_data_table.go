package legacy

import (
	"fmt"
	"strings"

	"kizuna/engine/brand/bishop/vm"
	"kizuna/engine/brand/bishop/vm/scf"
	"kizuna/engine/core/binutil"
)

const (
	legacyTableHeaderSize = 28
	legacyNameFieldSize   = 8
	legacyRecordSize      = 16
)

type LegacyDataTable struct {
	flagCount          int
	sourceCount        int
	scenarioBufferSize int
	graphicNames       []string
	trackNumbers       []string
	soundNames         []string
	functionSlots      []scf.ScfFunctionSlot
	slotNames          []string
}

func LoadLegacyDataTable(data []byte) (*LegacyDataTable, error) {
	if len(data) < legacyTableHeaderSize {
		return nil, fmt.Errorf("Data is too short for a legacy data table: %d bytes.", len(data))
	}
	sceneCount := int(binutil.U16(data[0:]))
	flagCount := int(binutil.U16(data[2:]))
	graphicCount := int(binutil.U16(data[4:]))
	trackCount := int(binutil.U16(data[6:]))
	slotACount := int(binutil.U16(data[8:]))
	functionCount := int(binutil.U16(data[10:]))
	soundCount := int(binutil.U32(data[12:]))
	sourceCount := int(binutil.U16(data[16:]))
	scenarioBufferSize := int(binutil.U32(data[20:]))
	offset := legacyTableHeaderSize + (sceneCount >> 3) + 1
	graphicNames, err := readLegacyNames(data, &offset, graphicCount)
	if err != nil {
		return nil, err
	}
	trackNumbers, err := readLegacyNames(data, &offset, trackCount)
	if err != nil {
		return nil, err
	}
	soundNames, err := readLegacyNames(data, &offset, soundCount)
	if err != nil {
		return nil, err
	}
	slotARecords, err := readLegacyRawRecords(data, &offset, slotACount)
	if err != nil {
		return nil, err
	}
	functionRecords, err := readLegacyRawRecords(data, &offset, functionCount)
	if err != nil {
		return nil, err
	}
	functionSlots := make([]scf.ScfFunctionSlot, len(functionRecords))
	for index, record := range functionRecords {
		functionSlots[index] = scf.ScfFunctionSlot{
			Number: decodeLegacySlotHalf(record[:legacyNameFieldSize]),
			Name:   decodeLegacySlotHalf(record[legacyNameFieldSize:]),
		}
	}
	slotNames := make([]string, 0, len(slotARecords)+len(functionRecords))
	for _, record := range slotARecords {
		slotNames = append(slotNames, decodeLegacyRecord(record))
	}
	for _, record := range functionRecords {
		slotNames = append(slotNames, decodeLegacyRecord(record))
	}
	return &LegacyDataTable{
		flagCount:          flagCount,
		sourceCount:        sourceCount,
		scenarioBufferSize: scenarioBufferSize,
		graphicNames:       graphicNames,
		trackNumbers:       trackNumbers,
		soundNames:         soundNames,
		functionSlots:      functionSlots,
		slotNames:          slotNames,
	}, nil
}

func (t *LegacyDataTable) FlagCount() int {
	return t.flagCount
}

func (t *LegacyDataTable) SourceCount() int {
	return t.sourceCount
}

func (t *LegacyDataTable) ScenarioBufferSize() int {
	return t.scenarioBufferSize
}

func (t *LegacyDataTable) GraphicNames() []string {
	return t.graphicNames
}

func (t *LegacyDataTable) TrackNumbers() []string {
	return t.trackNumbers
}

func (t *LegacyDataTable) SoundNames() []string {
	return t.soundNames
}

func (t *LegacyDataTable) FunctionSlots() []scf.ScfFunctionSlot {
	return t.functionSlots
}

func (t *LegacyDataTable) SlotNames() []string {
	return t.slotNames
}

func readLegacyNames(data []byte, offset *int, count int) ([]string, error) {
	names := make([]string, 0, count)
	for index := 0; index < count; index++ {
		field, err := readLegacyField(data, offset, legacyNameFieldSize)
		if err != nil {
			return nil, err
		}
		names = append(names, decodeLegacyNameField(field))
	}
	return names, nil
}

func readLegacyRawRecords(data []byte, offset *int, count int) ([][]byte, error) {
	records := make([][]byte, 0, count)
	for index := 0; index < count; index++ {
		field, err := readLegacyField(data, offset, legacyRecordSize)
		if err != nil {
			return nil, err
		}
		records = append(records, append([]byte(nil), field...))
	}
	return records, nil
}

func readLegacyField(data []byte, offset *int, size int) ([]byte, error) {
	if *offset+size > len(data) {
		return nil, fmt.Errorf("Legacy data table ends inside a %d-byte field at offset %d.", size, *offset)
	}
	field := data[*offset : *offset+size]
	*offset += size
	return field, nil
}

func decodeLegacyNameField(field []byte) string {
	end := len(field)
	for index, value := range field {
		if value == 0 {
			end = index
			break
		}
	}
	slice := field[:end]
	return vm.DecodeText(slice, vm.DetectTextEncoding(slice))
}

func decodeLegacyRecord(record []byte) string {
	return strings.TrimRight(vm.DecodeText(record, vm.DetectTextEncoding(record)), "\x00")
}

func decodeLegacySlotHalf(field []byte) string {
	return strings.TrimRight(strings.Trim(vm.DecodeText(field, vm.DetectTextEncoding(field)), "\x00"), " ")
}
