package legacy

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"kizuna/engine/brand/bishop/save"
	"kizuna/engine/brand/bishop/vm/scf"
	"kizuna/engine/core"
	"kizuna/engine/core/binutil"
)

const (
	legacySaveMagic   = uint32(0x4C534346)
	legacySaveVersion = int32(3)
)

const (
	dotNetTicksPerSecond = int64(10000000)
	unixToDotNetSeconds  = int64(62135596800)
)

type LegacySlotInfo struct {
	Index      int
	Path       string
	SavedAt    time.Time
	HasSavedAt bool
	Summary    string
	Exists     bool
}

type LegacySaveStore struct {
	directory string
}

func NewLegacySaveStore(savesDirectory string) *LegacySaveStore {
	return &LegacySaveStore{directory: filepath.Join(savesDirectory, "legacy")}
}

func (s *LegacySaveStore) DirectoryPath() string {
	return s.directory
}

func (s *LegacySaveStore) Write(state *LegacySaveState, slot int) bool {
	if err := os.MkdirAll(s.directory, 0o755); err != nil {
		core.Logf("legacy-save: slot %d write failed: %v", slot, err)
		return false
	}
	var buffer bytes.Buffer
	writeLegacyState(&buffer, state)
	if err := os.WriteFile(s.pathFor(slot), buffer.Bytes(), 0o644); err != nil {
		core.Logf("legacy-save: slot %d write failed: %v", slot, err)
		return false
	}
	return true
}

func (s *LegacySaveStore) Read(slot int) *LegacySaveState {
	data, err := os.ReadFile(s.pathFor(slot))
	if err != nil {
		return nil
	}
	state, err := readLegacyState(data)
	if err != nil {
		core.Logf("legacy-save: slot %d read failed: %v", slot, err)
		return nil
	}
	return state
}

func (s *LegacySaveStore) List() []LegacySlotInfo {
	return s.ListSlots(save.SlotCount)
}

func (s *LegacySaveStore) ListSlots(slots int) []LegacySlotInfo {
	result := make([]LegacySlotInfo, slots)
	for index := range result {
		result[index] = s.Inspect(index)
	}
	return result
}

func (s *LegacySaveStore) Inspect(slot int) LegacySlotInfo {
	path := s.pathFor(slot)
	state := s.Read(slot)
	if state == nil {
		return LegacySlotInfo{Index: slot, Path: path}
	}
	return LegacySlotInfo{
		Index:      slot,
		Path:       path,
		SavedAt:    TicksToTime(state.SavedAtTicks),
		HasSavedAt: true,
		Summary:    state.Summary,
		Exists:     true,
	}
}

func (s *LegacySaveStore) pathFor(slot int) string {
	return filepath.Join(s.directory, fmt.Sprintf("slot%03d.lgs", slot))
}

func TicksToTime(ticks int64) time.Time {
	seconds := ticks/dotNetTicksPerSecond - unixToDotNetSeconds
	nanos := (ticks % dotNetTicksPerSecond) * 100
	return time.Unix(seconds, nanos).UTC()
}

func TimeToTicks(value time.Time) int64 {
	utc := value.UTC()
	return (utc.Unix()-unixToDotNetSeconds)*dotNetTicksPerSecond + int64(utc.Nanosecond())/100
}

func writeLegacyState(buffer *bytes.Buffer, state *LegacySaveState) {
	writeLegacyU32(buffer, legacySaveMagic)
	writeLegacyI32(buffer, legacySaveVersion)
	writeLegacyI32(buffer, int32(state.ScenarioNumber))
	writeLegacyOptionalMachine(buffer, state.Machine)
	writeLegacyString(buffer, state.BackgroundName)
	writeLegacyBool(buffer, state.MenuVisible)
	writeLegacyI32(buffer, int32(len(state.Standees)))
	for _, entry := range state.Standees {
		writeLegacyI32(buffer, int32(entry.Index))
		writeLegacyString(buffer, entry.Name)
	}
	writeLegacyString(buffer, state.FaceName)
	writeLegacyI32(buffer, int32(state.BgmTrack))
	writeLegacyString(buffer, state.Speaker)
	writeLegacyString(buffer, state.Text)
	writeLegacyString(buffer, state.Summary)
	writeLegacyI64(buffer, state.SavedAtTicks)
	thumbnail := state.Thumbnail
	if thumbnail == nil {
		thumbnail = []byte{}
	}
	writeLegacyI32(buffer, int32(len(thumbnail)))
	buffer.Write(thumbnail)
	writeLegacyI32(buffer, int32(state.ThumbnailWidth))
	writeLegacyI32(buffer, int32(state.ThumbnailHeight))
	writeLegacyBool(buffer, state.CommandActive)
}

func readLegacyState(data []byte) (*LegacySaveState, error) {
	reader := &legacyReader{reader: binutil.NewReader(data)}
	if reader.readU32() != legacySaveMagic {
		return nil, fmt.Errorf("legacy save magic mismatch")
	}
	version := reader.readI32()
	// v1 没有末尾的 CommandActive；读取时兼容旧档，写入统一 v2。
	if version != 1 && version != 2 && version != legacySaveVersion {
		return nil, fmt.Errorf("legacy save version %d unsupported", version)
	}
	scenario := reader.readI32()
	machine, err := readLegacyOptionalMachine(reader, 0, version)
	if err != nil {
		return nil, err
	}
	background := reader.readString()
	menuVisible := reader.readBool()
	standeeCount := reader.readI32()
	if standeeCount < 0 || standeeCount > LegacyStandeeCapacity {
		return nil, fmt.Errorf("legacy save standee count invalid")
	}
	standees := make([]LegacyStandeeEntry, 0, standeeCount)
	for index := int32(0); index < standeeCount; index++ {
		entryIndex := reader.readI32()
		name := reader.readString()
		standees = append(standees, LegacyStandeeEntry{Index: int(entryIndex), Name: name})
	}
	face := reader.readString()
	bgmTrack := reader.readI32()
	speaker := reader.readString()
	text := reader.readString()
	summary := reader.readString()
	savedAt := reader.readI64()
	thumbnailLength := reader.readI32()
	if thumbnailLength < 0 {
		return nil, fmt.Errorf("legacy save thumbnail invalid")
	}
	thumbnail := reader.readBytes(int(thumbnailLength))
	thumbnailWidth := reader.readI32()
	thumbnailHeight := reader.readI32()
	commandActive := false
	if version >= 2 {
		commandActive = reader.readBool()
	}
	if err := reader.err; err != nil {
		return nil, err
	}
	return &LegacySaveState{
		Version:         int(version),
		ScenarioNumber:  int(scenario),
		Machine:         machine,
		BackgroundName:  background,
		MenuVisible:     menuVisible,
		Standees:        standees,
		FaceName:        face,
		BgmTrack:        int(bgmTrack),
		Speaker:         speaker,
		Text:            text,
		Summary:         summary,
		SavedAtTicks:    savedAt,
		Thumbnail:       thumbnail,
		ThumbnailWidth:  int(thumbnailWidth),
		ThumbnailHeight: int(thumbnailHeight),
		CommandActive:   commandActive,
	}, nil
}

func writeLegacyOptionalMachine(buffer *bytes.Buffer, machine *scf.ScfMachineState) {
	writeLegacyBool(buffer, machine != nil)
	if machine != nil {
		writeLegacyMachine(buffer, machine)
	}
}

func readLegacyOptionalMachine(reader *legacyReader, depth int, version int32) (*scf.ScfMachineState, error) {
	if !reader.readBool() {
		return nil, reader.err
	}
	return readLegacyMachine(reader, depth, version)
}

func writeLegacyMachine(buffer *bytes.Buffer, machine *scf.ScfMachineState) {
	writeLegacyI32(buffer, int32(machine.ProgramCounter))
	writeLegacyI32(buffer, int32(machine.ScenarioNumber))
	writeLegacyBytes(buffer, machine.Flags)
	writeLegacyBytes(buffer, machine.Source)
	writeLegacyStrings(buffer, machine.NameSlots)
	writeLegacyI32(buffer, int32(machine.CallDepth))
	writeLegacyI32(buffer, int32(len(machine.Frames)))
	for index := range machine.Frames {
		writeLegacyMachine(buffer, &machine.Frames[index])
	}
	writeLegacyBool(buffer, machine.Halted)
	writeLegacyBool(buffer, machine.Faulted)
	writeLegacyI32(buffer, int32(machine.CondDepth))
	writeLegacyInts(buffer, machine.CondStack)
	writeLegacyInts(buffer, machine.LoopSlots)
	writeLegacyInts(buffer, machine.ElseSlots)
	writeLegacyI32(buffer, int32(machine.Latch))
	writeLegacyI32(buffer, int32(machine.MessageOffset))
}

func readLegacyMachine(reader *legacyReader, depth int, version int32) (*scf.ScfMachineState, error) {
	if depth > 8 {
		return nil, fmt.Errorf("legacy save frame depth invalid")
	}
	programCounter := reader.readI32()
	scenario := reader.readI32()
	flags := reader.readByteBlock()
	source := reader.readByteBlock()
	names := reader.readStringBlock()
	callDepth := reader.readI32()
	frameCount := reader.readI32()
	if frameCount < 0 || frameCount > 8 {
		return nil, fmt.Errorf("legacy save frame count invalid")
	}
	frames := make([]scf.ScfMachineState, 0, frameCount)
	for index := int32(0); index < frameCount; index++ {
		frame, err := readLegacyMachine(reader, depth+1, version)
		if err != nil {
			return nil, err
		}
		frames = append(frames, *frame)
	}
	halted := reader.readBool()
	faulted := reader.readBool()
	condDepth := reader.readI32()
	condStack := reader.readIntBlock()
	loopSlots := reader.readIntBlock()
	var elseSlots []int
	if version >= 3 {
		elseSlots = reader.readIntBlock()
	}
	latch := reader.readI32()
	messageOffset := reader.readI32()
	if reader.err != nil {
		return nil, reader.err
	}
	return &scf.ScfMachineState{
		ProgramCounter: int(programCounter),
		ScenarioNumber: int(scenario),
		Flags:          flags,
		Source:         source,
		NameSlots:      names,
		CallDepth:      int(callDepth),
		Frames:         frames,
		Halted:         halted,
		Faulted:        faulted,
		CondDepth:      int(condDepth),
		CondStack:      condStack,
		LoopSlots:      loopSlots,
		ElseSlots:      elseSlots,
		Latch:          int(latch),
		MessageOffset:  int(messageOffset),
	}, nil
}

func writeLegacyBytes(buffer *bytes.Buffer, value []byte) {
	if value == nil {
		value = []byte{}
	}
	writeLegacyI32(buffer, int32(len(value)))
	buffer.Write(value)
}

func writeLegacyInts(buffer *bytes.Buffer, values []int) {
	writeLegacyI32(buffer, int32(len(values)))
	for _, value := range values {
		writeLegacyI32(buffer, int32(value))
	}
}

func writeLegacyStrings(buffer *bytes.Buffer, values []string) {
	writeLegacyI32(buffer, int32(len(values)))
	for _, value := range values {
		writeLegacyString(buffer, value)
	}
}

func writeLegacyU32(buffer *bytes.Buffer, value uint32) {
	var raw [4]byte
	binary.LittleEndian.PutUint32(raw[:], value)
	buffer.Write(raw[:])
}

func writeLegacyI32(buffer *bytes.Buffer, value int32) {
	var raw [4]byte
	binary.LittleEndian.PutUint32(raw[:], uint32(value))
	buffer.Write(raw[:])
}

func writeLegacyI64(buffer *bytes.Buffer, value int64) {
	var raw [8]byte
	binary.LittleEndian.PutUint64(raw[:], uint64(value))
	buffer.Write(raw[:])
}

func writeLegacyBool(buffer *bytes.Buffer, value bool) {
	if value {
		buffer.WriteByte(1)
	} else {
		buffer.WriteByte(0)
	}
}

func writeLegacyString(buffer *bytes.Buffer, value string) {
	raw := []byte(value)
	writeLegacy7BitEncodedInt(buffer, len(raw))
	buffer.Write(raw)
}

func writeLegacy7BitEncodedInt(buffer *bytes.Buffer, value int) {
	unsigned := uint32(value)
	for unsigned >= 0x80 {
		buffer.WriteByte(byte(unsigned | 0x80))
		unsigned >>= 7
	}
	buffer.WriteByte(byte(unsigned))
}

type legacyReader struct {
	reader *binutil.Reader
	err    error
}

func (r *legacyReader) readU32() uint32 {
	if r.err != nil {
		return 0
	}
	if r.reader.Remaining() < 4 {
		r.err = fmt.Errorf("legacy save ends inside a u32 at offset %d", r.reader.Pos())
		return 0
	}
	return r.reader.U32()
}

func (r *legacyReader) readI32() int32 {
	return int32(r.readU32())
}

func (r *legacyReader) readI64() int64 {
	if r.err != nil {
		return 0
	}
	if r.reader.Remaining() < 8 {
		r.err = fmt.Errorf("legacy save ends inside an i64 at offset %d", r.reader.Pos())
		return 0
	}
	low := r.reader.U32()
	high := r.reader.U32()
	return int64(uint64(high)<<32 | uint64(low))
}

func (r *legacyReader) readBool() bool {
	if r.err != nil {
		return false
	}
	if r.reader.Remaining() < 1 {
		r.err = fmt.Errorf("legacy save ends inside a bool at offset %d", r.reader.Pos())
		return false
	}
	return r.reader.U8() != 0
}

func (r *legacyReader) readBytes(length int) []byte {
	if r.err != nil {
		return nil
	}
	if length < 0 || length > r.reader.Remaining() {
		r.err = fmt.Errorf("legacy save ends inside a %d-byte block at offset %d", length, r.reader.Pos())
		return nil
	}
	if length == 0 {
		return nil
	}
	return r.reader.Bytes(length)
}

func (r *legacyReader) readByteBlock() []byte {
	length := r.readI32()
	if r.err != nil {
		return nil
	}
	if length < 0 {
		r.err = fmt.Errorf("legacy save byte block invalid")
		return nil
	}
	return r.readBytes(int(length))
}

func (r *legacyReader) readIntBlock() []int {
	count := r.readI32()
	if r.err != nil {
		return nil
	}
	if count < 0 || count > 4096 {
		r.err = fmt.Errorf("legacy save int block invalid")
		return nil
	}
	values := make([]int, count)
	for index := range values {
		values[index] = int(r.readI32())
	}
	return values
}

func (r *legacyReader) readStringBlock() []string {
	count := r.readI32()
	if r.err != nil {
		return nil
	}
	if count < 0 || count > 4096 {
		r.err = fmt.Errorf("legacy save string block invalid")
		return nil
	}
	values := make([]string, count)
	for index := range values {
		values[index] = r.readString()
	}
	return values
}

func (r *legacyReader) readString() string {
	if r.err != nil {
		return ""
	}
	length := r.read7BitEncodedInt()
	if r.err != nil {
		return ""
	}
	raw := r.readBytes(length)
	if r.err != nil {
		return ""
	}
	return string(raw)
}

func (r *legacyReader) read7BitEncodedInt() int {
	start := r.reader.Pos()
	value := r.reader.SevenBitInt()
	if r.reader.Err() != nil {
		if r.reader.Pos()-start >= 5 {
			r.err = fmt.Errorf("legacy save string length invalid")
		} else {
			r.err = fmt.Errorf("legacy save ends inside a string length at offset %d", r.reader.Pos())
		}
		return 0
	}
	return value
}
