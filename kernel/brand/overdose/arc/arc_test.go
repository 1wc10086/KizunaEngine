package arc

import (
	"os"
	"path/filepath"
	"testing"

	"kizuna/kernel/core"
)

var gameCandidates = []string{
	os.Getenv("OVERDOSE_DIR"),
	"/root/code/engine/游戏/YUI/優衣",
	"../../../engine/游戏/YUI/優衣",
}

func scriptArc(t *testing.T) string {
	t.Helper()
	for _, dir := range gameCandidates {
		if dir == "" {
			continue
		}
		path := filepath.Join(dir, "script.arc")
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	t.Skip("未找到 script.arc 样例")
	return ""
}

// 真数据回归：script.arc 直接解码成 3 万多条 unit，改一条再整体编码，
// 重解后译文仍在；这是“kernel 直接吃 script.arc”的主路径。
func TestScriptArcRoundTrip(t *testing.T) {
	path := scriptArc(t)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	c := codec{}
	doc, err := c.Decode(raw, filepath.Base(path))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(doc.Units) < 1000 {
		t.Fatalf("units = %d，看起来没有解出全部剧本", len(doc.Units))
	}
	target := ""
	for i := range doc.Units {
		if doc.Units[i].Kind == "dialogue" && doc.Units[i].Text != "" {
			target = doc.Units[i].ID
			doc.Units[i].Text = "这是翻译测试文本。"
			break
		}
	}
	if target == "" {
		t.Fatal("没有可翻译的对白")
	}
	out, err := c.Encode(doc)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	doc2, err := c.Decode(out, filepath.Base(path))
	if err != nil {
		t.Fatalf("re-Decode: %v", err)
	}
	if len(doc2.Units) != len(doc.Units) {
		t.Fatalf("units %d -> %d", len(doc.Units), len(doc2.Units))
	}
	found := false
	for _, u := range doc2.Units {
		if u.ID == target && u.Text == "这是翻译测试文本。" {
			found = true
		}
	}
	if !found {
		t.Fatalf("unit %s 的译文没有保留", target)
	}
}

// DetectFormat 必须只认含 .asb 条目的 ARC；graphic.arc 不该被当成剧本源。
func TestDetectScriptArcOnly(t *testing.T) {
	path := scriptArc(t)
	c := codec{}
	if _, ok := c.DetectFormat(path, nil); !ok {
		t.Fatal("script.arc 未被识别")
	}
	graphic := filepath.Join(filepath.Dir(path), "graphic.arc")
	if _, err := os.Stat(graphic); err == nil {
		if _, ok := c.DetectFormat(graphic, nil); ok {
			t.Fatal("graphic.arc 不应被识别为剧本 ARC")
		}
	}
}

// 注册链路：core.Detect(script.arc) 必须给出 overdose.arc 候选，
// 否则翻译界面里选不到这个 feature。
func TestCoreDetectScriptArc(t *testing.T) {
	path := scriptArc(t)
	dr, err := core.Detect(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range dr.Candidates {
		if c.FeatureID == FeatureID && c.Format == "ARC" {
			return
		}
	}
	t.Fatalf("core.Detect 未给出 %s: %+v", FeatureID, dr.Candidates)
}
