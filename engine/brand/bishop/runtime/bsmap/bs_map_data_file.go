package bsmap

import (
	"os"

	"kizuna/engine/brand/bishop/resource"
	"kizuna/engine/core/binutil"
)

const BsMapDataFileName = "map.dat"

const BsMapDataFallbackResultProperty = "#result"

var bsMapDataSignature = []byte("BSX MAP INFO r3\x00")

const bsMapDataMaximumTextLength = 256

type BsMapData struct {
	TimeProperty   string
	TimeValues     []int
	Images         []string
	Places         []BsMapPlace
	Tables         []BsMapUnlockTable
	ResultProperty string
}

var BsMapDataEmpty = BsMapData{
	TimeProperty:   "",
	TimeValues:     []int{},
	Images:         []string{},
	Places:         []BsMapPlace{},
	Tables:         []BsMapUnlockTable{},
	ResultProperty: BsMapDataFallbackResultProperty,
}

func (d BsMapData) TimeSlot(timeValue int) int {
	for index := 0; index < len(d.TimeValues); index++ {
		if d.TimeValues[index] == timeValue {
			return index
		}
	}
	return -1
}

func (d BsMapData) ImageName(imageIndex int, resources *resource.GameResourceIndex) string {
	position := imageIndex - 1
	if position < 0 || position >= len(d.Images) {
		return ""
	}
	name := d.Images[position]
	if len(name) > 0 && resources.Exists(name) {
		return name
	}
	return ""
}

type BsMapPlace struct {
	ImageIndex int
	BaseX      int
	BaseY      int
	Chunks     []BsMapChunk
}

type BsMapChunk struct {
	X int
	Y int
}

type BsMapUnlockTable struct {
	ProgressProperty string
	StateProperty    string
	Ranges           []BsMapUnlockRange
}

var BsMapUnlockTableEmpty = BsMapUnlockTable{ProgressProperty: "", StateProperty: "", Ranges: []BsMapUnlockRange{}}

func (t BsMapUnlockTable) RowFor(progress int) *BsMapUnlockRange {
	var selected *BsMapUnlockRange
	for i := range t.Ranges {
		candidate := &t.Ranges[i]
		if candidate.Threshold <= progress && (selected == nil || candidate.Threshold > selected.Threshold) {
			selected = candidate
		}
	}
	return selected
}

type BsMapUnlockRange struct {
	Threshold int
	Values    []uint16
}

func TryLoadBsMapData(gameRoot string, data *BsMapData) bool {
	*data = BsMapDataEmpty
	raw, err := os.ReadFile(gameRoot + "/" + BsMapDataFileName)
	if err != nil {
		return false
	}
	return TryParseBsMapData(raw, data)
}

func TryParseBsMapData(raw []byte, data *BsMapData) bool {
	*data = BsMapDataEmpty
	if len(raw) < len(bsMapDataSignature) {
		return false
	}
	for i := 0; i < len(bsMapDataSignature); i++ {
		if raw[i] != bsMapDataSignature[i] {
			return false
		}
	}
	reader := bsMapDataReader{reader: binutil.NewReader(raw)}
	if !reader.trySkip(len(bsMapDataSignature)) {
		return false
	}
	timeProperty, ok := reader.tryReadText()
	if !ok {
		return false
	}
	timeValueCount, ok := reader.tryReadByte()
	if !ok {
		return false
	}
	timeValues := make([]int, timeValueCount)
	for index := range timeValues {
		value, ok := reader.tryReadInt32()
		if !ok {
			return false
		}
		timeValues[index] = value
	}
	imageCount, ok := reader.tryReadByte()
	if !ok {
		return false
	}
	images := make([]string, imageCount)
	for index := range images {
		text, ok := reader.tryReadText()
		if !ok {
			return false
		}
		images[index] = text
	}
	placeCount, ok := reader.tryReadByte()
	if !ok || placeCount < 1 {
		return false
	}
	places := make([]BsMapPlace, placeCount)
	for index := range places {
		place, ok := tryReadBsMapPlace(&reader)
		if !ok {
			return false
		}
		places[index] = place
	}
	tableCount, ok := reader.tryReadByte()
	if !ok {
		return false
	}
	tables := make([]BsMapUnlockTable, tableCount)
	for index := range tables {
		table, ok := tryReadBsMapUnlockTable(&reader)
		if !ok {
			return false
		}
		tables[index] = table
	}
	resultCount, ok := reader.tryReadByte()
	if !ok || resultCount < 1 {
		return false
	}
	resultProperty := ""
	for index := 0; index < resultCount; index++ {
		name, ok := reader.tryReadText()
		if !ok {
			return false
		}
		if index == 0 {
			resultProperty = name
		}
	}
	if !reader.isConsumed() {
		return false
	}
	if len(resultProperty) == 0 {
		resultProperty = BsMapDataFallbackResultProperty
	}
	*data = BsMapData{
		TimeProperty:   timeProperty,
		TimeValues:     timeValues,
		Images:         images,
		Places:         places,
		Tables:         tables,
		ResultProperty: resultProperty,
	}
	return true
}

func tryReadBsMapPlace(reader *bsMapDataReader) (BsMapPlace, bool) {
	var place BsMapPlace
	imageIndex, ok := reader.tryReadByte()
	if !ok {
		return place, false
	}
	baseX, ok := reader.tryReadUInt16()
	if !ok {
		return place, false
	}
	baseY, ok := reader.tryReadUInt16()
	if !ok {
		return place, false
	}
	chunkCount, ok := reader.tryReadByte()
	if !ok || chunkCount < 1 {
		return place, false
	}
	chunks := make([]BsMapChunk, chunkCount)
	for index := range chunks {
		x, ok := reader.tryReadInt32()
		if !ok {
			return place, false
		}
		y, ok := reader.tryReadInt32()
		if !ok {
			return place, false
		}
		chunks[index] = BsMapChunk{X: x, Y: y}
	}
	place = BsMapPlace{ImageIndex: imageIndex, BaseX: baseX, BaseY: baseY, Chunks: chunks}
	return place, true
}

func tryReadBsMapUnlockTable(reader *bsMapDataReader) (BsMapUnlockTable, bool) {
	table := BsMapUnlockTableEmpty
	progressProperty, ok := reader.tryReadText()
	if !ok {
		return table, false
	}
	stateProperty, ok := reader.tryReadText()
	if !ok {
		return table, false
	}
	rangeCount, ok := reader.tryReadByte()
	if !ok {
		return table, false
	}
	ranges := make([]BsMapUnlockRange, rangeCount)
	for index := range ranges {
		threshold, ok := reader.tryReadInt32()
		if !ok {
			return table, false
		}
		valueCount, ok := reader.tryReadByte()
		if !ok {
			return table, false
		}
		values := make([]uint16, valueCount)
		for value := range values {
			packed, ok := reader.tryReadUInt16()
			if !ok {
				return table, false
			}
			values[value] = uint16(packed)
		}
		ranges[index] = BsMapUnlockRange{Threshold: threshold, Values: values}
	}
	table = BsMapUnlockTable{ProgressProperty: progressProperty, StateProperty: stateProperty, Ranges: ranges}
	return table, true
}

type bsMapDataReader struct {
	reader *binutil.Reader
}

func (r *bsMapDataReader) isConsumed() bool {
	return r.reader.Remaining() == 0
}

func (r *bsMapDataReader) trySkip(count int) bool {
	if count < 0 || count > r.reader.Remaining() {
		return false
	}
	r.reader.Skip(count)
	return true
}

func (r *bsMapDataReader) tryReadByte() (int, bool) {
	if r.reader.Remaining() < 1 {
		return 0, false
	}
	return int(r.reader.U8()), true
}

func (r *bsMapDataReader) tryReadUInt16() (int, bool) {
	if r.reader.Remaining() < 2 {
		return 0, false
	}
	return int(r.reader.U16()), true
}

func (r *bsMapDataReader) tryReadInt32() (int, bool) {
	if r.reader.Remaining() < 4 {
		return 0, false
	}
	// 这里必须是**有符号** int32：Chunks 的 y 偏移是负数（卡片向上排第二行），
	// 旧实现按 U32 读会把 -66 变成 4294967230，BaseY + offset 直接溢出到屏幕外。
	// 时间值/阈值/地点号仍然是正常正数，按 I32 读不会改变它们。
	return int(r.reader.I32()), true
}

func (r *bsMapDataReader) tryReadText() (string, bool) {
	start := r.reader.Pos()
	text := make([]byte, 0, bsMapDataMaximumTextLength)
	for {
		if r.reader.Remaining() <= 0 {
			r.reader.SetPos(start)
			return "", false
		}
		value := r.reader.U8()
		if value == 0 {
			return string(text), true
		}
		text = append(text, value)
		if len(text) > bsMapDataMaximumTextLength {
			r.reader.SetPos(start)
			return "", false
		}
	}
}
