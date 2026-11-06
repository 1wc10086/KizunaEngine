package scf

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"unicode/utf8"

	"kizuna/kernel/core"
	"kizuna/kernel/core/textcodec"
)

const (
	encCP932   = 0
	encUTF8    = 1
	encGBK     = 2
	encUTF16LE = 3
)

func encodeEncID(enc string) byte {
	switch enc {
	case "utf8":
		return encUTF8
	case "gbk", "gb2312", "gb18030":
		return encGBK
	case "utf16", "utf16le":
		return encUTF16LE
	default:
		return encCP932
	}
}

func decodeEncID(id byte) string {
	switch id {
	case encUTF8:
		return "utf8"
	case encGBK:
		return "gbk"
	case encUTF16LE:
		return "utf16le"
	default:
		return "cp932"
	}
}

func stripFF(raw []byte) ([]byte, bool) {
	hit := false
	for _, c := range raw {
		if c == 0xFF {
			hit = true
			break
		}
	}
	if !hit {
		return raw, false
	}
	out := make([]byte, 0, len(raw))
	for _, c := range raw {
		if c != 0xFF {
			out = append(out, c)
		}
	}
	return out, true
}

func resolveEncoding(raw []byte) (string, string, error) {
	cands := []string{textcodec.DetectTextEncoding(raw), "cp932", "gbk"}
	if utf8.Valid(raw) {
		cands = append(cands, "utf8")
	}
	done := map[string]bool{}
	for _, enc := range cands {
		if done[enc] {
			continue
		}
		done[enc] = true
		text := textcodec.Decode(enc, raw)
		back, err := textcodec.Encode(enc, text)
		if err != nil {
			continue
		}
		if bytes.Equal(back, raw) {
			return text, enc, nil
		}
	}
	return "", "", fmt.Errorf("scf: text %q has no stable encoding", raw)
}

func decodeText(raw []byte, codeArea bool) (string, string, error) {
	body := raw
	if codeArea {
		var stripped bool
		body, stripped = stripFF(raw)
		if stripped {
			return "", "", errors.New("scf: text operand contains filler bytes")
		}
	}
	text, enc, err := resolveEncoding(body)
	if err != nil {
		return "", "", err
	}
	return text, enc, nil
}

func encodeText(text string, enc string) ([]byte, error) {
	out, err := textcodec.Encode(enc, text)
	if err != nil && textcodec.CanonicalName(enc) != "utf8" {
		out, err = textcodec.Encode("utf8", text)
	}
	if err != nil {
		return nil, fmt.Errorf("scf: cannot encode text as %s: %w", enc, err)
	}
	for _, c := range out {
		if c == 0xFF {
			return nil, fmt.Errorf("scf: encoded text contains forbidden byte")
		}
	}
	return out, nil
}

func codeUnitKind(letter byte) string {
	if letter == 'p' {
		return "raw"
	}
	return "message"
}

func poolUnitID(prefix string, i int) string {
	return fmt.Sprintf("%spool/%d", prefix, i)
}

func codeUnitID(prefix string, k int) string {
	return fmt.Sprintf("%scode/%d", prefix, k)
}

func dominantEncoding(encs []string) string {
	if len(encs) == 0 {
		return "cp932"
	}
	count := map[string]int{}
	best := encs[0]
	for _, e := range encs {
		count[e]++
		if count[e] > count[best] {
			best = e
		}
	}
	return best
}

func buildPoolSidecar(c *container, encs []string) (string, error) {
	buf := make([]byte, 0, len(c.pool))
	tmp := make([]byte, 2)
	binary.LittleEndian.PutUint16(tmp, uint16(len(c.items)))
	buf = append(buf, tmp...)
	for i, it := range c.items {
		buf = append(buf, it.flag, encodeEncID(encs[i]))
		binary.LittleEndian.PutUint16(tmp, uint16(len(it.expr)))
		buf = append(buf, tmp...)
		buf = append(buf, it.expr...)
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}

func parsePoolSidecar(trailing string, n int) ([]byte, []string, [][]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(trailing)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("scf: bad pool sidecar: %w", err)
	}
	if len(raw) < 2 || int(binary.LittleEndian.Uint16(raw)) != n {
		return nil, nil, nil, errors.New("scf: pool sidecar count mismatch")
	}
	raw = raw[2:]
	flags := make([]byte, n)
	encs := make([]string, n)
	exprs := make([][]byte, n)
	for i := 0; i < n; i++ {
		if len(raw) < 4 {
			return nil, nil, nil, errors.New("scf: truncated pool sidecar")
		}
		flags[i] = raw[0]
		encs[i] = decodeEncID(raw[1])
		ln := int(binary.LittleEndian.Uint16(raw[2:]))
		raw = raw[4:]
		if len(raw) < ln {
			return nil, nil, nil, errors.New("scf: truncated pool sidecar")
		}
		exprs[i] = append([]byte(nil), raw[:ln]...)
		raw = raw[ln:]
	}
	if len(raw) != 0 {
		return nil, nil, nil, errors.New("scf: pool sidecar has trailing bytes")
	}
	return flags, encs, exprs, nil
}

func buildCodeSidecar(encs []string) string {
	if len(encs) == 0 {
		return ""
	}
	buf := make([]byte, 0, 2+len(encs))
	tmp := make([]byte, 2)
	binary.LittleEndian.PutUint16(tmp, uint16(len(encs)))
	buf = append(buf, tmp...)
	for _, e := range encs {
		buf = append(buf, encodeEncID(e))
	}
	return base64.StdEncoding.EncodeToString(buf)
}

func parseCodeSidecar(trailing string, n int) ([]string, error) {
	if n == 0 {
		if trailing != "" {
			return nil, errors.New("scf: unexpected code sidecar")
		}
		return nil, nil
	}
	raw, err := base64.StdEncoding.DecodeString(trailing)
	if err != nil {
		return nil, fmt.Errorf("scf: bad code sidecar: %w", err)
	}
	if len(raw) != 2+n || int(binary.LittleEndian.Uint16(raw)) != n {
		return nil, errors.New("scf: code sidecar count mismatch")
	}
	encs := make([]string, n)
	for i := 0; i < n; i++ {
		encs[i] = decodeEncID(raw[2+i])
	}
	return encs, nil
}

func buildUnits(c *container, prefix string) ([]core.Unit, []string, []string, error) {
	var units []core.Unit
	poolEncs := make([]string, len(c.items))
	for i, it := range c.items {
		text, enc, err := decodeText(it.text, false)
		if err != nil {
			return nil, nil, nil, err
		}
		poolEncs[i] = enc
		units = append(units, core.Unit{ID: poolUnitID(prefix, i), Kind: "choice", Text: text})
	}
	codeEncs := make([]string, len(c.strs))
	for k, s := range c.strs {
		raw := c.code[s.strOff : s.strOff+s.rawLen-1]
		text, enc, err := decodeText(raw, true)
		if err != nil {
			return nil, nil, nil, err
		}
		codeEncs[k] = enc
		units = append(units, core.Unit{ID: codeUnitID(prefix, k), Kind: codeUnitKind(s.letter), Text: text})
	}
	if units == nil {
		units = []core.Unit{}
	}
	return units, poolEncs, codeEncs, nil
}

func buildSections(c *container, prefix string, base int, poolEncs []string, codeEncs []string) ([]core.Section, error) {
	rel := base
	sections := []core.Section{}
	headerVals := []int32{int32(c.h.entries), int32(c.h.poolSize), int32(c.h.jumpCount), int32(c.h.codeLen), int32(c.h.transfers)}
	sections = append(sections, core.Section{Kind: "values", Name: secHeader, Offset: rel, Values: headerVals})
	rel += 10
	if c.h.entries > 0 {
		vals := make([]int32, 0, 3*c.h.entries)
		for _, v := range c.entryVals {
			vals = append(vals, int32(v))
		}
		sections = append(sections, core.Section{Kind: "values", Name: secEntries, Offset: rel, Values: vals})
		rel += 6 * c.h.entries
		offs := make([]int32, 0, c.h.entries)
		for _, v := range c.poolOff {
			offs = append(offs, int32(v))
		}
		sections = append(sections, core.Section{Kind: "values", Name: secPoolOffsets, Offset: rel, Values: offs})
		rel += 2 * c.h.entries
	}
	sections = append(sections, core.Section{Kind: "bytes", Name: secTransfer, Offset: rel, Size: len(c.transfer), Base64: base64.StdEncoding.EncodeToString(c.transfer)})
	rel += 4 * c.h.transfers
	if c.h.jumpCount > 0 {
		vals := make([]int32, 0, c.h.jumpCount)
		for _, v := range c.jump {
			vals = append(vals, int32(v))
		}
		sections = append(sections, core.Section{Kind: "values", Name: secJump, Offset: rel, Values: vals})
		rel += 2 * c.h.jumpCount
	}
	if c.h.entries > 0 {
		ids := make([]string, 0, c.h.entries)
		for i := range c.items {
			ids = append(ids, poolUnitID(prefix, i))
		}
		sidecar, err := buildPoolSidecar(c, poolEncs)
		if err != nil {
			return nil, err
		}
		sections = append(sections, core.Section{Kind: "strings", Name: secPool, Offset: rel, Encoding: dominantEncoding(poolEncs), UnitSize: 1, IDs: ids, Trailing: sidecar})
		rel += c.h.poolSize
	}
	codeIDs := make([]string, 0, len(c.strs))
	for k := range c.strs {
		codeIDs = append(codeIDs, codeUnitID(prefix, k))
	}
	sections = append(sections, core.Section{Kind: "code", Name: secCode, Offset: rel, Size: len(c.code), Base64: base64.StdEncoding.EncodeToString(c.code), Encoding: dominantEncoding(codeEncs), IDs: codeIDs, Trailing: buildCodeSidecar(codeEncs)})
	return sections, nil
}

func LooksLikeSCF(data []byte) bool {
	_, _, err := DecodeAt(data, "", 0)
	return err == nil
}

func DecodeAt(data []byte, idPrefix string, baseOffset int) ([]core.Unit, []core.Section, error) {
	c, err := parse(data)
	if err != nil {
		return nil, nil, err
	}
	units, poolEncs, codeEncs, err := buildUnits(c, idPrefix)
	if err != nil {
		return nil, nil, err
	}
	sections, err := buildSections(c, idPrefix, baseOffset, poolEncs, codeEncs)
	if err != nil {
		return nil, nil, err
	}
	return units, sections, nil
}

func Decode(data []byte, name string) (*core.Document, error) {
	_ = name
	units, sections, err := DecodeAt(data, "", 0)
	if err != nil {
		return nil, err
	}
	encs := []string{}
	for _, s := range sections {
		if s.Name == secPool || s.Name == secCode {
			encs = append(encs, s.Encoding)
		}
	}
	return &core.Document{
		Feature:  FeatureID,
		Format:   FormatName,
		Version:  FormatVersion,
		Encoding: dominantEncoding(encs),
		Units:    units,
		Sections: sections,
	}, nil
}

func findSection(sections []core.Section, name string) (core.Section, bool) {
	var hit core.Section
	count := 0
	for _, s := range sections {
		if s.Name == name {
			hit = s
			count++
		}
	}
	if count != 1 {
		return core.Section{}, false
	}
	return hit, true
}

func docEncodingOverride(doc *core.Document) string {
	if doc == nil {
		return ""
	}
	want := textcodec.CanonicalName(doc.Encoding)
	if want == "" {
		return ""
	}
	encs := []string{}
	for _, s := range doc.Sections {
		if s.Name == secPool || s.Name == secCode {
			encs = append(encs, s.Encoding)
		}
	}
	if want == dominantEncoding(encs) {
		return ""
	}
	return want
}

func EncodeParts(units []core.Unit, sections []core.Section, docEncoding string) ([]byte, error) {
	return encodePartsWithOverride(units, sections, textcodec.CanonicalName(docEncoding))
}

func encodePartsWithOverride(units []core.Unit, sections []core.Section, override string) ([]byte, error) {
	hs, ok := findSection(sections, secHeader)
	if !ok || len(hs.Values) != 5 {
		return nil, errors.New("scf: missing header section")
	}
	for _, v := range hs.Values {
		if v < 0 || v > 0xFFFF {
			return nil, errors.New("scf: header value out of range")
		}
	}
	h0 := int(hs.Values[0])
	h2 := int(hs.Values[2])
	h4 := int(hs.Values[4])
	textOf := make(map[string]string, len(units))
	for _, u := range units {
		if _, dup := textOf[u.ID]; dup {
			return nil, fmt.Errorf("scf: duplicate unit %s", u.ID)
		}
		textOf[u.ID] = u.Text
	}
	entryVals := []uint16{}
	if h0 > 0 {
		es, ok := findSection(sections, secEntries)
		if !ok || len(es.Values) != 3*h0 {
			return nil, errors.New("scf: bad entries section")
		}
		for _, v := range es.Values {
			if v < 0 || v > 0xFFFF {
				return nil, errors.New("scf: entry value out of range")
			}
			entryVals = append(entryVals, uint16(v))
		}
		ps, ok := findSection(sections, secPoolOffsets)
		if !ok || len(ps.Values) != h0 {
			return nil, errors.New("scf: bad pool offsets section")
		}
	}
	ts, ok := findSection(sections, secTransfer)
	if !ok {
		return nil, errors.New("scf: missing transfer section")
	}
	transfer, err := base64.StdEncoding.DecodeString(ts.Base64)
	if err != nil || len(transfer) != 4*h4 {
		return nil, errors.New("scf: bad transfer section")
	}
	oldJump := []uint16{}
	if h2 > 0 {
		js, ok := findSection(sections, secJump)
		if !ok || len(js.Values) != h2 {
			return nil, errors.New("scf: bad jump section")
		}
		for _, v := range js.Values {
			if v < 0 || v > 0xFFFF {
				return nil, errors.New("scf: jump value out of range")
			}
			oldJump = append(oldJump, uint16(v))
		}
	}
	poolIDs := []string{}
	var poolFlags []byte
	var poolEncs []string
	var poolExprs [][]byte
	if h0 > 0 {
		pls, ok := findSection(sections, secPool)
		if !ok || len(pls.IDs) != h0 {
			return nil, errors.New("scf: bad pool section")
		}
		poolIDs = pls.IDs
		var err error
		poolFlags, poolEncs, poolExprs, err = parsePoolSidecar(pls.Trailing, h0)
		if err != nil {
			return nil, err
		}
	}
	cs, ok := findSection(sections, secCode)
	if !ok {
		return nil, errors.New("scf: missing code section")
	}
	oldCode, err := base64.StdEncoding.DecodeString(cs.Base64)
	if err != nil {
		return nil, errors.New("scf: bad code section")
	}
	codeEncs, err := parseCodeSidecar(cs.Trailing, len(cs.IDs))
	if err != nil {
		return nil, err
	}
	instrs, slots, err := detectCode(oldCode)
	if err != nil {
		return nil, err
	}
	if len(slots) != len(cs.IDs) {
		return nil, errors.New("scf: code strings count mismatch")
	}
	for _, id := range cs.IDs {
		if _, has := textOf[id]; !has {
			return nil, fmt.Errorf("scf: missing unit %s", id)
		}
	}
	pool := make([]byte, 0)
	newOffs := make([]uint16, h0)
	for i, id := range poolIDs {
		text, has := textOf[id]
		if !has {
			return nil, fmt.Errorf("scf: missing unit %s", id)
		}
		enc := poolEncs[i]
		if override != "" {
			enc = override
		}
		raw, err := encodeText(text, enc)
		if err != nil {
			return nil, err
		}
		if len(pool)+1+len(raw)+1 > 0xFFFF {
			return nil, errors.New("scf: pool too large")
		}
		newOffs[i] = uint16(len(pool))
		pool = append(pool, poolFlags[i])
		pool = append(pool, raw...)
		pool = append(pool, 0)
		if poolFlags[i] == 0 {
			pool = append(pool, poolExprs[i]...)
			pool = append(pool, opLabel)
		}
	}
	slotText := make([][]byte, len(slots))
	for k, id := range cs.IDs {
		enc := codeEncs[k]
		if override != "" {
			enc = override
		}
		raw, err := encodeText(textOf[id], enc)
		if err != nil {
			return nil, err
		}
		slotText[k] = raw
	}
	newCode := make([]byte, 0, len(oldCode))
	newStart := make(map[int]int, len(instrs))
	for _, in := range instrs {
		newStart[in.start] = len(newCode)
		if in.strIdx < 0 {
			newCode = append(newCode, oldCode[in.start:in.start+in.length]...)
			continue
		}
		s := slots[in.strIdx]
		newCode = append(newCode, oldCode[in.start:s.strOff]...)
		newCode = append(newCode, slotText[in.strIdx]...)
		newCode = append(newCode, 0)
	}
	if len(newCode) > 0xFFFF {
		return nil, errors.New("scf: code too large")
	}
	newJump := make([]uint16, h2)
	for i, t := range oldJump {
		s := containingStart(instrs, int(t))
		ns, ok := newStart[s]
		if !ok {
			return nil, fmt.Errorf("scf: jump target %d unmappable", t)
		}
		ns += int(t) - s
		if ns < 0 || ns > len(newCode) {
			return nil, fmt.Errorf("scf: remapped jump %d out of range", ns)
		}
		newJump[i] = uint16(ns)
	}
	out := make([]byte, 0)
	tmp := make([]byte, 2)
	put16 := func(v int) {
		binary.LittleEndian.PutUint16(tmp, uint16(v))
		out = append(out, tmp...)
	}
	put16(h0)
	put16(len(pool))
	put16(h2)
	put16(len(newCode))
	put16(h4)
	for _, v := range entryVals {
		put16(int(v))
	}
	for _, v := range newOffs {
		put16(int(v))
	}
	out = append(out, transfer...)
	for _, v := range newJump {
		put16(int(v))
	}
	out = append(out, pool...)
	out = append(out, newCode...)
	return out, nil
}

func containingStart(instrs []instr, off int) int {
	start := instrs[0].start
	for _, in := range instrs {
		if in.start > off {
			break
		}
		start = in.start
	}
	return start
}

func Encode(doc *core.Document) ([]byte, error) {
	if doc == nil {
		return nil, errors.New("scf: nil document")
	}
	if doc.Feature != FeatureID {
		return nil, core.ErrNotMine
	}
	return encodePartsWithOverride(doc.Units, doc.Sections, docEncodingOverride(doc))
}
