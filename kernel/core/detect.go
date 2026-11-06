package core

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Candidate struct {
	FeatureID   string `json:"featureId"`
	FeatureName string `json:"featureName"`
	Action      string `json:"action"`
	ActionLabel string `json:"actionLabel"`
	Label       string `json:"label"`
	Format      string `json:"format"`
	InputPath   string `json:"inputPath"`
	OutputPath  string `json:"outputPath"`
}

type DetectResult struct {
	InputPath  string      `json:"inputPath"`
	InputKind  string      `json:"inputKind"`
	Suffix     string      `json:"suffix"`
	Target     Target      `json:"target"`
	OutputPath string      `json:"outputPath"`
	Candidates []Candidate `json:"candidates"`
	Message    string      `json:"message"`
}

func Detect(path string) (DetectResult, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return DetectResult{}, err
	}
	if fi.IsDir() {
		return detectDirectory(path), nil
	}
	return detectFile(path), nil
}

func DetectFormat(path string) (string, string, []string) {
	dr, err := Detect(path)
	if err != nil || len(dr.Candidates) == 0 {
		return "", "", nil
	}
	formats := make([]string, 0, len(dr.Candidates))
	seen := make(map[string]bool, len(dr.Candidates))
	for _, c := range dr.Candidates {
		if !seen[c.Format] {
			seen[c.Format] = true
			formats = append(formats, c.Format)
		}
	}
	return dr.Candidates[0].Format, dr.Candidates[0].FeatureID, formats
}

func detectDirectory(path string) DetectResult {
	res := DetectResult{
		InputPath:  path,
		InputKind:  "dir",
		Target:     TargetDirectory,
		Candidates: []Candidate{},
	}
	base := filepath.Base(path)
	suffix := ""
	stem := ""
	if HasFoldSuffix(base, ".dir") {
		stem = base[:len(base)-len(".dir")]
		suffix = LowerExt(stem)
	}
	if suffix != "" {
		for _, f := range Features() {
			if f.Target() != TargetDirectory {
				continue
			}
			for _, h := range f.Handlers() {
				hit := false
				for _, sfx := range h.Suffixes {
					if strings.EqualFold(sfx, suffix) {
						hit = true
						break
					}
				}
				if !hit {
					continue
				}
				res.Candidates = append(res.Candidates, Candidate{
					FeatureID:   f.ID(),
					FeatureName: f.Name(),
					Action:      h.Action,
					ActionLabel: h.Label,
					Label:       f.Name() + h.Label,
					InputPath:   path,
					OutputPath:  filepath.Join(filepath.Dir(path), stem),
				})
			}
		}
	}
	res.Suffix = suffix
	if len(res.Candidates) > 0 {
		res.OutputPath = res.Candidates[0].OutputPath
	} else {
		res.Message = "未识别的目录类型"
	}
	return res
}

type suffixHit struct {
	feat    Feature
	handler Handler
	suffix  string
}

func detectFile(path string) DetectResult {
	res := DetectResult{
		InputPath:  path,
		InputKind:  "file",
		Target:     TargetFile,
		Candidates: []Candidate{},
	}
	lower := strings.ToLower(path)
	var hits []suffixHit
	for _, f := range Features() {
		if f.Target() != TargetFile {
			continue
		}
		for _, h := range f.Handlers() {
			for _, sfx := range h.Suffixes {
				if sfx == "" {
					continue
				}
				if strings.HasSuffix(lower, strings.ToLower(sfx)) {
					hits = append(hits, suffixHit{feat: f, handler: h, suffix: sfx})
				}
			}
		}
	}
	if len(hits) == 0 {
		res.Message = "未识别的文件类型"
		return res
	}
	sort.SliceStable(hits, func(i, j int) bool {
		return len(hits[i].suffix) > len(hits[j].suffix)
	})
	head := readHead(path, 4096)
	for _, h := range hits {
		format, ok := h.feat.DetectFormat(path, head)
		if !ok {
			continue
		}
		var out string
		if h.handler.OutputSuffix == "" {
			out = TrimFoldSuffix(path, h.suffix)
		} else {
			out = path + h.handler.OutputSuffix
		}
		if len(res.Candidates) == 0 {
			res.Suffix = strings.ToLower(h.suffix)
		}
		res.Candidates = append(res.Candidates, Candidate{
			FeatureID:   h.feat.ID(),
			FeatureName: h.feat.Name(),
			Action:      h.handler.Action,
			ActionLabel: h.handler.Label,
			Label:       h.feat.Name() + h.handler.Label,
			Format:      format,
			InputPath:   path,
			OutputPath:  out,
		})
	}
	if len(res.Candidates) == 0 {
		res.Message = "未识别的文件类型"
		return res
	}
	res.OutputPath = res.Candidates[0].OutputPath
	return res
}

func readHead(path string, n int) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	buf := make([]byte, n)
	m, _ := io.ReadFull(f, buf)
	return buf[:m]
}
