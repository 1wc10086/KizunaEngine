package dsk

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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
		{Action: "decode", Label: "解码", Suffixes: []string{".dsk"}, OutputSuffix: ".dsk.json"},
		{Action: "encode", Label: "编码", Suffixes: []string{".dsk.json"}, OutputSuffix: ""},
	}
}

func (Feature) DetectFormat(path string, head []byte) (string, bool) {
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".dsk.json") {
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
	if strings.HasSuffix(lower, ".dsk") {
		return FormatName, true
	}
	return "", false
}

func (Feature) Run(action string, in string, out string, opt core.Options) (core.Result, error) {
	_ = opt
	switch action {
	case "decode":
		return runDecode(in, out)
	case "encode":
		return runEncode(in, out)
	default:
		return core.Result{}, fmt.Errorf("dsk: unknown action %q", action)
	}
}

func (Feature) Decode(data []byte, name string) (*core.Document, error) {
	return fallbackDocument(data, name), nil
}

func (Feature) Encode(doc *core.Document) ([]byte, error) {
	data, _, err := EncodeWithPFT(doc)
	return data, err
}

func (Feature) UnitText(doc *core.Document, id string) (string, bool) {
	return core.DefaultUnitText(doc, id)
}

func (Feature) SetUnitText(doc *core.Document, id string, text string) bool {
	return core.DefaultSetUnitText(doc, id, text)
}

func tableCounts(doc *core.Document) (int, int) {
	tab, err := parseTable(doc.Meta.Lists[listEntries], 1)
	if err != nil {
		return 0, 0
	}
	scfCount := 0
	for _, e := range tab {
		if e.scf {
			scfCount++
		}
	}
	return len(tab), scfCount
}

func runDecode(in string, out string) (core.Result, error) {
	start := time.Now()
	data, err := os.ReadFile(in)
	if err != nil {
		return core.Result{}, err
	}
	var doc *core.Document
	pftStatus := "missing"
	if pftPath, ok := FindPFT(in); ok {
		raw, err := os.ReadFile(pftPath)
		if err != nil {
			return core.Result{}, err
		}
		doc, err = DecodeWithPFT(data, raw, filepath.Base(in))
		if err != nil {
			return core.Result{}, err
		}
		pftStatus = "ok"
	} else {
		core.Logf("dsk: no sibling pft for %s, decoding as raw", in)
		doc = fallbackDocument(data, filepath.Base(in))
	}
	if err := core.WriteJSON(out, doc); err != nil {
		return core.Result{}, err
	}
	fi, err := os.Stat(out)
	if err != nil {
		return core.Result{}, err
	}
	total, scfCount := tableCounts(doc)
	return core.Result{
		InputPath:  in,
		OutputPath: out,
		BytesIn:    int64(len(data)),
		BytesOut:   fi.Size(),
		ElapsedMs:  time.Since(start).Milliseconds(),
		Summary:    fmt.Sprintf("decode %d entries (%d scf, %d units)", total, scfCount, len(doc.Units)),
		Details: map[string]string{
			"feature": doc.Feature,
			"format":  doc.Format,
			"entries": strconv.Itoa(total),
			"scf":     strconv.Itoa(scfCount),
			"pft":     pftStatus,
		},
	}, nil
}

func runEncode(in string, out string) (core.Result, error) {
	start := time.Now()
	raw, err := os.ReadFile(in)
	if err != nil {
		return core.Result{}, err
	}
	var doc core.Document
	if err := core.ReadJSON(in, &doc); err != nil {
		return core.Result{}, err
	}
	if doc.Feature != FeatureID {
		return core.Result{}, fmt.Errorf("%s: %w", FeatureID, core.ErrNotMine)
	}
	dskData, pftData, err := EncodeWithPFT(&doc)
	if err != nil {
		return core.Result{}, err
	}
	if err := core.AtomicWrite(out, dskData); err != nil {
		return core.Result{}, err
	}
	pftStatus := "none"
	if pftData != nil {
		if pftPath, ok := FindPFT(out); ok {
			if err := core.AtomicWrite(pftPath, pftData); err != nil {
				return core.Result{}, err
			}
			pftStatus = "updated"
		} else {
			core.Logf("dsk: no sibling pft for %s, wrote dsk only", out)
			pftStatus = "missing"
		}
	} else if doc.Meta.Values[metaMissing] == "true" {
		core.Logf("dsk: no sibling pft for %s, wrote dsk only", out)
		pftStatus = "missing"
	}
	total, scfCount := tableCounts(&doc)
	return core.Result{
		InputPath:  in,
		OutputPath: out,
		BytesIn:    int64(len(raw)),
		BytesOut:   int64(len(dskData)),
		ElapsedMs:  time.Since(start).Milliseconds(),
		Summary:    fmt.Sprintf("encode %d entries (%d scf)", total, scfCount),
		Details: map[string]string{
			"feature": doc.Feature,
			"format":  doc.Format,
			"entries": strconv.Itoa(total),
			"scf":     strconv.Itoa(scfCount),
			"pft":     pftStatus,
		},
	}, nil
}
