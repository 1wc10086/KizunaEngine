package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Codec interface {
	Feature
	Decode(data []byte, name string) (*Document, error)
	Encode(doc *Document) ([]byte, error)
	UnitText(doc *Document, id string) (string, bool)
	SetUnitText(doc *Document, id string, text string) bool
}

var ErrNotMine = errors.New("not mine")

func ProbeFeature(data []byte) (string, error) {
	var probe struct {
		Feature string `json:"feature"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return "", err
	}
	if probe.Feature == "" {
		return "", errors.New("missing feature")
	}
	return probe.Feature, nil
}

func DefaultUnitText(doc *Document, id string) (string, bool) {
	if doc == nil {
		return "", false
	}
	for _, u := range doc.Units {
		if u.ID == id {
			return u.Text, true
		}
	}
	return "", false
}

func DefaultSetUnitText(doc *Document, id string, text string) bool {
	if doc == nil {
		return false
	}
	for i := range doc.Units {
		if doc.Units[i].ID == id {
			doc.Units[i].Text = text
			return true
		}
	}
	return false
}

func RunCodecDecode(c Codec, in string, out string) (Result, error) {
	start := time.Now()
	data, err := os.ReadFile(in)
	if err != nil {
		return Result{}, err
	}
	doc, err := c.Decode(data, filepath.Base(in))
	if err != nil {
		return Result{}, err
	}
	if err := WriteJSON(out, doc); err != nil {
		return Result{}, err
	}
	fi, err := os.Stat(out)
	if err != nil {
		return Result{}, err
	}
	return Result{
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
}

func RunCodecEncode(c Codec, in string, out string) (Result, error) {
	start := time.Now()
	raw, err := os.ReadFile(in)
	if err != nil {
		return Result{}, err
	}
	var doc Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Result{}, err
	}
	data, err := c.Encode(&doc)
	if err != nil {
		if errors.Is(err, ErrNotMine) {
			return Result{}, fmt.Errorf("%s: %w", c.ID(), err)
		}
		return Result{}, err
	}
	if err := AtomicWrite(out, data); err != nil {
		return Result{}, err
	}
	return Result{
		InputPath:  in,
		OutputPath: out,
		BytesIn:    int64(len(raw)),
		BytesOut:   int64(len(data)),
		ElapsedMs:  time.Since(start).Milliseconds(),
		Summary:    fmt.Sprintf("encode %d units", len(doc.Units)),
		Details: map[string]string{
			"feature": doc.Feature,
			"format":  doc.Format,
		},
	}, nil
}
