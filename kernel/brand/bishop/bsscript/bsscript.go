package bsscript

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"kizuna/kernel/brand/bishop/scriptbin"
	"kizuna/kernel/core"
)

const (
	FeatureID       = "bishop.bsscript"
	FeatureName     = "Bishop-BSScript"
	Magic           = "BSScript"
	ModernMagic     = "BSXScript"
	DefaultEncoding = "cp932"
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
		{Action: "decode", Label: "解码", Suffixes: []string{".dat"}, OutputSuffix: ".dat.json"},
		{Action: "encode", Label: "编码", Suffixes: []string{".dat.json"}, OutputSuffix: ""},
	}
}

func matchMagic(head []byte) (string, bool) {
	if sig, ok := scriptbin.DetectSignature(head); ok && strings.HasPrefix(sig, Magic) && !strings.HasPrefix(sig, ModernMagic) {
		return sig, true
	}
	if len(head) >= 16 {
		neg := append([]byte(nil), head...)
		core.NegateBytes(neg)
		if sig, ok := scriptbin.DetectSignature(neg); ok && strings.HasPrefix(sig, Magic) && !strings.HasPrefix(sig, ModernMagic) {
			return sig, true
		}
	}
	return "", false
}

func (codec) DetectFormat(path string, head []byte) (string, bool) {
	if strings.HasSuffix(strings.ToLower(path), ".dat.json") {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", false
		}
		var probe struct {
			Feature string `json:"feature"`
			Format  string `json:"format"`
		}
		if err := json.Unmarshal(data, &probe); err != nil {
			return "", false
		}
		if probe.Feature != FeatureID {
			return "", false
		}
		if probe.Format != "" {
			return probe.Format, true
		}
		return FeatureID, true
	}
	sig, ok := matchMagic(head)
	if !ok {
		return "", false
	}
	return sig, true
}

func (c codec) Run(action string, in string, out string, opt core.Options) (core.Result, error) {
	switch action {
	case "decode":
		return core.RunCodecDecode(c, in, out)
	case "encode":
		return core.RunCodecEncode(c, in, out)
	default:
		return core.Result{}, fmt.Errorf("bsscript: unknown action %q", action)
	}
}

func (codec) UnitText(doc *core.Document, id string) (string, bool) {
	return core.DefaultUnitText(doc, id)
}

func (codec) SetUnitText(doc *core.Document, id string, text string) bool {
	return core.DefaultSetUnitText(doc, id, text)
}

func versionOf(sig string) string {
	if sig == Magic {
		return ""
	}
	return strings.TrimPrefix(sig, Magic+" ")
}

func (codec) Decode(data []byte, name string) (*core.Document, error) {
	c, err := scriptbin.Decode(data)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(c.Signature, Magic) || strings.HasPrefix(c.Signature, ModernMagic) {
		return nil, fmt.Errorf("bsscript: bad signature %q", c.Signature)
	}
	return c.ToDocument(FeatureID, versionOf(c.Signature), name), nil
}

func (codec) Encode(doc *core.Document) ([]byte, error) {
	if doc == nil {
		return nil, fmt.Errorf("bsscript: nil document")
	}
	if doc.Feature != FeatureID {
		return nil, core.ErrNotMine
	}
	c, err := scriptbin.FromDocument(doc, DefaultEncoding)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(c.Signature, Magic) || strings.HasPrefix(c.Signature, ModernMagic) {
		return nil, fmt.Errorf("bsscript: bad signature %q", c.Signature)
	}
	return c.Encode()
}
