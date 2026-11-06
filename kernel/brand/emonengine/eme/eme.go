package eme

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"kizuna/kernel/core"
	"kizuna/kernel/core/textcodec"
)

const (
	FeatureID       = "emonengine.eme"
	FeatureName     = "EmonEngine-Eme"
	FormatName      = "eme"
	DocumentVersion = "1"
	EncodingName    = "cp932"

	subTypeScript = 3

	bomUTF8  = "utf-8-bom"
	encUTF8  = "utf8"
	encCP932 = "cp932"
)

type codec struct{}

func init() {
	core.Register(codec{})
}

func (codec) ID() string          { return FeatureID }
func (codec) Name() string        { return FeatureName }
func (codec) Target() core.Target { return core.TargetFile }

func (codec) Handlers() []core.Handler {
	return []core.Handler{
		{Action: "decode", Label: "解码", Suffixes: []string{".eme"}, OutputSuffix: ".eme.json"},
		{Action: "encode", Label: "编码", Suffixes: []string{".eme.json"}, OutputSuffix: ""},
	}
}

func (codec) DetectFormat(path string, head []byte) (string, bool) {
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".eme.json") {
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", false
		}
		feat, err := core.ProbeFeature(raw)
		if err != nil || feat != FeatureID {
			return "", false
		}
		return FormatName, true
	}
	if strings.HasSuffix(lower, ".eme") {
		if len(head) >= magicSize && bytes.Equal(head[:magicSize], emeMagic) {
			return FormatName, true
		}
	}
	return "", false
}

func (c codec) Run(action string, in string, out string, opt core.Options) (core.Result, error) {
	switch action {
	case "decode":
		return core.RunCodecDecode(c, in, out)
	case "encode":
		return c.runEncode(in, out)
	default:
		return core.Result{}, fmt.Errorf("eme: unknown action %q", action)
	}
}

func (codec) UnitText(doc *core.Document, id string) (string, bool) {
	return core.DefaultUnitText(doc, id)
}

func (codec) SetUnitText(doc *core.Document, id string, text string) bool {
	return core.DefaultSetUnitText(doc, id, text)
}

func (c codec) runEncode(in string, out string) (core.Result, error) {
	start := time.Now()
	raw, err := os.ReadFile(in)
	if err != nil {
		return core.Result{}, fmt.Errorf("eme: read document %q: %w", in, err)
	}
	var doc core.Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return core.Result{}, fmt.Errorf("eme: parse document %q: %w", in, err)
	}
	if doc.Feature != FeatureID {
		return core.Result{}, fmt.Errorf("%s: %w", FeatureID, core.ErrNotMine)
	}
	srcPath := in
	if strings.HasSuffix(strings.ToLower(in), ".json") {
		srcPath = in[:len(in)-len(".json")]
	}
	abs, err := filepath.Abs(srcPath)
	if err != nil {
		return core.Result{}, fmt.Errorf("eme: resolve source archive for %q: %w", in, err)
	}
	if doc.Meta.Values == nil {
		doc.Meta.Values = map[string]string{}
	}
	doc.Meta.Values["archive"] = abs
	data, err := c.Encode(&doc)
	if err != nil {
		return core.Result{}, err
	}
	if err := core.AtomicWrite(out, data); err != nil {
		return core.Result{}, err
	}
	return core.Result{
		InputPath:  in,
		OutputPath: out,
		BytesIn:    int64(len(raw)),
		BytesOut:   int64(len(data)),
		ElapsedMs:  time.Since(start).Milliseconds(),
		Summary:    fmt.Sprintf("encode %d units", len(doc.Units)),
		Details: map[string]string{
			"feature": doc.Feature,
			"format":  doc.Format,
		},
	}, nil
}

type entryMeta struct {
	Name         string `json:"name"`
	NameHex      string `json:"nameHex,omitempty"`
	SubType      int    `json:"subType"`
	FrameSize    int    `json:"frameSize,omitempty"`
	RawPos       int    `json:"rawPos,omitempty"`
	Flags        uint32 `json:"flags,omitempty"`
	Tail         string `json:"tail,omitempty"`
	Size         int    `json:"size"`
	UnpackedSize int    `json:"unpackedSize,omitempty"`
	Offset       int    `json:"offset"`
	TextHash     string `json:"textHash,omitempty"`
	Packed       uint32 `json:"packed,omitempty"`
}

type archiveMeta struct {
	Size    int64       `json:"size"`
	Key     string      `json:"keyHex"`
	Entries []entryMeta `json:"entries"`
}

func (codec) Decode(data []byte, name string) (*core.Document, error) {
	key, entries, err := parseArchive(data)
	if err != nil {
		return nil, err
	}
	doc := &core.Document{
		Feature:  FeatureID,
		Format:   FormatName,
		Version:  DocumentVersion,
		Encoding: EncodingName,
		Meta: core.Meta{
			Values: map[string]string{
				"archive":     name,
				"archiveSize": fmt.Sprintf("%d", len(data)),
				"keyHex":      fmt.Sprintf("%x", key),
				"entryCount":  fmt.Sprintf("%d", len(entries)),
			},
			Lists: map[string][]string{},
		},
	}
	entryTable := archiveMeta{Size: int64(len(data)), Key: fmt.Sprintf("%x", key)}
	sections := []core.Section{}
	units := []core.Unit{}
	for _, e := range entries {
		em := entryMeta{
			Name:         e.Name,
			NameHex:      fmt.Sprintf("%x", e.RawName),
			SubType:      e.SubType,
			FrameSize:    e.FrameSize,
			RawPos:       e.RawPos,
			Flags:        e.Flags,
			Tail:         tailHex(e.Tail),
			Size:         e.Size,
			UnpackedSize: e.UnpackedSize,
			Offset:       e.Offset,
		}
		entryTable.Entries = append(entryTable.Entries, em)
		if e.SubType != subTypeScript {
			continue
		}
		if e.Offset+12+e.Size > len(data) {
			return nil, fmt.Errorf("eme: script entry %q extends past archive", e.Name)
		}
		hdr := make([]byte, 12)
		copy(hdr, data[e.Offset:e.Offset+12])
		packed, part2 := decryptScriptHeaderBytes(hdr, 0, key)
		em.Packed = packed
		bodyLimit := e.Size
		if part2 != 0 && part2 < uint32(e.UnpackedSize) {
			bodyLimit = e.Size + int(packed)
		}
		if e.Offset+12+bodyLimit > len(data) {
			return nil, fmt.Errorf("eme: script entry %q body extends past archive", e.Name)
		}
		raw := make([]byte, bodyLimit)
		copy(raw, data[e.Offset+12:e.Offset+12+bodyLimit])
		script := decodeScriptBody(raw, e.FrameSize, e.FrameInitPos(), e.UnpackedSize, part2, packed)
		payload, encName := decodeScriptText(script)
		text := textcodec.Decode(encName, payload)
		parts := splitScriptLines(text)
		section := core.Section{
			Kind:     "lines",
			Name:     e.Name,
			Encoding: encName,
		}
		eols := make([]string, len(parts))
		for i, p := range parts {
			id := unitIDForLine(e.Name, i)
			kind := "raw"
			if isScriptTranslatable(p.body) {
				kind = "text"
			}
			units = append(units, core.Unit{ID: id, Kind: kind, Text: p.body})
			section.IDs = append(section.IDs, id)
			eols[i] = p.eol
		}
		section.EOL = core.SummarizeEOL(eols)
		if len(eols) > 0 {
			section.Trailing = eols[len(eols)-1]
		}
		sections = append(sections, section)
		doc.Meta.Lists["eol:"+e.Name] = eols
		doc.Meta.Values["encoding:"+e.Name] = encName
		em.TextHash = textHash(text)
		em.UnpackedSize = len(script)
		entryTable.Entries[len(entryTable.Entries)-1] = em
	}
	tableJSON, err := json.Marshal(entryTable)
	if err != nil {
		return nil, fmt.Errorf("eme: marshal entry table: %w", err)
	}
	doc.Meta.Values["entries"] = string(tableJSON)
	doc.Sections = sections
	doc.Units = units
	return doc, nil
}

func textHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", sum[:16])
}

func (codec) Encode(doc *core.Document) ([]byte, error) {
	if doc == nil {
		return nil, errors.New("eme: nil document")
	}
	if doc.Feature != FeatureID {
		return nil, core.ErrNotMine
	}
	if doc.Meta.Values == nil {
		return nil, errors.New("eme: missing metadata")
	}
	srcPath := doc.Meta.Values["archive"]
	if srcPath == "" {
		return nil, errors.New("eme: missing archive source path")
	}
	srcData, err := os.ReadFile(srcPath)
	if err != nil {
		return nil, fmt.Errorf("eme: read source archive %q: %w", srcPath, err)
	}
	srcKeyStr := doc.Meta.Values["keyHex"]
	if srcKeyStr == "" {
		return nil, errors.New("eme: missing source archive key")
	}
	srcKey, err := hexDecode(srcKeyStr)
	if err != nil {
		return nil, fmt.Errorf("eme: decode source key: %w", err)
	}
	if len(srcKey) != keySize {
		return nil, fmt.Errorf("eme: source key length %d, want %d", len(srcKey), keySize)
	}
	tableStr := doc.Meta.Values["entries"]
	if tableStr == "" {
		return nil, errors.New("eme: missing entries table")
	}
	var table archiveMeta
	if err := json.Unmarshal([]byte(tableStr), &table); err != nil {
		return nil, fmt.Errorf("eme: decode entries table: %w", err)
	}
	srcEntries, err := parseStoredEntries(srcData)
	if err != nil {
		return nil, err
	}
	if err := checkEntriesCovered(table.Entries, srcEntries); err != nil {
		return nil, err
	}
	if err := checkSectionsMatch(doc.Sections, table.Entries); err != nil {
		return nil, err
	}
	unitText := make(map[string]string, len(doc.Units))
	for _, u := range doc.Units {
		unitText[u.ID] = u.Text
	}
	current := make(map[string]string, len(table.Entries))
	for _, em := range table.Entries {
		if em.SubType != subTypeScript {
			continue
		}
		sec := core.FindSection(doc.Sections, "lines", em.Name)
		if sec == nil {
			return nil, fmt.Errorf("eme: entry %q: no lines section for entry", em.Name)
		}
		text, err := assembleText(*sec, unitText, doc.Meta.Lists["eol:"+em.Name])
		if err != nil {
			return nil, fmt.Errorf("eme: entry %q: %w", em.Name, err)
		}
		current[em.Name] = text
	}
	changed := false
	for name, text := range current {
		stored, ok := storedText(srcData, srcKey, srcEntries[name])
		if !ok || stored != text {
			changed = true
			break
		}
	}
	if !changed {
		return srcData, nil
	}
	spans, err := dataSpans(srcEntries, table.Entries, len(srcData))
	if err != nil {
		return nil, err
	}
	blobs := make(map[string][]byte, len(table.Entries))
	updatedTable := archiveMeta{Size: table.Size, Key: table.Key}
	for _, em := range table.Entries {
		se, ok := srcEntries[em.Name]
		if !ok {
			return nil, fmt.Errorf("eme: entry %q: missing from source archive", em.Name)
		}
		newEM := em
		var blob []byte
		text, isScript := current[em.Name]
		stored, ok := "", false
		if isScript {
			stored, ok = storedText(srcData, srcKey, se)
		}
		switch {
		case isScript && ok && stored == text:
			blob, err = copyStoredScript(srcData, srcKey, se)
			if err != nil {
				return nil, fmt.Errorf("eme: entry %q: %w", em.Name, err)
			}
			newEM.Size = se.Size
			newEM.UnpackedSize = se.UnpackedSize
			newEM.Packed = storedPacked(srcData, srcKey, se)
		case isScript:
			encoded, encUsed, err := encodeScriptText(text)
			if err != nil {
				return nil, fmt.Errorf("eme: entry %q: encode script text as cp932: %w", em.Name, err)
			}
			rawBytes := encoded
			if encUsed == bomUTF8 {
				rawBytes = append([]byte{0xEF, 0xBB, 0xBF}, encoded...)
			}
			blob, err = encodeScriptEntry(rawBytes, em, srcKey)
			if err != nil {
				return nil, fmt.Errorf("eme: entry %q: %w", em.Name, err)
			}
			newEM.Size = len(blob) - 12
			newEM.UnpackedSize = len(rawBytes)
			newEM.Packed = 0
			if sec := core.FindSection(doc.Sections, "lines", em.Name); sec != nil {
				sec.Encoding = encUsed
			}
			if doc.Meta.Values == nil {
				doc.Meta.Values = map[string]string{}
			}
			doc.Meta.Values["encoding:"+em.Name] = encUsed
		default:
			blob, err = copyStoredSpan(srcData, se, spans[em.Name])
			if err != nil {
				return nil, fmt.Errorf("eme: entry %q: %w", em.Name, err)
			}
		}
		blobs[em.Name] = blob
		updatedTable.Entries = append(updatedTable.Entries, newEM)
	}
	return buildOutput(srcKey, updatedTable, blobs), nil
}

func parseStoredEntries(srcData []byte) (map[string]entry, error) {
	_, entries, err := parseArchive(srcData)
	if err != nil {
		return nil, fmt.Errorf("eme: parse source archive index: %w", err)
	}
	out := make(map[string]entry, len(entries))
	for _, e := range entries {
		if _, dup := out[e.Name]; !dup {
			out[e.Name] = e
		}
	}
	return out, nil
}

func checkEntriesCovered(table []entryMeta, src map[string]entry) error {
	inTable := make(map[string]bool, len(table))
	for _, em := range table {
		inTable[em.Name] = true
	}
	for name := range src {
		if !inTable[name] {
			return fmt.Errorf("eme: entry %q: entry table does not cover source archive", name)
		}
	}
	return nil
}

func checkSectionsMatch(sections []core.Section, entries []entryMeta) error {
	want := make(map[string]bool, len(entries))
	for _, em := range entries {
		if em.SubType == subTypeScript {
			want[em.Name] = true
		}
	}
	for _, sec := range sections {
		if !want[sec.Name] {
			return fmt.Errorf("eme: entry %q: unsupported structural change", sec.Name)
		}
		delete(want, sec.Name)
	}
	return nil
}

func storedScriptSpan(srcData, srcKey []byte, se entry) (offset, total int, packed, part2 uint32, ok bool) {
	if se.Offset < 0 || se.Offset+12 > len(srcData) || se.Size < 0 {
		return 0, 0, 0, 0, false
	}
	hdr := make([]byte, 12)
	copy(hdr, srcData[se.Offset:se.Offset+12])
	packed, part2 = decryptScriptHeaderBytes(hdr, 0, srcKey)
	total = 12 + se.Size
	if part2 != 0 && part2 < uint32(se.UnpackedSize) {
		total += int(packed)
	}
	if total < 0 || se.Offset+total > len(srcData) {
		return 0, 0, 0, 0, false
	}
	return se.Offset, total, packed, part2, true
}

func storedPacked(srcData, srcKey []byte, se entry) uint32 {
	_, _, packed, _, ok := storedScriptSpan(srcData, srcKey, se)
	if !ok {
		return 0
	}
	return packed
}

func storedText(srcData, srcKey []byte, se entry) (string, bool) {
	offset, total, packed, part2, ok := storedScriptSpan(srcData, srcKey, se)
	if !ok {
		return "", false
	}
	raw := make([]byte, total-12)
	copy(raw, srcData[offset+12:offset+total])
	script := decodeScriptBody(raw, se.FrameSize, se.FrameInitPos(), se.UnpackedSize, part2, packed)
	payload, encName := decodeScriptText(script)
	return textcodec.Decode(encName, payload), true
}

func copyStoredScript(srcData, srcKey []byte, se entry) ([]byte, error) {
	offset, total, _, _, ok := storedScriptSpan(srcData, srcKey, se)
	if !ok {
		return nil, fmt.Errorf("offset %d span out of range", se.Offset)
	}
	out := make([]byte, total)
	copy(out, srcData[offset:offset+total])
	return out, nil
}

func copyStoredSpan(srcData []byte, se entry, span int) ([]byte, error) {
	if se.Offset < 0 || span < 0 || se.Offset+span > len(srcData) {
		return nil, fmt.Errorf("offset %d span %d out of range", se.Offset, span)
	}
	out := make([]byte, span)
	copy(out, srcData[se.Offset:se.Offset+span])
	return out, nil
}

func dataSpans(src map[string]entry, order []entryMeta, dataLen int) (map[string]int, error) {
	dataEnd := dataLen - 4 - len(order)*recordSize - keySize
	if dataEnd < magicSize || dataEnd > dataLen {
		return nil, fmt.Errorf("eme: corrupt source layout (data end %d)", dataEnd)
	}
	byOff := make([]entry, 0, len(src))
	for _, e := range src {
		byOff = append(byOff, e)
	}
	sort.Slice(byOff, func(i, j int) bool { return byOff[i].Offset < byOff[j].Offset })
	spans := make(map[string]int, len(src))
	for i, e := range byOff {
		end := dataEnd
		if i+1 < len(byOff) {
			end = byOff[i+1].Offset
		}
		span := end - e.Offset
		if e.Offset < magicSize || span < 0 || e.Offset+span > dataLen {
			return nil, fmt.Errorf("eme: entry %q: offset %d span %d out of range", e.Name, e.Offset, span)
		}
		spans[e.Name] = span
	}
	return spans, nil
}

func encodeScriptText(text string) ([]byte, string, error) {
	if encoded, err := textcodec.Encode(encCP932, text); err == nil {
		return encoded, encCP932, nil
	}
	encoded, err := textcodec.Encode(encUTF8, text)
	if err != nil {
		return nil, "", fmt.Errorf("encode script text: %w", err)
	}
	return encoded, bomUTF8, nil
}

func decodeScriptBody(raw []byte, frameSize, frameInitPos, unpackedSize int, part2, packed uint32) []byte {
	var script []byte
	if frameSize == 0 {
		script = make([]byte, unpackedSize)
		limit := unpackedSize
		if limit > len(raw) {
			limit = len(raw)
		}
		copy(script, raw[:limit])
		return script
	}
	if part2 != 0 && part2 < uint32(unpackedSize) && packed <= uint32(len(raw)) {
		prefixSize := int(packed)
		part1Size := unpackedSize - int(part2)
		part1 := lzssDecompress(raw[prefixSize:prefixSize+len(raw)-prefixSize], part1Size, frameSize, frameInitPos)
		part2Decoded := lzssDecompress(raw[:prefixSize], int(part2), frameSize, frameInitPos)
		script = append(script, part1...)
		script = append(script, part2Decoded...)
	} else {
		script = lzssDecompress(raw, unpackedSize, frameSize, frameInitPos)
	}
	return script
}

func decodeScriptText(script []byte) ([]byte, string) {
	if len(script) >= 3 && script[0] == 0xEF && script[1] == 0xBB && script[2] == 0xBF {
		return script[3:], bomUTF8
	}
	enc := textcodec.DetectTextEncoding(script)
	return script, enc
}

func tailHex(tail [8]byte) string {
	if tail == [8]byte{} {
		return ""
	}
	return fmt.Sprintf("%x", tail[:])
}

func tailBytes(s string) [8]byte {
	var tail [8]byte
	if s == "" {
		return tail
	}
	raw, err := hexDecode(s)
	if err != nil {
		return tail
	}
	copy(tail[:], raw)
	return tail
}

func assembleText(sec core.Section, unitText map[string]string, eols []string) (string, error) {
	var b strings.Builder
	for i, id := range sec.IDs {
		text, ok := unitText[id]
		if !ok {
			return "", fmt.Errorf("missing unit %s", id)
		}
		b.WriteString(text)
		eol := ""
		if i < len(eols) {
			eol = eols[i]
		} else if i == len(sec.IDs)-1 {
			eol = sec.Trailing
		} else {
			eol = sec.EOL
		}
		bs, err := eolBytes(eol)
		if err != nil {
			return "", err
		}
		b.Write(bs)
	}
	return b.String(), nil
}

func encodeScriptEntry(rawBytes []byte, em entryMeta, key []byte) ([]byte, error) {
	body := lzssCompress(rawBytes, em.FrameSize, frameInitFromMeta(em))
	hdr := make([]byte, 12)
	binary.LittleEndian.PutUint32(hdr[0:4], 0)
	binary.LittleEndian.PutUint32(hdr[4:8], 0)
	binary.LittleEndian.PutUint32(hdr[8:12], 0)
	encrypt(hdr, key)
	out := make([]byte, 0, 12+len(body))
	out = append(out, hdr...)
	out = append(out, body...)
	return out, nil
}

func frameInitFromMeta(em entryMeta) int {
	if em.FrameSize == 0 {
		return 0
	}
	return (em.FrameSize - em.RawPos) % em.FrameSize
}

type placedEntry struct {
	em   entryMeta
	data []byte
}

func buildOutput(srcKey []byte, table archiveMeta, blobs map[string][]byte) []byte {
	all := make([]placedEntry, 0, len(table.Entries))
	for _, em := range table.Entries {
		blob, ok := blobs[em.Name]
		if !ok {
			blob = nil
		}
		all = append(all, placedEntry{em: em, data: blob})
	}
	dataRegion := make([]byte, 0)
	for i := range all {
		p := all[i]
		p.em.Offset = len(dataRegion) + magicSize
		all[i] = p
		dataRegion = append(dataRegion, p.data...)
	}
	tail := make([]byte, 4)
	binary.LittleEndian.PutUint32(tail, uint32(len(table.Entries)))
	key := make([]byte, keySize)
	copy(key, srcKey)
	idxBlock := make([]byte, len(table.Entries)*recordSize)
	for i, p := range all {
		block := make([]byte, recordSize)
		copy(block, storedNameBytes(p.em))
		binary.LittleEndian.PutUint16(block[0x40:0x42], uint16(p.em.FrameSize))
		binary.LittleEndian.PutUint16(block[0x42:0x44], uint16(p.em.RawPos))
		binary.LittleEndian.PutUint32(block[0x44:0x48], p.em.Flags)
		binary.LittleEndian.PutUint32(block[0x48:0x4C], uint32(p.em.SubType))
		binary.LittleEndian.PutUint32(block[0x4C:0x50], uint32(p.em.Size))
		binary.LittleEndian.PutUint32(block[0x50:0x54], uint32(p.em.UnpackedSize))
		binary.LittleEndian.PutUint32(block[0x54:0x58], uint32(p.em.Offset))
		tb := tailBytes(p.em.Tail)
		copy(block[0x58:0x60], tb[:])
		encrypt(block, key)
		copy(idxBlock[i*recordSize:(i+1)*recordSize], block)
	}
	out := make([]byte, 0, len(dataRegion)+len(idxBlock)+keySize+4+magicSize)
	out = append(out, emeMagic...)
	out = append(out, dataRegion...)
	out = append(out, key...)
	out = append(out, idxBlock...)
	out = append(out, tail...)
	return out
}

func storedNameBytes(em entryMeta) []byte {
	out := make([]byte, nameSize)
	if em.NameHex != "" {
		if raw, err := hexDecode(em.NameHex); err == nil {
			copy(out, raw)
			return out
		}
	}
	if raw, err := textcodec.Encode(encCP932, em.Name); err == nil {
		copy(out, raw)
		return out
	}
	copy(out, em.Name)
	return out
}

func unitIDForLine(entry string, idx int) string {
	return fmt.Sprintf("%s/%d", entry, idx)
}

func hexDecode(s string) ([]byte, error) {
	if len(s)%2 != 0 {
		return nil, fmt.Errorf("odd hex length %d", len(s))
	}
	out := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		hi, ok1 := unhex(s[i])
		lo, ok2 := unhex(s[i+1])
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("invalid hex at %d", i)
		}
		out[i/2] = hi<<4 | lo
	}
	return out, nil
}

func unhex(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}
