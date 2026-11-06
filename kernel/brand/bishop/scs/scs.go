package scs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kizuna/kernel/core"
	"kizuna/kernel/core/textcodec"
)

const FeatureID = "bishop.scs"

const FormatName = "scs"

const sigFirst = 0x50

const sigSecond = 0x45

const keyStep = 69069

type Codec struct{}

func init() {
	core.Register(Codec{})
}

func (Codec) ID() string {
	return FeatureID
}

func (Codec) Name() string {
	return "Bishop-Scs"
}

func (Codec) Target() core.Target {
	return core.TargetFile
}

func (Codec) Handlers() []core.Handler {
	return []core.Handler{
		{Action: "decode", Label: "解码", Suffixes: []string{".scs"}, OutputSuffix: ".scs.json"},
		{Action: "encode", Label: "编码", Suffixes: []string{".scs.json"}, OutputSuffix: ""},
	}
}

func (Codec) DetectFormat(path string, head []byte) (string, bool) {
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".scs.json") {
		raw, err := os.ReadFile(path)
		if err != nil {
			raw = head
		}
		feature, err := core.ProbeFeature(raw)
		if err != nil || feature != FeatureID {
			return "", false
		}
		return FormatName, true
	}
	if strings.HasSuffix(lower, ".scs") {
		return FormatName, true
	}
	return "", false
}

func (c Codec) Run(action string, in string, out string, opt core.Options) (core.Result, error) {
	switch action {
	case "decode":
		start := time.Now()
		data, err := os.ReadFile(in)
		if err != nil {
			return core.Result{}, err
		}
		doc, err := c.Decode(data, filepath.Base(in))
		if err != nil {
			return core.Result{}, err
		}
		if enc := textcodec.CanonicalName(opt.TextEncoding); enc != "" {
			doc.Encoding = enc
			for i := range doc.Sections {
				if doc.Sections[i].Kind == "lines" {
					doc.Sections[i].Encoding = enc
				}
			}
			if doc.Meta.Lists != nil {
				delete(doc.Meta.Lists, "encoding")
			}
		}
		if err := core.WriteJSON(out, doc); err != nil {
			return core.Result{}, err
		}
		fi, err := os.Stat(out)
		if err != nil {
			return core.Result{}, err
		}
		return core.Result{
			InputPath:  in,
			OutputPath: out,
			BytesIn:    int64(len(data)),
			BytesOut:   fi.Size(),
			ElapsedMs:  time.Since(start).Milliseconds(),
			Summary:    fmt.Sprintf("decode %d units", len(doc.Units)),
			Details: map[string]string{
				"feature": doc.Feature,
				"format":  doc.Format,
			},
		}, nil
	case "encode":
		return core.RunCodecEncode(c, in, out)
	default:
		return core.Result{}, fmt.Errorf("scs: unknown action %q", action)
	}
}

func (Codec) Decode(data []byte, name string) (*core.Document, error) {
	encrypted := len(data) >= 2 && data[0] == sigFirst && data[1] == sigSecond
	var plains [][]byte
	var encs []string
	var eols []string
	if encrypted {
		pos := 2
		for pos < len(data) {
			if pos+2 > len(data) {
				return nil, fmt.Errorf("scs: truncated length field at offset %d in %s", pos, name)
			}
			start := pos
			length := int(data[pos]) | int(data[pos+1])<<8
			pos += 2
			if pos+length > len(data) {
				return nil, fmt.Errorf("scs: truncated record at offset %d in %s", start, name)
			}
			plain := make([]byte, length)
			crypt(data[pos:pos+length], start, plain)
			pos += length
			plains = append(plains, plain)
			encs = append(encs, textcodec.DetectTextEncoding(plain))
		}
	} else {
		pos := 0
		for pos < len(data) {
			end := pos
			for end < len(data) && data[end] != 0x0A {
				end++
			}
			length := end - pos
			eol := "none"
			if end < len(data) {
				eol = "lf"
				if length > 0 && data[end-1] == 0x0D {
					length--
					eol = "crlf"
				}
			} else if length > 0 && data[end-1] == 0x0D {
				length--
			}
			plain := make([]byte, length)
			copy(plain, data[pos:pos+length])
			plains = append(plains, plain)
			encs = append(encs, textcodec.DetectTextEncoding(plain))
			eols = append(eols, eol)
			if end < len(data) {
				pos = end + 1
			} else {
				pos = end
			}
		}
	}
	docEnc := "cp932"
	for i, plain := range plains {
		if len(plain) > 0 {
			docEnc = encs[i]
			break
		}
	}
	units := make([]core.Unit, 0, len(plains))
	ids := make([]string, 0, len(plains))
	for i, plain := range plains {
		id := fmt.Sprintf("line/%d", i)
		units = append(units, core.Unit{
			ID:   id,
			Kind: "line",
			Text: textcodec.Decode(encs[i], plain),
		})
		ids = append(ids, id)
	}
	offset := 0
	if encrypted {
		offset = 2
	}
	section := core.Section{
		Kind:     "lines",
		Name:     "records",
		Offset:   offset,
		Size:     len(data) - offset,
		Encoding: docEnc,
		IDs:      ids,
	}
	if !encrypted {
		section.EOL = dominantEOL(eols)
	}
	meta := core.Meta{Values: map[string]string{}}
	if encrypted {
		meta.Values["encrypted"] = "true"
	} else {
		meta.Values["encrypted"] = "false"
	}
	mixed := false
	for _, enc := range encs {
		if enc != docEnc {
			mixed = true
			break
		}
	}
	if mixed {
		meta.Lists = map[string][]string{"encoding": append([]string(nil), encs...)}
	}
	if !encrypted {
		if meta.Lists == nil {
			meta.Lists = map[string][]string{}
		}
		meta.Lists["eol"] = append([]string(nil), eols...)
	}
	return &core.Document{
		Feature:  FeatureID,
		Format:   FormatName,
		Version:  "1",
		Encoding: docEnc,
		Units:    units,
		Sections: []core.Section{section},
		Meta:     meta,
	}, nil
}

func (Codec) Encode(doc *core.Document) ([]byte, error) {
	if doc == nil {
		return nil, fmt.Errorf("scs: nil document")
	}
	if doc.Feature != "" && doc.Feature != FeatureID {
		return nil, core.ErrNotMine
	}
	var section *core.Section
	for i := range doc.Sections {
		if doc.Sections[i].Kind == "lines" {
			section = &doc.Sections[i]
			break
		}
	}
	encrypted := true
	if doc.Meta.Values != nil {
		if v := doc.Meta.Values["encrypted"]; v == "false" || v == "0" {
			encrypted = false
		}
	}
	textByID := make(map[string]string, len(doc.Units))
	order := make([]string, 0, len(doc.Units))
	if section != nil && len(section.IDs) > 0 {
		order = append(order, section.IDs...)
	} else {
		for _, u := range doc.Units {
			order = append(order, u.ID)
		}
	}
	for _, u := range doc.Units {
		if _, dup := textByID[u.ID]; !dup {
			textByID[u.ID] = u.Text
		}
	}
	base := doc.Encoding
	if base == "" && section != nil {
		base = section.Encoding
	}
	if base == "" {
		base = "cp932"
	}
	per := doc.Meta.Lists["encoding"]
	usePer := len(per) == len(order)
	eols := doc.Meta.Lists["eol"]
	defaultEOL := "lf"
	if section != nil && section.EOL != "" {
		defaultEOL = section.EOL
	}
	if encrypted {
		out := []byte{sigFirst, sigSecond}
		for i, id := range order {
			text, ok := textByID[id]
			if !ok {
				return nil, fmt.Errorf("scs: missing unit %q", id)
			}
			enc := base
			if usePer {
				enc = per[i]
			}
			plain, used, err := encodeRecord(text, enc)
			if err != nil {
				return nil, err
			}
			if usePer && used != enc {
				per[i] = used
			}
			if len(plain) > 0xFFFF {
				return nil, fmt.Errorf("scs: record %q too long (%d bytes)", id, len(plain))
			}
			start := len(out)
			out = append(out, byte(len(plain)), byte(len(plain)>>8))
			body := make([]byte, len(plain))
			crypt(plain, start, body)
			out = append(out, body...)
		}
		return out, nil
	}
	out := []byte{}
	for i, id := range order {
		text, ok := textByID[id]
		if !ok {
			return nil, fmt.Errorf("scs: missing unit %q", id)
		}
		enc := base
		if usePer {
			enc = per[i]
		}
		plain, used, err := encodeRecord(text, enc)
		if err != nil {
			return nil, err
		}
		if usePer && used != enc {
			per[i] = used
		}
		out = append(out, plain...)
		eol := defaultEOL
		if i < len(eols) {
			eol = eols[i]
		}
		switch eol {
		case "crlf":
			out = append(out, 0x0D, 0x0A)
		case "lf":
			out = append(out, 0x0A)
		}
	}
	return out, nil
}

func encodeRecord(text string, enc string) ([]byte, string, error) {
	plain, err := textcodec.Encode(enc, text)
	if err != nil && textcodec.CanonicalName(enc) != "utf8" {
		if back, ferr := textcodec.Encode("utf8", text); ferr == nil {
			return back, "utf8", nil
		}
	}
	return plain, enc, err
}

func (Codec) UnitText(doc *core.Document, id string) (string, bool) {
	return core.DefaultUnitText(doc, id)
}

func (Codec) SetUnitText(doc *core.Document, id string, text string) bool {
	return core.DefaultSetUnitText(doc, id, text)
}

func crypt(in []byte, seedOffset int, out []byte) {
	seed := uint32(seedOffset | 1)
	for i, b := range in {
		seed = seed * keyStep
		out[i] = b ^ byte(seed&0xFF)
	}
}

func dominantEOL(eols []string) string {
	count := map[string]int{}
	for _, e := range eols {
		count[e]++
	}
	best := "lf"
	bestCount := -1
	for _, e := range []string{"crlf", "lf", "none"} {
		if count[e] > bestCount {
			bestCount = count[e]
			best = e
		}
	}
	return best
}
