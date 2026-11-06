package scriptbin

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strings"
	"unicode/utf8"

	"kizuna/kernel/core"
	"kizuna/kernel/core/textcodec"
)

var TableNames = [6]string{"labels", "names", "variables", "properties", "characters", "messages"}

var TableKinds = [6]string{"label", "name", "variable", "property", "character", "message"}

const (
	ModernHeaderWords  = 42
	ClassicHeaderWords = 37
	AncientHeaderWords = 36
	MinHeaderSize      = 0x94
	SignatureSize      = 16
)

type Kind int

const (
	Modern Kind = iota
	Classic
	Ancient
)

func (k Kind) String() string {
	switch k {
	case Modern:
		return "modern"
	case Classic:
		return "classic"
	default:
		return "ancient"
	}
}

func ParseKind(s string) (Kind, bool) {
	switch s {
	case "modern":
		return Modern, true
	case "classic":
		return Classic, true
	case "ancient":
		return Ancient, true
	default:
		return Classic, false
	}
}

type Layout struct {
	Kind            Kind
	CodeOffsetField int
	Groups          [6]int
	HeaderEnd       int
	HeaderWords     int
}

func LayoutOf(k Kind) Layout {
	switch k {
	case Modern:
		return Layout{Modern, 0x2C, [6]int{0x38, 0x48, 0x58, 0x68, 0x88, 0x98}, 0xA8, ModernHeaderWords}
	case Ancient:
		return Layout{Ancient, 0x24, [6]int{0x30, 0x40, 0x50, 0x60, 0x70, 0x80}, 0x90, AncientHeaderWords}
	default:
		return Layout{Classic, 0x28, [6]int{0x34, 0x44, 0x54, 0x64, 0x74, 0x84}, 0x94, ClassicHeaderWords}
	}
}

func LayoutForWords(n int) (Layout, bool) {
	switch n {
	case ModernHeaderWords:
		return LayoutOf(Modern), true
	case ClassicHeaderWords:
		return LayoutOf(Classic), true
	case AncientHeaderWords:
		return LayoutOf(Ancient), true
	default:
		return Layout{}, false
	}
}

const (
	modernPrefix  = "BSXScript"
	classicPrefix = "BSScript Rev."
	ancientName   = "BSScript"
)

func hasPrefix(b []byte, s string) bool {
	if len(b) < len(s) {
		return false
	}
	for i := 0; i < len(s); i++ {
		if b[i] != s[i] {
			return false
		}
	}
	return true
}

func versionDigits(header []byte, start int) string {
	var sb strings.Builder
	for i := start; i < len(header) && i < SignatureSize; i++ {
		c := header[i]
		if c == 0 {
			break
		}
		if (c >= '0' && c <= '9') || c == '.' {
			sb.WriteByte(c)
		}
	}
	return sb.String()
}

func DetectSignature(data []byte) (string, bool) {
	if len(data) < SignatureSize {
		return "", false
	}
	header := data[:SignatureSize]
	if hasPrefix(header, classicPrefix) {
		return classicPrefix + versionDigits(header, len(classicPrefix)), true
	}
	if hasPrefix(header, modernPrefix) {
		return modernPrefix + " " + versionDigits(header, len(modernPrefix)), true
	}
	if hasPrefix(header, ancientName) {
		return ancientName, true
	}
	return "", false
}

func PreferredKind(sig string) Kind {
	if strings.HasPrefix(sig, ancientName) {
		if strings.HasPrefix(sig, classicPrefix) {
			return Classic
		}
		return Ancient
	}
	if strings.HasPrefix(sig, modernPrefix) {
		if strings.Contains(sig, " 3.") {
			return Modern
		}
		return Classic
	}
	return Modern
}

type Table struct {
	Present      bool
	ListOffset   int
	ListSize     int
	BlockOffset  int
	BlockSize    int
	Count        int
	Encoding     string
	Texts        []string
	LabelCode    []int32
	UTF8Override []bool
}

type Container struct {
	Signature string
	Layout    Layout
	Header    []int32
	SysData   []byte
	Tables    [6]Table
	Code      []byte
	Negated   bool
}

func readInt32(data []byte, off int) int {
	if off < 0 || off+4 > len(data) {
		return 0
	}
	return int(int32(binary.LittleEndian.Uint32(data[off:])))
}

func tableValid(data []byte, listOff, listSize, blockOff, blockSize int) bool {
	if listOff == 0 && listSize == 0 && blockOff == 0 && blockSize == 0 {
		return true
	}
	if listOff < MinHeaderSize || listOff >= len(data) {
		return false
	}
	if blockOff < MinHeaderSize || blockOff >= len(data) {
		return false
	}
	if listSize == 0 || blockSize == 0 {
		return true
	}
	if listSize < 0 || blockSize < 0 {
		return false
	}
	return int64(listOff)+int64(listSize) <= int64(len(data))
}

func tryLayout(data []byte, layout Layout) bool {
	if len(data) < layout.CodeOffsetField+8 || len(data) < MinHeaderSize {
		return false
	}
	codeOff := readInt32(data, layout.CodeOffsetField)
	codeSize := readInt32(data, layout.CodeOffsetField+4)
	if codeOff < MinHeaderSize || codeSize <= 0 || int64(codeOff)+int64(codeSize) > int64(len(data)) {
		return false
	}
	for g := 0; g < 6; g++ {
		base := layout.Groups[g]
		if base+16 > len(data) {
			return false
		}
		if !tableValid(data, readInt32(data, base), readInt32(data, base+4), readInt32(data, base+8), readInt32(data, base+12)) {
			return false
		}
	}
	return true
}

func selectLayout(data []byte, sig string) (Layout, bool) {
	preferred := PreferredKind(sig)
	if tryLayout(data, LayoutOf(preferred)) {
		return LayoutOf(preferred), true
	}
	for _, k := range []Kind{Modern, Classic, Ancient} {
		if k == preferred {
			continue
		}
		if tryLayout(data, LayoutOf(k)) {
			return LayoutOf(k), true
		}
	}
	return Layout{}, false
}

func (c *Container) tableEncoding(block []byte) string {
	if strings.HasPrefix(c.Signature, modernPrefix) {
		if textcodec.IsUTF8Block(block) {
			return "utf8"
		}
		return "utf16le"
	}
	return textcodec.DetectLegacy(block)
}

func roundtripOK(enc string, raw []byte, text string) bool {
	back, err := textcodec.Encode(enc, text)
	if err != nil {
		return false
	}
	if len(back) != len(raw) {
		return false
	}
	for i := range back {
		if back[i] != raw[i] {
			return false
		}
	}
	return true
}

func decodeSingle(raw []byte, tableEnc string) (string, string, error) {
	if textcodec.HasHighByte(raw) && utf8.Valid(raw) {
		return textcodec.Decode("utf8", raw), "utf8", nil
	}
	text := textcodec.Decode(tableEnc, raw)
	if roundtripOK(tableEnc, raw, text) {
		return text, tableEnc, nil
	}
	for _, enc := range []string{"cp932", "gbk"} {
		if enc == tableEnc {
			continue
		}
		text := textcodec.Decode(enc, raw)
		if roundtripOK(enc, raw, text) {
			return text, enc, nil
		}
	}
	if utf8.Valid(raw) {
		return textcodec.Decode("utf8", raw), "utf8", nil
	}
	return "", "", fmt.Errorf("scriptbin: undecodable %d bytes", len(raw))
}

func findTerminator(block []byte, off int, unit int) int {
	if unit == 2 {
		pos := off
		for pos+1 < len(block) {
			if block[pos] == 0 && block[pos+1] == 0 {
				return pos
			}
			pos += 2
		}
		return len(block)
	}
	end := off
	for end < len(block) && block[end] != 0 {
		end++
	}
	return end
}

func decodeTable(data []byte, listOff, listSize, blockOff, blockSize int, isLabel bool, enc string) (Table, error) {
	t := Table{Encoding: enc}
	if listOff == 0 || blockOff == 0 || listSize <= 0 || blockSize <= 0 {
		return t, nil
	}
	stride := 4
	if isLabel {
		stride = 8
	}
	count := listSize / stride
	if count <= 0 {
		return t, nil
	}
	unit := textcodec.UnitSize(enc)
	block := data[blockOff : blockOff+blockSize]
	t.Present = true
	t.ListOffset = listOff
	t.ListSize = listSize
	t.BlockOffset = blockOff
	t.BlockSize = blockSize
	t.Count = count
	t.Texts = make([]string, count)
	t.UTF8Override = make([]bool, count)
	if isLabel {
		t.LabelCode = make([]int32, count)
	}
	rels := make([]int, count)
	for i := 0; i < count; i++ {
		if isLabel {
			t.LabelCode[i] = int32(readInt32(data, listOff+i*8))
			rels[i] = readInt32(data, listOff+i*8+4)
		} else {
			rels[i] = readInt32(data, listOff+i*4)
		}
		if rels[i] < 0 || rels[i]*unit > len(block) {
			return t, fmt.Errorf("scriptbin: table offset %d out of range", rels[i])
		}
		off := rels[i] * unit
		end := findTerminator(block, off, unit)
		raw := block[off:end]
		if unit == 2 {
			text := textcodec.DecodeUTF16LE(raw)
			if !roundtripOK(enc, raw, text) {
				return t, fmt.Errorf("scriptbin: utf16 string %d does not roundtrip", i)
			}
			t.Texts[i] = text
			continue
		}
		text, used, err := decodeSingle(raw, enc)
		if err != nil {
			return t, fmt.Errorf("scriptbin: string %d: %w", i, err)
		}
		t.Texts[i] = text
		t.UTF8Override[i] = used != enc
	}
	rebuilt, rebuiltRels := buildPool(t.Texts, t.UTF8Override, enc)
	if len(rebuilt) != len(block) {
		return t, fmt.Errorf("scriptbin: rebuilt pool size %d != %d", len(rebuilt), len(block))
	}
	for i := range rebuilt {
		if rebuilt[i] != block[i] {
			return t, fmt.Errorf("scriptbin: rebuilt pool differs at byte %d", i)
		}
	}
	for i := range rels {
		if rebuiltRels[i] != rels[i] {
			return t, fmt.Errorf("scriptbin: rebuilt offset %d != %d at index %d", rebuiltRels[i], rels[i], i)
		}
	}
	return t, nil
}

func buildPool(texts []string, override []bool, enc string) ([]byte, []int) {
	unit := textcodec.UnitSize(enc)
	var out []byte
	rels := make([]int, len(texts))
	for i, text := range texts {
		used := enc
		if override != nil && i < len(override) && override[i] && unit == 1 {
			used = "utf8"
		}
		raw, err := textcodec.Encode(used, text)
		if err != nil {
			raw = []byte{}
		}
		if unit == 2 {
			rels[i] = len(out) / 2
		} else {
			rels[i] = len(out)
		}
		out = append(out, raw...)
		if unit == 2 {
			out = append(out, 0, 0)
		} else {
			out = append(out, 0)
		}
	}
	return out, rels
}

func Decode(data []byte) (*Container, error) {
	if len(data) < MinHeaderSize {
		return nil, fmt.Errorf("scriptbin: file too small")
	}
	raw := data
	negated := false
	sig, ok := DetectSignature(raw)
	if !ok {
		raw = append([]byte(nil), data...)
		core.NegateBytes(raw)
		sig, ok = DetectSignature(raw)
		if !ok {
			return nil, fmt.Errorf("scriptbin: missing script signature")
		}
		negated = true
	}
	layout, ok := selectLayout(raw, sig)
	if !ok {
		return nil, fmt.Errorf("scriptbin: unsupported layout for %q", sig)
	}
	c := &Container{Signature: sig, Layout: layout, Negated: negated}
	c.Header = make([]int32, layout.HeaderWords)
	for i := range c.Header {
		c.Header[i] = int32(readInt32(raw, i*4))
	}
	codeOff := readInt32(raw, layout.CodeOffsetField)
	codeSize := readInt32(raw, layout.CodeOffsetField+4)
	c.Code = append([]byte{}, raw[codeOff:codeOff+codeSize]...)
	if codeOff > layout.HeaderEnd {
		c.SysData = append([]byte{}, raw[layout.HeaderEnd:codeOff]...)
	}
	for i := 0; i < 6; i++ {
		base := layout.Groups[i]
		lo := readInt32(raw, base)
		ls := readInt32(raw, base+4)
		bo := readInt32(raw, base+8)
		bs := readInt32(raw, base+12)
		var enc string
		if lo != 0 && bo != 0 && ls > 0 && bs > 0 {
			enc = c.tableEncoding(raw[bo : bo+bs])
		}
		t, err := decodeTable(raw, lo, ls, bo, bs, i == 0, enc)
		if err != nil {
			return nil, fmt.Errorf("scriptbin: %s: %w", TableNames[i], err)
		}
		c.Tables[i] = t
	}
	return c, nil
}

func align16(n int) int {
	return (n + 15) &^ 15
}

func (c *Container) Encode() ([]byte, error) {
	layout := c.Layout
	header := append([]int32{}, c.Header...)
	if len(header) != layout.HeaderWords {
		return nil, fmt.Errorf("scriptbin: bad header length %d", len(header))
	}
	out := make([]byte, 0, len(c.Code)+1024)
	for _, v := range header {
		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], uint32(v))
		out = append(out, b[:]...)
	}
	out = append(out, c.SysData...)
	codeOff := align16(len(out))
	for len(out) < codeOff {
		out = append(out, 0)
	}
	out = append(out, c.Code...)
	header[layout.CodeOffsetField/4] = int32(codeOff)
	header[layout.CodeOffsetField/4+1] = int32(len(c.Code))
	for i := 0; i < 6; i++ {
		t := &c.Tables[i]
		if !t.Present {
			continue
		}
		unit := textcodec.UnitSize(t.Encoding)
		term := []byte{0}
		if unit == 2 {
			term = []byte{0, 0}
		}
		pool := make([]byte, 0)
		rels := make([]int, t.Count)
		for j := 0; j < t.Count; j++ {
			used := t.Encoding
			if j < len(t.UTF8Override) && t.UTF8Override[j] && unit == 1 {
				used = "utf8"
			}
			raw, err := textcodec.Encode(used, t.Texts[j])
			if err != nil && used != "utf8" && unit == 1 {
				raw, err = textcodec.Encode("utf8", t.Texts[j])
				if err == nil && j < len(t.UTF8Override) {
					t.UTF8Override[j] = true
				}
			}
			if err != nil {
				return nil, fmt.Errorf("scriptbin: %s/%d: %w", TableNames[i], j, err)
			}
			if unit == 2 {
				rels[j] = len(pool) / 2
			} else {
				rels[j] = len(pool)
			}
			pool = append(pool, raw...)
			pool = append(pool, term...)
		}
		stride := 4
		if i == 0 {
			stride = 8
		}
		listSize := t.Count * stride
		listOff := align16(len(out))
		for len(out) < listOff {
			out = append(out, 0)
		}
		for j := 0; j < t.Count; j++ {
			if i == 0 {
				var code int32
				if j < len(t.LabelCode) {
					code = t.LabelCode[j]
				}
				var b [8]byte
				binary.LittleEndian.PutUint32(b[:], uint32(code))
				binary.LittleEndian.PutUint32(b[4:], uint32(int32(rels[j])))
				out = append(out, b[:]...)
			} else {
				var b [4]byte
				binary.LittleEndian.PutUint32(b[:], uint32(int32(rels[j])))
				out = append(out, b[:]...)
			}
		}
		blockOff := align16(len(out))
		for len(out) < blockOff {
			out = append(out, 0)
		}
		out = append(out, pool...)
		base := layout.Groups[i] / 4
		header[base] = int32(listOff)
		header[base+1] = int32(listSize)
		header[base+2] = int32(blockOff)
		header[base+3] = int32(len(pool))
	}
	finalHead := make([]byte, 0, len(header)*4)
	for _, v := range header {
		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], uint32(v))
		finalHead = append(finalHead, b[:]...)
	}
	copy(out, finalHead)
	if c.Negated {
		core.NegateBytes(out)
	}
	return out, nil
}

func (c *Container) ToDocument(featureID, version, name string) *core.Document {
	msgEnc := ""
	if c.Tables[5].Present {
		msgEnc = c.Tables[5].Encoding
	}
	doc := &core.Document{
		Feature:  featureID,
		Format:   c.Signature,
		Version:  version,
		Encoding: msgEnc,
		Meta: core.Meta{
			Values: map[string]string{
				"source":    name,
				"signature": c.Signature,
				"layout":    c.Layout.Kind.String(),
			},
			Lists: map[string][]string{},
		},
	}
	if c.Negated {
		doc.Meta.Values["negated"] = "1"
	}
	doc.Sections = append(doc.Sections, core.Section{
		Kind:   "values",
		Name:   "header",
		Offset: 0,
		Size:   c.Layout.HeaderEnd,
		Values: append([]int32{}, c.Header...),
	})
	doc.Sections = append(doc.Sections, core.Section{
		Kind:   "bytes",
		Name:   "sysparams",
		Offset: c.Layout.HeaderEnd,
		Size:   len(c.SysData),
		Base64: base64.StdEncoding.EncodeToString(c.SysData),
	})
	for i := 0; i < 6; i++ {
		t := &c.Tables[i]
		sec := core.Section{
			Kind:     "strings",
			Name:     TableNames[i],
			Offset:   t.ListOffset,
			Size:     t.ListSize,
			Encoding: t.Encoding,
			UnitSize: textcodec.UnitSize(t.Encoding),
		}
		if t.Present {
			sec.IDs = make([]string, t.Count)
			for j := 0; j < t.Count; j++ {
				id := fmt.Sprintf("%s/%d", TableNames[i], j)
				sec.IDs[j] = id
				doc.Units = append(doc.Units, core.Unit{ID: id, Kind: TableKinds[i], Text: t.Texts[j]})
			}
			if i == 0 && len(t.LabelCode) > 0 {
				sec.Values = append([]int32{}, t.LabelCode...)
			}
			var over []string
			for j := 0; j < t.Count && j < len(t.UTF8Override); j++ {
				if t.UTF8Override[j] {
					over = append(over, sec.IDs[j])
				}
			}
			if len(over) > 0 {
				doc.Meta.Lists["utf8."+TableNames[i]] = over
			}
		}
		doc.Sections = append(doc.Sections, sec)
	}
	codeOff := readInt32FromHeader(c.Header, c.Layout.CodeOffsetField)
	doc.Sections = append(doc.Sections, core.Section{
		Kind:   "code",
		Name:   "code",
		Offset: codeOff,
		Size:   len(c.Code),
		Base64: base64.StdEncoding.EncodeToString(c.Code),
	})
	return doc
}

func readInt32FromHeader(header []int32, off int) int {
	idx := off / 4
	if idx < 0 || idx >= len(header) {
		return 0
	}
	return int(header[idx])
}

func hasNonASCII(s string) bool {
	for _, r := range s {
		if r >= 0x80 {
			return true
		}
	}
	return false
}

func retargetedToUtf8(doc *core.Document) bool {
	if textcodec.CanonicalName(doc.Encoding) != "utf8" {
		return false
	}
	fresh := ""
	if ms := core.FindSection(doc.Sections, "strings", "messages"); ms != nil && len(ms.IDs) > 0 {
		fresh = ms.Encoding
	}
	return textcodec.CanonicalName(fresh) != "utf8"
}

func FromDocument(doc *core.Document, defaultEncoding string) (*Container, error) {
	if doc == nil {
		return nil, fmt.Errorf("scriptbin: nil document")
	}
	hs := core.FindSection(doc.Sections, "values", "header")
	if hs == nil {
		return nil, fmt.Errorf("scriptbin: missing header section")
	}
	layout, ok := LayoutForWords(len(hs.Values))
	if !ok {
		return nil, fmt.Errorf("scriptbin: bad header length %d", len(hs.Values))
	}
	c := &Container{Layout: layout}
	c.Header = append([]int32{}, hs.Values...)
	if sig, ok := doc.Meta.Values["signature"]; ok && sig != "" {
		c.Signature = sig
	} else {
		headBytes := make([]byte, 0, len(c.Header)*4)
		for _, v := range c.Header {
			var b [4]byte
			binary.LittleEndian.PutUint32(b[:], uint32(v))
			headBytes = append(headBytes, b[:]...)
		}
		if sig, ok := DetectSignature(headBytes); ok {
			c.Signature = sig
		}
	}
	if doc.Meta.Values["negated"] == "1" {
		c.Negated = true
	}
	if ss := core.FindSection(doc.Sections, "bytes", "sysparams"); ss != nil && ss.Base64 != "" {
		raw, err := base64.StdEncoding.DecodeString(ss.Base64)
		if err != nil {
			return nil, fmt.Errorf("scriptbin: bad sysparams base64: %w", err)
		}
		c.SysData = raw
	}
	byID := make(map[string]string, len(doc.Units))
	for _, u := range doc.Units {
		if _, dup := byID[u.ID]; !dup {
			byID[u.ID] = u.Text
		}
	}
	overSets := make(map[string]map[string]bool)
	for key, ids := range doc.Meta.Lists {
		if !strings.HasPrefix(key, "utf8.") {
			continue
		}
		set := make(map[string]bool, len(ids))
		for _, id := range ids {
			set[id] = true
		}
		overSets[strings.TrimPrefix(key, "utf8.")] = set
	}
	retarget := retargetedToUtf8(doc)
	for i := 0; i < 6; i++ {
		sec := core.FindSection(doc.Sections, "strings", TableNames[i])
		if sec == nil || len(sec.IDs) == 0 {
			continue
		}
		t := &c.Tables[i]
		t.Present = true
		t.Count = len(sec.IDs)
		t.Encoding = sec.Encoding
		if t.Encoding == "" {
			t.Encoding = defaultEncoding
		}
		t.Texts = make([]string, t.Count)
		t.UTF8Override = make([]bool, t.Count)
		for j, id := range sec.IDs {
			text, ok := byID[id]
			if !ok {
				return nil, fmt.Errorf("scriptbin: missing unit %q", id)
			}
			t.Texts[j] = text
			if overSets[TableNames[i]][id] {
				t.UTF8Override[j] = true
			}
			if retarget && textcodec.UnitSize(t.Encoding) == 1 && hasNonASCII(text) {
				t.UTF8Override[j] = true
			}
		}
		if i == 0 {
			if len(sec.Values) != t.Count {
				return nil, fmt.Errorf("scriptbin: labels values length %d != %d", len(sec.Values), t.Count)
			}
			t.LabelCode = append([]int32{}, sec.Values...)
		}
	}
	cs := core.FindSection(doc.Sections, "code", "code")
	if cs == nil {
		return nil, fmt.Errorf("scriptbin: missing code section")
	}
	raw, err := base64.StdEncoding.DecodeString(cs.Base64)
	if err != nil {
		return nil, fmt.Errorf("scriptbin: bad code base64: %w", err)
	}
	c.Code = raw
	return c, nil
}
