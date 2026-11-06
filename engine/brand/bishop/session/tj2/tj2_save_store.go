package tj2

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	vmtj2 "kizuna/engine/brand/bishop/vm/tj2"
	"kizuna/engine/core"
	"kizuna/engine/core/binutil"
)

const (
	tj2SaveMagic             = 0x544A3244
	tj2SaveVersion           = 6
	tj2SaveMinSupported      = 4
	tj2SaveMinHistoryVersion = 5
	tj2SaveMaxHistoryEntries = 64
	// 版本 6 起在末尾追加"分页对白"块；旧版本读到 history 就结束。
	tj2SaveMinMessagePages = 6
	tj2SaveMaxMessagePages = 256
	tj2SaveMaxIntBlock     = 4096
	tj2SaveSlotCount       = 24
)

const dotNetTicksPerSecond = int64(10000000)

const unixToDotNetSeconds = int64(62135596800)

type Tj2SaveSlotInfo struct {
	Index      int
	Path       string
	SavedAt    time.Time
	HasSavedAt bool
	Summary    string
	Exists     bool
}

type Tj2SaveStore struct {
	directory string
}

func NewTj2SaveStore(savesDirectory string) *Tj2SaveStore {
	return &Tj2SaveStore{directory: filepath.Join(savesDirectory, "tj2")}
}

func (s *Tj2SaveStore) DirectoryPath() string {
	return s.directory
}

func (s *Tj2SaveStore) Write(state *Tj2SaveState, slot int) bool {
	if err := os.MkdirAll(s.directory, 0o755); err != nil {
		core.Logf("tj2-save: slot %d write failed: %v", slot, err)
		return false
	}
	var buf bytes.Buffer
	writeTj2SaveState(&buf, state)
	if err := os.WriteFile(s.pathFor(slot), buf.Bytes(), 0o644); err != nil {
		core.Logf("tj2-save: slot %d write failed: %v", slot, err)
		return false
	}
	return true
}

func (s *Tj2SaveStore) Read(slot int) *Tj2SaveState {
	data, err := os.ReadFile(s.pathFor(slot))
	if err != nil {
		return nil
	}
	state, err := readTj2SaveState(data)
	if err != nil {
		core.Logf("tj2-save: slot %d read failed: %v", slot, err)
		return nil
	}
	return state
}

func (s *Tj2SaveStore) List(slots int) []Tj2SaveSlotInfo {
	if slots <= 0 {
		slots = tj2SaveSlotCount
	}
	result := make([]Tj2SaveSlotInfo, slots)
	for index := range result {
		result[index] = s.Inspect(index)
	}
	return result
}

func (s *Tj2SaveStore) Inspect(slot int) Tj2SaveSlotInfo {
	path := s.pathFor(slot)
	state := s.Read(slot)
	if state == nil {
		return Tj2SaveSlotInfo{Index: slot, Path: path}
	}
	return Tj2SaveSlotInfo{
		Index:      slot,
		Path:       path,
		SavedAt:    ticksToTj2Time(state.SavedAtTicks),
		HasSavedAt: true,
		Summary:    state.Summary,
		Exists:     true,
	}
}

func (s *Tj2SaveStore) pathFor(slot int) string {
	return filepath.Join(s.directory, fmt.Sprintf("slot%03d.tjs", slot))
}

func ticksToTj2Time(ticks int64) time.Time {
	seconds := ticks/dotNetTicksPerSecond - unixToDotNetSeconds
	nanos := (ticks % dotNetTicksPerSecond) * 100
	return time.Unix(seconds, nanos).UTC().Local()
}

func tj2TimeToTicks(t time.Time) int64 {
	u := t.UTC()
	return u.Unix()*dotNetTicksPerSecond + unixToDotNetSeconds*dotNetTicksPerSecond + int64(u.Nanosecond()/100)
}

func writeTj2SaveState(buf *bytes.Buffer, state *Tj2SaveState) {
	var scratch [8]byte
	binary.LittleEndian.PutUint32(scratch[:4], tj2SaveMagic)
	buf.Write(scratch[:4])
	binary.LittleEndian.PutUint32(scratch[:4], uint32(int32(tj2SaveVersion)))
	buf.Write(scratch[:4])
	writeTj2Int32(buf, int32(state.ScenarioNumber))
	writeTj2OptionalMachine(buf, state.Machine)
	writeTj2String(buf, state.BackgroundName)
	writeTj2Int32(buf, int32(state.BackgroundOffsetX))
	writeTj2Int32(buf, int32(state.BackgroundOffsetY))
	writeTj2Int32(buf, int32(len(state.Standees)))
	for _, entry := range state.Standees {
		writeTj2Int32(buf, int32(entry.Layer))
		writeTj2Int32(buf, int32(entry.Index))
		writeTj2String(buf, entry.Name)
		writeTj2Int32(buf, int32(entry.OffsetX))
		writeTj2Int32(buf, int32(entry.OffsetY))
	}
	writeTj2Int32(buf, int32(state.BgmTrack))
	writeTj2String(buf, state.Speaker)
	writeTj2String(buf, state.Text)
	writeTj2String(buf, state.Summary)
	binary.LittleEndian.PutUint64(scratch[:8], uint64(state.SavedAtTicks))
	buf.Write(scratch[:8])
	thumbnail := state.Thumbnail
	if thumbnail == nil {
		thumbnail = []byte{}
	}
	writeTj2Int32(buf, int32(len(thumbnail)))
	buf.Write(thumbnail)
	writeTj2Int32(buf, int32(state.ThumbnailWidth))
	writeTj2Int32(buf, int32(state.ThumbnailHeight))
	writeTj2Int32(buf, int32(len(state.BackgroundHistory)))
	for _, layer := range state.BackgroundHistory {
		writeTj2String(buf, layer.Name)
		writeTj2Int32(buf, int32(layer.OffsetX))
		writeTj2Int32(buf, int32(layer.OffsetY))
	}
	writeTj2Int32(buf, int32(len(state.MessagePages)))
	for _, page := range state.MessagePages {
		writeTj2String(buf, page)
	}
	writeTj2Int32(buf, int32(state.MessagePage))
}

type tj2SaveReader struct {
	reader *binutil.Reader
}

func readTj2SaveState(data []byte) (*Tj2SaveState, error) {
	r := &tj2SaveReader{reader: binutil.NewReader(data)}
	magic, ok := r.readUint32()
	if !ok || magic != tj2SaveMagic {
		return nil, errors.New("tj2 save magic mismatch")
	}
	version, ok := r.readInt32()
	if !ok {
		return nil, errors.New("tj2 save truncated")
	}
	if version < tj2SaveMinSupported || version > tj2SaveVersion {
		return nil, fmt.Errorf("tj2 save version %d unsupported", version)
	}
	scenario, ok := r.readInt32()
	if !ok {
		return nil, errors.New("tj2 save truncated")
	}
	machine, err := r.readOptionalMachine()
	if err != nil {
		return nil, err
	}
	background, err := r.readString()
	if err != nil {
		return nil, err
	}
	backgroundOffsetX, ok := r.readInt32()
	if !ok {
		return nil, errors.New("tj2 save truncated")
	}
	backgroundOffsetY, ok := r.readInt32()
	if !ok {
		return nil, errors.New("tj2 save truncated")
	}
	standeeCount, ok := r.readInt32()
	if !ok {
		return nil, errors.New("tj2 save truncated")
	}
	if standeeCount < 0 || standeeCount > Tj2StandeeCapacity {
		return nil, errors.New("tj2 save standee count invalid")
	}
	standees := make([]Tj2StandeeEntry, 0, standeeCount)
	for i := int32(0); i < standeeCount; i++ {
		layer, ok := r.readInt32()
		if !ok {
			return nil, errors.New("tj2 save truncated")
		}
		entryIndex, ok := r.readInt32()
		if !ok {
			return nil, errors.New("tj2 save truncated")
		}
		name, err := r.readString()
		if err != nil {
			return nil, err
		}
		offsetX, ok := r.readInt32()
		if !ok {
			return nil, errors.New("tj2 save truncated")
		}
		offsetY, ok := r.readInt32()
		if !ok {
			return nil, errors.New("tj2 save truncated")
		}
		standees = append(standees, Tj2StandeeEntry{Layer: int(layer), Index: int(entryIndex), Name: name, OffsetX: int(offsetX), OffsetY: int(offsetY)})
	}
	bgmTrack, ok := r.readInt32()
	if !ok {
		return nil, errors.New("tj2 save truncated")
	}
	speaker, err := r.readString()
	if err != nil {
		return nil, err
	}
	text, err := r.readString()
	if err != nil {
		return nil, err
	}
	summary, err := r.readString()
	if err != nil {
		return nil, err
	}
	savedAt, ok := r.readInt64()
	if !ok {
		return nil, errors.New("tj2 save truncated")
	}
	thumbnailLength, ok := r.readInt32()
	if !ok || thumbnailLength < 0 {
		return nil, errors.New("tj2 save thumbnail invalid")
	}
	thumbnail, ok := r.readBytes(int(thumbnailLength))
	if !ok {
		return nil, errors.New("tj2 save thumbnail truncated")
	}
	thumbnailWidth, ok := r.readInt32()
	if !ok {
		return nil, errors.New("tj2 save truncated")
	}
	thumbnailHeight, ok := r.readInt32()
	if !ok {
		return nil, errors.New("tj2 save truncated")
	}
	history := readTj2BackgroundHistory(r, version)
	messagePages, messagePage := readTj2MessagePages(r, version)
	return &Tj2SaveState{
		Version:           int(version),
		ScenarioNumber:    int(scenario),
		Machine:           machine,
		BackgroundName:    background,
		BackgroundOffsetX: int(backgroundOffsetX),
		BackgroundOffsetY: int(backgroundOffsetY),
		BackgroundHistory: history,
		Standees:          standees,
		BgmTrack:          int(bgmTrack),
		Speaker:           speaker,
		Text:              text,
		Summary:           summary,
		SavedAtTicks:      savedAt,
		Thumbnail:         thumbnail,
		ThumbnailWidth:    int(thumbnailWidth),
		ThumbnailHeight:   int(thumbnailHeight),
		MessagePages:      messagePages,
		MessagePage:       messagePage,
	}, nil
}

func readTj2BackgroundHistory(r *tj2SaveReader, version int32) []Tj2BackgroundLayer {
	if version < tj2SaveMinHistoryVersion {
		return []Tj2BackgroundLayer{}
	}
	count, ok := r.readInt32()
	if !ok {
		core.Logf("tj2-save: background history truncated, ignoring")
		return []Tj2BackgroundLayer{}
	}
	if count < 0 || count > tj2SaveMaxHistoryEntries {
		core.Logf("tj2-save: background history count %d invalid, ignoring", count)
		return []Tj2BackgroundLayer{}
	}
	history := make([]Tj2BackgroundLayer, 0, count)
	for i := int32(0); i < count; i++ {
		name, err := r.readString()
		if err != nil {
			core.Logf("tj2-save: background history truncated, ignoring")
			return []Tj2BackgroundLayer{}
		}
		offsetX, ok := r.readInt32()
		if !ok {
			core.Logf("tj2-save: background history truncated, ignoring")
			return []Tj2BackgroundLayer{}
		}
		offsetY, ok := r.readInt32()
		if !ok {
			core.Logf("tj2-save: background history truncated, ignoring")
			return []Tj2BackgroundLayer{}
		}
		history = append(history, Tj2BackgroundLayer{Name: name, OffsetX: int(offsetX), OffsetY: int(offsetY)})
	}
	return history
}

func writeTj2OptionalMachine(buf *bytes.Buffer, machine *vmtj2.Tj2MachineState) {
	if machine == nil {
		buf.WriteByte(0)
		return
	}
	buf.WriteByte(1)
	writeTj2Machine(buf, machine)
}

func (r *tj2SaveReader) readOptionalMachine() (*vmtj2.Tj2MachineState, error) {
	if r.reader.Remaining() < 1 {
		return nil, errors.New("tj2 save truncated")
	}
	present := r.reader.U8() != 0
	if !present {
		return nil, nil
	}
	machine, err := r.readMachine()
	if err != nil {
		return nil, err
	}
	return machine, nil
}

func writeTj2Machine(buf *bytes.Buffer, machine *vmtj2.Tj2MachineState) {
	writeTj2Int32(buf, int32(machine.ProgramCounter))
	writeTj2Int32(buf, int32(machine.ScenarioNumber))
	writeTj2Bytes(buf, machine.Flags)
	writeTj2Bytes(buf, machine.Source)
	writeTj2Strings(buf, machine.NameSlots)
	writeTj2Int32(buf, int32(machine.CondDepth))
	writeTj2Ints(buf, machine.CondStack)
	writeTj2Ints(buf, machine.LoopSlots)
	writeTj2Int32(buf, int32(machine.LoopHead))
	writeTj2Int32(buf, int32(machine.Latch))
	writeTj2Int32(buf, int32(machine.MessageOffset))
	if machine.Halted {
		buf.WriteByte(1)
	} else {
		buf.WriteByte(0)
	}
	if machine.Faulted {
		buf.WriteByte(1)
	} else {
		buf.WriteByte(0)
	}
	writeTj2Int32(buf, int32(len(machine.Frames)))
	for _, frame := range machine.Frames {
		writeTj2Frame(buf, &frame)
	}
}

func (r *tj2SaveReader) readMachine() (*vmtj2.Tj2MachineState, error) {
	programCounter, ok := r.readInt32()
	if !ok {
		return nil, errors.New("tj2 save truncated")
	}
	scenario, ok := r.readInt32()
	if !ok {
		return nil, errors.New("tj2 save truncated")
	}
	flags, err := r.readByteBlock("flags")
	if err != nil {
		return nil, err
	}
	source, err := r.readByteBlock("source")
	if err != nil {
		return nil, err
	}
	names, err := r.readStrings()
	if err != nil {
		return nil, err
	}
	condDepth, ok := r.readInt32()
	if !ok {
		return nil, errors.New("tj2 save truncated")
	}
	condStack, err := r.readInts()
	if err != nil {
		return nil, err
	}
	loopSlots, err := r.readInts()
	if err != nil {
		return nil, err
	}
	loopHead, ok := r.readInt32()
	if !ok {
		return nil, errors.New("tj2 save truncated")
	}
	latch, ok := r.readInt32()
	if !ok {
		return nil, errors.New("tj2 save truncated")
	}
	messageOffset, ok := r.readInt32()
	if !ok {
		return nil, errors.New("tj2 save truncated")
	}
	halted, ok := r.readBool()
	if !ok {
		return nil, errors.New("tj2 save truncated")
	}
	faulted, ok := r.readBool()
	if !ok {
		return nil, errors.New("tj2 save truncated")
	}
	frameCount, ok := r.readInt32()
	if !ok {
		return nil, errors.New("tj2 save truncated")
	}
	if frameCount < 0 || frameCount > vmtj2.MaxCallDepth {
		return nil, errors.New("tj2 save frame count invalid")
	}
	frames := make([]vmtj2.Tj2CallFrameState, 0, frameCount)
	for i := int32(0); i < frameCount; i++ {
		frame, err := r.readFrame()
		if err != nil {
			return nil, err
		}
		frames = append(frames, *frame)
	}
	return &vmtj2.Tj2MachineState{
		ProgramCounter: int(programCounter),
		ScenarioNumber: int(scenario),
		Flags:          flags,
		Source:         source,
		NameSlots:      names,
		CondDepth:      int(condDepth),
		CondStack:      condStack,
		LoopSlots:      loopSlots,
		LoopHead:       int(loopHead),
		Latch:          int(latch),
		MessageOffset:  int(messageOffset),
		Halted:         halted,
		Faulted:        faulted,
		Frames:         frames,
	}, nil
}

func writeTj2Frame(buf *bytes.Buffer, frame *vmtj2.Tj2CallFrameState) {
	writeTj2Int32(buf, int32(frame.ScenarioNumber))
	writeTj2Int32(buf, int32(frame.ProgramCounter))
	writeTj2Int32(buf, int32(frame.CondDepth))
	writeTj2Ints(buf, frame.CondStack)
	writeTj2Ints(buf, frame.LoopSlots)
	writeTj2Int32(buf, int32(frame.Latch))
	writeTj2Int32(buf, int32(frame.MessageOffset))
	writeTj2Bytes(buf, frame.Source)
}

func (r *tj2SaveReader) readFrame() (*vmtj2.Tj2CallFrameState, error) {
	scenario, ok := r.readInt32()
	if !ok {
		return nil, errors.New("tj2 save truncated")
	}
	programCounter, ok := r.readInt32()
	if !ok {
		return nil, errors.New("tj2 save truncated")
	}
	condDepth, ok := r.readInt32()
	if !ok {
		return nil, errors.New("tj2 save truncated")
	}
	condStack, err := r.readInts()
	if err != nil {
		return nil, err
	}
	loopSlots, err := r.readInts()
	if err != nil {
		return nil, err
	}
	latch, ok := r.readInt32()
	if !ok {
		return nil, errors.New("tj2 save truncated")
	}
	messageOffset, ok := r.readInt32()
	if !ok {
		return nil, errors.New("tj2 save truncated")
	}
	source, err := r.readByteBlock("frame source")
	if err != nil {
		return nil, err
	}
	return &vmtj2.Tj2CallFrameState{
		ScenarioNumber: int(scenario),
		ProgramCounter: int(programCounter),
		CondDepth:      int(condDepth),
		CondStack:      condStack,
		LoopSlots:      loopSlots,
		Latch:          int(latch),
		MessageOffset:  int(messageOffset),
		Source:         source,
	}, nil
}

func writeTj2Int32(buf *bytes.Buffer, value int32) {
	var scratch [4]byte
	binary.LittleEndian.PutUint32(scratch[:], uint32(value))
	buf.Write(scratch[:])
}

func writeTj2Bytes(buf *bytes.Buffer, value []byte) {
	writeTj2Int32(buf, int32(len(value)))
	buf.Write(value)
}

func writeTj2Ints(buf *bytes.Buffer, values []int) {
	writeTj2Int32(buf, int32(len(values)))
	for _, value := range values {
		writeTj2Int32(buf, int32(value))
	}
}

func writeTj2Strings(buf *bytes.Buffer, values []string) {
	writeTj2Int32(buf, int32(len(values)))
	for _, value := range values {
		writeTj2String(buf, value)
	}
}

func writeTj2String(buf *bytes.Buffer, value string) {
	writeTj27Bit(buf, len(value))
	buf.WriteString(value)
}

func writeTj27Bit(buf *bytes.Buffer, value int) {
	v := uint32(value)
	for v >= 0x80 {
		buf.WriteByte(byte(v) | 0x80)
		v >>= 7
	}
	buf.WriteByte(byte(v))
}

func (r *tj2SaveReader) readUint32() (uint32, bool) {
	if r.reader.Remaining() < 4 {
		return 0, false
	}
	return r.reader.U32(), true
}

func (r *tj2SaveReader) readInt32() (int32, bool) {
	value, ok := r.readUint32()
	return int32(value), ok
}

func (r *tj2SaveReader) readInt64() (int64, bool) {
	if r.reader.Remaining() < 8 {
		return 0, false
	}
	low := r.reader.U32()
	high := r.reader.U32()
	return int64(uint64(high)<<32 | uint64(low)), true
}

func (r *tj2SaveReader) readBool() (bool, bool) {
	if r.reader.Remaining() < 1 {
		return false, false
	}
	return r.reader.U8() != 0, true
}

func (r *tj2SaveReader) readBytes(length int) ([]byte, bool) {
	if length < 0 || length > r.reader.Remaining() {
		return nil, false
	}
	return r.reader.Bytes(length), true
}

func (r *tj2SaveReader) readByteBlock(field string) ([]byte, error) {
	length, ok := r.readInt32()
	if !ok || length < 0 {
		return nil, fmt.Errorf("tj2 save %s invalid", field)
	}
	value, ok := r.readBytes(int(length))
	if !ok {
		return nil, fmt.Errorf("tj2 save %s truncated", field)
	}
	return value, nil
}

func (r *tj2SaveReader) readInts() ([]int, error) {
	count, ok := r.readInt32()
	if !ok || count < 0 || count > tj2SaveMaxIntBlock {
		return nil, errors.New("tj2 save int block invalid")
	}
	values := make([]int, count)
	for i := range values {
		value, ok := r.readInt32()
		if !ok {
			return nil, errors.New("tj2 save int block truncated")
		}
		values[i] = int(value)
	}
	return values, nil
}

func (r *tj2SaveReader) readStrings() ([]string, error) {
	count, ok := r.readInt32()
	if !ok || count < 0 || count > tj2SaveMaxIntBlock {
		return nil, errors.New("tj2 save string block invalid")
	}
	values := make([]string, count)
	for i := range values {
		value, err := r.readString()
		if err != nil {
			return nil, err
		}
		values[i] = value
	}
	return values, nil
}

func (r *tj2SaveReader) readString() (string, error) {
	length, ok := r.read7Bit()
	if !ok {
		return "", errors.New("tj2 save string truncated")
	}
	value, ok := r.readBytes(length)
	if !ok {
		return "", errors.New("tj2 save string truncated")
	}
	return string(value), nil
}

func (r *tj2SaveReader) read7Bit() (int, bool) {
	value := r.reader.SevenBitInt()
	if r.reader.Err() != nil {
		return 0, false
	}
	return value, true
}

// readTj2MessagePages 读取版本 6 追加的分页对白块。
// 截断时返回空页而不是让整份存档失败：旧存档/损坏的尾部只损失分页，
// 仍能恢复到当前显示的文字。
func readTj2MessagePages(r *tj2SaveReader, version int32) ([]string, int) {
	if version < tj2SaveMinMessagePages {
		return nil, 0
	}
	count, ok := r.readInt32()
	if !ok {
		return nil, 0
	}
	if count < 0 || count > tj2SaveMaxMessagePages {
		core.Logf("tj2-save: message page count %d invalid, ignoring", count)
		return nil, 0
	}
	pages := make([]string, 0, count)
	for i := int32(0); i < count; i++ {
		page, err := r.readString()
		if err != nil {
			core.Logf("tj2-save: message pages truncated, ignoring")
			return nil, 0
		}
		pages = append(pages, page)
	}
	page, ok := r.readInt32()
	if !ok {
		return pages, 0
	}
	if page < 0 || int(page) >= len(pages) {
		page = 0
	}
	return pages, int(page)
}
