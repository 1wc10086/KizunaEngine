package shell_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kizuna/kernel/shell"
	"kizuna/kernel/translator"
)

func call(t *testing.T, payload string) map[string]any {
	t.Helper()
	var resp map[string]any
	if err := json.Unmarshal([]byte(shell.Invoke(payload)), &resp); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	return resp
}

func TestPing(t *testing.T) {
	resp := call(t, `{"op":"ping"}`)
	if resp["ok"] != true {
		t.Fatalf("ping ok = %v", resp)
	}
	if resp["version"] != shell.Version {
		t.Fatalf("version = %v, want %v", resp["version"], shell.Version)
	}
	if _, ok := resp["features"]; !ok {
		t.Fatalf("ping missing features: %v", resp)
	}
}

func TestFeatures(t *testing.T) {
	resp := call(t, `{"op":"features"}`)
	if resp["ok"] != true {
		t.Fatalf("features ok = %v", resp)
	}
	if _, ok := resp["features"]; !ok {
		t.Fatalf("missing features: %v", resp)
	}
}

func TestDetectFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sample.unknown")
	if err := os.WriteFile(p, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{"op": "detect", "path": p})
	resp := call(t, string(payload))
	if resp["ok"] != true {
		t.Fatalf("detect ok = %v", resp)
	}
	det, ok := resp["detect"].(map[string]any)
	if !ok {
		t.Fatalf("missing detect: %v", resp)
	}
	if det["inputPath"] != p {
		t.Fatalf("inputPath = %v, want %v", det["inputPath"], p)
	}
}

func TestDetectMissing(t *testing.T) {
	p := filepath.Join(t.TempDir(), "nope.dat")
	payload, _ := json.Marshal(map[string]any{"op": "detect", "path": p})
	resp := call(t, string(payload))
	if resp["ok"] != false {
		t.Fatalf("expected failure: %v", resp)
	}
}

func TestBadRequests(t *testing.T) {
	for _, payload := range []string{`{"op":"bogus"}`, `{}`, `not json`} {
		resp := call(t, payload)
		if resp["ok"] != false {
			t.Fatalf("payload %q: expected failure: %v", payload, resp)
		}
		if _, ok := resp["error"]; !ok {
			t.Fatalf("payload %q: missing error: %v", payload, resp)
		}
	}
	resp := call(t, `{"op":"translate.poll","taskId":999999}`)
	if resp["ok"] != false {
		t.Fatalf("expected unknown task failure: %v", resp)
	}
}

func TestTranslateDetectEmpty(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.unknown")
	if err := os.WriteFile(p, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{"op": "translate.detect", "path": p})
	resp := call(t, string(payload))
	if resp["ok"] != true {
		t.Fatalf("translate.detect ok = %v", resp)
	}
	formats, ok := resp["formats"].([]any)
	if !ok {
		t.Fatalf("missing formats: %v", resp)
	}
	if len(formats) != 0 {
		t.Fatalf("expected no formats, got %v", formats)
	}
}

func pollOnce(t *testing.T, id int64) (bool, []map[string]any) {
	t.Helper()
	resp := call(t, fmt.Sprintf(`{"op":"translate.poll","taskId":%d}`, id))
	if resp["ok"] != true {
		t.Fatalf("poll ok = %v", resp)
	}
	done, _ := resp["done"].(bool)
	raw, ok := resp["events"].([]any)
	if !ok {
		t.Fatalf("missing events: %v", resp)
	}
	var out []map[string]any
	for _, e := range raw {
		if m, ok := e.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return done, out
}

func waitDone(t *testing.T, id int64) []map[string]any {
	t.Helper()
	var all []map[string]any
	deadline := time.Now().Add(5 * time.Second)
	for {
		done, evs := pollOnce(t, id)
		all = append(all, evs...)
		if done {
			return all
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for task")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func startTask(t *testing.T, payload map[string]any) int64 {
	t.Helper()
	raw, _ := json.Marshal(payload)
	resp := call(t, string(raw))
	if resp["ok"] != true {
		t.Fatalf("start ok = %v", resp)
	}
	id, ok := resp["taskId"].(float64)
	if !ok || id <= 0 {
		t.Fatalf("bad taskId: %v", resp)
	}
	return int64(id)
}

func TestTranslateStartPoll(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.dat")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	id := startTask(t, map[string]any{
		"op":      "translate.start",
		"source":  src,
		"target":  filepath.Join(dir, "a.out.dat"),
		"feature": "bishop.bdt",
	})
	events := waitDone(t, id)
	if len(events) == 0 {
		t.Fatal("expected at least one event")
	}
	done, evs := pollOnce(t, id)
	if !done {
		t.Fatal("expected done to stay true")
	}
	if len(evs) != 0 {
		t.Fatalf("expected drained events, got %v", evs)
	}
}

func TestTranslateCancel(t *testing.T) {
	old := shell.TranslationRunner
	shell.TranslationRunner = func(ctx context.Context, req translator.Request, emit func(translator.Event)) error {
		<-ctx.Done()
		return ctx.Err()
	}
	defer func() { shell.TranslationRunner = old }()
	dir := t.TempDir()
	src := filepath.Join(dir, "a.dat")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	id := startTask(t, map[string]any{
		"op":      "translate.start",
		"source":  src,
		"target":  filepath.Join(dir, "o.dat"),
		"feature": "bishop.bdt",
	})
	cresp := call(t, fmt.Sprintf(`{"op":"translate.cancel","taskId":%d}`, id))
	if cresp["ok"] != true || cresp["canceled"] != true {
		t.Fatalf("cancel = %v", cresp)
	}
	events := waitDone(t, id)
	found := false
	for _, e := range events {
		if e["type"] == "log" && e["level"] == "warn" && e["message"] == "已取消" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing cancel log event: %v", events)
	}
}
