package save

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"time"

	"kizuna/engine/brand/bishop/vm"
	"kizuna/engine/core/binutil"
)

// SlotCount 是旧品牌（Legacy/TJ2/Gothic/BDT）沿用的 24 槽布局。
// Bishop Modern 的存档画面是 10 页 x 28 格，改用 NewSaveStoreWithSlots 指定
// GridSlotCount；把 24 固定成默认值可以避免为了一个品牌的界面改坏其它品牌。
const SlotCount = 24

// Bishop Modern 的 [data screen] 槽位布局：10 页 x 28 格 = 280 个手动槽。
// 快速存档/自动存档放在网格之外的两个内部槽位，不显示在存档画面里。
const (
	GridSlotCount   = 280
	QuickSlot       = GridSlotCount
	AutoSlot        = GridSlotCount + 1
	ModernSlotCount = GridSlotCount + 2
)

const saveMagic = "KIZUNA-SAVE1"

var saveMagicBytes = []byte(saveMagic)

const (
	maxSaveArrayBytes  = 1 << 20
	maxSaveLayerCount  = 4096
	maxSaveStringBytes = 1 << 28
)

const (
	dotNetTicksPerSecond = int64(10000000)
	unixToDotNetSeconds  = int64(62135596800)
)

var errInvalidSave = errors.New("invalid save data")

type SaveSlotInfo struct {
	Index      int
	Path       string
	SavedAt    time.Time
	HasSavedAt bool
	Summary    string
	Exists     bool
}

type SaveStore struct {
	directory string
	slotCount int
}

func NewSaveStore(directory string) *SaveStore {
	return NewSaveStoreWithSlots(directory, SlotCount)
}

// NewSaveStoreWithSlots 让品牌按自己的存档画面决定槽位数。
func NewSaveStoreWithSlots(directory string, slotCount int) *SaveStore {
	_ = os.MkdirAll(directory, 0o755)
	if slotCount <= 0 {
		slotCount = SlotCount
	}
	return &SaveStore{directory: directory, slotCount: slotCount}
}

func (s *SaveStore) DirectoryPath() string {
	return s.directory
}

func (s *SaveStore) List() []SaveSlotInfo {
	result := make([]SaveSlotInfo, s.slotCount)
	for index := range result {
		result[index] = s.Inspect(index)
	}
	return result
}

func (s *SaveStore) Inspect(index int) SaveSlotInfo {
	path := s.pathFor(index)
	data, err := os.ReadFile(path)
	if err != nil {
		return SaveSlotInfo{Index: index, Path: path}
	}
	reader := &saveReader{reader: binutil.NewReader(data)}
	if reader.readMagic() != saveMagic {
		return SaveSlotInfo{Index: index, Path: path}
	}
	version, ok := reader.readInt32()
	if !ok {
		return SaveSlotInfo{Index: index, Path: path}
	}
	ticks, ok := reader.readInt64()
	if !ok {
		return SaveSlotInfo{Index: index, Path: path}
	}
	name, ok := reader.readString()
	if !ok {
		return SaveSlotInfo{Index: index, Path: path}
	}
	return SaveSlotInfo{Index: index, Path: path, SavedAt: ticksToTime(ticks), HasSavedAt: true, Summary: name, Exists: version > 0}
}

func (s *SaveStore) Write(data *SaveData, index int) bool {
	var buf bytes.Buffer
	buf.Write(saveMagicBytes)
	writeInt32(&buf, int32(CurrentVersion))
	writeInt64(&buf, timeToTicks(data.SavedAt))
	writeSaveString(&buf, data.GameName)
	writeSaveString(&buf, data.Summary)
	writeSaveString(&buf, data.Speaker)
	writeSaveString(&buf, data.Text)
	bgm := ""
	if data.Bgm != nil {
		bgm = *data.Bgm
	}
	writeSaveString(&buf, bgm)
	machine := data.Machine
	writeInt32(&buf, int32(machine.ProgramCounter))
	writeInt32(&buf, int32(machine.FrameBase))
	writeInt32(&buf, int32(machine.CallDepth))
	writeBool(&buf, machine.Deferred)
	writeBool(&buf, machine.DeferredIsCall)
	writeInt32(&buf, int32(machine.DeferredTarget))
	writeBool(&buf, machine.Halted)
	writeInt32s(&buf, machine.Frame)
	writeInt32s(&buf, machine.Registers)
	writeInt32s(&buf, machine.Variables)
	writeInt32s(&buf, machine.Properties)
	writeInt32s(&buf, machine.CallReturnAddresses)
	writeInt32s(&buf, machine.CallFrameBases)
	writeInt32(&buf, int32(len(machine.Scalars)))
	for _, scalar := range machine.Scalars {
		writeFloat64(&buf, scalar)
	}
	writeSaveLayers(&buf, data.Layers)
	writeInt32(&buf, int32(data.ThumbnailWidth))
	writeInt32(&buf, int32(data.ThumbnailHeight))
	writeInt32(&buf, int32(len(data.Thumbnail)))
	buf.Write(data.Thumbnail)
	// 版本 5 扩展：JSON 便于跨版本增删字段，且不影响前面的二进制布局。
	// 旧版本 4 的存档不会被重写；新档统一写成 5。
	extension := saveExtension{
		ScriptDB:             data.ScriptDB,
		PrivateData:          data.PrivateData,
		CommonData:           data.CommonData,
		UnlockedAchievements: data.UnlockedAchievements,
		MapSource:            data.MapSource,
		WordTemp:             data.WordTemp,
		HasWordMemory:        data.HasWordMemory,
		WordActive:           data.WordActive,
		ChoiceOptions:        data.ChoiceOptions,
	}
	if data.HasWordMemory {
		extension.WordSaved = make([][]int, 0, 3)
		for _, page := range data.WordSaved {
			extension.WordSaved = append(extension.WordSaved, append([]int(nil), page[:]...))
		}
	}
	for _, button := range data.MapButtons {
		extension.MapButtons = append(extension.MapButtons, saveMapButton{
			Image: button.Image, X: button.X, Y: button.Y, Value: button.Value,
			Frames: button.Frames, Width: button.Width, Height: button.Height,
			Layer: button.Layer, Disabled: button.Disabled,
			HoverFrame: button.HoverFrame, HasHoverFrame: button.HasHoverFrame,
		})
	}
	if blob, err := json.Marshal(extension); err == nil {
		writeInt32(&buf, int32(len(blob)))
		buf.Write(blob)
	} else {
		writeInt32(&buf, 0)
	}
	if err := os.WriteFile(s.pathFor(index), buf.Bytes(), 0o644); err != nil {
		return false
	}
	return true
}

func (s *SaveStore) Read(index int) *SaveData {
	path := s.pathFor(index)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	reader := &saveReader{reader: binutil.NewReader(data)}
	if reader.readMagic() != saveMagic {
		return nil
	}
	version, ok := reader.readInt32()
	if !ok || version < 1 || version > CurrentVersion {
		return nil
	}
	ticks, ok := reader.readInt64()
	if !ok {
		return nil
	}
	gameName, ok := reader.readString()
	if !ok {
		return nil
	}
	summary, ok := reader.readString()
	if !ok {
		return nil
	}
	speaker, ok := reader.readString()
	if !ok {
		return nil
	}
	text, ok := reader.readString()
	if !ok {
		return nil
	}
	bgmName, ok := reader.readString()
	if !ok {
		return nil
	}
	machine := vm.MachineState{}
	if machine.ProgramCounter, ok = reader.readInt(); !ok {
		return nil
	}
	if machine.FrameBase, ok = reader.readInt(); !ok {
		return nil
	}
	if machine.CallDepth, ok = reader.readInt(); !ok {
		return nil
	}
	if machine.Deferred, ok = reader.readBool(); !ok {
		return nil
	}
	if machine.DeferredIsCall, ok = reader.readBool(); !ok {
		return nil
	}
	if machine.DeferredTarget, ok = reader.readInt(); !ok {
		return nil
	}
	if machine.Halted, ok = reader.readBool(); !ok {
		return nil
	}
	if machine.Frame, ok = reader.readInt32s(); !ok {
		return nil
	}
	if machine.Registers, ok = reader.readInt32s(); !ok {
		return nil
	}
	if machine.Variables, ok = reader.readInt32s(); !ok {
		return nil
	}
	if machine.Properties, ok = reader.readInt32s(); !ok {
		return nil
	}
	if machine.CallReturnAddresses, ok = reader.readInt32s(); !ok {
		return nil
	}
	if machine.CallFrameBases, ok = reader.readInt32s(); !ok {
		return nil
	}
	if machine.Scalars, ok = reader.readFloat64s(); !ok {
		return nil
	}
	layers, ok := reader.readSaveLayers()
	if !ok {
		return nil
	}
	width, ok := reader.readInt()
	if !ok {
		return nil
	}
	height, ok := reader.readInt()
	if !ok {
		return nil
	}
	thumbnail, ok := reader.readThumbnail()
	if !ok {
		return nil
	}
	var bgm *string
	if len(bgmName) > 0 {
		value := bgmName
		bgm = &value
	}
	extension := saveExtension{}
	if version >= saveExtensionVersion {
		if blob, ok := reader.readBlob(); ok && len(blob) > 0 {
			_ = json.Unmarshal(blob, &extension)
		}
	}
	result := &SaveData{
		Version:              int(version),
		GameName:             gameName,
		SavedAt:              ticksToTime(ticks),
		Machine:              machine,
		Layers:               layers,
		Speaker:              speaker,
		Text:                 text,
		Bgm:                  bgm,
		Thumbnail:            thumbnail,
		ThumbnailWidth:       width,
		ThumbnailHeight:      height,
		Summary:              summary,
		ScriptDB:             extension.ScriptDB,
		PrivateData:          extension.PrivateData,
		CommonData:           extension.CommonData,
		UnlockedAchievements: extension.UnlockedAchievements,
		MapSource:            extension.MapSource,
		MapButtons:           readMapButtons(extension.MapButtons),
		WordTemp:             extension.WordTemp,
		HasWordMemory:        extension.HasWordMemory,
		WordActive:           extension.WordActive,
		ChoiceOptions:        extension.ChoiceOptions,
	}
	if extension.HasWordMemory {
		for page := 0; page < 3 && page < len(extension.WordSaved); page++ {
			for slot := 0; slot < 7 && slot < len(extension.WordSaved[page]); slot++ {
				result.WordSaved[page][slot] = extension.WordSaved[page][slot]
			}
		}
	}
	return result
}

func readMapButtons(buttons []saveMapButton) []SaveMapButton {
	if len(buttons) == 0 {
		return nil
	}
	out := make([]SaveMapButton, 0, len(buttons))
	for _, button := range buttons {
		out = append(out, SaveMapButton{
			Image: button.Image, X: button.X, Y: button.Y, Value: button.Value,
			Frames: button.Frames, Width: button.Width, Height: button.Height,
			Layer: button.Layer, Disabled: button.Disabled,
			HoverFrame: button.HoverFrame, HasHoverFrame: button.HasHoverFrame,
		})
	}
	return out
}

// saveExtension 是版本 5 追加的 JSON 段。
type saveExtension struct {
	ScriptDB             map[string]map[string]map[string]int32 `json:"scriptDb,omitempty"`
	PrivateData          map[string]any                         `json:"privateData,omitempty"`
	CommonData           map[string]any                         `json:"commonData,omitempty"`
	UnlockedAchievements map[string]bool                        `json:"unlockedAchievements,omitempty"`
	MapSource            string                                 `json:"mapSource,omitempty"`
	MapButtons           []saveMapButton                        `json:"mapButtons,omitempty"`
	WordTemp             []int                                  `json:"wordTemp,omitempty"`
	WordSaved            [][]int                                `json:"wordSaved,omitempty"`
	HasWordMemory        bool                                   `json:"hasWordMemory,omitempty"`
	WordActive           bool                                   `json:"wordActive,omitempty"`
	ChoiceOptions        []SaveChoiceOption                     `json:"choiceOptions,omitempty"`
}

// saveMapButton 是 SaveMapButton 的 JSON 形态；单独定义是为了让存储层
// 不依赖 session 的按钮结构，字段变化时旧档仍能按零值读取。
type saveMapButton struct {
	Image         string `json:"image"`
	X             int    `json:"x"`
	Y             int    `json:"y"`
	Value         int    `json:"value"`
	Frames        int    `json:"frames"`
	Width         int    `json:"width"`
	Height        int    `json:"height"`
	Layer         int    `json:"layer"`
	Disabled      bool   `json:"disabled,omitempty"`
	HoverFrame    int    `json:"hoverFrame,omitempty"`
	HasHoverFrame bool   `json:"hasHoverFrame,omitempty"`
}

func (s *SaveStore) Delete(index int) {
	_ = os.Remove(s.pathFor(index))
}

func (s *SaveStore) pathFor(index int) string {
	return filepath.Join(s.directory, fmt.Sprintf("slot%03d.dat", index))
}

func timeToTicks(t time.Time) int64 {
	u := t.UTC()
	return u.Unix()*dotNetTicksPerSecond + unixToDotNetSeconds*dotNetTicksPerSecond + int64(u.Nanosecond()/100)
}

func ticksToTime(ticks int64) time.Time {
	seconds := ticks/dotNetTicksPerSecond - unixToDotNetSeconds
	nanos := (ticks % dotNetTicksPerSecond) * 100
	return time.Unix(seconds, nanos).UTC().Local()
}

func writeInt32(buf *bytes.Buffer, value int32) {
	var scratch [4]byte
	binary.LittleEndian.PutUint32(scratch[:], uint32(value))
	buf.Write(scratch[:])
}

func writeInt64(buf *bytes.Buffer, value int64) {
	var scratch [8]byte
	binary.LittleEndian.PutUint64(scratch[:], uint64(value))
	buf.Write(scratch[:])
}

func writeBool(buf *bytes.Buffer, value bool) {
	if value {
		buf.WriteByte(1)
	} else {
		buf.WriteByte(0)
	}
}

func writeFloat64(buf *bytes.Buffer, value float64) {
	writeInt64(buf, int64(math.Float64bits(value)))
}

func write7Bit(buf *bytes.Buffer, value int) {
	v := uint32(value)
	for v >= 0x80 {
		buf.WriteByte(byte(v | 0x80))
		v >>= 7
	}
	buf.WriteByte(byte(v))
}

func writeSaveString(buf *bytes.Buffer, value string) {
	data := []byte(value)
	write7Bit(buf, len(data))
	buf.Write(data)
}

func writeInt32s(buf *bytes.Buffer, values []int32) {
	writeInt32(buf, int32(len(values)))
	for _, value := range values {
		writeInt32(buf, value)
	}
}

func writeSaveLayers(buf *bytes.Buffer, layers []SaveLayer) {
	writeInt32(buf, int32(len(layers)))
	for _, layer := range layers {
		writeInt32(buf, int32(layer.Slot))
		writeSaveString(buf, layer.Image)
		writeFloat64(buf, layer.Alpha)
		writeBool(buf, layer.Character)
		writeInt32(buf, int32(len(layer.Artworks)))
		for _, artwork := range layer.Artworks {
			writeSaveString(buf, artwork.Image)
			writeInt32(buf, int32(artwork.X))
			writeInt32(buf, int32(artwork.Y))
			writeInt32(buf, int32(artwork.Frames))
			writeInt32(buf, int32(artwork.AnimatedPart))
		}
	}
}

type saveReader struct {
	reader *binutil.Reader
	err    error
}

func (r *saveReader) readMagic() string {
	first := r.reader.Bytes(1)
	if r.reader.Err() != nil {
		r.err = r.reader.Err()
		return ""
	}
	if int(first[0]) == len(saveMagicBytes) {
		legacy := r.reader.Bytes(len(saveMagicBytes))
		if r.reader.Err() != nil {
			r.err = r.reader.Err()
			return ""
		}
		return string(legacy)
	}
	rest := r.reader.Bytes(len(saveMagicBytes) - 1)
	if r.reader.Err() != nil {
		r.err = r.reader.Err()
		return ""
	}
	buffer := make([]byte, 0, len(saveMagicBytes))
	buffer = append(buffer, first...)
	buffer = append(buffer, rest...)
	return string(buffer)
}

func (r *saveReader) readInt32() (int32, bool) {
	if r.err != nil {
		return 0, false
	}
	if r.reader.Remaining() < 4 {
		r.err = io.ErrUnexpectedEOF
		return 0, false
	}
	return r.reader.I32(), true
}

func (r *saveReader) readInt() (int, bool) {
	value, ok := r.readInt32()
	return int(value), ok
}

func (r *saveReader) readInt64() (int64, bool) {
	if r.err != nil {
		return 0, false
	}
	if r.reader.Remaining() < 8 {
		r.err = io.ErrUnexpectedEOF
		return 0, false
	}
	low := r.reader.U32()
	high := r.reader.U32()
	return int64(uint64(high)<<32 | uint64(low)), true
}

func (r *saveReader) readBool() (bool, bool) {
	if r.err != nil {
		return false, false
	}
	if r.reader.Remaining() < 1 {
		r.err = io.ErrUnexpectedEOF
		return false, false
	}
	return r.reader.U8() != 0, true
}

func (r *saveReader) readFloat64() (float64, bool) {
	bits, ok := r.readInt64()
	if !ok {
		return 0, false
	}
	return math.Float64frombits(uint64(bits)), true
}

func (r *saveReader) read7Bit() (int, bool) {
	if r.err != nil {
		return 0, false
	}
	value := r.reader.SevenBitInt()
	if r.reader.Err() != nil {
		r.err = errInvalidSave
		return 0, false
	}
	return value, true
}

func (r *saveReader) readString() (string, bool) {
	length, ok := r.read7Bit()
	if !ok || length < 0 || length > maxSaveStringBytes {
		r.err = errInvalidSave
		return "", false
	}
	value := r.reader.Bytes(length)
	if r.reader.Err() != nil {
		r.err = r.reader.Err()
		return "", false
	}
	return string(value), true
}

func (r *saveReader) readInt32s() ([]int32, bool) {
	count, ok := r.readInt32()
	if !ok || count < 0 || count > maxSaveArrayBytes {
		r.err = errInvalidSave
		return nil, false
	}
	values := make([]int32, count)
	for i := range values {
		value, ok := r.readInt32()
		if !ok {
			return nil, false
		}
		values[i] = value
	}
	return values, true
}

func (r *saveReader) readFloat64s() ([]float64, bool) {
	count, ok := r.readInt32()
	if !ok || count < 0 || count > maxSaveArrayBytes {
		r.err = errInvalidSave
		return nil, false
	}
	values := make([]float64, count)
	for i := range values {
		value, ok := r.readFloat64()
		if !ok {
			return nil, false
		}
		values[i] = value
	}
	return values, true
}

func (r *saveReader) readSaveLayers() ([]SaveLayer, bool) {
	count, ok := r.readInt32()
	if !ok || count < 0 || count > maxSaveLayerCount {
		r.err = errInvalidSave
		return nil, false
	}
	layers := make([]SaveLayer, 0, count)
	for i := int32(0); i < count; i++ {
		slot, ok := r.readInt()
		if !ok {
			return nil, false
		}
		image, ok := r.readString()
		if !ok {
			return nil, false
		}
		alpha, ok := r.readFloat64()
		if !ok {
			return nil, false
		}
		character, ok := r.readBool()
		if !ok {
			return nil, false
		}
		artworkCount, ok := r.readInt32()
		if !ok || artworkCount < 0 || artworkCount > maxSaveLayerCount {
			r.err = errInvalidSave
			return nil, false
		}
		artworks := make([]SaveArtwork, 0, artworkCount)
		for j := int32(0); j < artworkCount; j++ {
			artwork := SaveArtwork{}
			if artwork.Image, ok = r.readString(); !ok {
				return nil, false
			}
			if artwork.X, ok = r.readInt(); !ok {
				return nil, false
			}
			if artwork.Y, ok = r.readInt(); !ok {
				return nil, false
			}
			if artwork.Frames, ok = r.readInt(); !ok {
				return nil, false
			}
			if artwork.AnimatedPart, ok = r.readInt(); !ok {
				return nil, false
			}
			artworks = append(artworks, artwork)
		}
		layers = append(layers, SaveLayer{Slot: slot, Image: image, Alpha: alpha, Character: character, Artworks: artworks})
	}
	return layers, true
}

// readBlob 读取长度前缀的二进制块，用于版本 5 的 JSON 扩展。
func (r *saveReader) readBlob() ([]byte, bool) {
	length, ok := r.readInt()
	if !ok || length < 0 || length > maxSaveArrayBytes*16 {
		return nil, false
	}
	if length == 0 {
		return nil, true
	}
	data := r.reader.Bytes(length)
	if data == nil {
		return nil, false
	}
	return data, true
}

func (r *saveReader) readThumbnail() ([]byte, bool) {
	length, ok := r.readInt32()
	if !ok || length < 0 {
		r.err = errInvalidSave
		return nil, false
	}
	count := int(length)
	if count > r.reader.Remaining() {
		count = r.reader.Remaining()
	}
	return r.reader.Bytes(count), true
}
