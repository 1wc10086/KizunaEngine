package dsk

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"kizuna/kernel/brand/bishop/scf"
	"kizuna/kernel/core"
)

const (
	FeatureID     = "bishop.dsk"
	FeatureName   = "Bishop-Dsk"
	FormatName    = "DSK"
	FormatVersion = "1"
)

const (
	secGap   = "dsk.gap"
	secEntry = "dsk.entry"
)

const (
	metaPFT     = "dsk.pft"
	metaMissing = "dsk.pft.missing"
	listEntries = "dsk.entries"
	listSlots   = "dsk.slots"
)

const maxEntries = 1000000

type tableEntry struct {
	pftIndex int
	name     string
	cluster  uint32
	size     uint32
	offset   int64
	scf      bool
	nsec     int
}

type slot struct {
	gap bool
	idx int
}

func parsePFT(data []byte) (int, int, []string, []uint32, []uint32, error) {
	if len(data) < 8 {
		return 0, 0, nil, nil, nil, fmt.Errorf("dsk: pft too short (%d bytes)", len(data))
	}
	hs := int(binary.LittleEndian.Uint16(data[0:]))
	cs := int(binary.LittleEndian.Uint16(data[2:]))
	count := int(int32(binary.LittleEndian.Uint32(data[4:])))
	if hs < 6 || hs > len(data) {
		return 0, 0, nil, nil, nil, fmt.Errorf("dsk: bad pft header size %d", hs)
	}
	if cs == 0 {
		return 0, 0, nil, nil, nil, fmt.Errorf("dsk: pft cluster size is zero")
	}
	if count < 0 || count > maxEntries {
		return 0, 0, nil, nil, nil, fmt.Errorf("dsk: bad pft entry count %d", count)
	}
	if int64(hs)+int64(count)*16 > int64(len(data)) {
		return 0, 0, nil, nil, nil, fmt.Errorf("dsk: pft truncated for %d entries", count)
	}
	names := make([]string, count)
	clusters := make([]uint32, count)
	sizes := make([]uint32, count)
	for i := 0; i < count; i++ {
		off := hs + i*16
		names[i] = displayName(data[off : off+8])
		clusters[i] = binary.LittleEndian.Uint32(data[off+8:])
		sizes[i] = binary.LittleEndian.Uint32(data[off+12:])
	}
	return hs, cs, names, clusters, sizes, nil
}

func displayName(raw []byte) string {
	if i := bytes.IndexByte(raw, 0); i >= 0 {
		raw = raw[:i]
	}
	return strings.TrimRight(string(raw), " \t")
}

func layoutOrder(tab []tableEntry) []int {
	order := make([]int, len(tab))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool {
		if tab[order[a]].offset != tab[order[b]].offset {
			return tab[order[a]].offset < tab[order[b]].offset
		}
		return tab[order[a]].pftIndex < tab[order[b]].pftIndex
	})
	return order
}

func dominantEncoding(sections []core.Section) string {
	count := map[string]int{}
	best := ""
	for _, s := range sections {
		if s.Encoding == "" {
			continue
		}
		count[s.Encoding]++
		if best == "" || count[s.Encoding] > count[best] {
			best = s.Encoding
		}
	}
	if best == "" {
		return "cp932"
	}
	return best
}

func gapSection(data []byte, offset int64) core.Section {
	return core.Section{
		Kind:   "bytes",
		Name:   secGap,
		Offset: int(offset),
		Size:   len(data),
		Base64: base64.StdEncoding.EncodeToString(data),
	}
}

func DecodeWithPFT(dskData, pftData []byte, name string) (*core.Document, error) {
	_, cs, names, clusters, sizes, err := parsePFT(pftData)
	if err != nil {
		return nil, err
	}
	tab := make([]tableEntry, 0, len(names))
	for i := range names {
		if names[i] == "" {
			core.Logf("dsk: %s entry %d has empty name, skipped", name, i)
			continue
		}
		off := int64(clusters[i]) * int64(cs)
		if off < 0 || off+int64(sizes[i]) > int64(len(dskData)) {
			core.Logf("dsk: %s entry %q outside archive, skipped", name, names[i])
			continue
		}
		tab = append(tab, tableEntry{
			pftIndex: i,
			name:     names[i],
			cluster:  clusters[i],
			size:     sizes[i],
			offset:   off,
		})
	}
	if len(tab) == 0 {
		return nil, fmt.Errorf("dsk: no usable entries in %s", name)
	}
	order := layoutOrder(tab)
	layoutOf := make([]int, len(tab))
	for pos, k := range order {
		layoutOf[k] = pos
	}
	units := []core.Unit{}
	sections := []core.Section{}
	slots := []string{}
	cursor := int64(0)
	for _, k := range order {
		e := &tab[k]
		if e.offset < cursor {
			return nil, fmt.Errorf("dsk: %s entry %q overlaps previous entry", name, e.name)
		}
		if e.offset > cursor {
			sections = append(sections, gapSection(dskData[cursor:e.offset], cursor))
			slots = append(slots, "g")
			cursor = e.offset
		}
		raw := dskData[cursor : cursor+int64(e.size)]
		prefix := "e" + strconv.Itoa(layoutOf[k]) + "/"
		if u, s, err := scf.DecodeAt(raw, prefix, int(cursor)); err == nil {
			e.scf = true
			e.nsec = len(s)
			units = append(units, u...)
			sections = append(sections, s...)
		} else {
			e.nsec = 1
			sections = append(sections, core.Section{
				Kind:   "bytes",
				Name:   secEntry,
				Offset: int(cursor),
				Size:   len(raw),
				Base64: base64.StdEncoding.EncodeToString(raw),
			})
		}
		slots = append(slots, "e"+strconv.Itoa(k))
		cursor += int64(e.size)
	}
	if cursor < int64(len(dskData)) {
		sections = append(sections, gapSection(dskData[cursor:], cursor))
		slots = append(slots, "g")
	}
	entries := make([]string, 0, len(tab))
	for _, e := range tab {
		flag := "0"
		if e.scf {
			flag = "1"
		}
		entries = append(entries, strings.Join([]string{
			strconv.Itoa(e.pftIndex),
			e.name,
			strconv.FormatUint(uint64(e.cluster), 10),
			strconv.FormatUint(uint64(e.size), 10),
			flag,
			strconv.Itoa(e.nsec),
		}, "\t"))
	}
	return &core.Document{
		Feature:  FeatureID,
		Format:   FormatName,
		Version:  FormatVersion,
		Encoding: dominantEncoding(sections),
		Units:    units,
		Sections: sections,
		Meta: core.Meta{
			Values: map[string]string{metaPFT: base64.StdEncoding.EncodeToString(pftData)},
			Lists:  map[string][]string{listEntries: entries, listSlots: slots},
		},
	}, nil
}

func fallbackDocument(data []byte, name string) *core.Document {
	_ = name
	sections := []core.Section{}
	if len(data) > 0 {
		sections = append(sections, core.Section{
			Kind:   "bytes",
			Name:   secEntry,
			Offset: 0,
			Size:   len(data),
			Base64: base64.StdEncoding.EncodeToString(data),
		})
	}
	return &core.Document{
		Feature:  FeatureID,
		Format:   FormatName,
		Version:  FormatVersion,
		Encoding: "cp932",
		Units:    []core.Unit{},
		Sections: sections,
		Meta: core.Meta{
			Values: map[string]string{metaMissing: "true"},
		},
	}
}

func parseTable(lines []string, cs int) ([]tableEntry, error) {
	tab := make([]tableEntry, 0, len(lines))
	for _, line := range lines {
		parts := strings.Split(line, "\t")
		if len(parts) < 6 {
			return nil, fmt.Errorf("dsk: bad entry record %q", line)
		}
		n := len(parts)
		pi, err := strconv.Atoi(parts[0])
		if err != nil || pi < 0 {
			return nil, fmt.Errorf("dsk: bad entry record %q", line)
		}
		cluster, err := strconv.ParseUint(parts[n-4], 10, 32)
		if err != nil {
			return nil, fmt.Errorf("dsk: bad entry record %q", line)
		}
		size, err := strconv.ParseUint(parts[n-3], 10, 32)
		if err != nil {
			return nil, fmt.Errorf("dsk: bad entry record %q", line)
		}
		scfFlag := parts[n-2] == "1"
		if parts[n-2] != "0" && parts[n-2] != "1" {
			return nil, fmt.Errorf("dsk: bad entry record %q", line)
		}
		nsec, err := strconv.Atoi(parts[n-1])
		if err != nil || nsec < 0 {
			return nil, fmt.Errorf("dsk: bad entry record %q", line)
		}
		tab = append(tab, tableEntry{
			pftIndex: pi,
			name:     strings.Join(parts[1:n-4], "\t"),
			cluster:  uint32(cluster),
			size:     uint32(size),
			offset:   int64(cluster) * int64(cs),
			scf:      scfFlag,
			nsec:     nsec,
		})
	}
	return tab, nil
}

func parseSlots(lines []string) ([]slot, error) {
	slots := make([]slot, 0, len(lines))
	for _, line := range lines {
		if line == "g" {
			slots = append(slots, slot{gap: true})
			continue
		}
		if strings.HasPrefix(line, "e") {
			n, err := strconv.Atoi(line[1:])
			if err != nil || n < 0 {
				return nil, fmt.Errorf("dsk: bad slot %q", line)
			}
			slots = append(slots, slot{idx: n})
			continue
		}
		return nil, fmt.Errorf("dsk: bad slot %q", line)
	}
	return slots, nil
}

func rebuildEntries(doc *core.Document, tab []tableEntry, slots []slot) ([][]byte, [][]byte, error) {
	secs := doc.Sections
	si := 0
	entryData := make([][]byte, len(tab))
	gaps := [][]byte{}
	for _, sl := range slots {
		if sl.gap {
			if si >= len(secs) {
				return nil, nil, fmt.Errorf("dsk: section stream truncated at gap")
			}
			s := secs[si]
			si++
			if s.Kind != "bytes" || s.Name != secGap {
				return nil, nil, fmt.Errorf("dsk: expected gap section, got %q/%q", s.Kind, s.Name)
			}
			raw, err := base64.StdEncoding.DecodeString(s.Base64)
			if err != nil {
				return nil, nil, fmt.Errorf("dsk: bad gap encoding: %w", err)
			}
			gaps = append(gaps, raw)
			continue
		}
		if sl.idx < 0 || sl.idx >= len(tab) {
			return nil, nil, fmt.Errorf("dsk: slot references missing entry %d", sl.idx)
		}
		e := tab[sl.idx]
		if si+e.nsec > len(secs) {
			return nil, nil, fmt.Errorf("dsk: section stream truncated at entry %q", e.name)
		}
		run := secs[si : si+e.nsec]
		si += e.nsec
		if e.scf {
			if e.nsec == 0 {
				return nil, nil, fmt.Errorf("dsk: entry %q has no sections", e.name)
			}
			out, err := scf.EncodeParts(doc.Units, run, doc.Encoding)
			if err != nil {
				return nil, nil, fmt.Errorf("dsk: entry %q: %w", e.name, err)
			}
			entryData[sl.idx] = out
		} else {
			if e.nsec != 1 {
				return nil, nil, fmt.Errorf("dsk: entry %q has %d sections, want 1", e.name, e.nsec)
			}
			if run[0].Kind != "bytes" || run[0].Name != secEntry {
				return nil, nil, fmt.Errorf("dsk: expected entry section, got %q/%q", run[0].Kind, run[0].Name)
			}
			raw, err := base64.StdEncoding.DecodeString(run[0].Base64)
			if err != nil {
				return nil, nil, fmt.Errorf("dsk: entry %q: %w", e.name, err)
			}
			entryData[sl.idx] = raw
		}
	}
	if si != len(secs) {
		return nil, nil, fmt.Errorf("dsk: %d trailing sections", len(secs)-si)
	}
	return entryData, gaps, nil
}

func concatSections(sections []core.Section) ([]byte, error) {
	out := []byte{}
	for _, s := range sections {
		switch s.Kind {
		case "bytes", "code":
			raw, err := base64.StdEncoding.DecodeString(s.Base64)
			if err != nil {
				return nil, fmt.Errorf("dsk: bad section encoding: %w", err)
			}
			out = append(out, raw...)
		default:
			return nil, fmt.Errorf("dsk: cannot rebuild section kind %q without archive table", s.Kind)
		}
	}
	return out, nil
}

func EncodeWithPFT(doc *core.Document) ([]byte, []byte, error) {
	if doc == nil {
		return nil, nil, fmt.Errorf("dsk: nil document")
	}
	if doc.Feature != FeatureID {
		return nil, nil, core.ErrNotMine
	}
	encoded, ok := doc.Meta.Values[metaPFT]
	if !ok {
		out, err := concatSections(doc.Sections)
		if err != nil {
			return nil, nil, err
		}
		return out, nil, nil
	}
	pftImage, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, nil, fmt.Errorf("dsk: bad stored pft: %w", err)
	}
	hs, cs, _, _, _, err := parsePFT(pftImage)
	if err != nil {
		return nil, nil, err
	}
	tab, err := parseTable(doc.Meta.Lists[listEntries], cs)
	if err != nil {
		return nil, nil, err
	}
	slots, err := parseSlots(doc.Meta.Lists[listSlots])
	if err != nil {
		return nil, nil, err
	}
	entryData, gaps, err := rebuildEntries(doc, tab, slots)
	if err != nil {
		return nil, nil, err
	}
	identity := true
	for k := range tab {
		if uint64(len(entryData[k])) != uint64(tab[k].size) {
			identity = false
			break
		}
	}
	if identity {
		out := []byte{}
		cursor := int64(0)
		gi := 0
		for _, sl := range slots {
			if sl.gap {
				out = append(out, gaps[gi]...)
				cursor += int64(len(gaps[gi]))
				gi++
				continue
			}
			e := tab[sl.idx]
			if cursor != e.offset {
				return nil, nil, fmt.Errorf("dsk: layout drift at entry %q", e.name)
			}
			out = append(out, entryData[sl.idx]...)
			cursor += int64(len(entryData[sl.idx]))
		}
		return out, append([]byte(nil), pftImage...), nil
	}
	order := layoutOrder(tab)
	first := 0
	if hs > 0 {
		first = (hs + cs - 1) / cs
	}
	clusters := make([]uint32, len(tab))
	cursor := first
	for _, k := range order {
		sz := len(entryData[k])
		if uint64(sz) > 0xFFFFFFFF {
			return nil, nil, fmt.Errorf("dsk: entry %q too large", tab[k].name)
		}
		if int64(cursor) > (int64(1)<<32-1)/int64(cs) {
			return nil, nil, fmt.Errorf("dsk: archive too large")
		}
		clusters[k] = uint32(cursor)
		if sz > 0 {
			cursor += (sz + cs - 1) / cs
		}
	}
	out := make([]byte, int64(cursor)*int64(cs))
	for _, k := range order {
		off := int64(clusters[k]) * int64(cs)
		copy(out[off:], entryData[k])
	}
	newPFT := append([]byte(nil), pftImage...)
	for k := range tab {
		pos := hs + tab[k].pftIndex*16
		if pos+16 > len(newPFT) {
			return nil, nil, fmt.Errorf("dsk: pft image too short for entry %q", tab[k].name)
		}
		binary.LittleEndian.PutUint32(newPFT[pos+8:], clusters[k])
		binary.LittleEndian.PutUint32(newPFT[pos+12:], uint32(len(entryData[k])))
	}
	return out, newPFT, nil
}

func FindPFT(dskPath string) (string, bool) {
	dir := filepath.Dir(dskPath)
	base := filepath.Base(dskPath)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	for _, cand := range []string{filepath.Join(dir, stem+".pft"), filepath.Join(dir, stem+".PFT")} {
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			return cand, true
		}
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	want := stem + ".pft"
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		if strings.EqualFold(f.Name(), want) {
			return filepath.Join(dir, f.Name()), true
		}
	}
	return "", false
}
