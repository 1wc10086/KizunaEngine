package shell_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"kizuna/kernel/core"
	"kizuna/kernel/shell"
)

type stubFeature struct{}

func (stubFeature) ID() string {
	return "test.stub"
}

func (stubFeature) Name() string {
	return "Stub"
}

func (stubFeature) Target() core.Target {
	return core.TargetFile
}

func (stubFeature) Handlers() []core.Handler {
	return []core.Handler{
		{Action: "decode", Label: "解码", Suffixes: []string{".stub"}, OutputSuffix: ".stub.json"},
		{Action: "encode", Label: "编码", Suffixes: []string{".stub.json"}},
	}
}

func (stubFeature) DetectFormat(path string, head []byte) (string, bool) {
	return "stub", true
}

func (stubFeature) Run(action string, in string, out string, opt core.Options) (core.Result, error) {
	if err := os.WriteFile(out, []byte("stub:"+action), 0o644); err != nil {
		return core.Result{}, err
	}
	return core.Result{
		InputPath:  in,
		OutputPath: out,
		BytesIn:    4,
		BytesOut:   9,
		ElapsedMs:  7,
		Summary:    "stub run",
		Details:    map[string]string{"feature": "test.stub"},
	}, nil
}

func init() {
	core.Register(stubFeature{})
}

func rawResponse(t *testing.T, payload string) map[string]any {
	t.Helper()
	var resp map[string]any
	if err := json.Unmarshal([]byte(shell.Invoke(payload)), &resp); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if resp["ok"] != true {
		t.Fatalf("expected ok response: %v", resp)
	}
	return resp
}

func requireKeys(t *testing.T, m map[string]any, keys []string) {
	t.Helper()
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			t.Fatalf("missing key %q in %v", k, m)
		}
	}
}

func forbidKeys(t *testing.T, m map[string]any, keys []string) {
	t.Helper()
	for _, k := range keys {
		if _, ok := m[k]; ok {
			t.Fatalf("forbidden key %q present in %v", k, m)
		}
	}
}

var pascalKeys = []string{
	"InputPath", "InputKind", "Suffix", "Target", "OutputPath", "Candidates", "Message",
	"FeatureID", "FeatureName", "ActionLabel", "Label", "Format", "Action",
	"BytesIn", "BytesOut", "ElapsedMs", "Summary", "Details",
	"Handlers", "Suffixes", "OutputSuffix",
}

func TestDetectCamelCaseKeys(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sample.stub")
	if err := os.WriteFile(p, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{"op": "detect", "path": p})
	resp := rawResponse(t, string(payload))
	det, ok := resp["detect"].(map[string]any)
	if !ok {
		t.Fatalf("missing detect: %v", resp)
	}
	requireKeys(t, det, []string{"inputPath", "inputKind", "suffix", "target", "outputPath", "candidates", "message"})
	forbidKeys(t, det, pascalKeys)
	if det["inputPath"] != p {
		t.Fatalf("inputPath = %v, want %v", det["inputPath"], p)
	}
	if det["target"] != "file" {
		t.Fatalf("target = %v, want %q", det["target"], "file")
	}
	cands, ok := det["candidates"].([]any)
	if !ok || len(cands) == 0 {
		t.Fatalf("missing candidates: %v", det)
	}
	first, ok := cands[0].(map[string]any)
	if !ok {
		t.Fatalf("candidate is not an object: %v", cands[0])
	}
	requireKeys(t, first, []string{"featureId", "featureName", "action", "actionLabel", "label", "format", "inputPath", "outputPath"})
	forbidKeys(t, first, pascalKeys)
	if first["featureId"] != "test.stub" {
		t.Fatalf("featureId = %v", first["featureId"])
	}
	if first["outputPath"] != p+".stub.json" {
		t.Fatalf("outputPath = %v, want %v", first["outputPath"], p+".stub.json")
	}
}

func TestProcessCamelCaseKeys(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "sample.stub")
	out := filepath.Join(dir, "sample.stub.json")
	if err := os.WriteFile(in, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{
		"op": "process", "feature": "test.stub", "action": "decode", "input": in, "output": out,
	})
	resp := rawResponse(t, string(payload))
	res, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("missing result: %v", resp)
	}
	requireKeys(t, res, []string{"inputPath", "outputPath", "bytesIn", "bytesOut", "elapsedMs", "summary", "details"})
	forbidKeys(t, res, pascalKeys)
	if res["inputPath"] != in || res["outputPath"] != out {
		t.Fatalf("paths = %v", res)
	}
}

func TestFeaturesCamelCaseKeys(t *testing.T) {
	resp := rawResponse(t, `{"op":"features"}`)
	raw, ok := resp["features"].([]any)
	if !ok || len(raw) == 0 {
		t.Fatalf("missing features: %v", resp)
	}
	found := false
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("feature is not an object: %v", item)
		}
		requireKeys(t, m, []string{"id", "name", "target", "handlers"})
		forbidKeys(t, m, pascalKeys)
		if m["id"] != "test.stub" {
			continue
		}
		found = true
		if m["target"] != "file" {
			t.Fatalf("target = %v", m["target"])
		}
		hs, ok := m["handlers"].([]any)
		if !ok || len(hs) != 2 {
			t.Fatalf("handlers = %v", m["handlers"])
		}
		for _, h := range hs {
			hm, ok := h.(map[string]any)
			if !ok {
				t.Fatalf("handler is not an object: %v", h)
			}
			requireKeys(t, hm, []string{"action", "label", "suffixes", "outputSuffix"})
			forbidKeys(t, hm, pascalKeys)
		}
	}
	if !found {
		t.Fatalf("stub feature missing: %v", raw)
	}
}

func TestTranslateDetectCamelCaseKeys(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sample.stub")
	if err := os.WriteFile(p, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{"op": "translate.detect", "path": p})
	resp := rawResponse(t, string(payload))
	raw, ok := resp["formats"].([]any)
	if !ok || len(raw) == 0 {
		t.Fatalf("missing formats: %v", resp)
	}
	m, ok := raw[0].(map[string]any)
	if !ok {
		t.Fatalf("format is not an object: %v", raw[0])
	}
	requireKeys(t, m, []string{"featureId", "format", "name"})
	forbidKeys(t, m, pascalKeys)
}

func TestTargetWireValues(t *testing.T) {
	var dr core.DetectResult
	if err := json.Unmarshal([]byte(`{"target":"dir"}`), &dr); err != nil {
		t.Fatal(err)
	}
	if dr.Target != core.TargetDirectory {
		t.Fatalf("target = %v, want dir", dr.Target)
	}
	data, err := json.Marshal(core.DetectResult{Target: core.TargetDirectory, Candidates: []core.Candidate{}})
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back["target"] != "dir" {
		t.Fatalf("target = %v, want %q", back["target"], "dir")
	}
}
