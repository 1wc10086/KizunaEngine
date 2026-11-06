package core

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Target int

const (
	TargetFile Target = iota
	TargetDirectory
)

func (t Target) MarshalJSON() ([]byte, error) {
	if t == TargetDirectory {
		return []byte(`"dir"`), nil
	}
	return []byte(`"file"`), nil
}

func (t *Target) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		switch {
		case strings.EqualFold(s, "dir"):
			*t = TargetDirectory
		case strings.EqualFold(s, "file"):
			*t = TargetFile
		default:
			return fmt.Errorf("unknown target %q", s)
		}
		return nil
	}
	var n int
	if err := json.Unmarshal(data, &n); err != nil {
		return err
	}
	if n == int(TargetDirectory) {
		*t = TargetDirectory
		return nil
	}
	*t = TargetFile
	return nil
}

type Handler struct {
	Action       string
	Label        string
	Suffixes     []string
	OutputSuffix string
}

type Feature interface {
	ID() string
	Name() string
	Target() Target
	Handlers() []Handler
	DetectFormat(path string, head []byte) (string, bool)
	Run(action string, in string, out string, opt Options) (Result, error)
}

type Options struct {
	TextEncoding string
	Extra        map[string]string
}

type Result struct {
	InputPath  string            `json:"inputPath"`
	OutputPath string            `json:"outputPath"`
	BytesIn    int64             `json:"bytesIn"`
	BytesOut   int64             `json:"bytesOut"`
	ElapsedMs  int64             `json:"elapsedMs"`
	Summary    string            `json:"summary"`
	Details    map[string]string `json:"details"`
}
