package gothic

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"time"

	vmgothic "kizuna/engine/brand/bishop/vm/gothic"
	"kizuna/engine/core"
	"kizuna/engine/core/binutil"
)

const (
	GothicSaveSlotCount = 15
	GothicSaveGameLabel = "MAZE"
)

const (
	gothicSaveMagic         = uint32(0x47544843)
	gothicSaveVersion       = int32(3)
	gothicSaveMaxVariables  = 4096
	gothicSaveMaxCallStack  = 4096
	gothicSaveMaxTextLength = 1 << 20
	gothicSaveMaxThumbnail  = 1 << 24
)

type GothicSaveState struct {
	StatementIndex  int32
	Variables       []vmgothic.GothicVariableEntry
	CallStack       []int32
	Branches        []bool
	PageLines       []string
	BackgroundLayer int32
	MessageVisible  bool
	PageBreak       bool
	Screen          []byte
	Background      string
	Speaker         string
	Text            string
	Summary         string
	Bgm             string
	PlayerName      string
	Thumbnail       []byte
	ThumbnailWidth  int32
	ThumbnailHeight int32
}

type GothicSaveSlotInfo struct {
	Index      int
	Path       string
	SavedAt    time.Time
	HasSavedAt bool
	Summary    string
	Exists     bool
}

type GothicSavePreview struct {
	SavedAt         time.Time
	HasSavedAt      bool
	Background      string
	Bgm             string
	HasBgm          bool
	Speaker         string
	Text            string
	Thumbnail       []byte
	ThumbnailWidth  int
	ThumbnailHeight int
	Summary         string
}

type GothicSaveStore struct {
	directory string
}

func NewGothicSaveStore(directory string) *GothicSaveStore {
	return &GothicSaveStore{directory: directory}
}

func (s *GothicSaveStore) DirectoryPath() string {
	return s.directory
}

func (s *GothicSaveStore) ListSlots() []GothicSaveSlotInfo {
	result := make([]GothicSaveSlotInfo, GothicSaveSlotCount)
	for slot := 0; slot < GothicSaveSlotCount; slot++ {
		result[slot] = s.Inspect(slot)
	}
	return result
}

func (s *GothicSaveStore) Exists(slot int) bool {
	path := s.pathFor(slot)
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func (s *GothicSaveStore) Inspect(slot int) GothicSaveSlotInfo {
	path := s.pathFor(slot)
	if path == "" {
		return GothicSaveSlotInfo{Index: slot}
	}
	state := s.Load(slot)
	if state == nil {
		return GothicSaveSlotInfo{Index: slot, Path: path}
	}
	info := GothicSaveSlotInfo{Index: slot, Path: path, Summary: state.Summary, Exists: true}
	if stamp, ok := gothicSaveStamp(path); ok {
		info.SavedAt = stamp
		info.HasSavedAt = true
	} else {
		info.SavedAt = time.Now()
		info.HasSavedAt = true
	}
	return info
}

func (s *GothicSaveStore) PreviewSlot(slot int) *GothicSavePreview {
	path := s.pathFor(slot)
	if path == "" {
		return nil
	}
	state := s.Load(slot)
	if state == nil {
		return nil
	}
	preview := &GothicSavePreview{
		Background:      state.Background,
		Speaker:         state.Speaker,
		Text:            state.Text,
		Thumbnail:       state.Thumbnail,
		ThumbnailWidth:  int(state.ThumbnailWidth),
		ThumbnailHeight: int(state.ThumbnailHeight),
		Summary:         state.Summary,
	}
	if len(state.Bgm) > 0 {
		preview.Bgm = state.Bgm
		preview.HasBgm = true
	}
	if stamp, ok := gothicSaveStamp(path); ok {
		preview.SavedAt = stamp
		preview.HasSavedAt = true
	} else {
		preview.SavedAt = time.Now()
		preview.HasSavedAt = true
	}
	return preview
}

func (s *GothicSaveStore) Save(slot int, state *GothicSaveState) bool {
	path := s.pathFor(slot)
	if path == "" || state == nil {
		return false
	}
	if len(s.directory) > 0 {
		if err := os.MkdirAll(s.directory, 0o755); err != nil {
			core.Logf("gothic-save: slot %d write failed: %v", slot, err)
			return false
		}
	}
	buffer := &bytes.Buffer{}
	writeGothicSaveState(buffer, state)
	if err := os.WriteFile(path, buffer.Bytes(), 0o644); err != nil {
		core.Logf("gothic-save: slot %d write failed: %v", slot, err)
		return false
	}
	return true
}

func (s *GothicSaveStore) Load(slot int) *GothicSaveState {
	path := s.pathFor(slot)
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	state, err := readGothicSaveState(data)
	if err != nil {
		core.Logf("gothic-save: slot %d read failed: %v", slot, err)
		return nil
	}
	return state
}

func (s *GothicSaveStore) Delete(slot int) bool {
	path := s.pathFor(slot)
	if path == "" {
		return false
	}
	if err := os.Remove(path); err != nil {
		return false
	}
	if _, err := os.Stat(path); err == nil {
		return false
	}
	return true
}

func (s *GothicSaveStore) pathFor(slot int) string {
	if slot < 0 || slot >= GothicSaveSlotCount {
		return ""
	}
	return filepath.Join(s.directory, "slot"+gothicPad3(slot)+".gts")
}

func gothicPad3(slot int) string {
	digits := "0123456789"
	return string([]byte{digits[(slot/100)%10], digits[(slot/10)%10], digits[slot%10]})
}

func gothicSaveStamp(path string) (time.Time, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, false
	}
	return info.ModTime(), true
}

func writeGothicSaveState(buffer *bytes.Buffer, state *GothicSaveState) {
	variables := state.Variables
	callStack := state.CallStack
	thumbnail := state.Thumbnail
	writeGothicUint32(buffer, gothicSaveMagic)
	writeGothicInt32(buffer, gothicSaveVersion)
	writeGothicInt32(buffer, state.StatementIndex)
	writeGothicInt32(buffer, int32(len(variables)))
	for _, variable := range variables {
		writeGothicString(buffer, variable.Name)
		writeGothicInt32(buffer, variable.Value)
	}
	writeGothicInt32(buffer, int32(len(callStack)))
	for _, entry := range callStack {
		writeGothicInt32(buffer, entry)
	}
	writeGothicString(buffer, state.Background)
	writeGothicString(buffer, state.Speaker)
	writeGothicString(buffer, state.Text)
	writeGothicString(buffer, state.Summary)
	writeGothicString(buffer, state.Bgm)
	writeGothicInt32(buffer, state.ThumbnailWidth)
	writeGothicInt32(buffer, state.ThumbnailHeight)
	writeGothicInt32(buffer, int32(len(thumbnail)))
	buffer.Write(thumbnail)
	writeGothicString(buffer, state.PlayerName)
	// v2 追加：条件分支栈、当前页文本、图层/可见性。
	// 追加在文件末尾，v1 存档仍可按旧字段读完（缺的字段走默认值）。
	writeGothicInt32(buffer, int32(len(state.Branches)))
	for _, branch := range state.Branches {
		if branch {
			buffer.WriteByte(1)
		} else {
			buffer.WriteByte(0)
		}
	}
	writeGothicInt32(buffer, int32(len(state.PageLines)))
	for _, line := range state.PageLines {
		writeGothicString(buffer, line)
	}
	writeGothicInt32(buffer, state.BackgroundLayer)
	if state.MessageVisible {
		writeGothicInt32(buffer, 1)
	} else {
		writeGothicInt32(buffer, 0)
	}
	if state.PageBreak {
		writeGothicInt32(buffer, 1)
	} else {
		writeGothicInt32(buffer, 0)
	}
	writeGothicInt32(buffer, int32(len(state.Screen)))
	buffer.Write(state.Screen)
}

func writeGothicUint32(buffer *bytes.Buffer, value uint32) {
	var raw [4]byte
	binary.LittleEndian.PutUint32(raw[:], value)
	buffer.Write(raw[:])
}

func writeGothicInt32(buffer *bytes.Buffer, value int32) {
	var raw [4]byte
	binary.LittleEndian.PutUint32(raw[:], uint32(value))
	buffer.Write(raw[:])
}

func writeGothicString(buffer *bytes.Buffer, value string) {
	data := []byte(value)
	writeGothic7Bit(buffer, len(data))
	buffer.Write(data)
}

func writeGothic7Bit(buffer *bytes.Buffer, value int) {
	v := uint32(value)
	for v >= 0x80 {
		buffer.WriteByte(byte(v&0x7F) | 0x80)
		v >>= 7
	}
	buffer.WriteByte(byte(v))
}

type gothicSaveReader struct {
	reader *binutil.Reader
	err    bool
}

func readGothicSaveState(data []byte) (*GothicSaveState, error) {
	reader := &gothicSaveReader{reader: binutil.NewReader(data)}
	if reader.readUint32() != gothicSaveMagic {
		return nil, errGothicSave("gothic save magic mismatch")
	}
	version := reader.readInt32()
	if version < 1 || version > gothicSaveVersion {
		return nil, errGothicSave("gothic save version unsupported")
	}
	statementIndex := reader.readInt32()
	variables := reader.readVariables()
	callStack := reader.readInts(gothicSaveMaxCallStack)
	background := reader.readText()
	speaker := reader.readText()
	text := reader.readText()
	summary := reader.readText()
	bgm := reader.readText()
	width := reader.readInt32()
	height := reader.readInt32()
	thumbnail := reader.readThumbnail()
	playerName := reader.readTrailingText()
	if reader.err {
		return nil, errGothicSave("gothic save truncated")
	}
	state := &GothicSaveState{
		StatementIndex:  statementIndex,
		Variables:       variables,
		CallStack:       callStack,
		Background:      background,
		Speaker:         speaker,
		Text:            text,
		Summary:         summary,
		Bgm:             bgm,
		PlayerName:      playerName,
		Thumbnail:       thumbnail,
		ThumbnailWidth:  width,
		ThumbnailHeight: height,
		MessageVisible:  true,
	}
	if version >= 2 && reader.reader.Remaining() > 0 {
		state.Branches = reader.readBools(gothicSaveMaxCallStack)
		state.PageLines = reader.readStrings()
		state.BackgroundLayer = reader.readInt32()
		state.MessageVisible = reader.readInt32() != 0
		state.PageBreak = reader.readInt32() != 0
		if version >= 3 && reader.reader.Remaining() > 0 {
			length := reader.readInt32()
			if length > 0 && int(length) <= reader.reader.Remaining() {
				state.Screen = reader.reader.Bytes(int(length))
			}
		}
		if reader.err {
			// v2 尾部损坏时仍允许读到旧字段，避免整个槽位不可用。
			reader.err = false
		}
	}
	return state, nil
}

func (r *gothicSaveReader) readUint32() uint32 {
	if r.reader.Remaining() < 4 {
		r.err = true
		return 0
	}
	return r.reader.U32()
}

func (r *gothicSaveReader) readInt32() int32 {
	return int32(r.readUint32())
}

func (r *gothicSaveReader) readVariables() []vmgothic.GothicVariableEntry {
	count := r.readInt32()
	if r.err || count < 0 || count > gothicSaveMaxVariables {
		r.err = true
		return nil
	}
	variables := make([]vmgothic.GothicVariableEntry, 0, count)
	for index := int32(0); index < count; index++ {
		name := r.readText()
		value := r.readInt32()
		if r.err {
			return nil
		}
		variables = append(variables, vmgothic.GothicVariableEntry{Name: name, Value: value})
	}
	return variables
}

func (r *gothicSaveReader) readInts(limit int) []int32 {
	count := r.readInt32()
	if r.err || count < 0 || count > int32(limit) {
		r.err = true
		return nil
	}
	values := make([]int32, count)
	for index := range values {
		values[index] = r.readInt32()
	}
	return values
}

func (r *gothicSaveReader) readBools(limit int) []bool {
	count := r.readInt32()
	if r.err || count < 0 || count > int32(limit) {
		r.err = true
		return nil
	}
	values := make([]bool, count)
	for index := range values {
		if r.reader.Remaining() < 1 {
			r.err = true
			return nil
		}
		values[index] = r.reader.U8() != 0
	}
	return values
}

func (r *gothicSaveReader) readStrings() []string {
	count := r.readInt32()
	if r.err || count < 0 || count > 4096 {
		r.err = true
		return nil
	}
	values := make([]string, 0, count)
	for index := int32(0); index < count; index++ {
		values = append(values, r.readText())
	}
	return values
}

func (r *gothicSaveReader) readText() string {
	length := r.read7Bit()
	if r.err || length < 0 || length > gothicSaveMaxTextLength {
		r.err = true
		return ""
	}
	if length > r.reader.Remaining() {
		r.err = true
		return ""
	}
	return string(r.reader.Bytes(length))
}

func (r *gothicSaveReader) readTrailingText() string {
	if r.err || r.reader.Remaining() <= 0 {
		return ""
	}
	value := r.readText()
	if r.err {
		r.err = false
		return ""
	}
	return value
}

func (r *gothicSaveReader) readThumbnail() []byte {
	length := r.readInt32()
	if r.err || length < 0 || length > gothicSaveMaxThumbnail {
		r.err = true
		return nil
	}
	if int(length) > r.reader.Remaining() {
		r.err = true
		return nil
	}
	return r.reader.Bytes(int(length))
}

func (r *gothicSaveReader) read7Bit() int {
	value := r.reader.SevenBitInt()
	if r.reader.Err() != nil {
		r.err = true
		return 0
	}
	return value
}

type gothicSaveError struct {
	message string
}

func errGothicSave(message string) *gothicSaveError {
	return &gothicSaveError{message: message}
}

func (e *gothicSaveError) Error() string {
	return e.message
}
