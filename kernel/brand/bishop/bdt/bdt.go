package bdt

import (
	"fmt"
	"os"
	"strings"

	"kizuna/kernel/core"
	"kizuna/kernel/core/textcodec"
)

const (
	FeatureID       = "bishop.bdt"
	FeatureName     = "Bishop-Bdt"
	FormatName      = "bdt"
	DocumentVersion = "1"
	EncodingName    = "cp932"
)

type codec struct{}

func init() {
	core.Register(codec{})
}

func (codec) ID() string {
	return FeatureID
}

func (codec) Name() string {
	return FeatureName
}

func (codec) Target() core.Target {
	return core.TargetFile
}

func (codec) Handlers() []core.Handler {
	return []core.Handler{
		{Action: "decode", Label: "解码", Suffixes: []string{".bdt"}, OutputSuffix: ".bdt.json"},
		{Action: "encode", Label: "编码", Suffixes: []string{".bdt.json"}, OutputSuffix: ""},
	}
}

func (codec) DetectFormat(path string, head []byte) (string, bool) {
	if strings.HasSuffix(strings.ToLower(path), ".bdt.json") {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", false
		}
		feature, err := core.ProbeFeature(data)
		if err != nil || feature != FeatureID {
			return "", false
		}
		return FormatName, true
	}
	return FormatName, true
}

func (c codec) Run(action string, in string, out string, opt core.Options) (core.Result, error) {
	switch action {
	case "decode":
		return core.RunCodecDecode(c, in, out)
	case "encode":
		return core.RunCodecEncode(c, in, out)
	default:
		return core.Result{}, fmt.Errorf("bdt: unknown action %q", action)
	}
}

func (codec) UnitText(doc *core.Document, id string) (string, bool) {
	return core.DefaultUnitText(doc, id)
}

func (codec) SetUnitText(doc *core.Document, id string, text string) bool {
	return core.DefaultSetUnitText(doc, id, text)
}

func normalizeTarget(name string) string {
	s := strings.ToLower(name)
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "_", "")
	s = strings.ReplaceAll(s, " ", "")
	return s
}

func isUTF8Target(name string) bool {
	switch normalizeTarget(name) {
	case "", "utf8":
		return true
	default:
		return false
	}
}

func (codec) Decode(data []byte, name string) (*core.Document, error) {
	raw := append([]byte(nil), data...)
	core.NegateBytes(raw)
	enc := textcodec.DetectTextEncoding(raw)
	lines := splitLines(raw)
	doc := &core.Document{
		Feature:  FeatureID,
		Format:   FormatName,
		Version:  DocumentVersion,
		Encoding: enc,
		Meta: core.Meta{
			Values: map[string]string{"source": name},
			Lists:  map[string][]string{},
		},
	}
	section := core.Section{
		Kind:     "lines",
		Name:     "lines",
		Encoding: enc,
	}
	eols := make([]string, 0, len(lines))
	for i, ln := range lines {
		text := textcodec.Decode(enc, ln.body)
		kind := "raw"
		prefix := ""
		inner := text
		suffix := ""
		if isTranslatable(text) {
			kind = "text"
			prefix, inner, suffix = splitAffixes(text)
		}
		id := fmt.Sprintf("line/%d", i)
		doc.Units = append(doc.Units, core.Unit{ID: id, Kind: kind, Text: inner, Prefix: prefix, Suffix: suffix})
		section.IDs = append(section.IDs, id)
		eols = append(eols, ln.eol)
	}
	section.EOL = core.SummarizeEOL(eols)
	if len(eols) > 0 {
		section.Trailing = eols[len(eols)-1]
	}
	doc.Meta.Lists["eol"] = eols
	doc.Sections = []core.Section{section}
	return doc, nil
}

func (codec) Encode(doc *core.Document) ([]byte, error) {
	if doc == nil {
		return nil, fmt.Errorf("bdt: nil document")
	}
	if doc.Feature != FeatureID {
		return nil, core.ErrNotMine
	}
	var section *core.Section
	for i := range doc.Sections {
		if doc.Sections[i].Kind == "lines" {
			section = &doc.Sections[i]
			break
		}
	}
	if section == nil {
		return nil, fmt.Errorf("bdt: missing lines section")
	}
	byID := make(map[string]string, len(doc.Units))
	byUnit := make(map[string]core.Unit, len(doc.Units))
	for _, u := range doc.Units {
		if _, dup := byID[u.ID]; !dup {
			byID[u.ID] = u.Text
			byUnit[u.ID] = u
		}
	}
	var eols []string
	if doc.Meta.Lists != nil {
		eols = doc.Meta.Lists["eol"]
	}
	target := doc.Encoding
	if target == "" {
		target = section.Encoding
	}
	if target == "" {
		target = EncodingName
	}
	full := make([]string, len(section.IDs))
	for i, id := range section.IDs {
		text, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("bdt: missing unit %q", id)
		}
		full[i] = joinAffixes(byUnit[id], text)
	}
	bodies := make([][]byte, len(full))
	if err := encodeLines(target, full, bodies); err != nil {
		if isUTF8Target(target) {
			return nil, err
		}
		if err := encodeLines("utf8", full, bodies); err != nil {
			return nil, err
		}
		if doc.Meta.Values == nil {
			doc.Meta.Values = map[string]string{}
		}
		doc.Meta.Values["encoding.fallback"] = target + "->utf8"
		doc.Encoding = "utf8"
		section.Encoding = "utf8"
		target = "utf8"
	}
	raw := make([]byte, 0, len(section.IDs)*32)
	for i := range section.IDs {
		raw = append(raw, bodies[i]...)
		eol := ""
		if len(eols) == len(section.IDs) {
			eol = eols[i]
		} else if i < len(section.IDs)-1 {
			eol = section.EOL
		} else {
			eol = section.Trailing
		}
		switch eol {
		case "crlf":
			raw = append(raw, '\r', '\n')
		case "lf":
			raw = append(raw, '\n')
		case "":
		default:
			return nil, fmt.Errorf("bdt: unknown eol %q", eol)
		}
	}
	core.NegateBytes(raw)
	return raw, nil
}

func encodeLines(target string, full []string, bodies [][]byte) error {
	for i, text := range full {
		body, err := textcodec.Encode(target, text)
		if err != nil {
			return err
		}
		bodies[i] = body
	}
	return nil
}

func joinAffixes(u core.Unit, text string) string {
	if u.Prefix == "" && u.Suffix == "" {
		return text
	}
	if len(text) >= len(u.Prefix)+len(u.Suffix) &&
		strings.HasPrefix(text, u.Prefix) &&
		strings.HasSuffix(text, u.Suffix) {
		return text
	}
	return u.Prefix + text + u.Suffix
}
