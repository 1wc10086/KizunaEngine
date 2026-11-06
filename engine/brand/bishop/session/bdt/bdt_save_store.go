package bdt

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unicode/utf8"

	"kizuna/engine/brand/bishop/save"
	bdtvm "kizuna/engine/brand/bishop/vm/bdt"
	"kizuna/engine/core"
	"kizuna/engine/core/binutil"
)

const (
	bdtSaveMagic   = 0x31444254
	bdtSaveVersion = 1
	// bdtSlotCount 覆盖 manin 的 3 页 ×19 行（含自动存档）以及 AFDVD 的
	// 24 槽；只扩大列表，不改变已有槽号。
	bdtSlotCount = 100
)

type BdtSaveStore struct {
	directory string
}

func NewBdtSaveStore(savesDirectory string) *BdtSaveStore {
	return &BdtSaveStore{directory: filepath.Join(savesDirectory, "bdt")}
}

func (s *BdtSaveStore) DirectoryPath() string {
	return s.directory
}

func (s *BdtSaveStore) Write(state *BdtSaveState, slot int) bool {
	if err := os.MkdirAll(s.directory, 0o755); err != nil {
		core.Logf("bdt-save: slot %d write failed: %s", slot, err.Error())
		return false
	}
	var buf bytes.Buffer
	writeBdtSaveState(&buf, state)
	if err := os.WriteFile(s.pathFor(slot), buf.Bytes(), 0o644); err != nil {
		core.Logf("bdt-save: slot %d write failed: %s", slot, err.Error())
		return false
	}
	return true
}

func (s *BdtSaveStore) Read(slot int) *BdtSaveState {
	data, err := os.ReadFile(s.pathFor(slot))
	if err != nil {
		return nil
	}
	state, err := readBdtSaveState(data)
	if err != nil {
		core.Logf("bdt-save: slot %d read failed: %s", slot, err.Error())
		return nil
	}
	return state
}

type BdtSlotInfo struct {
	Index   int
	Path    string
	SavedAt int64
	HasTime bool
	Summary string
	Exists  bool
}

func (s *BdtSaveStore) List() []BdtSlotInfo {
	return s.ListCount(save.SlotCount)
}

// ListCount 允许 manin 的 3 页 ×19 行读到 57 个槽；AFDVD 仍用 List() 的
// 24 槽默认值，不会改变旧界面/旧测试的语义。
func (s *BdtSaveStore) ListCount(count int) []BdtSlotInfo {
	if count <= 0 {
		return nil
	}
	result := make([]BdtSlotInfo, count)
	for index := range result {
		result[index] = s.Inspect(index)
	}
	return result
}

func (s *BdtSaveStore) Inspect(slot int) BdtSlotInfo {
	path := s.pathFor(slot)
	state := s.Read(slot)
	if state == nil {
		return BdtSlotInfo{Index: slot, Path: path}
	}
	return BdtSlotInfo{Index: slot, Path: path, SavedAt: state.SavedAtTicks, HasTime: true, Summary: state.Summary, Exists: true}
}

func (s *BdtSaveStore) pathFor(slot int) string {
	return filepath.Join(s.directory, fmt.Sprintf("slot%03d.bds", slot))
}

func writeBdtSaveState(buf *bytes.Buffer, state *BdtSaveState) {
	writeBdtU32(buf, bdtSaveMagic)
	writeBdtI32(buf, int32(bdtSaveVersion))
	writeBdtString(buf, state.Label)
	writeBdtOptionalMachine(buf, state.Machine)
	writeBdtInts(buf, state.X)
	writeBdtInts(buf, state.Z)
	writeBdtString(buf, state.BackgroundName)
	writeBdtI32(buf, int32(len(state.Sprites)))
	for _, entry := range state.Sprites {
		writeBdtI32(buf, int32(entry.Slot))
		writeBdtString(buf, entry.Name)
		writeBdtI32(buf, int32(entry.X))
		writeBdtI32(buf, int32(entry.Y))
		writeBdtI32(buf, int32(entry.Mode))
	}
	writeBdtI32(buf, int32(state.BgmTrack))
	writeBdtString(buf, state.Speaker)
	writeBdtString(buf, state.Text)
	writeBdtString(buf, state.Summary)
	writeBdtI64(buf, state.SavedAtTicks)
	thumbnail := state.Thumbnail
	if thumbnail == nil {
		thumbnail = []byte{}
	}
	writeBdtI32(buf, int32(len(thumbnail)))
	buf.Write(thumbnail)
	writeBdtI32(buf, int32(state.ThumbnailWidth))
	writeBdtI32(buf, int32(state.ThumbnailHeight))
}

func readBdtSaveState(data []byte) (*BdtSaveState, error) {
	r := &bdtSaveReader{reader: binutil.NewReader(data)}
	if r.readU32() != bdtSaveMagic {
		return nil, errors.New("bdt save magic mismatch")
	}
	version, ok := r.readI32()
	if !ok || version != bdtSaveVersion {
		return nil, fmt.Errorf("bdt save version %d unsupported", version)
	}
	label, ok := r.readString()
	if !ok {
		return nil, errors.New("bdt save label invalid")
	}
	machine, ok := readBdtOptionalMachine(r)
	if !ok {
		return nil, errors.New("bdt save machine invalid")
	}
	x, ok := readBdtInts(r)
	if !ok {
		return nil, errors.New("bdt save x invalid")
	}
	z, ok := readBdtInts(r)
	if !ok {
		return nil, errors.New("bdt save z invalid")
	}
	background, ok := r.readString()
	if !ok {
		return nil, errors.New("bdt save background invalid")
	}
	spriteCount, ok := r.readI32()
	if !ok || spriteCount < 0 || spriteCount > int32(SpriteSlots*64) {
		return nil, errors.New("bdt save sprite count invalid")
	}
	sprites := make([]BdtSpriteEntry, 0, spriteCount)
	for i := int32(0); i < spriteCount; i++ {
		slot, ok := r.readI32()
		if !ok {
			return nil, errors.New("bdt save sprite invalid")
		}
		name, ok := r.readString()
		if !ok {
			return nil, errors.New("bdt save sprite invalid")
		}
		spriteX, ok := r.readI32()
		if !ok {
			return nil, errors.New("bdt save sprite invalid")
		}
		spriteY, ok := r.readI32()
		if !ok {
			return nil, errors.New("bdt save sprite invalid")
		}
		mode, ok := r.readI32()
		if !ok {
			return nil, errors.New("bdt save sprite invalid")
		}
		sprites = append(sprites, BdtSpriteEntry{Slot: int(slot), Name: name, X: int(spriteX), Y: int(spriteY), Mode: int(mode)})
	}
	bgmTrack, ok := r.readI32()
	if !ok {
		return nil, errors.New("bdt save bgm invalid")
	}
	speaker, ok := r.readString()
	if !ok {
		return nil, errors.New("bdt save speaker invalid")
	}
	text, ok := r.readString()
	if !ok {
		return nil, errors.New("bdt save text invalid")
	}
	summary, ok := r.readString()
	if !ok {
		return nil, errors.New("bdt save summary invalid")
	}
	savedAt, ok := r.readI64()
	if !ok {
		return nil, errors.New("bdt save time invalid")
	}
	thumbnailLength, ok := r.readI32()
	if !ok || thumbnailLength < 0 {
		return nil, errors.New("bdt save thumbnail invalid")
	}
	thumbnail, ok := r.readBytes(int(thumbnailLength))
	if !ok {
		return nil, errors.New("bdt save thumbnail invalid")
	}
	thumbnailWidth, ok := r.readI32()
	if !ok {
		return nil, errors.New("bdt save thumbnail invalid")
	}
	thumbnailHeight, ok := r.readI32()
	if !ok {
		return nil, errors.New("bdt save thumbnail invalid")
	}
	return &BdtSaveState{
		Version:         bdtSaveVersion,
		Label:           label,
		Machine:         machine,
		X:               x,
		Z:               z,
		BackgroundName:  background,
		Sprites:         sprites,
		BgmTrack:        int(bgmTrack),
		Speaker:         speaker,
		Text:            text,
		Summary:         summary,
		SavedAtTicks:    savedAt,
		Thumbnail:       thumbnail,
		ThumbnailWidth:  int(thumbnailWidth),
		ThumbnailHeight: int(thumbnailHeight),
	}, nil
}

func writeBdtOptionalMachine(buf *bytes.Buffer, machine *bdtvm.BdtMachineState) {
	buf.WriteByte(boolToBdtByte(machine != nil))
	if machine != nil {
		writeBdtMachine(buf, machine)
	}
}

func readBdtOptionalMachine(r *bdtSaveReader) (*bdtvm.BdtMachineState, bool) {
	present, ok := r.readBool()
	if !ok {
		return nil, false
	}
	if !present {
		return nil, true
	}
	return readBdtMachine(r)
}

func writeBdtMachine(buf *bytes.Buffer, machine *bdtvm.BdtMachineState) {
	writeBdtI32(buf, int32(machine.ProgramCounter))
	writeBdtInts(buf, machine.X)
	writeBdtInts(buf, machine.Z)
	writeBdtInts(buf, machine.Flags)
	writeBdtInts(buf, machine.SystemFlags)
	writeBdtI32(buf, int32(len(machine.CallStack)))
	for _, value := range machine.CallStack {
		writeBdtI32(buf, int32(value))
	}
	writeBdtI32(buf, int32(machine.Stop))
}

func readBdtMachine(r *bdtSaveReader) (*bdtvm.BdtMachineState, bool) {
	programCounter, ok := r.readI32()
	if !ok {
		return nil, false
	}
	x, ok := readBdtInts(r)
	if !ok {
		return nil, false
	}
	z, ok := readBdtInts(r)
	if !ok {
		return nil, false
	}
	flags, ok := readBdtInts(r)
	if !ok {
		return nil, false
	}
	systemFlags, ok := readBdtInts(r)
	if !ok {
		return nil, false
	}
	callCount, ok := r.readI32()
	if !ok || callCount < 0 || callCount > 4096 {
		return nil, false
	}
	callStack := make([]int, 0, callCount)
	for i := int32(0); i < callCount; i++ {
		value, ok := r.readI32()
		if !ok {
			return nil, false
		}
		callStack = append(callStack, int(value))
	}
	stop, ok := r.readI32()
	if !ok {
		return nil, false
	}
	return &bdtvm.BdtMachineState{
		ProgramCounter: int(programCounter),
		X:              x,
		Z:              z,
		Flags:          flags,
		SystemFlags:    systemFlags,
		CallStack:      callStack,
		Stop:           bdtvm.BdtStop(stop),
	}, true
}

func writeBdtInts(buf *bytes.Buffer, values []int32) {
	writeBdtI32(buf, int32(len(values)))
	for _, value := range values {
		writeBdtI32(buf, value)
	}
}

func readBdtInts(r *bdtSaveReader) ([]int32, bool) {
	count, ok := r.readI32()
	if !ok || count < 0 || count > 4096 {
		return nil, false
	}
	values := make([]int32, 0, count)
	for i := int32(0); i < count; i++ {
		value, ok := r.readI32()
		if !ok {
			return nil, false
		}
		values = append(values, value)
	}
	return values, true
}

func writeBdtU32(buf *bytes.Buffer, value uint32) {
	var raw [4]byte
	binary.LittleEndian.PutUint32(raw[:], value)
	buf.Write(raw[:])
}

func writeBdtI32(buf *bytes.Buffer, value int32) {
	var raw [4]byte
	binary.LittleEndian.PutUint32(raw[:], uint32(value))
	buf.Write(raw[:])
}

func writeBdtI64(buf *bytes.Buffer, value int64) {
	var raw [8]byte
	binary.LittleEndian.PutUint64(raw[:], uint64(value))
	buf.Write(raw[:])
}

func writeBdtString(buf *bytes.Buffer, value string) {
	raw := []byte(value)
	writeBdt7Bit(buf, len(raw))
	buf.Write(raw)
}

func writeBdt7Bit(buf *bytes.Buffer, value int) {
	v := uint32(value)
	for v >= 0x80 {
		buf.WriteByte(byte(v | 0x80))
		v >>= 7
	}
	buf.WriteByte(byte(v))
}

func boolToBdtByte(value bool) byte {
	if value {
		return 1
	}
	return 0
}

type bdtSaveReader struct {
	reader *binutil.Reader
}

func (r *bdtSaveReader) readBytes(count int) ([]byte, bool) {
	if count < 0 || count > r.reader.Remaining() {
		return nil, false
	}
	return r.reader.Bytes(count), true
}

func (r *bdtSaveReader) readU32() uint32 {
	if r.reader.Remaining() < 4 {
		return 0
	}
	return r.reader.U32()
}

func (r *bdtSaveReader) readI32() (int32, bool) {
	if r.reader.Remaining() < 4 {
		return 0, false
	}
	return r.reader.I32(), true
}

func (r *bdtSaveReader) readI64() (int64, bool) {
	if r.reader.Remaining() < 8 {
		return 0, false
	}
	low := r.reader.U32()
	high := r.reader.U32()
	return int64(uint64(high)<<32 | uint64(low)), true
}

func (r *bdtSaveReader) readBool() (bool, bool) {
	if r.reader.Remaining() < 1 {
		return false, false
	}
	return r.reader.U8() != 0, true
}

func (r *bdtSaveReader) read7Bit() (int, bool) {
	value := r.reader.SevenBitInt()
	if r.reader.Err() != nil {
		return 0, false
	}
	return value, true
}

func (r *bdtSaveReader) readString() (string, bool) {
	count, ok := r.read7Bit()
	if !ok || count < 0 {
		return "", false
	}
	raw, ok := r.readBytes(count)
	if !ok {
		return "", false
	}
	if !utf8.Valid(raw) {
		return "", false
	}
	return string(raw), true
}
