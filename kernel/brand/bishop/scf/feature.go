package scf

import (
	"errors"
	"os"
	"strings"

	"kizuna/kernel/core"
)

type Feature struct{}

func init() {
	core.Register(Feature{})
}

func (Feature) ID() string {
	return FeatureID
}

func (Feature) Name() string {
	return FeatureName
}

func (Feature) Target() core.Target {
	return core.TargetFile
}

func (Feature) Handlers() []core.Handler {
	return []core.Handler{
		{Action: "decode", Label: "解码", Suffixes: []string{".scf"}, OutputSuffix: ".scf.json"},
		{Action: "encode", Label: "编码", Suffixes: []string{".scf.json"}, OutputSuffix: ""},
	}
}

func (Feature) DetectFormat(path string, head []byte) (string, bool) {
	_ = head
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".scf.json") {
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", false
		}
		feat, err := core.ProbeFeature(raw)
		if err != nil {
			return "", false
		}
		if feat != FeatureID {
			return "", false
		}
		return FormatName, true
	}
	if strings.HasSuffix(lower, ".scf") {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", false
		}
		if !LooksLikeSCF(data) {
			return "", false
		}
		return FormatName, true
	}
	return "", false
}

func (f Feature) Run(action string, in string, out string, opt core.Options) (core.Result, error) {
	_ = opt
	switch action {
	case "decode":
		return core.RunCodecDecode(f, in, out)
	case "encode":
		return core.RunCodecEncode(f, in, out)
	}
	return core.Result{}, errors.New("scf: unknown action")
}

func (Feature) Decode(data []byte, name string) (*core.Document, error) {
	return Decode(data, name)
}

func (Feature) Encode(doc *core.Document) ([]byte, error) {
	return Encode(doc)
}

func (Feature) UnitText(doc *core.Document, id string) (string, bool) {
	return core.DefaultUnitText(doc, id)
}

func (Feature) SetUnitText(doc *core.Document, id string, text string) bool {
	return core.DefaultSetUnitText(doc, id, text)
}
